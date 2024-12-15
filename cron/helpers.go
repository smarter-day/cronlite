package cron

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/redis/go-redis/v9"
	"time"
)

// LoadJobStateFromRedis retrieves the job state from Redis using the provided state key.
//
// Parameters:
//
//	ctx - The context for managing request lifetime and cancellation.
//	redisClient - The Redis client used to execute commands.
//	stateKey - The key in Redis where the job state is stored.
//
// Returns:
//
//	A pointer to CronJobState if successful, or an error if the operation fails.
func LoadJobStateFromRedis(ctx context.Context, redisClient redis.Cmdable, stateKey string) (*CronJobState, error) {
	data, err := redisClient.Get(ctx, stateKey).Result()
	if err != nil {
		return nil, err
	}

	var state CronJobState
	if err := json.Unmarshal([]byte(data), &state); err != nil {
		return nil, fmt.Errorf("failed to parse job state: %w", err)
	}

	return &state, nil
}

// SaveJobStateToRedis saves the job state to Redis and updates the sorted set with the job's recency.
//
// Parameters:
//
//	ctx - The context for managing request lifetime and cancellation.
//	redisClient - The Redis client used to execute commands.
//	stateKey - The key in Redis where the job state will be stored.
//	state - A pointer to the CronJobState that needs to be saved.
//
// Returns:
//
//	An error if the operation fails, or nil if successful.
func SaveJobStateToRedis(ctx context.Context, redisClient redis.Cmdable, stateKey string, state *CronJobState) error {
	serializedState, err := json.Marshal(state)
	if err != nil {
		return fmt.Errorf("failed to serialize job state: %w", err)
	}

	pipe := redisClient.TxPipeline()

	pipe.Set(ctx, stateKey, serializedState, 0)

	latestTimestamp := state.LastRun.Unix()
	if latestTimestamp == 0 {
		latestTimestamp = state.NextRun.Unix()
	}
	pipe.ZAdd(ctx, JobsList, redis.Z{
		Score:  float64(latestTimestamp),
		Member: stateKey,
	})

	_, err = pipe.Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to save job state and update sorted set: %w", err)
	}

	return nil
}

// ListJobsByRecency retrieves a list of job names from Redis, ordered by recency.
//
// Parameters:
//
//	ctx - The context for managing request lifetime and cancellation.
//	redisClient - The Redis client used to execute commands.
//	limit - The maximum number of job names to retrieve.
//
// Returns:
//
//	A slice of job names ordered by recency if successful, or an error if the operation fails.
func ListJobsByRecency(ctx context.Context, redisClient *redis.Client, limit int) ([]string, error) {
	jobNames, err := redisClient.ZRevRange(ctx, JobsList, 0, int64(limit-1)).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to fetch jobs by recency: %w", err)
	}
	return jobNames, nil
}

// ListJobsByRecencyWithState retrieves a list of job names from Redis, ordered by recency,
// along with their respective states.
//
// Parameters:
//
//	ctx - The context for managing request lifetime and cancellation.
//	redisClient - The Redis client used to execute commands.
//	limit - The maximum number of job names to retrieve.
//
// Returns:
//
//	A slice of structs containing job names and their states if successful, or an error if the operation fails.
func ListJobsByRecencyWithState(ctx context.Context, redisClient *redis.Client, limit int) ([]struct {
	JobName string
	State   CronJobState
}, error) {
	jobNames, err := ListJobsByRecency(ctx, redisClient, limit)
	if err != nil {
		return nil, err
	}

	var jobsWithState []struct {
		JobName string
		State   CronJobState
	}
	for _, jobName := range jobNames {
		state, err := LoadJobStateFromRedis(ctx, redisClient, jobName)
		if err != nil {
			// Handle error or skip
			continue
		}
		jobsWithState = append(jobsWithState, struct {
			JobName string
			State   CronJobState
		}{
			JobName: jobName,
			State:   *state,
		})
	}
	return jobsWithState, nil
}

// SerializeJobState serializes the CronJobState to JSON.
func SerializeJobState(state *CronJobState) (string, error) {
	data, err := json.Marshal(state)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// DeserializeJobState deserializes JSON data into a CronJobState.
func DeserializeJobState(data string) (*CronJobState, error) {
	var state CronJobState
	err := json.Unmarshal([]byte(data), &state)
	if err != nil {
		return nil, err
	}
	return &state, nil
}

// GetJobStatus determines the current status of a job based on its state.
//
// Parameters:
//
//	state - The CronJobState containing the job's current status, next run time, and last update time.
//
// Returns:
//
//	A string representing the job's status. If the job is running and its next run time is in the past
//	and it hasn't been updated recently, the status is considered "Stale". Otherwise, it returns the
//	current status from the CronJobState.
func GetJobStatus(state CronJobState) string {
	status := string(state.Status)
	if state.NextRun.Before(time.Now()) && time.Now().Add(-staleDuration).After(state.UpdatedAt) && status == string(JobStatusRunning) {
		status = string(JobStatusStale)
	}
	return status
}
