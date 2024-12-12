package cron

import (
	"context"
	"cronlite/locker"
	"cronlite/logger"
	"errors"
	"fmt"
	"github.com/gorhill/cronexpr"
	"github.com/redis/go-redis/v9"
	"time"
)

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
		lockerOptions := locker.LockerOptions{
			Name:    options.Name,
			Redis:   options.Redis,
			Logger:  options.Logger,
			LockTTL: time.Until(expr.Next(time.Now())) + 5*time.Second,
		}
		options.Locker = locker.NewLocker(lockerOptions)
	}

	return &Job{
		Options:    options,
		CronExpr:   expr,
		StateKey:   fmt.Sprintf(JobStateKeyFormat, options.Name),
		StopSignal: make(chan bool),
	}, nil
}

func (cj *Job) Stop(ctx context.Context) error {
	close(cj.StopSignal)
	return nil
}

func (cj *Job) getLatestState(ctx context.Context, currentState *JobState) (*JobState, error) {
	redisState, err := LoadJobStateFromRedis(ctx, cj.Options.Redis, cj.Options.Name)
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return currentState, nil // no newer state exists
		}
		return nil, fmt.Errorf("failed to fetch latest job state: %w", err)
	}

	if redisState.LastRun.After(currentState.LastRun) ||
		redisState.NextRun.After(currentState.NextRun) ||
		redisState.UpdatedAt.After(currentState.UpdatedAt) {
		cj.Options.Logger.Debug(ctx, "Updated job settings detected in Redis", map[string]interface{}{"name": cj.Options.Name})
		return redisState, nil
	}
	return currentState, nil
}

func (cj *Job) execute(ctx context.Context, state *JobState) {
	cj.Mutex.Lock()
	defer cj.Mutex.Unlock()

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
				state.Data = map[string]interface{}{}
			}
			state.Data["error_message"] = err.Error()
			state.UpdatedAt = time.Now()
			if saveErr := cj.SaveState(ctx, state); saveErr != nil {
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
			state.Data = map[string]interface{}{}
		}
		state.Data["error_message"] = err.Error()
		state.UpdatedAt = time.Now()
		if saveErr := cj.SaveState(ctx, state); saveErr != nil {
			cj.Options.Logger.Error(ctx, "Failed to save job state", map[string]interface{}{"error": saveErr})
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

	if err = cj.SaveState(ctx, state); err != nil {
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
			state.Data = map[string]interface{}{}
		}
		state.Data["error_message"] = jobErr.Error()
	} else {
		cj.Options.Logger.Info(ctx, "Cron job executed successfully", map[string]interface{}{"name": cj.Options.Name})
		state.Status = JobStatusSuccess
		if state.Data != nil {
			delete(state.Data, "error_message")
		}
	}

	// Final update after execution
	state.UpdatedAt = time.Now()
	if err = cj.SaveState(ctx, state); err != nil {
		cj.Options.Logger.Error(ctx, "Failed to save job state", map[string]interface{}{"error": err})
	}

	// Call AfterExecute if defined
	if cj.Options.AfterExecute != nil {
		afterErr := cj.Options.AfterExecute(ctx, cj, jobErr)
		if afterErr != nil {
			cj.Options.Logger.Error(ctx, "AfterExecute callback returned an error", map[string]interface{}{"name": cj.Options.Name, "error": afterErr})
		}
	}
}

func (cj *Job) SaveState(ctx context.Context, state *JobState) error {
	return SaveJobStateToRedis(ctx, cj.Options.Redis, cj.Options.Name, state)
}

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
			if saveErr := cj.SaveState(ctx, state); saveErr != nil {
				cj.Options.Logger.Error(ctx, "Failed to update heartbeat in job state", map[string]interface{}{"error": saveErr})
			}
		}
	}
}

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

