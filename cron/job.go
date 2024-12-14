package cron

import (
	"context"
	"cronlite/locker"
	"cronlite/logger"
	"fmt"
	"github.com/gorhill/cronexpr"
	"github.com/redis/go-redis/v9"
	"sync"
	"time"
)

type ICronJob interface {
	Start(ctx context.Context) error
	Stop(ctx context.Context) error
	GetState() IState
	OnStateUpdated(ctx context.Context, state *CronJobState) error
	GetOptions() *CronJobOptions
}

// CronJobOptions defines the options for creating a new cron job.
type CronJobOptions struct {
	Name   string         // The unique name of the cron job.
	Spec   string         // The cron expression specifying the schedule for the job.
	Redis  redis.Cmdable  // A Redis client used for state management and locking.
	Locker locker.ILocker // An optional locker interface for distributed locking.

	// ExecuteFunc The function to be executed by the cron job.
	ExecuteFunc func(ctx context.Context, job ICronJob) error

	// BeforeStartFunc An optional callback executed before the job starts.
	// Returns a boolean indicating whether to continue and an error if any.
	BeforeStartFunc func(ctx context.Context, job ICronJob) (bool, error)

	// BeforeExecuteFunc An optional callback executed before the job's main function.
	// Returns a boolean indicating whether to continue and an error if any.
	BeforeExecuteFunc func(ctx context.Context, job ICronJob) (bool, error)

	// AfterExecuteFunc An optional callback executed after the job's main function.
	// Receives the error from the main function execution, if any.
	AfterExecuteFunc func(ctx context.Context, job ICronJob, err error) error

	// WorkerIdProvider An interface for providing a unique worker ID.
	WorkerIdProvider IWorkerIdProvider
}

type CronJob struct {
	Options        CronJobOptions
	Expression     *cronexpr.Expression
	StopSignal     chan bool
	State          IState
	isRunning      bool
	isRunningMutex sync.RWMutex
}

// NewCronJob creates a new CronJob instance with the provided options.
// It validates the necessary options and initializes the cron job with a parsed cron expression.
// If a locker is not provided, it creates a default locker using the provided Redis client.
//
// Parameters:
//   - options: CronJobOptions
//     The options for creating a new cron job, including the job's name, cron expression,
//     Redis client, execution function, and optional callbacks.
//
// Returns:
//   - ICronJob: The created CronJob instance implementing the ICronJob interface.
//   - error: An error if any required option is missing or invalid, or if the cron expression is invalid.
func NewCronJob(options CronJobOptions) (ICronJob, error) {
	if options.Redis == nil {
		return nil, ErrRedisClientRequired
	}

	if options.Name == "" {
		return nil, ErrCronNameRequired
	}

	if options.Spec == "" {
		return nil, ErrCronExpressionRequired
	}
	expr, err := cronexpr.Parse(options.Spec)
	if err != nil {
		return nil, fmt.Errorf("invalid cron expression: %w", err)
	}

	if options.ExecuteFunc == nil {
		return nil, ErrCronFunctionRequired
	}

	if options.WorkerIdProvider == nil {
		options.WorkerIdProvider = &DefaultWorkerIDProvider{}
	}

	if options.Locker == nil {
		lockerOptions := locker.Options{
			Name:    options.Name,
			Redis:   options.Redis,
			LockTTL: time.Until(expr.Next(time.Now())) + 5*time.Second,
		}
		options.Locker = locker.NewLocker(lockerOptions)
	}

	cron := &CronJob{
		Options:    options,
		Expression: expr,
		StopSignal: make(chan bool),
	}

	stateKey := fmt.Sprintf(JobStateKeyFormat, options.Name)
	cron.State = NewState(
		options.Redis,
		stateKey,
		cron.OnStateUpdated,
	)

	return cron, nil
}

// OnStateUpdated handles updates to the cron job's state.
//
// This function is triggered when the state of the cron job is updated.
// It logs the updated state and checks if the cron expression (Spec) has changed.
// If the Spec has changed, it calls the onSpecUpdated method to handle the update.
//
// Parameters:
//
//   - ctx: context.Context
//     The context for managing request-scoped values, cancellation signals, and deadlines.
//
//   - state: *CronJobState
//     The updated state of the cron job, which includes the new cron expression (Spec) if it has changed.
//
// Returns:
//   - error: An error if the cron expression update fails, otherwise nil.
func (cj *CronJob) OnStateUpdated(ctx context.Context, state *CronJobState) error {
	logger.Log(ctx).
		WithValues("name", cj.Options.Name, "state", state).
		Debug(MessageJobStateUpdated)
	if state.Spec != "" && state.Spec != cj.Options.Spec {
		return cj.onSpecUpdated(ctx, state)
	}
	return nil
}

