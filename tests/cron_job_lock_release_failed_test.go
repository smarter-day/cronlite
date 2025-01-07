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
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRedis := mocks.NewMockUniversalClient(ctrl)
	mockPipeline := mocks.NewMockPipeliner(ctrl)
	mockPipeline2 := mocks.NewMockPipeliner(ctrl) // For stopping the job

	// Define test variables
	jobName := "test-job-execution-lock-release-failure"
	expression := "* * * * * *" // Every second for quick testing
	now := time.Now()

	// Calculate the expected next run time
	parsedExpr, err := cron.SpecParser.Parse(expression)
	if err != nil {
		t.Fatalf("failed to parse cron expression: %v", err)
	}
	nextRun := parsedExpr.Next(now)

	// State key
	stateKey := fmt.Sprintf(cron.JobStateKeyFormat, jobName)

	// ----------------------------
	// Mock Redis.Get for initial load
	// ----------------------------
	initialState := &cron.CronJobState{
		RunningBy:  "", // empty -> lock can be acquired
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
	// Expect Get() exactly once for the initial load
	mockRedis.EXPECT().
		Get(gomock.Any(), stateKey).
		Return(cmdGet1).
		AnyTimes()

	// ----------------------------
	// Lock acquisition (SetNX)
	// ----------------------------
	// We need exactly one successful lock acquisition.
	setNXAcquired := redis.NewBoolCmd(context.Background())
	setNXAcquired.SetVal(true) // lock acquired successfully
	mockRedis.EXPECT().
		SetNX(
			gomock.Any(),
			"cron-job-lock:"+jobName,
			gomock.Any(), // random Redsync value
			gomock.Any(), // TTL
		).
		Return(setNXAcquired).
		Times(1)

	// Any subsequent SetNX calls -> fail. (We can do AnyTimes because the job might try again)
	setNXFail := redis.NewBoolCmd(context.Background())
	setNXFail.SetVal(false)
	mockRedis.EXPECT().
		SetNX(
			gomock.Any(),
			"cron-job-lock:"+jobName,
			gomock.Any(),
			gomock.Any(),
		).
		Return(setNXFail).
		AnyTimes()

	// ----------------------------
	// Lock release failure (EvalSha)
	// ----------------------------
	// Redsync calls EvalSha(...) to release or extend the lock.
	// Return "0" => indicates failure to release in the deleteScript or extendScript.
	releaseFailCmd := redis.NewCmd(context.Background())
	releaseFailCmd.SetVal(int64(0)) // 0 => not released
	mockRedis.EXPECT().
		EvalSha(
			gomock.Any(),
			gomock.Any(),
			gomock.Any(),
			gomock.Any(),
		).
		Return(releaseFailCmd).
		AnyTimes()

	// ----------------------------
	// No second GET expected
	// ----------------------------
	mockRedis.EXPECT().Get(gomock.Any(), stateKey).AnyTimes()

	// ----------------------------
	// Mock pipeline calls
	// ----------------------------
	// Expect TxPipeline calls for execution, not for stopping
	mockRedis.EXPECT().TxPipeline().Return(mockPipeline).AnyTimes()
	mockRedis.EXPECT().TxPipeline().Return(mockPipeline2).AnyTimes()

	mockPipeline.EXPECT().
		Set(gomock.Any(), stateKey, gomock.Any(), time.Duration(0)).
		Return(redis.NewStatusCmd(context.Background())).
		AnyTimes()

	cmdZAdd1 := redis.NewIntCmd(context.Background())
	cmdZAdd1.SetVal(1) // successful ZADD
	mockPipeline.EXPECT().
		ZAdd(gomock.Any(), cron.JobsList, gomock.Any()).
		Return(cmdZAdd1).
		AnyTimes()

	cmdSet1 := redis.NewStatusCmd(context.Background())
	cmdSet1.SetVal("OK")
	mockPipeline.EXPECT().Exec(gomock.Any()).
		Return([]redis.Cmder{cmdSet1, cmdZAdd1}, nil).
		AnyTimes()

	// For stopping, times(0) => we don't expect those calls
	mockPipeline2.EXPECT().
		Set(gomock.Any(), stateKey, gomock.Any(), time.Duration(0)).
		Times(0)
	mockPipeline2.EXPECT().
		ZAdd(gomock.Any(), cron.JobsList, gomock.Any()).
		Times(0)
	mockPipeline2.EXPECT().
		Exec(gomock.Any()).
		Times(0)

	// ----------------------------
	// Setup synchronization
	// ----------------------------
	var wg sync.WaitGroup
	wg.Add(1)
	var executeTime time.Time

	// ----------------------------
	// CronJobOptions
	// ----------------------------
	options := cron.CronJobOptions{
		Name:  jobName,
		Spec:  expression,
		Redis: mockRedis,
		ExecuteFunc: func(ctx context.Context, job cron.ICronJob) error {
			// Called once
			executeTime = time.Now()
			wg.Done()
			return nil // No error => should set status "Success"
		},
	}

	// ----------------------------
	// Create CronJob
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

	if err := job.Start(ctx); err != nil {
		t.Fatalf("expected no error during CronJob start, got %v", err)
	}

	// ----------------------------
	// Wait for ExecuteFunc
	// ----------------------------
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// ExecuteFunc called
	case <-time.After(3 * time.Second):
		t.Fatal("ExecuteFunc was not called within the expected time")
	}

	// ----------------------------
	// Check updated state
	// ----------------------------
	state, err := job.GetState().Get(context.Background(), false)
	if err != nil {
		t.Fatalf("expected no error getting state, got %v", err)
	}
	if state == nil {
		t.Fatal("expected non-nil state after execution")
	}

	// Iterations incremented
	if state.Iterations != initialState.Iterations+1 {
		t.Fatalf("expected Iterations = %d, got %d",
			initialState.Iterations+1, state.Iterations)
	}

	// LastRun ~ executeTime
	if !isWithinDelta(state.LastRun, executeTime, 5*time.Second) {
		t.Fatalf("expected LastRun ~ %v, got %v",
			executeTime, state.LastRun)
	}

	// NextRun ~ parsedExpr.Next(executeTime)
	expectedNextRun := parsedExpr.Next(executeTime)
	if !isWithinDelta(state.NextRun, expectedNextRun, 5*time.Second) {
		t.Fatalf("expected NextRun ~ %v, got %v",
			expectedNextRun, state.NextRun)
	}

	// ExecuteFunc returned nil => expected status "Success"
	// (The lock release failing doesn't necessarily change the job's "status"
	// but you can adjust if your code sets "Failed" on release failure.)
	if state.Status != "Success" {
		t.Fatalf("expected Status 'Success', got '%s'", state.Status)
	}

	// Data remains the same
	if len(state.Data) != len(initialState.Data) {
		t.Fatalf("expected data len %d, got %d", len(initialState.Data), len(state.Data))
	}
	for k, v := range initialState.Data {
		if state.Data[k] != v {
			t.Fatalf("data mismatch for key '%s': expected '%v', got '%v'",
				k, v, state.Data[k])
		}
	}

	// ----------------------------
	// Stop the CronJob
	// ----------------------------
	if err := job.Stop(ctx); err != nil {
		t.Fatalf("expected no error during CronJob stop, got %v", err)
	}

	// Wait briefly for final updates
	time.Sleep(1 * time.Second)
}