func (cj *Job) Start(ctx context.Context) error {
	// Load initial state
	state, err := cj.GetState(ctx)
	if err != nil {
		cj.Options.Logger.Error(ctx, "Failed to fetch job state", map[string]interface{}{"error": err})
		return err
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
		timer := time.NewTicker(1 * time.Second)
		defer timer.Stop()

		isRunning := false
		refreshInterval := time.Until(state.NextRun) / 2
		if refreshInterval < time.Second {
			refreshInterval = time.Second
		}
		lastFetchTime := time.Now()

		for {
			select {
			case <-ctx.Done():
				if isRunning {
					state.Status = JobStatusCancelled
					state.UpdatedAt = time.Now()
					if err = cj.SaveState(ctx, state); err != nil {
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
					if err = cj.SaveState(ctx, state); err != nil {
						cj.Options.Logger.Error(ctx, "Failed to save job state", map[string]interface{}{"error": err})
					}
					cj.Options.Logger.Info(ctx, "Cron job force stopped while running", map[string]interface{}{"name": cj.Options.Name})
				} else {
					cj.Options.Logger.Info(ctx, "Cron job force stopped without active execution", map[string]interface{}{"name": cj.Options.Name})
				}
				return
			case <-timer.C:
				now := time.Now()

				// Update state if needed
				if now.Sub(lastFetchTime) >= refreshInterval && (now.After(state.NextRun) || state.NextRun.Sub(now) <= 10*time.Second) {
					updatedState, err := cj.getLatestState(ctx, state)
					if err != nil {
						cj.Options.Logger.Error(ctx, "Failed to fetch updated job state", map[string]interface{}{"error": err})
					} else {
						state = updatedState
						lastFetchTime = time.Now()
						refreshInterval = time.Until(state.NextRun) / 2
						if refreshInterval < time.Second {
							refreshInterval = time.Second
						}
					}

					// Check if spec has been dynamically updated and change it and expression
					//currentSpec := cj.Options.Spec
					//updatedSpecData := updatedState.Data["spec"]
					//var updatedSpec string
					//if updatedSpecData != nil {
					//	updatedSpec = updatedSpecData.(string)
					//}
					//if currentSpec != updatedSpec {
					//	cj.Options.Logger.Info(ctx, "Cron job spec has been dynamically updated", map[string]interface{}{
					//		"name":    cj.Options.Name,
					//		"current": currentSpec,
					//		"updated": updatedSpec,
					//	})
					//	newCronExpr, err := cronexpr.Parse(updatedSpec)
					//	if err != nil {
					//		cj.Options.Logger.Error(ctx, "Failed to parse cron expression", map[string]interface{}{
					//			"name":       cj.Options.Name,
					//			"expression": cj.Options.Spec,
					//			"error":      err,
					//		})
					//		state.Data["spec"] = currentSpec
					//		err := cj.SaveState(ctx, state)
					//		if err != nil {
					//			cj.Options.Logger.Error(ctx, "Failed to save stale job state", map[string]interface{}{"error": err})
					//		}
					//		continue
					//	}
					//	cj.Options.Spec = updatedSpec
					//	cj.CronExpr = newCronExpr
					//	state.NextRun = cj.CronExpr.Next(time.Now())
					//	cj.Options.Logger.Info(ctx, "Cron job spec updated spec successfully", map[string]interface{}{"name": cj.Options.Name})
					//}
				}

				// Check if stale
				if state.Status == JobStatusRunning && time.Since(state.UpdatedAt) > StaleThreshold {
					cj.Options.Logger.Warning(ctx, "Job appears stale, considering it not running", map[string]interface{}{"name": cj.Options.Name})
					state.RunningBy = ""
					state.Status = JobStatusFailed
					if state.Data == nil {
						state.Data = map[string]interface{}{}
					}
					state.Data["error_message"] = "stale job detected"
					state.UpdatedAt = time.Now()
					if err := cj.SaveState(ctx, state); err != nil {
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
				time.Sleep(time.Until(state.NextRun) - 1*time.Second)
			}
		}
	}()
	return nil
}

func (cj *Job) GetState(ctx context.Context) (*JobState, error) {
	state, err := LoadJobStateFromRedis(ctx, cj.Options.Redis, cj.Options.Name)
	if err != nil {
		if errors.Is(err, redis.Nil) {
			// Initialize default state if not found, incorporating MaxIterations and UntilTime from JobOptions
			defaultState := &JobState{
				Status:     "", // Not running
				LastRun:    time.Time{},
				NextRun:    cj.CronExpr.Next(time.Now()),
				Iterations: 0,                        // Initially 0 iterations
				Data:       map[string]interface{}{}, // Initialize as empty map
				UpdatedAt:  time.Now(),
				CreatedAt:  time.Now(),
			}
			// Optionally, store the cron spec in Data for potential updates
			defaultState.Data["spec"] = cj.Options.Spec

			if saveErr := cj.SaveState(ctx, defaultState); saveErr != nil {
				return nil, fmt.Errorf("failed to initialize job state: %w", saveErr)
			}
			return defaultState, nil
		}
		return nil, fmt.Errorf("failed to fetch job state from Redis: %w", err)
	}

	// Update cron expr if spec is present in state
	if spec, ok := state.Data["spec"].(string); ok {
		cj.CronExpr = cronexpr.MustParse(spec)
		state.NextRun = cj.CronExpr.Next(time.Now())
	}

	// If UpdatedAt is zero, set it now
	if state.UpdatedAt.IsZero() {
		state.UpdatedAt = time.Now()
		if saveErr := cj.SaveState(ctx, state); saveErr != nil {
			cj.Options.Logger.Error(ctx, "Failed to update job state with updated_at", map[string]interface{}{"error": saveErr})
		}
	}

	return state, nil
}

func (cj *Job) Delete(ctx context.Context) error {
	pipe := cj.Options.Redis.TxPipeline()
	pipe.Del(ctx, cj.StateKey)
	pipe.ZRem(ctx, JobsList, cj.Options.Name)

	_, err := pipe.Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to delete job from Redis: %w", err)
	}
	cj.Options.Logger.Info(ctx, "Job deleted successfully", map[string]interface{}{"name": cj.Options.Name})
	return nil
}