// onSpecUpdated updates the cron job's schedule based on a new cron expression.
//
// This function is called when the cron expression (Spec) of the job is updated.
// It parses the new cron expression, updates the job's schedule, and recalculates
// the next run time based on the new expression.
//
// Parameters:
//
//   - ctx: context.Context
//     The context for managing request-scoped values, cancellation signals, and deadlines.
//
//   - newState: *CronJobState
//     The updated state of the cron job, which includes the new cron expression (Spec).
//
// Returns:
//   - error: An error if the new cron expression is invalid, otherwise nil.
func (cj *CronJob) onSpecUpdated(ctx context.Context, newState *CronJobState) error {
	log := cj.getLogger(ctx)
	log.WithValues("new_spec", newState.Spec, "old_spec", cj.Options.Spec).
		Debug(MessageUpdatingCronExpression)

	expr, err := cronexpr.Parse(newState.Spec)
	if err != nil {
		log.WithError(err).Error(ErrMessageInvalidCronExpression)
		return err
	}

	cj.Expression = expr
	cj.Options.Spec = newState.Spec
	cj.Options.Locker.SetTTL(time.Until(cj.Expression.Next(time.Now())))

	if newState.LastRun.IsZero() {
		newState.LastRun = time.Now()
		log.Debug(MessageInitializingLastRunToCurrentTime)
	}
	newState.NextRun = cj.Expression.Next(newState.LastRun)
	log.WithValues("next_run", newState.NextRun).
		Debug(MessageRecalculatedNextRunFromNewSpec)
	return err
}

// Stop halts the execution of the cron job by closing the StopSignal channel.
//
// This function is used to signal the cron job to stop its execution loop.
// It is typically called when the job needs to be gracefully terminated.
//
// Parameters:
//   - ctx: context.Context
//     The context for managing request-scoped values, cancellation signals, and deadlines.
//
// Returns:
//   - error: Always returns nil as there is no error handling in this function.
func (cj *CronJob) Stop(ctx context.Context) error {
	close(cj.StopSignal)
	return nil
}

// onBeforeExecute executes the BeforeExecuteFunc callback if defined,
// and updates the job state based on the callback's result.
//
// Parameters:
//
//   - ctx: context.Context
//     The context for managing request-scoped values, cancellation signals, and deadlines.
//
//   - state: *CronJobState
//     The current state of the cron job, which will be updated based on the callback's result.
//
// Returns:
//   - bool: A boolean indicating whether to continue execution based on the callback's result.
//   - error: An error if the callback execution fails, otherwise nil.
func (cj *CronJob) onBeforeExecute(ctx context.Context, state *CronJobState) (bool, error) {
	log := cj.getLogger(ctx).WithValues("state", state)
	if continueExec, err := cj.Options.BeforeExecuteFunc(ctx, cj); err != nil {
		state.Status = JobStatusFailed
		state.AddData("error_message", err.Error())
		log = log.WithValues("state", state)
		log.WithError(err).Error(MessageFailedToExecuteBeforeExecuteFunc)
		saveErr := cj.State.Save(ctx, state)
		log = log.WithValues("state", state)
		if saveErr != nil {
			log.WithError(saveErr).Error(MessageFailedToSaveJobStateAfterBeforeExecuteFunc)
		}
		return false, err
	} else {
		if !continueExec {
			log.Debug(MessageJobExecutionCanceledByBeforeExecuteFunc)
		}
		return continueExec, nil
	}
}

