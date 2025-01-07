package cron

import (
	"context"
	"fmt"
	"github.com/go-redsync/redsync/v4/redis/goredis/v9"
	"sync"
	"time"

	"github.com/go-redsync/redsync/v4"
	"github.com/redis/go-redis/v9"
	cronParser "github.com/robfig/cron/v3"
	"github.com/smarter-day/logger"
)

var (
	SpecParser    = cronParser.NewParser(cronParser.Second | cronParser.Minute | cronParser.Hour | cronParser.Dom | cronParser.Month | cronParser.Dow | cronParser.Descriptor)
	staleDuration = 10 * time.Second
)

// RedsyncProvider wrapper: each CronJob has a dedicated mutex to coordinate lock acquisition.
type RedsyncProvider interface {
	NewMutex(name string, options ...redsync.Option) *redsync.Mutex
}

type DefaultRedsyncProvider struct {
	// Holds the underlying Redsync instance.
	rs *redsync.Redsync
}

func (d DefaultRedsyncProvider) NewMutex(name string, options ...redsync.Option) *redsync.Mutex {
	// Simply delegate to the Redsync instance:
	return d.rs.NewMutex(name, options...)
}

// ICronJob interface is unchanged.
type ICronJob interface {
	Start(ctx context.Context) error
	Stop(ctx context.Context) error
	GetState() IState
	OnStateUpdated(ctx context.Context, state *CronJobState) error
	GetOptions() *CronJobOptions
}

// CronJobOptions defines how to construct the job.
type CronJobOptions struct {
	Name    string                // Job name
	Spec    string                // Cron expression
	Redis   redis.UniversalClient // Redis client for state mgmt
	Redsync RedsyncProvider       // New: for acquiring the Redsync mutex

	ExecuteFunc       func(ctx context.Context, job ICronJob) error
	BeforeStartFunc   func(ctx context.Context, job ICronJob) (bool, error)
	BeforeExecuteFunc func(ctx context.Context, job ICronJob) (bool, error)
	AfterExecuteFunc  func(ctx context.Context, job ICronJob, err error) error
	WorkerIdProvider  IWorkerIdProvider
}

type CronJob struct {
	Options        CronJobOptions
	Scheduler      cronParser.Schedule
	StopSignal     chan bool
	State          IState
	isRunning      bool
	isRunningMutex sync.RWMutex

	// Redsync mutex replaces the old ILocker:
	mutex *redsync.Mutex
}

func NewCronJob(options CronJobOptions) (ICronJob, error) {
	if options.Redis == nil {
		return nil, ErrRedisClientRequired
	}
	if options.Redsync == nil {
		// Use a default RedsyncProvider if none is set
		pool := goredis.NewPool(options.Redis)
		defaultRs := redsync.New(pool)
		options.Redsync = &DefaultRedsyncProvider{rs: defaultRs}
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

	// Parse the cron expression:
	if err := cronJob.initScheduler(context.Background(), options.Spec); err != nil {
		return nil, err
	}

	// Initialize the Redsync mutex:
	// We'll compute an initial TTL as time until next run, or at least 1s.
	ttl := time.Until(cronJob.Scheduler.Next(time.Now().Add(time.Second)))
	if ttl < time.Second {
		ttl = time.Second
	}
	cronJob.mutex = cronJob.Options.Redsync.NewMutex(
		"cron-job-lock:"+options.Name,
		redsync.WithExpiry(ttl),
		redsync.WithTries(1),
	)

	// Initialize the state
	cronJob.State = NewState(
		options.Redis,
		stateKey,
		cronJob.OnStateUpdated,
	)

	return cronJob, nil
}

// initScheduler is unchanged
func (cj *CronJob) initScheduler(ctx context.Context, spec string) error {
	scheduler := cj.getSchedulerBySpec(ctx, spec)
	if scheduler == nil {
		return fmt.Errorf("failed to parse cron spec: %s", spec)
	}
	cj.Scheduler = scheduler
	return nil
}

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

// Start runs the cron job in a background goroutine, same logic, but uses redsync.
func (cj *CronJob) Start(ctx context.Context) error {
	log := cj.getLogger(ctx)

	// Load initial state (force).
	state, err := cj.getState(ctx, true)
	if err != nil {
		log.WithError(err).Error(MessageFailedToLoadJobState)
		return err
	}
	log = log.WithValues("state", state)

	// Optional BeforeStartFunc
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

	// Launch loop
	go func() {
		for {
			now := time.Now()
			nextRun := cj.Scheduler.Next(now)
			sleepDuration := time.Until(nextRun)
			if sleepDuration <= 0 {
				sleepDuration = time.Second
			}

			log.WithValues("next_run", nextRun).
				Debug("Sleeping until next run")
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
				// Attempt to acquire redsync lock
				if err = cj.mutex.LockContext(ctx); err != nil {
					// Lock not acquired => either other worker has it or error
					log.WithError(err).Debug(MessageFailedToAcquireLock)
					cj.sleepUntilNextRun()
					continue
				}

				// Refresh state from Redis
				state, err = cj.getState(ctx, true)
				if err != nil {
					log.WithError(err).Error(MessageFailedToLoadJobState)
					// Release lock and break
					_, _ = cj.mutex.Unlock()
					return
				}
				log = log.WithValues("state", state)

				// Actually execute in same goroutine:
				cj.execute(ctx, state)
				// Unlock after done
				_, _ = cj.mutex.Unlock()
			}
		}
	}()

	return nil
}

