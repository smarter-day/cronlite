package cron

import (
	"context"
	"cronlite/locker"
	"fmt"
	"github.com/redis/go-redis/v9"
	cronParser "github.com/robfig/cron/v3"
	"github.com/smarter-day/logger"
	"sync"
	"time"
)

var (
	SpecParser = cronParser.NewParser(
		cronParser.Second |
			cronParser.Minute |
			cronParser.Hour |
			cronParser.Dom |
			cronParser.Month |
			cronParser.Dow |
			cronParser.Descriptor,
	)
	staleDuration = 10 * time.Second
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
	Scheduler      cronParser.Schedule
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

	if options.ExecuteFunc == nil {
		return nil, ErrCronFunctionRequired
	}

	if options.WorkerIdProvider == nil {
		options.WorkerIdProvider = &DefaultWorkerIDProvider{}
	}

	stateKey := fmt.Sprintf(JobStateKeyFormat, options.Name)
	cronJob := &CronJob{
		Options:    options,
		StopSignal: make(chan bool),
	}

	// Initialize the scheduler
	if err := cronJob.initScheduler(context.Background(), options.Spec); err != nil {
		return nil, err
	}

	// Initialize the locker
	cronJob.initLocker()

	// Initialize the state
	cronJob.State = NewState(
		options.Redis,
		stateKey,
		cronJob.OnStateUpdated,
	)

	return cronJob, nil
}

// initScheduler initializes the scheduler for the cron job based on the provided spec.
// Returns an error if the spec is invalid.
func (cj *CronJob) initScheduler(ctx context.Context, spec string) error {
	scheduler := cj.getSchedulerBySpec(ctx, spec)
	if scheduler == nil {
		return fmt.Errorf("failed to parse cron spec: %s", spec)
	}
	cj.Scheduler = scheduler
	return nil
}

// initLocker initializes the locker for the cron job.
func (cj *CronJob) initLocker() {
	if cj.Options.Locker == nil {
		ttl := time.Until(cj.Scheduler.Next(time.Now().Add(time.Second)))
		if ttl < time.Second {
			ttl = time.Second
		}
		cj.Options.Locker = locker.NewLocker(locker.Options{
			Name:    cj.Options.Name,
			Redis:   cj.Options.Redis,
			LockTTL: ttl,
		})
	}
}

// getSchedulerBySpec parses the spec. Uses "%v" in log to avoid incorrect "%w" usage.
func (cj *CronJob) getSchedulerBySpec(ctx context.Context, spec string) cronParser.Schedule {
	scheduler, err := SpecParser.Parse(spec)
	if err != nil {
		logger.Log(ctx).
			WithError(err).
			WithValues("spec", spec).
			Error("Failed to parse cron spec")
		return nil
	}
	return scheduler
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

	scheduler := cj.getSchedulerBySpec(ctx, newState.Spec)
	if scheduler == nil {
		return fmt.Errorf("failed to parse new cron spec: %s", newState.Spec)
	}

	cj.Scheduler = scheduler
	cj.Options.Spec = newState.Spec
	ttl := time.Until(scheduler.Next(time.Now()))
	if ttl < time.Second {
		ttl = time.Second
	}
	cj.Options.Locker.SetTTL(ttl)

	if newState.LastRun.IsZero() {
		newState.LastRun = time.Now()
		log.Debug(MessageInitializingLastRunToCurrentTime)
	}
	newState.NextRun = cj.Scheduler.Next(newState.LastRun)
	log.WithValues("next_run", newState.NextRun).
		Debug(MessageRecalculatedNextRunFromNewSpec)
	return nil
}