// execute runs the main execution logic of the cron job, including pre-execution
// hooks, the main execution function, and post-execution hooks. It also manages
// the job's state and handles errors during execution.
//
// Parameters:
//
//   - ctx: context.Context
//     The context for managing request-scoped values, cancellation signals, and deadlines.
//
//   - state: *CronJobState
//     The current state of the cron job, which will be updated throughout the execution process.
func (cj *CronJob) execute(ctx context.Context, state *CronJobState) {
	log := cj.getLogger(ctx).WithValues("state", state)

	// Execute BeforeExecuteFunc hook
	if cj.Options.BeforeExecuteFunc != nil {
		continueExec, err := cj.onBeforeExecute(ctx, state)
		if err != nil || !continueExec {
			return
		}
	}

	// Increment iterations and set RunningBy
	state.Iterations++
	var err error
	state.RunningBy, err = cj.Options.WorkerIdProvider.Id()
	if err != nil {
		state.Status = JobStatusFailed
		state.AddData("error_message", err.Error())
		log = log.WithValues("state", state)
		log.WithError(err).Error(MessageFailedToGetWorkerId)
		saveErr := cj.State.Save(ctx, state)
		log = log.WithValues("state", state)
		if saveErr != nil {
			log.WithError(saveErr).Error(MessageFailedToSaveJobStateAfterWorkerId)
		}
		return
	} else {
		now := time.Now()
		state.LastRun = now
		state.NextRun = cj.Expression.Next(now)
		state.Status = JobStatusRunning
		log = log.WithValues("state", state)
	}

	if cj.Options.AfterExecuteFunc != nil {
		afterErr := cj.Options.AfterExecuteFunc(ctx, cj, err)
		if afterErr != nil {
			state.Status = JobStatusFailed
			state.AddData("error_message", err.Error())
			log = log.WithValues("state", state)
			log.WithError(afterErr).Error(MessageFailedToExecuteAfterExecuteFunc)
		}

		// Save state after pre-execute hook
		if err = cj.State.Save(ctx, state); err != nil {
			log = log.WithValues("state", state)
			log.WithError(err).Error(MessageFailedToSaveJobAfterExecution)
		}
	}
	// Keep lock alive and heartbeat updated periodically
	lockCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	go cj.extendLockPeriodically(lockCtx, state)

	log.Debug(MessageExecutingCronJob)
	jobErr := cj.Options.ExecuteFunc(ctx, cj)

	if jobErr != nil {
		state.Status = JobStatusFailed
		state.AddData("error_message", jobErr.Error())
		log = log.WithValues("state", state)
		log.WithError(jobErr).Error(MessageExecutionFailed)
	} else {
		state.Status = JobStatusSuccess
		state.RemoveData("error_message")
		log = log.WithValues("state", state)
		log.Debug(MessageExecutionSucceeded)
	}

	if err = cj.State.Save(ctx, state); err != nil {
		log = log.WithValues("state", state)
		log.WithError(err).Error(MessageFailedToSaveJobAfterExecution)
	}

	// Call AfterExecuteFunc if defined
	if cj.Options.AfterExecuteFunc != nil {
		afterErr := cj.Options.AfterExecuteFunc(ctx, cj, jobErr)
		log = log.WithValues("state", state)
		if afterErr != nil {
			log.WithError(afterErr).Error(MessageFailedToExecuteAfterExecuteFunc)
		}
	}

	// Save final state
	if err = cj.State.Save(ctx, state); err != nil {
		log = log.WithValues("state", state)
		log.WithError(err).Error(MessageFailedToSaveJobStateAfterPostExecutionFunc)
	}
}

// extendLockPeriodically extends the lock on the cron job at regular intervals
// to ensure that the job remains active and prevents other instances from acquiring the lock.
//
// Parameters:
//   - ctx: context.Context
//     The context for managing request-scoped values, cancellation signals, and deadlines.
//   - state: *CronJobState
//     The current state of the cron job, which is used for logging and updating the job's heartbeat.
//
// This function does not return any values. It runs indefinitely until the context is done
// or a stop signal is received, at which point it logs the reason for stopping and exits.
func (cj *CronJob) extendLockPeriodically(ctx context.Context, state *CronJobState) {
	ttl := cj.Options.Locker.GetLockTTL()

	var extensionInterval time.Duration
	if ttl < 20*time.Second {
		extensionInterval = ttl / 2
		if ttl < 1*time.Second {
			extensionInterval = 1 * time.Second
		}
	} else {
		extensionInterval = 10 * time.Second
	}

	ticker := time.NewTicker(extensionInterval)
	defer ticker.Stop()

	log := cj.getLogger(ctx).WithValues("state", state)

	// Release lock when the function exits
	defer func() {
		if err := cj.Options.Locker.Release(ctx); err != nil {
			log.WithError(err).Error(MessageFailedToReleaseLock)
		}
	}()

	for {
		select {
		case <-ctx.Done():
			log.WithValues("reason", "context.Done").
				Debug(MessageLockExtensionGoRoutineStopped)
			return
		case <-cj.StopSignal:
			log.WithValues("reason", "StopSignal").
				Debug(MessageLockExtensionGoRoutineStopped)
			return
		case <-ticker.C:
			err := cj.Options.Locker.Extend(ctx)
			if err != nil {
				log.WithError(err).Error(MessageFailedToExtendLock)
				return
			}
			log.Debug(MessageLockExtended)

			// Update heartbeat to mark the job as still alive
			if err := cj.State.Save(ctx, state); err != nil {
				log = log.WithValues("state", state)
				log.WithError(err).Error(MessageFailedToSaveJobStateAfterLockExtension)
			}
		}
	}
}