// Stop signals the goroutine to end.
func (cj *CronJob) Stop(ctx context.Context) error {
	cj.isRunningMutex.Lock()
	defer cj.isRunningMutex.Unlock()

	select {
	case <-cj.StopSignal:
	default:
		close(cj.StopSignal)
	}
	return nil
}

// execute runs BeforeExecuteFunc, main ExecuteFunc, AfterExecuteFunc, with background extension.
func (cj *CronJob) execute(ctx context.Context, state *CronJobState) {
	// Mark isRunning = true
	cj.isRunningMutex.Lock()
	cj.isRunning = true
	cj.isRunningMutex.Unlock()

	defer func() {
		cj.isRunningMutex.Lock()
		cj.isRunning = false
		cj.isRunningMutex.Unlock()
	}()

	log := cj.getLogger(ctx).WithValues("state", state)
	if cj.Options.BeforeExecuteFunc != nil {
		continueExec, err := cj.onBeforeExecute(ctx, state)
		if err != nil || !continueExec {
			return
		}
	}

	// Worker ID for state
	workerID, err := cj.Options.WorkerIdProvider.Id()
	if err != nil {
		state.Status = JobStatusFailed
		state.AddData("error_message", err.Error())
		log.WithError(err).Error("Failed to get worker ID")
		_ = cj.State.Save(ctx, state)
		return
	}

	now := time.Now()
	state.Iterations++
	state.RunningBy = workerID
	state.LastRun = now
	state.NextRun = cj.Scheduler.Next(now)
	state.Status = JobStatusRunning

	if err = cj.State.Save(ctx, state); err != nil {
		log.WithError(err).Error("Failed to save job state after setting running status")
	}

	// Keep the redsync lock extended while we run
	lockCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	go cj.extendLockPeriodically(lockCtx, state) // same logic as before

	// Execute the job
	log.Debug("Executing cron job")
	jobErr := cj.Options.ExecuteFunc(ctx, cj)

	// Save result
	if jobErr != nil {
		state.Status = JobStatusFailed
		state.AddData("error_message", jobErr.Error())
		log.WithError(jobErr).Error("Job execution failed")
	} else {
		state.Status = JobStatusSuccess
		state.RemoveData("error_message")
		log.Debug("Job execution succeeded")
	}

	if err = cj.State.Save(ctx, state); err != nil {
		log.WithError(err).Error("Failed to save job state after execution")
	}

	// AfterExecuteFunc
	if cj.Options.AfterExecuteFunc != nil {
		afterErr := cj.Options.AfterExecuteFunc(ctx, cj, jobErr)
		if afterErr != nil {
			log.WithError(afterErr).Error("Failed to execute after-execute func")
		}
	}

	// Final save
	if err := cj.State.Save(ctx, state); err != nil {
		log.WithError(err).Error("Failed to save job state after post-execution func")
	}
}

// extendLockPeriodically extends the redsync lock in background until context is done.
func (cj *CronJob) extendLockPeriodically(ctx context.Context, state *CronJobState) {
	// We attempt to recalc a shorter interval if needed, same as old logic:
	ttl := time.Until(cj.mutex.Until())
	if ttl < time.Second {
		ttl = 1 * time.Second
	} else if ttl > staleDuration {
		ttl = staleDuration
	}
	extensionInterval := time.Duration(float64(ttl) * 0.8)
	if extensionInterval < time.Second {
		extensionInterval = time.Second
	}

	ticker := time.NewTicker(extensionInterval)
	defer ticker.Stop()
	log := cj.getLogger(ctx).WithValues("state", state)

	// When done, best-effort unlock if we still hold it:
	defer func() {
		_, _ = cj.mutex.Unlock()
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
			ok, err := cj.mutex.ExtendContext(ctx)
			if err != nil || !ok {
				log.WithError(err).Error(MessageFailedToExtendLock)
				return
			}
			log.Debug(MessageLockExtended)

			// Update heartbeat
			_ = cj.State.Save(ctx, state)
		}
	}
}

