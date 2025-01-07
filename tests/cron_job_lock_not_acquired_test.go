package tests

import (
	"context"
	"fmt"
	"testing"
	"time"

	"cronlite/cron"
	"cronlite/mocks"

	"github.com/redis/go-redis/v9"
	"go.uber.org/mock/gomock"
)

// TestCronJob_Execution_LockAcquisitionFailure verifies that the CronJob does not execute
// the ExecuteFunc and does not perform Redis pipeline operations when it fails to acquire the lock.
func TestCronJob_Execution_LockAcquisitionFailure(t *testing.T) {
	// Initialize mock controller
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRedis := mocks.NewMockUniversalClient(ctrl)

	// Define test variables
	jobName := "test-job-execution-lock-failure"
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

	// Simulate that the job state exists in Redis
	initialState := &cron.CronJobState{
		RunningBy:  "", // Indicates no current runner
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

	cmdGet := redis.NewStringCmd(context.Background(), "GET", stateKey)
	cmdGet.SetVal(serializedState)
	// Expect Get to be called once for state retrieval
	mockRedis.EXPECT().Get(gomock.Any(), stateKey).
		Return(cmdGet).
		AnyTimes()

	cmdSetNX := redis.NewBoolCmd(context.Background())
	cmdSetNX.SetVal(false) // means lock not acquired
	mockRedis.EXPECT().
		SetNX(
			gomock.Any(),
			"cron-job-lock:"+jobName,
			gomock.Any(), // Redsync's random value
			gomock.Any(), // TTL
		).
		Return(cmdSetNX).
		AnyTimes()

	mockRedis.EXPECT().
		EvalSha(
			gomock.Any(),
			gomock.Any(),
			gomock.Any(),
			gomock.Any(),
		).
		Return(redis.NewCmd(context.Background(), "EVALSHA", 0)).
		AnyTimes()

	// ----------------------------
	// No Redis.TxPipeline Expectations
	// ----------------------------
	// Since the lock wasn't acquired, TxPipeline should not be called
	// No need to set any expectations for TxPipeline

	// ----------------------------
	// Setup Synchronization
	// ----------------------------
	// ExecuteFunc should not be called, so no WaitGroup is needed
	// However, to ensure ExecuteFunc isn't called, we set it to fail the test if invoked

	// ----------------------------
	// Create CronJobOptions
	// ----------------------------
	options := cron.CronJobOptions{
		Name:  jobName,
		Spec:  expression,
		Redis: mockRedis,
		ExecuteFunc: func(ctx context.Context, job cron.ICronJob) error {
			// If ExecuteFunc is called, the test should fail
			t.Fatal("ExecuteFunc should not be called when lock acquisition fails")
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
	// Wait to Ensure ExecuteFunc is Not Called
	// ----------------------------
	// Since ExecuteFunc should not be called, we'll wait for a short duration
	select {
	case <-time.After(2 * time.Second):
		// Test passes as ExecuteFunc was not called
	case <-ctx.Done():
		t.Fatal("context was canceled unexpectedly")
	}

	// ----------------------------
	// Stop the CronJob
	// ----------------------------
	err = job.Stop(ctx)
	if err != nil {
		t.Fatalf("expected no error during CronJob stop, got %v", err)
	}

	time.Sleep(2 * time.Second)
}
