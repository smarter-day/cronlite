package tests

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"cronlite/cron"
	"cronlite/mocks"

	"github.com/redis/go-redis/v9"
	"go.uber.org/mock/gomock"
)

// TestCronJob_Execution_LockReleaseFailure verifies that the CronJob correctly handles
// failures when releasing the lock, ensuring consistent state and proper error handling.
func TestCronJob_Execution_LockReleaseFailure(t *testing.T) {
	// Initialize mock controller
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// Mock dependencies
	mockLocker := mocks.NewMockILocker(ctrl)
	mockRedis := mocks.NewMockCmdable(ctrl)
	mockPipeline := mocks.NewMockPipeliner(ctrl)
	mockPipeline2 := mocks.NewMockPipeliner(ctrl) // For stopping the job

	// Define test variables
	jobName := "test-job-execution-lock-release-failure"
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
	mockLocker.EXPECT().Acquire(gomock.Any()).Return(true, nil).Times(1)
	// Simulate GetLockTTL
	lockTTL := 5 * time.Second
	mockLocker.EXPECT().GetLockTTL().Return(lockTTL).Times(1)
	// Simulate lock release failure
	mockLocker.EXPECT().Release(gomock.Any()).Return(fmt.Errorf("lock release failed")).Times(1)

	// ----------------------------
	// Mock Redis.Get Call
	// ----------------------------
	// Simulate that the job state exists in Redis
	initialState := &cron.CronJobState{
		RunningBy:  "", // Empty to allow lock acquisition
		Status:     "succeeded",
		Iterations: 5,
		Spec:       expression,
		Data:       map[string]interface{}{"key1": "value1"},
		LastRun:    now.Add(-2 * time.Second),
		NextRun:    nextRun,
		CreatedAt:  now.Add(-10 * time.Second),
		UpdatedAt:  now.Add(-2 * time.Second),
	}

	serializedState, err := SerializeJobState(initialState)
	if err != nil {
		t.Fatalf("failed to serialize job state: %v", err)
	}

	cmdGet1 := redis.NewStringCmd(context.Background(), "GET", stateKey)
	cmdGet1.SetVal(serializedState)
	// Expect Get to be called once for initial state retrieval
	mockRedis.EXPECT().Get(gomock.Any(), stateKey).
		Return(cmdGet1).
		Times(1)

	// After acquiring lock, retrieve updated state
	updatedState := *initialState
	updatedState.RunningBy = "worker-id" // Simulate acquiring the lock
	updatedState.Status = "Running"
	updatedState.UpdatedAt = now.Add(1 * time.Second) // Simulate updated time

	serializedUpdatedState, err := SerializeJobState(&updatedState)
	if err != nil {
		t.Fatalf("failed to serialize updated job state: %v", err)
	}

	cmdGet2 := redis.NewStringCmd(context.Background(), "GET", stateKey)
	cmdGet2.SetVal(serializedUpdatedState)
	// Expect Get to be called once for updated state retrieval
	mockRedis.EXPECT().Get(gomock.Any(), stateKey).
		Return(cmdGet2).
		Times(1)

	// ----------------------------
	// Mock Redis.TxPipeline Calls
	// ----------------------------
	// Expect TxPipeline to be called twice: once for execution, once for stopping
	mockRedis.EXPECT().TxPipeline().
		Return(mockPipeline).
		Times(1)
	mockRedis.EXPECT().TxPipeline().
		Return(mockPipeline2).
		Times(1)

	// ----------------------------
	// Mock Pipeline.Set Call for mockPipeline (Execution)
	// ----------------------------
	mockPipeline.EXPECT().Set(gomock.Any(), stateKey, gomock.Any(), time.Duration(0)).
		Return(redis.NewStatusCmd(context.Background(), "SET", stateKey, "", time.Duration(0))).
		Times(1)

	// ----------------------------
	// Mock Pipeline.ZAdd Call for mockPipeline (Execution)
	// ----------------------------
	cmdZAdd1 := redis.NewIntCmd(context.Background(), "ZADD", cron.JobsList, 0)
	cmdZAdd1.SetVal(1) // Simulate successful ZADD
	mockPipeline.EXPECT().ZAdd(gomock.Any(), cron.JobsList, gomock.Any()).
		Return(cmdZAdd1).
		Times(1)

	// ----------------------------
	// Mock Pipeline.Exec Call for mockPipeline (Execution) - Simulate success
	// ----------------------------
	cmdSet1 := redis.NewStatusCmd(context.Background(), "SET", stateKey, "", time.Duration(0))
	cmdSet1.SetVal("OK") // Simulate successful SET
	mockPipeline.EXPECT().Exec(gomock.Any()).
		Return([]redis.Cmder{cmdSet1, cmdZAdd1}, nil).
		Times(1)

	// ----------------------------
	// Mock Pipeline.Set Call for mockPipeline2 (Stop)
	// ----------------------------
	mockPipeline2.EXPECT().Set(gomock.Any(), stateKey, gomock.Any(), time.Duration(0)).
		Return(redis.NewStatusCmd(context.Background(), "SET", stateKey, "", time.Duration(0))).
		Times(1)

	// ----------------------------
	// Mock Pipeline.ZAdd Call for mockPipeline2 (Stop)
	// ----------------------------
	cmdZAdd2 := redis.NewIntCmd(context.Background(), "ZADD", cron.JobsList, 0)
	cmdZAdd2.SetVal(1) // Simulate successful ZADD
	mockPipeline2.EXPECT().ZAdd(gomock.Any(), cron.JobsList, gomock.Any()).
		Return(cmdZAdd2).
		Times(1)

	// ----------------------------
	// Mock Pipeline.Exec Call for mockPipeline2 (Stop) - Simulate success
	// ----------------------------
	cmdSet2 := redis.NewStatusCmd(context.Background(), "SET", stateKey, "", time.Duration(0))
	cmdSet2.SetVal("OK") // Simulate successful SET
	mockPipeline2.EXPECT().Exec(gomock.Any()).
		Return([]redis.Cmder{cmdSet2, cmdZAdd2}, nil).
		Times(1)

	// ----------------------------
	// Setup Synchronization
	// ----------------------------
	var wg sync.WaitGroup
	wg.Add(1)
	var executeTime time.Time

	// ----------------------------
	// Create CronJobOptions
	// ----------------------------
	options := cron.CronJobOptions{
		Name:   jobName,
		Spec:   expression,
		Locker: mockLocker,
		Redis:  mockRedis,
		ExecuteFunc: func(ctx context.Context, job cron.ICronJob) error {
			// Capture the execution time
			executeTime = time.Now()
			// Signal that ExecuteFunc was called
			wg.Done()
			// Simulate execution normal (no error), focusing on lock release failure
			return nil
		},
	}

	// ----------------------------
	// Create a new CronJob instance
	// ----------------------------
	job, err := cron.NewCronJob(options)
	if err != nil {
		t.Fatalf("expected no error during CronJob initialization, got %v", err)
	}

	// ----------------------------
	// Start the CronJob
	// ----------------------------
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err = job.Start(ctx)
	if err != nil {
		t.Fatalf("expected no error during CronJob start, got %v", err)
	}

	// ----------------------------
	// Wait for ExecuteFunc to be called
	// ----------------------------
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// ExecuteFunc was called as expected
	case <-time.After(3 * time.Second):
		t.Fatal("ExecuteFunc was not called within the expected time")
	}

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
	if state.Iterations != initialState.Iterations+1 {
		t.Fatalf("expected Iterations to be %d, got %d", initialState.Iterations+1, state.Iterations)
	}

	// Verify that LastRun is updated to executeTime (allowing for 5s delta)
	if !isWithinDelta(state.LastRun, executeTime, 5*time.Second) {
		t.Fatalf("expected LastRun to be around %v, got %v", executeTime, state.LastRun)
	}

	// Verify that NextRun is correctly updated (allowing for 5s delta)
	expectedNextRun := parsedExpr.Next(executeTime)
	if !isWithinDelta(state.NextRun, expectedNextRun, 5*time.Second) {
		t.Fatalf("expected NextRun to be around %v, got %v", expectedNextRun, state.NextRun)
	}

	// Verify that Status is updated to "Success" (since ExecuteFunc did not return an error)
	// However, if the pipeline Exec failed, Status should be "Failed"
	// Depending on implementation, adjust accordingly
	if state.Status != "Success" {
		t.Fatalf("expected Status to be 'Success', got '%s'", state.Status)
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
		t.Fatalf("expected no error during CronJob stop, got %v", err)
	}

	// Allow some time for state updates
	time.Sleep(1 * time.Second)
}