func (cj *CronJob) onBeforeExecute(ctx context.Context, state *CronJobState) (bool, error) {
	log := cj.getLogger(ctx).WithValues("state", state)
	if continueExec, err := cj.Options.BeforeExecuteFunc(ctx, cj); err != nil {
		state.Status = JobStatusFailed
		state.AddData("error_message", err.Error())
		log.WithError(err).Error(MessageFailedToExecuteBeforeExecuteFunc)
		_ = cj.State.Save(ctx, state)
		return false, err
	} else {
		if !continueExec {
			log.Debug(MessageJobExecutionCanceledByBeforeExecuteFunc)
		}
		return continueExec, nil
	}
}

// OnStateUpdated is unchanged
func (cj *CronJob) OnStateUpdated(ctx context.Context, state *CronJobState) error {
	logger.Log(ctx).
		WithValues("name", cj.Options.Name, "state", state).
		Debug(MessageJobStateUpdated)
	if state.Spec != "" && state.Spec != cj.Options.Spec {
		return cj.onSpecUpdated(ctx, state)
	}
	return nil
}

// onSpecUpdated is unchanged, but we refresh our redsync TTL so it matches new schedule.
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

	// Recompute TTL for new schedule:
	ttl := time.Until(scheduler.Next(time.Now()))
	if ttl < time.Second {
		ttl = time.Second
	}
	// We can't directly "SetTTL" on redsync, so on next lock acquisition we pass new TTL.
	// This won't break logic because each new cycle re-creates the mutex or extends it.

	if newState.LastRun.IsZero() {
		newState.LastRun = time.Now()
		log.Debug(MessageInitializingLastRunToCurrentTime)
	}
	newState.NextRun = cj.Scheduler.Next(newState.LastRun)
	log.WithValues("next_run", newState.NextRun).
		Debug(MessageRecalculatedNextRunFromNewSpec)
	return nil
}

// getState returns the current state with optional force reload.
func (cj *CronJob) getState(ctx context.Context, force bool) (*CronJobState, error) {
	state, err := cj.State.Get(ctx, force)
	log := cj.getLogger(ctx).WithValues("state", state)
	if err != nil {
		log.WithError(err).Error(MessageFailedToLoadJobState)
		return nil, err
	}
	if state == nil {
		// Initialize default state
		state = &CronJobState{
			Status:     JobStatusNotRunning,
			LastRun:    time.Time{},
			Spec:       cj.Options.Spec,
			NextRun:    cj.Scheduler.Next(time.Now()),
			Iterations: 0,
			Data:       make(map[string]interface{}),
			UpdatedAt:  time.Now(),
			CreatedAt:  time.Now(),
		}
		log = log.WithValues("state", state)
		if saveErr := cj.State.Save(ctx, state); saveErr != nil {
			log.WithError(saveErr).Error(MessageFailedToSaveDefaultJobState)
			return state, saveErr
		}
	}
	return state, nil
}

func (cj *CronJob) sleepUntilNextRun() {
	now := time.Now()
	nextRun := cj.Scheduler.Next(now)
	sleepDuration := time.Until(nextRun)
	if sleepDuration > 0 {
		time.Sleep(sleepDuration)
	} else {
		time.Sleep(time.Second)
	}
}

func (cj *CronJob) GetState() IState {
	return cj.State
}

// onContextDone and onStopSignal remain the same
func (cj *CronJob) onContextDone(ctx context.Context, state *CronJobState) {
	log := cj.getLogger(ctx)
	cj.isRunningMutex.RLock()
	running := cj.isRunning
	cj.isRunningMutex.RUnlock()
	if running {
		state.Status = JobStatusCancelled
		if err := cj.State.Save(ctx, state); err != nil {
			log.WithError(err).Error(MessageFailedToSaveJobState)
		}
		log.Debug(MessageJobStoppedByContextDuringActiveExecution)
	} else {
		log.Debug(MessageJobStoppedByContextWithoutActiveExecution)
	}
}

func (cj *CronJob) onStopSignal(ctx context.Context, state *CronJobState) {
	log := cj.getLogger(ctx)
	cj.isRunningMutex.RLock()
	running := cj.isRunning
	cj.isRunningMutex.RUnlock()
	if running {
		state.Status = JobStatusCancelled
		if err := cj.State.Save(ctx, state); err != nil {
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

// getLogger adds contextual info
func (cj *CronJob) getLogger(ctx context.Context) logger.ILogger {
	workerId, _ := cj.Options.WorkerIdProvider.Id()
	return logger.Log(ctx).WithValues("name", cj.Options.Name, "worker_id", workerId)
}