// Stop halts the execution of the cron job by closing the StopSignal channel safely.
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
	// Protect channel close with sync.Once or a check to avoid panic on multiple closes:
	cj.isRunningMutex.Lock()
	defer cj.isRunningMutex.Unlock()

	select {
	case <-cj.StopSignal:
		// already closed
	default:
		close(cj.StopSignal)
	}
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
	cj.isRunningMutex.Lock()
	cj.isRunning = true
	cj.isRunningMutex.Unlock()

	defer func() {
		cj.isRunningMutex.Lock()
		cj.isRunning = false
		cj.isRunningMutex.Unlock()
	}()

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
	workerID, err := cj.Options.WorkerIdProvider.Id()
	if err != nil {
		state.Status = JobStatusFailed
		state.AddData("error_message", err.Error())
		log = log.WithValues("state", state)
		log.WithError(err).Error("Failed to get worker ID")
		if saveErr := cj.State.Save(ctx, state); saveErr != nil {
			log.WithError(saveErr).Error("Failed to save job state after worker ID error")
		}
		return
	}

	now := time.Now()
	state.RunningBy = workerID
	state.LastRun = now
	state.NextRun = cj.Scheduler.Next(now)
	state.Status = JobStatusRunning
	log = log.WithValues("state", state)

	// Save state after we set it Running
	if err = cj.State.Save(ctx, state); err != nil {
		log.WithError(err).Error("Failed to save job state after setting running status")
	}

	// Keep lock alive periodically
	lockCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	go cj.extendLockPeriodically(lockCtx, state)

	log.Debug("Executing cron job")

	// Execute main job
	jobErr := cj.Options.ExecuteFunc(ctx, cj)

	// Update state based on execution result
	if jobErr != nil {
		state.Status = JobStatusFailed
		state.AddData("error_message", jobErr.Error())
		log.WithError(jobErr).Error("Job execution failed")
	} else {
		state.Status = JobStatusSuccess
		state.RemoveData("error_message")
		log.Debug("Job execution succeeded")
	}

	// Save state after execution
	if err := cj.State.Save(ctx, state); err != nil {
		log.WithError(err).Error("Failed to save job state after execution")
	}

	// Call AfterExecuteFunc if defined
	if cj.Options.AfterExecuteFunc != nil {
		afterErr := cj.Options.AfterExecuteFunc(ctx, cj, jobErr)
		if afterErr != nil {
			log.WithError(afterErr).Error("Failed to execute after-execute func")
		}
	}

	// Final state save
	if err := cj.State.Save(ctx, state); err != nil {
		log.WithError(err).Error("Failed to save job state after post-execution func")
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

	if ttl < time.Second {
		ttl = 1 * time.Second
	} else if ttl > staleDuration {
		ttl = staleDuration
	}

	// Set extension interval to 80% of TTL to ensure timely extensions.
	extensionInterval := time.Duration(float64(ttl) * 0.8)

	// Ensure a minimum extension interval to avoid very short intervals.

	ticker := time.NewTicker(extensionInterval)
	defer ticker.Stop()

	log := cj.getLogger(ctx).WithValues("state", state)

	// Ensure the lock is released when the function exits.
	defer func() {
		_ = cj.Options.Locker.Release(ctx)
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

func (cj *CronJob) getState(ctx context.Context, force bool) (*CronJobState, error) {
	// Load initial state
	state, err := cj.State.Get(ctx, force)
	log := cj.getLogger(ctx).WithValues("state", state)
	if err != nil {
		log.WithError(err).Error(MessageFailedToLoadJobState)
		return nil, err
	}

	// If state is nil, initialize a default state
	if state == nil {
		state = &CronJobState{
			Status:     JobStatusNotRunning,
			LastRun:    time.Time{},
			Spec:       cj.Options.Spec,
			NextRun:    cj.Scheduler.Next(time.Now()),
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
			return state, saveErr
		}
	}
	return state, nil
}

// Start initiates the cron job in a goroutine, removed the extra defer-stop to avoid stopping it prematurely.
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
	log := cj.getLogger(ctx)
	state, err := cj.getState(ctx, true)
	if err != nil {
		log.WithError(err).Error(MessageFailedToLoadJobState)
		return err
	}
	log = log.WithValues("state", state)

	// Execute BeforeStartFunc if defined.
	if cj.Options.BeforeStartFunc != nil {
		shouldContinue, err := cj.Options.BeforeStartFunc(ctx, cj)
		if err != nil {
			log.WithError(err).Error(MessageFailedToExecuteBeforeStartFunc)
			return err
		}
		if !shouldContinue {
			log.Debug(MessageStoppingJobDueToBeforeStartFunc)
			return cj.Stop(ctx)
		}
	}

	go func() {
		for {
			now := time.Now()
			nextRun := cj.Scheduler.Next(now)
			sleepDuration := time.Until(nextRun)
			if sleepDuration <= 0 {
				sleepDuration = 1 * time.Second // Minimal sleep to prevent tight loop.
			}

			log.WithValues("next_run", nextRun).Debug("Sleeping until next run")
			timer := time.NewTimer(sleepDuration)

			select {
			case <-ctx.Done():
				cj.onContextDone(ctx, state)
				timer.Stop()
				return
			case <-cj.StopSignal:
				cj.onStopSignal(ctx, state)
				timer.Stop()
				return
			case <-timer.C:
				// Try acquiring the lock
				success, lockErr := cj.Options.Locker.Acquire(ctx)
				if lockErr != nil || !success {
					log.WithError(lockErr).Debug(MessageFailedToAcquireLock)
					if now.After(state.NextRun.Add(time.Second)) {
						log.Debug(MessageJobTakenByOtherWorkers)
						cj.sleepUntilNextRun()
					}
					continue
				}

				// Lock acquired; execute
				cj.execute(ctx, state)
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
	nextRun := cj.Scheduler.Next(now)
	sleepDuration := time.Until(nextRun)
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
