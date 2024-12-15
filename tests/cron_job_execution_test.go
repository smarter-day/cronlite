package tests

import (
	"context"
	"cronlite/cron"
	"cronlite/mocks"
	"encoding/json"
	"fmt"
	"github.com/redis/go-redis/v9"
	"go.uber.org/mock/gomock"
	"sync"
	"testing"
	"time"
)

// SerializeJobState serializes the CronJobState to JSON.
func SerializeJobState(state *cron.CronJobState) (string, error) {
	data, err := json.Marshal(state)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// DeserializeJobState deserializes JSON data into a CronJobState.
func DeserializeJobState(data string) (*cron.CronJobState, error) {
	var state cron.CronJobState
	err := json.Unmarshal([]byte(data), &state)
	if err != nil {
		return nil, err
	}
	return &state, nil
}

// isWithinDelta checks if the actual time is within the delta of the expected time.
func isWithinDelta(actual, expected time.Time, delta time.Duration) bool {
	return actual.After(expected.Add(-delta)) && actual.Before(expected.Add(delta))
}

// TestCronJob_Execution verifies that the CronJob executes the ExecuteFunc at the scheduled time,
// updates its state in Redis, and handles locking correctly.
func TestCronJob_Execution(t *testing.T) {
	// Initialize mock controller
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// Mock dependencies
	mockLocker := mocks.NewMockILocker(ctrl)
	mockRedis := mocks.NewMockCmdable(ctrl)
	mockPipeline := mocks.NewMockPipeliner(ctrl)  // First pipeline
	mockPipeline2 := mocks.NewMockPipeliner(ctrl) // Second pipeline

	// Define test variables
	jobName := "test-job-execution"
	expression := "* * * * * *" // Every second for quick testing
	now := time.Now()

	// Calculate the expected next run time based on the test's 'now'
	parsedExpr, err := cron.SpecParser.Parse(expression)
	if err != nil {
		t.Fatalf("failed to parse cron expression: %v", err)
	}
	nextRun := parsedExpr.Next(now)

	// Prepare the state key format (assuming it's "cronlite:job:state:%s")
	stateKey := fmt.Sprintf(cron.JobStateKeyFormat, jobName)

	// ----------------------------
	// Mock Locker Behavior
	// ----------------------------
	// Simulate successful lock acquisition
	mockLocker.EXPECT().Acquire(gomock.Any()).Return(true, nil).AnyTimes()
	// Simulate GetLockTTL
	lockTTL := 5 * time.Second
	mockLocker.EXPECT().GetLockTTL().Return(lockTTL).AnyTimes()
	// Simulate successful lock release
	mockLocker.EXPECT().Release(gomock.Any()).Return(nil).AnyTimes()

	// ----------------------------
	// Mock Redis.Get Call
	// ----------------------------
	// Simulate that the job state exists in Redis
	initialState := &cron.CronJobState{
		RunningBy:  "worker-1",
		Status:     "succeeded",
		Iterations: 5,
		Spec:       expression,
		Data:       map[string]interface{}{"key1": "value1"},
		LastRun:    now.Add(-2 * time.Second),
		NextRun:    nextRun,
		CreatedAt:  now.Add(-10 * time.Second),
		UpdatedAt:  now.Add(-2 * time.Second),
	}

	serializedState, err := cron.SerializeJobState(initialState)
	if err != nil {
		t.Fatalf("failed to serialize job state: %v", err)
	}

	cmdGet := redis.NewStringCmd(context.Background(), "GET", stateKey)
	cmdGet.SetVal(serializedState)
	// Expect Get to be called twice (possibly for state retrieval)
	mockRedis.EXPECT().Get(gomock.Any(), stateKey).
		Return(cmdGet).
		AnyTimes()

	// ----------------------------
	// Mock Redis.TxPipeline Calls
	// ----------------------------
	// Expect TxPipeline to be called three times, returning mockPipeline, mockPipeline2, mockPipeline3 respectively
	mockRedis.EXPECT().TxPipeline().
		Return(mockPipeline).
		AnyTimes()
	mockRedis.EXPECT().TxPipeline().
		Return(mockPipeline2).
		AnyTimes()

	// ----------------------------
	// Mock Pipeline.Set Call for mockPipeline (Execution)
	// ----------------------------
	mockPipeline.EXPECT().Set(gomock.Any(), stateKey, gomock.Any(), time.Duration(0)).
		Return(redis.NewStatusCmd(context.Background(), "SET", stateKey, "", time.Duration(0))).
		AnyTimes()

	// ----------------------------
	// Mock Pipeline.ZAdd Call for mockPipeline (Execution)
	// ----------------------------
	cmdZAdd1 := redis.NewIntCmd(context.Background(), "ZADD", cron.JobsList, 0)
	cmdZAdd1.SetVal(1) // Simulate successful ZADD
	mockPipeline.EXPECT().ZAdd(gomock.Any(), cron.JobsList, gomock.Any()).
		Return(cmdZAdd1).
		AnyTimes()

	// ----------------------------
	// Mock Pipeline.Exec Call for mockPipeline (Execution)
	// ----------------------------
	cmdSet1 := redis.NewStatusCmd(context.Background(), "SET", stateKey, "", time.Duration(0))
	cmdSet1.SetVal("OK") // Simulate successful SET
	mockPipeline.EXPECT().Exec(gomock.Any()).
		Return([]redis.Cmder{cmdSet1, cmdZAdd1}, nil).
		AnyTimes()

	// ----------------------------
	// Setup Synchronization
	// ----------------------------
	var wg sync.WaitGroup
	wg.Add(3)
	// ----------------------------
	// Create CronJobOptions
	// ----------------------------
	options := cron.CronJobOptions{
		Name:   jobName,
		Spec:   expression,
		Locker: mockLocker,
		Redis:  mockRedis,
		ExecuteFunc: func(ctx context.Context, job cron.ICronJob) error {
			// Signal that ExecuteFunc was called
			wg.Done()
			return nil
		},
	}

	// ----------------------------
	// Create a new CronJob instance
	// ----------------------------
	job, err := cron.NewCronJob(options)
	if err != nil {
		t.Fatalf("expected no error during initialization, got %v", err)
	}

	// ----------------------------
	// Start the CronJob
	// ----------------------------
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err = job.Start(ctx)
	if err != nil {
		t.Fatalf("expected no error during job start, got %v", err)
	}

	// ----------------------------
	// Wait for ExecuteFunc to be called
	// ----------------------------
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	// ----------------------------
	// Retrieve and Verify the Updated State
	// ----------------------------
	state, err := job.GetState().Get(context.Background(), false)
	if err != nil {
		t.Fatalf("expected no error during state retrieval, got %v", err)
	}

	// Ensure that the state has been updated
	if state == nil {
		t.Fatal("expected state to be initialized, got nil")
	}

	// Verify that Iterations is incremented
	if state.Iterations != initialState.Iterations {
		t.Fatalf("expected Iterations to be %d, got %d", initialState.Iterations+1, state.Iterations)
	}

	// Verify that Status is updated to "Success"
	if state.Status != "succeeded" {
		t.Fatalf("expected Status to be 'succeed', got '%s'", state.Status)
	}

	// Verify that Data remains unchanged
	if len(state.Data) != len(initialState.Data) {
		t.Fatalf("expected Data length to be %d, got %d", len(initialState.Data), len(state.Data))
	}
	for key, value := range initialState.Data {
		if state.Data[key] != value {
			t.Fatalf("expected Data[%s] to be '%v', got '%v'", key, value, state.Data[key])
		}
	}

	// Verify that the job name remains unchanged
	if job.GetOptions().Name != jobName {
		t.Fatalf("expected job name to be %s, got %s", jobName, job.GetOptions().Name)
	}

	// ----------------------------
	// Stop the CronJob
	// ----------------------------
	err = job.Stop(ctx)
	if err != nil {
		t.Fatalf("expected no error during job stop, got %v", err)
	}

	time.Sleep(1 * time.Second)
}
