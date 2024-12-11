package cronlite

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/redis/go-redis/v9"
	"sync"
	"time"

	"github.com/gorhill/cronexpr"
)

var (
	ErrJobStateFetchFailed = errors.New("failed to fetch job state from Redis")
)

type ICronJob interface {
	Start(ctx context.Context) error
	Stop() error
	GetJobState(ctx context.Context) (*JobState, error)
	SaveJobState(ctx context.Context, state *JobState) error
}

type JobStatus string

const (
	JobStatusRunning   JobStatus = "Running"
	JobStatusCancelled JobStatus = "Cancelled"
	JobStatusFailed    JobStatus = "Failed"
	JobStatusSuccess   JobStatus = "Success"
)

// JobState defines the state of the job
type JobState struct {
	Status  JobStatus              `json:"status"`   // Job status
	LastRun time.Time              `json:"last_run"` // Timestamp of the last execution
	NextRun time.Time              `json:"next_run"` // Timestamp of the next execution
	Data    map[string]interface{} `json:"data"`     // Custom options like max iterations
}

// CronJobOptions defines all configurable options for a CronJob
type CronJobOptions struct {
	Redis  redis.Cmdable                                 // Redis client for state and locking
	Name   string                                        // Unique name for the job
	Spec   string                                        // Default cron specification (can be updated via Redis)
	Job    func(ctx context.Context, job *CronJob) error // The job function to execute
	Logger ILogger                                       // Logger for logging
	Locker ILocker                                       // Optional locker instance
}

// CronJob manages job scheduling and execution
type CronJob struct {
	Mutex      sync.Mutex
	Options    CronJobOptions
	CronExpr   *cronexpr.Expression
	StateKey   string // Redis key for job state
	StopSignal chan bool
}

// NewCronJob initializes a new CronJob
func NewCronJob(options CronJobOptions) (*CronJob, error) {
	// Set default logger if not provided
	if options.Logger == nil {
		options.Logger = &LogLogger{}
	}

	// Validate and parse the cron expression
	expr, err := cronexpr.Parse(options.Spec)
	if err != nil {
		return nil, fmt.Errorf("invalid cron expression: %w", err)
	}

	// Create a locker if not provided
	if options.Locker == nil {
		lockerOptions := LockerOptions{
			Name:    options.Name,
			Redis:   options.Redis,
			Logger:  options.Logger,
			LockTTL: time.Until(expr.Next(time.Now())) + 5*time.Second,
		}
		options.Locker = NewLocker(lockerOptions)
	}

	return &CronJob{
		Options:    options,
		CronExpr:   expr,
		StateKey:   fmt.Sprintf("cronlite:job::%s", options.Name),
		StopSignal: make(chan bool),
	}, nil
}

func (cj *CronJob) Start(ctx context.Context) {
	go func() {
		for {
			select {
			case <-ctx.Done():
				cj.SetJobStatus(ctx, JobStatusCancelled)
				cj.Options.Logger.Info(ctx, "Cron job stopped by context", map[string]interface{}{"name": cj.Options.Name})
				return
			case <-cj.StopSignal:
				cj.SetJobStatus(ctx, JobStatusCancelled)
				cj.Options.Logger.Info(ctx, "Cron job force stopped", map[string]interface{}{"name": cj.Options.Name})
				return
			default:
				cj.Execute(ctx)
				time.Sleep(1 * time.Second) // Small delay to avoid busy looping
			}
		}
	}()
}

func (cj *CronJob) Stop() {
	close(cj.StopSignal)
}

// GetJobState retrieves the latest job state from Redis
func (cj *CronJob) GetJobState(ctx context.Context) (*JobState, error) {
	data, err := cj.Options.Redis.Get(ctx, cj.StateKey).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			// Initialize a default state with empty status
			defaultState := &JobState{
				Status:  "",
				LastRun: time.Time{},
				NextRun: cj.CronExpr.Next(time.Now()),
				Data:    map[string]interface{}{},
			}
			if saveErr := cj.SaveJobState(ctx, defaultState); saveErr != nil {
				return nil, fmt.Errorf("failed to initialize job state: %w", saveErr)
			}
			return defaultState, nil
		}
		return nil, fmt.Errorf("%w: %v", ErrJobStateFetchFailed, err)
	}

	var state JobState
	if err := json.Unmarshal([]byte(data), &state); err != nil {
		return nil, fmt.Errorf("failed to parse job state: %w", err)
	}

	// Update the cron expression if provided
	if spec, ok := state.Data["spec"].(string); ok {
		cj.CronExpr = cronexpr.MustParse(spec)
		state.NextRun = cj.CronExpr.Next(time.Now())
	}
	return &state, nil
}

