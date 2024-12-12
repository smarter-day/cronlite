package cron

import (
	"context"
	"cronlite/locker"
	"cronlite/logger"
	"github.com/gorhill/cronexpr"
	"github.com/redis/go-redis/v9"
	"sync"
	"time"
)

type IJob interface {
	Start(ctx context.Context) error
	Stop(ctx context.Context) error
	GetState(ctx context.Context) (*JobState, error)
	SaveState(ctx context.Context, state *JobState) error
	Delete(ctx context.Context) error
}

// JobStatus is unchanged
type JobStatus string

type JobState struct {
	Status     JobStatus              `json:"status"`
	RunningBy  string                 `json:"running_by"`
	LastRun    time.Time              `json:"last_run"`
	NextRun    time.Time              `json:"next_run"`
	Iterations int                    `json:"iterations"`
	Data       map[string]interface{} `json:"data"`
	CreatedAt  time.Time              `json:"created_at"`
	UpdatedAt  time.Time              `json:"updated_at"`
}

type JobOptions struct {
	Redis  redis.Cmdable                             // Redis client for state and locking
	Name   string                                    // Unique name for the job
	Spec   string                                    // Default cron specification (can be updated via Redis)
	Job    func(ctx context.Context, job *Job) error // The job function to execute
	Logger logger.ILogger                            // Logger for logging
	Locker locker.ILocker                            // Optional locker instance

	// New optional hooks:
	// BeforeStart: Called when starting the job and after initial state is loaded.
	//              Returns a boolean indicating whether to continue starting the job.
	BeforeStart func(ctx context.Context, job *Job) (bool, error)

	// BeforeExecute: Called before each execution attempt. If returns false, skip execution.
	BeforeExecute func(ctx context.Context, job *Job) (bool, error)

	// AfterExecute: Called after job execution attempt with the result of the execution (error from Job).
	//               Returns an error and if not nil, it's logged out.
	AfterExecute func(ctx context.Context, job *Job, err error) error

	WorkerIdProvider IWorkerIdProvider
}

// Job struct is unchanged except it now may use the new callbacks
type Job struct {
	Mutex      sync.Mutex
	Options    JobOptions
	CronExpr   *cronexpr.Expression
	StateKey   string
	StopSignal chan bool
}

type IWorkerIdProvider interface {
	Id() (string, error)
}