// IsRunningByMe checks if the current cron job instance is the one running the job.
//
// Parameters:
//   - ctx: context.Context
//     The context for managing request-scoped values, cancellation signals, and deadlines.
//   - state: *CronJobState
//     The current state of the cron job, which includes information about the job's execution status.
//
// Returns:
//   - bool: A boolean indicating whether the current instance is running the job.
//   - error: An error if there is a failure in retrieving the worker ID or if the state is nil.
func (cj *CronJob) IsRunningByMe(ctx context.Context, state *CronJobState) (bool, error) {
	if state == nil {
		return false, ErrCronStateEmpty
	}
	if state.RunningBy == "" || state.Status != JobStatusRunning {
		return false, nil
	}

	cj.isRunningMutex.RLock()
	running := cj.isRunning
	cj.isRunningMutex.RUnlock()

	if !running {
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

// getLogger returns a logger instance with the cron job's name and worker ID.
//
// This function is used to create a logger that includes contextual information
// about the cron job, such as its name and the ID of the worker executing it.
//
// Parameters:
//   - ctx: context.Context
//     The context for managing request-scoped values, cancellation signals, and deadlines.
//
// Returns:
//   - logger.ILogger: A logger instance with the cron job's name and worker ID as context.
func (cj *CronJob) getLogger(ctx context.Context) logger.ILogger {
	workerId, _ := cj.Options.WorkerIdProvider.Id()
	return logger.Log(ctx).WithValues("name", cj.Options.Name, "worker_id", workerId)
}

// Start initiates the execution of the cron job, managing its lifecycle and state.
//
// This function loads the initial state of the cron job, initializes it if necessary,
// and starts a goroutine to handle the job's execution based on its schedule and state.
// It also handles pre-start hooks and manages the job's locking mechanism.
//
// Parameters:
//   - ctx: context.Context
//     The context for managing request-scoped values, cancellation signals, and deadlines.
//
// Returns:
//   - error: An error if the initial state loading or saving fails, or if the pre-start hook returns an error.
func (cj *CronJob) Start(ctx context.Context) error {
	// Load initial state
	state, err := cj.State.Get(ctx, true)
	log := cj.getLogger(ctx).WithValues("state", state)
	if err != nil {
		log.WithError(err).Error(MessageFailedToLoadJobState)
		return err
	}

	// If state is nil, initialize a default state
	if state == nil {
		state = &CronJobState{
			Status:     JobStatusNotRunning,
			LastRun:    time.Time{},
			Spec:       cj.Options.Spec,
			NextRun:    cj.Expression.Next(time.Now()),
			Iterations: 0,                            // Initially 0 iterations
			Data:       make(map[string]interface{}), // Initialize as empty map
			UpdatedAt:  time.Now(),
			CreatedAt:  time.Now(),
		}
		log = log.WithValues("state", state)

		// Save the default state
		if saveErr := cj.State.Save(ctx, state); saveErr != nil {
			log = log.WithValues("state", state)
			log.WithError(saveErr).Error(MessageFailedToSaveDefaultJobState)
			return saveErr
		}
	}

	// Call BeforeStartFunc if defined
	if cj.Options.BeforeStartFunc != nil {
		shouldContinue, err := cj.Options.BeforeStartFunc(ctx, cj)
		log = log.WithValues("state", state)
		if err != nil {
			log.WithError(err).Error(MessageFailedToExecuteBeforeStartFunc)
			return err
		}
		if !shouldContinue {
			log.Debug(MessageStoppingJobDueToBeforeStartFunc)
			return nil
		}
	}

	go func() {
		tickerPeriod := time.Second
		ticker := time.NewTicker(tickerPeriod)
		defer ticker.Stop()

		cj.isRunningMutex.Lock()
		cj.isRunning = false
		cj.isRunningMutex.Unlock()
		for {
			select {
			case <-ctx.Done():
				cj.onContextDone(ctx, state)
				return
			case <-cj.StopSignal:
				cj.onStopSignal(ctx, state)
				return
			case <-ticker.C:
				// Load fresh state
				state, err = cj.State.Get(ctx, true)
				log = log.WithValues("state", state)
				if err != nil {
					log.WithError(err).Error(MessageFailedToLoadJobState)
					continue
				}

				// Job is still running but too long since last update? It is considered stale.
				if state.Status == JobStatusRunning && time.Since(state.UpdatedAt) > StaleThreshold {
					log.Warn(MessageJobIsStale)
				}

				// Check if it's too early to run the job
				now := time.Now()
				if now.Before(state.NextRun) {
					log.Debug(MessageJobTooEarlyToRun)
					// Calculate sleep time until next run
					// If it is more than ticker, sleep until next run and continue to next cycle
					// Otherwise don't sleep and just let the locker try to acquire
					if tickerPeriod < time.Until(state.NextRun) {
						time.Sleep(time.Until(state.NextRun))
						continue
					}
				}

				// Acquire lock to execute the job
				now = time.Now() // <- get new now before acquiring lock (which can be slow)
				success, err := cj.Options.Locker.Acquire(ctx)
				if err != nil || !success {
					log.WithError(err).Debug(MessageFailedToAcquireLock)
					if now.After(state.NextRun.Add(time.Second)) {
						log.Debug(MessageJobTakenByOtherWorkers)
						cj.sleepUntilNextRun()
					}
					continue
				}

				cj.isRunningMutex.Lock()
				cj.isRunning = true
				cj.isRunningMutex.Unlock()
				cj.execute(ctx, state)
				cj.isRunningMutex.Lock()
				cj.isRunning = false
				cj.isRunningMutex.Unlock()
				cj.sleepUntilNextRun()
			}
		}
	}()
	return nil
}

// sleepUntilNextRun pauses the execution of the cron job until the next scheduled run time.
//
// This function calculates the duration to sleep based on the next scheduled run time
// of the cron job. If the next run time is in the past, it defaults to a minimal sleep duration.
func (cj *CronJob) sleepUntilNextRun() {
	now := time.Now()
	nextRun := cj.Expression.Next(now)
	sleepDuration := time.Until(nextRun) + 1*time.Second
	if sleepDuration > 0 {
		time.Sleep(sleepDuration)
	} else {
		// If NextRun is in the past, set sleep to minimal duration
		time.Sleep(1 * time.Second)
	}
}

func (cj *CronJob) GetState() IState {
	return cj.State
}

// onContextDone handles the termination of a cron job when the context is done.
//
// This function is called when the context associated with the cron job is cancelled or times out.
// It updates the job's state to indicate cancellation if the job was actively running,
// and logs the appropriate message based on the job's execution status.
//
// Parameters:
//   - ctx: context.Context
//     The context for managing request-scoped values, cancellation signals, and deadlines.
//   - state: *CronJobState
//     The current state of the cron job, which will be updated to reflect cancellation if running.
func (cj *CronJob) onContextDone(ctx context.Context, state *CronJobState) {
	log := cj.getLogger(ctx)
	cj.isRunningMutex.RLock()
	running := cj.isRunning
	cj.isRunningMutex.RUnlock()
	if running {
		state.Status = JobStatusCancelled
		err := cj.State.Save(ctx, state)
		log = log.WithValues("state", state)
		if err != nil {
			log.WithError(err).Error(MessageFailedToSaveJobState)
		}
		log.Debug(MessageJobStoppedByContextDuringActiveExecution)
	} else {
		log.Debug(MessageJobStoppedByContextWithoutActiveExecution)
	}
}

// onStopSignal handles the termination of a cron job when a stop signal is received.
//
// This function is called when the cron job receives a stop signal, indicating that
// it should cease execution. It updates the job's state to indicate cancellation if
// the job was actively running, and logs the appropriate message based on the job's
// execution status.
//
// Parameters:
//   - ctx: context.Context
//     The context for managing request-scoped values, cancellation signals, and deadlines.
//   - state: *CronJobState
//     The current state of the cron job, which will be updated to reflect cancellation if running.
func (cj *CronJob) onStopSignal(ctx context.Context, state *CronJobState) {
	log := cj.getLogger(ctx)
	cj.isRunningMutex.RLock()
	running := cj.isRunning
	cj.isRunningMutex.RUnlock()
	if running {
		state.Status = JobStatusCancelled
		err := cj.State.Save(ctx, state)
		log = log.WithValues("state", state)
		if err != nil {
			log.WithError(err).Error(MessageFailedToSaveJobState)
		}
		log.Debug(MessageJobStoppedBySignalDuringActiveExecution)
	} else {
		log.Debug(MessageJobStoppedBySignalWithoutActiveExecution)
	}
}

func (cj *CronJob) GetOptions() *CronJobOptions {
	return &cj.Options
}