// SaveJobState writes the job state to Redis
func (cj *CronJob) SaveJobState(ctx context.Context, state *JobState) error {
	serializedState, err := json.Marshal(state)
	if err != nil {
		return fmt.Errorf("failed to serialize job state: %w", err)
	}

	// Save the job state to Redis
	err = cj.Options.Redis.Set(ctx, cj.StateKey, serializedState, 0).Err()
	if err != nil {
		return fmt.Errorf("failed to save job state: %w", err)
	}

	// Update the sorted set with the most recent timestamp (e.g., LastRun)
	latestTimestamp := state.LastRun.Unix()
	if latestTimestamp == 0 {
		// Use NextRun if LastRun is not yet set
		latestTimestamp = state.NextRun.Unix()
	}

	// Add the job to the sorted set
	err = cj.Options.Redis.ZAdd(ctx, "cronlite:jobs", redis.Z{
		Score:  float64(latestTimestamp),
		Member: cj.Options.Name,
	}).Err()
	if err != nil {
		return fmt.Errorf("failed to update job metadata in sorted set: %w", err)
	}

	return nil
}

func (cj *CronJob) SetJobStatus(ctx context.Context, status JobStatus) {
	state, err := cj.GetJobState(ctx)
	if err != nil {
		cj.Options.Logger.Error(ctx, "Failed to fetch job state", map[string]interface{}{"name": cj.Options.Name, "error": err})
		return
	}
	state.Status = status
	if err := cj.SaveJobState(ctx, state); err != nil {
		cj.Options.Logger.Error(ctx, "Failed to update job state", map[string]interface{}{"name": cj.Options.Name, "error": err})
	}
}

func (cj *CronJob) Execute(ctx context.Context) {
	// Fetch the latest job state
	state, err := cj.GetJobState(ctx)
	if err != nil {
		cj.Options.Logger.Error(ctx, "Failed to fetch job state", map[string]interface{}{"name": cj.Options.Name, "error": err})
		return
	}

	// Ensure it's time to run the job based on `LastRun` and `NextRun`
	now := time.Now()
	if now.Before(state.NextRun) {
		cj.Options.Logger.Debug(ctx, "Cron job skipped as the next run is in the future", map[string]interface{}{
			"name":     cj.Options.Name,
			"next_run": state.NextRun,
			"current":  now,
		})
		time.Sleep(time.Until(state.NextRun)) // Sleep until `NextRun`
		return
	}

	// Acquire lock before running the job
	success, err := cj.Options.Locker.Acquire(ctx)
	if err != nil || !success {
		cj.Options.Logger.Debug(ctx, "Cron job skipped due to existing lock", map[string]interface{}{"name": cj.Options.Name})
		return
	}
	defer func(Locker ILocker, ctx context.Context) {
		err := Locker.Release(ctx)
		if err != nil {
			cj.Options.Logger.Error(ctx, "Failed to release lock after job execution", map[string]interface{}{"name": cj.Options.Name, "error": err})
		}
	}(cj.Options.Locker, ctx)

	// Re-fetch state to ensure consistency
	updatedState, err := cj.GetJobState(ctx)
	if err != nil {
		cj.Options.Logger.Error(ctx, "Failed to fetch updated job state after acquiring lock", map[string]interface{}{"name": cj.Options.Name, "error": err})
		return
	}

	if updatedState.LastRun.After(state.LastRun) {
		cj.Options.Logger.Debug(ctx, "Cron job skipped as another worker already executed the job", map[string]interface{}{
			"name":     cj.Options.Name,
			"last_run": updatedState.LastRun,
			"current":  now,
			"next_run": updatedState.NextRun,
		})
		return
	}

	// Set job status to running
	cj.SetJobStatus(ctx, JobStatusRunning)

	// Start lock extension in a separate goroutine
	lockCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	go cj.extendLockPeriodically(lockCtx)

	// Execute the job
	cj.Options.Logger.Info(ctx, "Executing cron job", map[string]interface{}{"name": cj.Options.Name})
	err = cj.Options.Job(ctx, cj)
	if err != nil {
		cj.Options.Logger.Error(ctx, "Error executing cron job", map[string]interface{}{"name": cj.Options.Name, "error": err})
		cj.SetJobStatus(ctx, JobStatusFailed)
		state.Data["error_message"] = err.Error()
	} else {
		cj.Options.Logger.Info(ctx, "Cron job executed successfully", map[string]interface{}{"name": cj.Options.Name})
		cj.SetJobStatus(ctx, JobStatusSuccess)
	}

	// Update `LastRun`, `NextRun`, and save the updated state
	state.LastRun = now
	state.NextRun = cj.CronExpr.Next(now)

	// Avoid redundant save if state hasn't changed
	if state.LastRun != updatedState.LastRun || state.NextRun != updatedState.NextRun {
		if saveErr := cj.SaveJobState(ctx, state); saveErr != nil {
			cj.Options.Logger.Error(ctx, "Failed to update job state after execution", map[string]interface{}{"error": saveErr})
		}
	}
}

func (cj *CronJob) extendLockPeriodically(ctx context.Context) {
	ttl := cj.Options.Locker.GetLockTTL()
	extensionInterval := ttl / 2 // Extend the lock at 50% of the TTL

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
		}
	}
}

func ListJobsByRecency(ctx context.Context, redisClient redis.Cmdable, limit int) ([]string, error) {
	// Fetch job names sorted by most recent updates
	jobNames, err := redisClient.ZRevRange(ctx, "cronlite:jobs", 0, int64(limit-1)).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to fetch jobs by recency: %w", err)
	}
	return jobNames, nil
}
