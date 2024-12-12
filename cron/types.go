package cron

import (
	"context"
	"cronlite/locker"
	"cronlite/logger"
	"github.com/gorhill/cronexpr"
	"github.com/redis/go-redis/v9"
	"time"
)

type IJob interface {
	Start(ctx context.Context) error
	Stop(ctx context.Context) error
	GetState() IState
	OnStateUpdated(ctx context.Context, state *JobState) error
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

	BeforeStart   func(ctx context.Context, job IJob) (bool, error)
	BeforeExecute func(ctx context.Context, job IJob) (bool, error)
	AfterExecute  func(ctx context.Context, job IJob, err error) error

	WorkerIdProvider IWorkerIdProvider
}

// Job struct is unchanged except it now may use the new callbacks
type Job struct {
	Options    JobOptions
	CronExpr   *cronexpr.Expression
	StopSignal chan bool
	State      IState
}

type IWorkerIdProvider interface {
	Id() (string, error)
}

type IState interface {
	Get(ctx context.Context, force bool) (*JobState, error)
	Save(ctx context.Context, state *JobState) error
	Delete(ctx context.Context) error
	Exists(ctx context.Context) (bool, error)
}
