package cron

import (
	"context"
	"cronlite/locker"
	"cronlite/logger"
	"errors"
	"fmt"
	"github.com/gorhill/cronexpr"
	"time"
)

// NewJob creates a new Job instance with the provided options
func NewJob(options JobOptions) (IJob, error) {
	if options.Logger == nil {
		options.Logger = &logger.LogLogger{}
	}
	if options.Redis == nil {
		return nil, errors.New("redis client is required")
	}
	if options.Name == "" {
		return nil, errors.New("job name is required")
	}
	if options.Spec == "" {
		return nil, errors.New("cron expression is required")
	}
	if options.Job == nil {
		return nil, errors.New("job function is required")
	}
	if options.WorkerIdProvider == nil {
		options.WorkerIdProvider = &DefaultWorkerIDProvider{}
	}

	expr, err := cronexpr.Parse(options.Spec)
	if err != nil {
		return nil, fmt.Errorf("invalid cron expression: %w", err)
	}

	if options.Locker == nil {
		lockerOptions := locker.Options{
			Name:    options.Name,
			Redis:   options.Redis,
			Logger:  options.Logger,
			LockTTL: time.Until(expr.Next(time.Now())) + 5*time.Second,
		}
		options.Locker = locker.NewLocker(lockerOptions)
	}

	job := &Job{
		Options:    options,
		CronExpr:   expr,
		StopSignal: make(chan bool),
	}

	stateKey := fmt.Sprintf(JobStateKeyFormat, options.Name)
	job.State = NewState(
		options.Redis,
		stateKey,
		options.Logger,
		job.OnStateUpdated,
	)

	return job, nil
}

// OnStateUpdated is called whenever the job state changes
func (cj *Job) OnStateUpdated(ctx context.Context, state *JobState) error {
	cj.Options.Logger.Info(ctx, "Job state changed", map[string]interface{}{"name": cj.Options.Name})
	if stateSpec, ok := state.Data["spec"].(string); ok && stateSpec != cj.Options.Spec {
		cj.Options.Logger.Info(ctx, "Updating cron expression", map[string]interface{}{"name": cj.Options.Name, "new_spec": stateSpec, "old_spec": cj.Options.Spec})
		if expr, err := cronexpr.Parse(stateSpec); err == nil {
			cj.CronExpr = expr
			cj.Options.Spec = stateSpec
			cj.Options.Locker.SetTTL(time.Until(cj.CronExpr.Next(time.Now())) + 5*time.Second)
			now := time.Now()
			state.LastRun = now
			state.NextRun = cj.CronExpr.Next(now)
			state.UpdatedAt = time.Now()
		} else {
			cj.Options.Logger.Error(ctx, "Invalid cron expression", map[string]interface{}{"name": cj.Options.Name, "error": err})
		}
	}
	return nil
}

// Run starts the job execution loop

// Stop gracefully stops the job
func (cj *Job) Stop(ctx context.Context) error {
	close(cj.StopSignal)
	return nil
}

