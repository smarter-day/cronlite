package cron

import (
	"errors"
	"time"
)

const (
	// JobsList Stores the job names in a sorted set keyed by their most recent timestamp
	JobsList = "cronlite_jobs"

	// JobStateKeyFormat format for job state
	JobStateKeyFormat = "cronlite:job:state:%s"

	// StaleThreshold to consider a job stale if not updated.
	// When no updates (heartbeats) within 20s, consider not running.
	StaleThreshold = 20 * time.Second

	JobStatusNotRunning               = ""
	JobStatusRunning    CronJobStatus = "Running"
	JobStatusCancelled  CronJobStatus = "Cancelled"
	JobStatusFailed     CronJobStatus = "Failed"
	JobStatusSuccess    CronJobStatus = "Success"

	MessageJobStateUpdated                            = "Job state updated"
	MessageUpdatingCronExpression                     = "Updating cron expression"
	MessageInitializingLastRunToCurrentTime           = "Initializing last run to current time"
	MessageRecalculatedNextRunFromNewSpec             = "Recalculated NextRun based on new spec"
	MessageFailedToExecuteBeforeExecuteFunc           = "Failed to execute before execute func"
	MessageFailedToSaveJobStateAfterBeforeExecuteFunc = "Failed to save job state after before execute func"
	MessageJobExecutionCanceledByBeforeExecuteFunc    = "Job execution canceled by before execute func"
	MessageFailedToGetWorkerId                        = "Failed to get worker ID"
	MessageFailedToSaveJobStateAfterWorkerId          = "Failed to get job state after worker ID"
	MessageFailedToExecuteAfterExecuteFunc            = "Failed to execute AfterExecuteFunc callback"
	MessageFailedToSaveJobAfterExecution              = "Failed to save job state after execution"
	MessageExecutingCronJob                           = "Executing cron job"
	MessageExecutionFailed                            = "Execution failed"
	MessageExecutionSucceeded                         = "Execution succeeded"
	MessageFailedToSaveJobStateAfterPostExecutionFunc = "Failed to save job state after post execution func"
	MessageLockExtensionGoRoutineStopped              = "Lock extension goroutine stopped"
	MessageFailedToExtendLock                         = "Failed to extend lock"
	MessageFailedToReleaseLock                        = "Failed to release lock"
	MessageLockExtended                               = "Lock extended"
	MessageFailedToSaveJobStateAfterLockExtension     = "Failed to save job state after lock extension"
	MessageFailedToLoadJobState                       = "Failed to load job state"
	MessageFailedToSaveDefaultJobState                = "Failed to save default job state"
	MessageFailedToExecuteBeforeStartFunc             = "Failed to execute BeforeStartFunc"
	MessageStoppingJobDueToBeforeStartFunc            = "BeforeStartFunc callback returned false, stopping job"
	MessageJobIsStale                                 = "Job is stale"
	MessageJobTooEarlyToRun                           = "Job is too early to run"
	MessageFailedToAcquireLock                        = "Failed to acquire lock"
	MessageJobTakenByOtherWorkers                     = "Job is too late to run when lock is acquired. Sleeping until next run"
	MessageFailedToSaveJobState                       = "Failed to save job state"
	MessageJobStoppedByContextDuringActiveExecution   = "CronJob job stopped by context with active execution"
	MessageJobStoppedByContextWithoutActiveExecution  = "CronJob job stopped by context without active execution"
	MessageJobStoppedBySignalDuringActiveExecution    = "CronJob job stopped by StopSignal with active execution"
	MessageJobStoppedBySignalWithoutActiveExecution   = "CronJob job stopped by StopSignal without active execution"

	ErrMessageInvalidCronExpression = "Invalid cron expression"
)

var (
	ErrRedisClientRequired    = errors.New("redis client is required")
	ErrCronNameRequired       = errors.New("cron name is required")
	ErrCronExpressionRequired = errors.New("cron expression is required")
	ErrCronFunctionRequired   = errors.New("cron function is required")
	ErrCronStateEmpty         = errors.New("cron state is empty")
)
