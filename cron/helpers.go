package cron

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/redis/go-redis/v9"
	"net"
	"os"
)

// LoadJobStateFromRedis loads the job state from Redis by job name
func LoadJobStateFromRedis(ctx context.Context, redisClient redis.Cmdable, jobName string) (*JobState, error) {
	data, err := redisClient.Get(ctx, fmt.Sprintf(JobStateKeyFormat, jobName)).Result()
	if err != nil {
		return nil, err
	}

	var state JobState
	if err := json.Unmarshal([]byte(data), &state); err != nil {
		return nil, fmt.Errorf("failed to parse job state: %w", err)
	}

	return &state, nil
}

// SaveJobStateToRedis saves the job state to Redis and updates the sorted set
func SaveJobStateToRedis(ctx context.Context, redisClient redis.Cmdable, jobName string, state *JobState) error {
	serializedState, err := json.Marshal(state)
	if err != nil {
		return fmt.Errorf("failed to serialize job state: %w", err)
	}

	pipe := redisClient.TxPipeline()

	pipe.Set(ctx, fmt.Sprintf(JobStateKeyFormat, jobName), serializedState, 0)

	latestTimestamp := state.LastRun.Unix()
	if latestTimestamp == 0 {
		latestTimestamp = state.NextRun.Unix()
	}
	pipe.ZAdd(ctx, JobsList, redis.Z{
		Score:  float64(latestTimestamp),
		Member: jobName,
	})

	_, err = pipe.Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to save job state and update sorted set: %w", err)
	}

	return nil
}

func ListJobsByRecency(ctx context.Context, redisClient *redis.Client, limit int) ([]string, error) {
	jobNames, err := redisClient.ZRevRange(ctx, JobsList, 0, int64(limit-1)).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to fetch jobs by recency: %w", err)
	}
	return jobNames, nil
}

func ListJobsByRecencyWithState(ctx context.Context, redisClient *redis.Client, limit int) ([]struct {
	JobName string
	State   JobState
}, error) {
	jobNames, err := ListJobsByRecency(ctx, redisClient, limit)
	if err != nil {
		return nil, err
	}

	var jobsWithState []struct {
		JobName string
		State   JobState
	}
	for _, jobName := range jobNames {
		state, err := LoadJobStateFromRedis(ctx, redisClient, jobName)
		if err != nil {
			// Handle error or skip
			continue
		}
		jobsWithState = append(jobsWithState, struct {
			JobName string
			State   JobState
		}{
			JobName: jobName,
			State:   *state,
		})
	}
	return jobsWithState, nil
}

type DefaultWorkerIDProvider struct{}

func (d DefaultWorkerIDProvider) Id() (string, error) {
	return GetWorkerID()
}

// GetWorkerID generates a unique machine ID based on system characteristics.
func GetWorkerID() (string, error) {
	var data string

	// Get MAC addresses
	interfaces, err := net.Interfaces()
	if err != nil {
		return "", fmt.Errorf("failed to get network interfaces: %w", err)
	}
	for _, iface := range interfaces {
		if iface.HardwareAddr != nil && len(iface.HardwareAddr) > 0 {
			data += iface.HardwareAddr.String()
		}
	}

	// Get IP addresses
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return "", fmt.Errorf("failed to get IP addresses: %w", err)
	}
	for _, addr := range addrs {
		data += addr.String()
	}

	// Get hostname
	hostname, err := os.Hostname()
	if err != nil {
		return "", fmt.Errorf("failed to get hostname: %w", err)
	}
	data += hostname

	// Include the process ID to differentiate instances on the same machine
	pid := os.Getpid()
	data += fmt.Sprintf("%d", pid)

	// Hash the collected data
	hasher := sha256.New()
	_, err = hasher.Write([]byte(data))
	if err != nil {
		return "", fmt.Errorf("failed to hash data: %w", err)
	}

	machineID := hex.EncodeToString(hasher.Sum(nil))
	return machineID, nil
}