// execute runs the job's function with proper state management
func (cj *Job) execute(ctx context.Context, state *JobState) {
	// IMPORTANT: Do NOT acquire the stateMutex here as it's already held by the caller.

	// Acquire the lock
	success, err := cj.Options.Locker.Acquire(ctx)
	if err != nil || !success {
		cj.Options.Logger.Debug(ctx, "Cron job skipped due to existing lock", map[string]interface{}{"name": cj.Options.Name})
		return
	}
	defer func(Locker locker.ILocker, ctx context.Context) {
		err := Locker.Release(ctx)
		if err != nil {
			cj.Options.Logger.Error(ctx, "Failed to release lock", map[string]interface{}{"name": cj.Options.Name, "error": err})
		}
	}(cj.Options.Locker, ctx)

	// Execute BeforeExecute hook
	if cj.Options.BeforeExecute != nil {
		continueExec, err := cj.Options.BeforeExecute(ctx, cj)
		if err != nil {
			cj.Options.Logger.Error(ctx, "Failed to execute BeforeExecute callback", map[string]interface{}{"name": cj.Options.Name, "error": err})
			state.Status = JobStatusFailed
			if state.Data == nil {
				state.Data = make(map[string]interface{})
			}
			state.Data["error_message"] = err.Error()
			state.UpdatedAt = time.Now()
			if saveErr := cj.State.Save(ctx, state); saveErr != nil { // Save in-memory state
				cj.Options.Logger.Error(ctx, "Failed to save updated job state", map[string]interface{}{"name": cj.Options.Name, "error": saveErr})
			}
			return
		}
		if !continueExec {
			cj.Options.Logger.Debug(ctx, "Execution skipped by BeforeExecute callback", map[string]interface{}{"name": cj.Options.Name})
			return
		}
	}

	// Increment iterations and set RunningBy
	state.Iterations++
	state.RunningBy, err = cj.Options.WorkerIdProvider.Id()
	if err != nil {
		cj.Options.Logger.Error(ctx, "Failed to get worker ID", map[string]interface{}{"error": err})
		state.Status = JobStatusFailed
		if state.Data == nil {
			state.Data = make(map[string]interface{})
		}
		state.Data["error_message"] = err.Error()
		state.UpdatedAt = time.Now()
		if saveErr := cj.State.Save(ctx, state); saveErr != nil { // Save in-memory state
			cj.Options.Logger.Error(ctx, "Failed to save job state", map[string]interface{}{"name": cj.Options.Name, "error": saveErr})
		}
		// AfterExecute is called only after an actual execution attempt.
		if cj.Options.AfterExecute != nil {
			afterErr := cj.Options.AfterExecute(ctx, cj, err)
			if afterErr != nil {
				cj.Options.Logger.Error(ctx, "AfterExecute callback returned an error", map[string]interface{}{"name": cj.Options.Name, "error": afterErr})
			}
		}
		return
	} else {
		now := time.Now()
		state.LastRun = now
		state.NextRun = cj.CronExpr.Next(now)
		state.Status = JobStatusRunning
		state.UpdatedAt = time.Now()
	}

	if err = cj.State.Save(ctx, state); err != nil { // Save in-memory state
		cj.Options.Logger.Error(ctx, "Failed to save job state", map[string]interface{}{"error": err})
	}

	// Keep lock alive and heartbeat updated periodically
	lockCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	go cj.extendLockPeriodically(lockCtx, state)

	cj.Options.Logger.Info(ctx, "Executing cron job", map[string]interface{}{"name": cj.Options.Name})
	jobErr := cj.Options.Job(ctx, cj) // Execute the actual job function

	if jobErr != nil {
		cj.Options.Logger.Error(ctx, "Error executing cron job", map[string]interface{}{"name": cj.Options.Name, "error": jobErr})
		state.Status = JobStatusFailed
		if state.Data == nil {
			state.Data = make(map[string]interface{})
		}
		state.Data["error_message"] = jobErr.Error()
	} else {
		cj.Options.Logger.Info(ctx, "Cron job executed successfully", map[string]interface{}{"name": cj.Options.Name})
		state.Status = JobStatusSuccess
		if state.Data != nil {
			delete(state.Data, "error_message")
		}
	}

	// Call AfterExecute if defined
	if cj.Options.AfterExecute != nil {
		afterErr := cj.Options.AfterExecute(ctx, cj, jobErr)
		if afterErr != nil {
			cj.Options.Logger.Error(ctx, "AfterExecute callback returned an error", map[string]interface{}{"name": cj.Options.Name, "error": afterErr})
		}
	}

	// Final update after execution
	state.UpdatedAt = time.Now()
	if err = cj.State.Save(ctx, state); err != nil { // Save in-memory state
		cj.Options.Logger.Error(ctx, "Failed to save job state", map[string]interface{}{"error": err})
	}
}

// extendLockPeriodically keeps the lock alive and updates the heartbeat
func (cj *Job) extendLockPeriodically(ctx context.Context, state *JobState) {
	ttl := cj.Options.Locker.GetLockTTL()

	var extensionInterval time.Duration
	if ttl < 20*time.Second {
		extensionInterval = ttl / 2
	} else {
		extensionInterval = 10 * time.Second
	}

	ticker := time.NewTicker(extensionInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			cj.Options.Logger.Debug(ctx, "Stopped lock extension goroutine", map[string]interface{}{"name": cj.Options.Name})
			return
		case <-ticker.C:
			err := cj.Options.Locker.Extend(ctx)
			if err != nil {
				cj.Options.Logger.Error(ctx, "Failed to extend lock during job execution", map[string]interface{}{"name": cj.Options.Name, "error": err})
				return
			}
			cj.Options.Logger.Debug(ctx, "Lock extended successfully", map[string]interface{}{"name": cj.Options.Name})

			// Update heartbeat to mark the job as still alive
			state.UpdatedAt = time.Now()
			if err := cj.State.Save(ctx, state); err != nil { // Save in-memory state
				cj.Options.Logger.Error(ctx, "Failed to update heartbeat in job state", map[string]interface{}{"error": err})
			}
		}
	}
}

// IsRunningByMe checks if the job is currently running by this worker
func (cj *Job) IsRunningByMe(ctx context.Context, state *JobState) (bool, error) {
	if state == nil {
		return false, fmt.Errorf("job state is nil")
	}
	if state.RunningBy == "" || state.Status != JobStatusRunning {
		return false, nil
	}
	if time.Since(state.UpdatedAt) > StaleThreshold {
		return false, nil
	}

	runningBy, err := cj.Options.WorkerIdProvider.Id()
	if err != nil {
		return false, fmt.Errorf("failed to get worker ID: %w", err)
	}

	return state.RunningBy == runningBy, nil
}

// Start begins the job's execution loop
func (cj *Job) Start(ctx context.Context) error {
	// Load initial state
	state, err := cj.State.Get(ctx, true)
	if err != nil {
		cj.Options.Logger.Error(ctx, "Failed to load job state", map[string]interface{}{"error": err})
		return err
	}

	// If state is nil, initialize a default state
	if state == nil {
		state = &JobState{
			Status:     JobStatusNotRunning,
			LastRun:    time.Time{},
			NextRun:    cj.CronExpr.Next(time.Now()),
			Iterations: 0,                            // Initially 0 iterations
			Data:       make(map[string]interface{}), // Initialize as empty map
			UpdatedAt:  time.Now(),
			CreatedAt:  time.Now(),
		}
		// Optionally, store the cron spec in Data for potential updates
		state.Data["spec"] = cj.Options.Spec

		// Save the default state
		if saveErr := cj.State.Save(ctx, state); saveErr != nil {
			cj.Options.Logger.Error(ctx, "Failed to initialize job state", map[string]interface{}{"error": saveErr})
			return fmt.Errorf("failed to initialize job state: %w", saveErr)
		}
	}

	// Call BeforeStart if defined
	if cj.Options.BeforeStart != nil {
		shouldContinue, err := cj.Options.BeforeStart(ctx, cj)
		if err != nil {
			cj.Options.Logger.Error(ctx, "BeforeStart callback returned an error", map[string]interface{}{"name": cj.Options.Name, "error": err})
			return err
		}
		if !shouldContinue {
			cj.Options.Logger.Info(ctx, "Job start was cancelled by BeforeStart callback", map[string]interface{}{"name": cj.Options.Name})
			return nil
		}
	}

	go func() {
		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()

		isRunning := false
		refreshInterval := time.Until(state.NextRun) / 2
		if refreshInterval < time.Second {
			refreshInterval = time.Second
		}

		for {
			select {
			case <-ctx.Done():
				if isRunning {
					state.Status = JobStatusCancelled
					state.UpdatedAt = time.Now()
					if err := cj.State.Save(ctx, state); err != nil { // Save in-memory state
						cj.Options.Logger.Error(ctx, "Failed to save job state", map[string]interface{}{"error": err})
					}
					cj.Options.Logger.Info(ctx, "Cron job stopped by context", map[string]interface{}{"name": cj.Options.Name})
				} else {
					cj.Options.Logger.Info(ctx, "Cron job stopped by context without active execution", map[string]interface{}{"name": cj.Options.Name})
				}
				return
			case <-cj.StopSignal:
				if isRunning {
					state.Status = JobStatusCancelled
					state.UpdatedAt = time.Now()
					if err := cj.State.Save(ctx, state); err != nil { // Save in-memory state
						cj.Options.Logger.Error(ctx, "Failed to save job state", map[string]interface{}{"error": err})
					}
					cj.Options.Logger.Info(ctx, "Cron job force stopped while running", map[string]interface{}{"name": cj.Options.Name})
				} else {
					cj.Options.Logger.Info(ctx, "Cron job force stopped without active execution", map[string]interface{}{"name": cj.Options.Name})
				}
				return
			case <-ticker.C:
				now := time.Now()

				// Check if the job is stale
				if state.Status == JobStatusRunning && time.Since(state.UpdatedAt) > StaleThreshold {
					cj.Options.Logger.Warning(ctx, "Job appears stale, considering it not running", map[string]interface{}{"name": cj.Options.Name})
					state.RunningBy = ""
					state.Status = JobStatusFailed
					if state.Data == nil {
						state.Data = make(map[string]interface{})
					}
					state.Data["error_message"] = "stale job detected"
					state.UpdatedAt = time.Now()
					if err := cj.State.Save(ctx, state); err != nil { // Save in-memory state
						cj.Options.Logger.Error(ctx, "Failed to save stale job state", map[string]interface{}{"error": err})
					}
				}

				runningByMe, err := cj.IsRunningByMe(ctx, state)
				if err != nil {
					cj.Options.Logger.Error(ctx, "Failed to check if job is running by myself", map[string]interface{}{"error": err})
					continue
				}

				if runningByMe {
					isRunning = true
					continue
				} else {
					isRunning = false
				}

				if now.Before(state.NextRun) {
					cj.Options.Logger.Debug(ctx, "Skipping job execution as it's not yet time", map[string]interface{}{
						"name":     cj.Options.Name,
						"next_run": state.NextRun,
						"current":  now,
					})
					continue
				}

				// Execute the job
				isRunning = true
				cj.execute(ctx, state)
				isRunning = false

				// Sleep until just before the next run
				sleepDuration := time.Until(state.NextRun) - 1*time.Second
				if sleepDuration > 0 {
					time.Sleep(sleepDuration)
				} else {
					// If NextRun is in the past, set sleep to minimal duration
					time.Sleep(1 * time.Second)
				}
			}
		}
	}()
	return nil
}

func (cj *Job) GetState() IState {
	return cj.State
}
