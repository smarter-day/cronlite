package tests

import (
	"context"
	"cronlite/cron"
	"cronlite/mocks"
	"fmt"
	"github.com/redis/go-redis/v9"
	"go.uber.org/mock/gomock"
	"sync"
	"testing"
	"time"
)

// TestCronJob_Execution_ExecuteFuncFailure verifies that the CronJob correctly
// handles failures within ExecuteFunc, updating the job's status accordingly.
func TestCronJob_Execution_ExecuteFuncFailure(t *testing.T) {
	// Initialize mock controller
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// Mock dependencies
	mockLocker := mocks.NewMockILocker(ctrl)
	mockRedis := mocks.NewMockCmdable(ctrl)
	mockPipeline := mocks.NewMockPipeliner(ctrl)
	mockPipeline2 := mocks.NewMockPipeliner(ctrl)

	// Define test variables
	jobName := "test-job-execution-func-failure"
	expression := "* * * * * *" // every second
	now := time.Now()

	// Parse expression with robfig/cron/v3 SpecParser
	parsedExpr, err := cron.SpecParser.Parse(expression)
	if err != nil {
		t.Fatalf("failed to parse cron expression: %v", err)
	}
	nextRun := parsedExpr.Next(now)

	// State key in Redis
	stateKey := fmt.Sprintf(cron.JobStateKeyFormat, jobName)

	// ----------------------------
	// Mock Locker Behavior
	// ----------------------------
	mockLocker.EXPECT().Acquire(gomock.Any()).Return(true, nil).Times(1)
	lockTTL := 5 * time.Second
	mockLocker.EXPECT().GetLockTTL().Return(lockTTL).Times(1)
	mockLocker.EXPECT().Release(gomock.Any()).Return(nil).Times(1)

	// ----------------------------
	// Mock Redis.Get Call
	// ----------------------------
	initialState := &cron.CronJobState{
		RunningBy:  "",
		Status:     cron.JobStatusSuccess,
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

	cmdGet1 := redis.NewStringCmd(context.Background(), "GET", stateKey)
	cmdGet1.SetVal(serializedState)

	// Expect Get() once for initial load
	mockRedis.EXPECT().Get(gomock.Any(), stateKey).Return(cmdGet1).Times(1)

	// Simulate state with lock acquired
	updatedState := *initialState
	updatedState.RunningBy = "worker-id"
	updatedState.Status = cron.JobStatusRunning
	updatedState.UpdatedAt = now.Add(1 * time.Second)

	serializedUpdatedState, err := cron.SerializeJobState(&updatedState)
	if err != nil {
		t.Fatalf("failed to serialize updated job state: %v", err)
	}

	cmdGet2 := redis.NewStringCmd(context.Background(), "GET", stateKey)
	cmdGet2.SetVal(serializedUpdatedState)

	// ----------------------------
	// Mock Redis.TxPipeline Calls
	// ----------------------------
	mockRedis.EXPECT().TxPipeline().Return(mockPipeline).AnyTimes()
	mockRedis.EXPECT().TxPipeline().Return(mockPipeline2).AnyTimes()

	// ----------------------------
	// Pipeline for execution (mockPipeline)
	// ----------------------------
	mockPipeline.EXPECT().
		Set(gomock.Any(), stateKey, gomock.Any(), time.Duration(0)).
		Return(redis.NewStatusCmd(context.Background(), "SET", stateKey, "", time.Duration(0))).
		AnyTimes()

	cmdZAdd1 := redis.NewIntCmd(context.Background(), "ZADD", cron.JobsList, 0)
	cmdZAdd1.SetVal(1)
	mockPipeline.EXPECT().ZAdd(gomock.Any(), cron.JobsList, gomock.Any()).
		Return(cmdZAdd1).
		AnyTimes()

	cmdSet1 := redis.NewStatusCmd(context.Background(), "SET", stateKey, "", time.Duration(0))
	cmdSet1.SetVal("OK")
	mockPipeline.EXPECT().Exec(gomock.Any()).
		Return([]redis.Cmder{cmdSet1, cmdZAdd1}, nil).
		AnyTimes()

	// ----------------------------
	// Pipeline for stop (mockPipeline2)
	// ----------------------------
	mockPipeline2.EXPECT().
		Set(gomock.Any(), stateKey, gomock.Any(), time.Duration(0)).
		Return(redis.NewStatusCmd(context.Background(), "SET", stateKey, "", time.Duration(0))).
		AnyTimes()

	cmdZAdd2 := redis.NewIntCmd(context.Background(), "ZADD", cron.JobsList, 0)
	cmdZAdd2.SetVal(1)
	mockPipeline2.EXPECT().ZAdd(gomock.Any(), cron.JobsList, gomock.Any()).
		Return(cmdZAdd2).
		AnyTimes()

	cmdSet2 := redis.NewStatusCmd(context.Background(), "SET", stateKey, "", time.Duration(0))
	cmdSet2.SetVal("OK")
	mockPipeline2.EXPECT().Exec(gomock.Any()).
		Return([]redis.Cmder{cmdSet2, cmdZAdd2}, nil).
		AnyTimes()

	// ----------------------------
	// Sync
	// ----------------------------
	var wg sync.WaitGroup
	wg.Add(1)
	var executeTime time.Time

	// ----------------------------
	// CronJobOptions
	// ----------------------------
	options := cron.CronJobOptions{
		Name:   jobName,
		Spec:   expression,
		Locker: mockLocker,
		Redis:  mockRedis,
		ExecuteFunc: func(ctx context.Context, job cron.ICronJob) error {
			executeTime = time.Now()
			wg.Done()
			return fmt.Errorf("execute func failed")
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
	// Start CronJob
	// ----------------------------
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := job.Start(ctx); err != nil {
		t.Fatalf("expected no error during CronJob start, got %v", err)
	}

	// Wait for ExecuteFunc
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// ExecuteFunc was called
	case <-time.After(3 * time.Second):
		t.Fatal("ExecuteFunc was not called within the expected time")
	}

	// ----------------------------
	// Verify state after failure
	// ----------------------------
	state, err := job.GetState().Get(context.Background(), false)
	if err != nil {
		t.Fatalf("expected no error during state retrieval, got %v", err)
	}
	if state == nil {
		t.Fatal("expected state to be initialized, got nil")
	}

	// Iterations should increment
	if state.Iterations != initialState.Iterations+1 {
		t.Fatalf("expected Iterations to be %d, got %d",
			initialState.Iterations+1, state.Iterations)
	}

	// LastRun ~ executeTime
	if !isWithinDelta(state.LastRun, executeTime, 5*time.Second) {
		t.Fatalf("expected LastRun ~ %v, got %v", executeTime, state.LastRun)
	}

	// NextRun ~ parsedExpr.Next(executeTime)
	expectedNextRun := parsedExpr.Next(executeTime)
	if !isWithinDelta(state.NextRun, expectedNextRun, 5*time.Second) {
		t.Fatalf("expected NextRun ~ %v, got %v", expectedNextRun, state.NextRun)
	}

	// Status should be Failed
	if state.Status != cron.JobStatusFailed {
		t.Fatalf("expected Status to be '%s', got '%s'",
			cron.JobStatusFailed, state.Status)
	}

	// Data remains unchanged
	for key, value := range initialState.Data {
		if state.Data[key] != value {
			t.Fatalf("expected Data[%s] to be '%v', got '%v'",
				key, value, state.Data[key])
		}
	}

	// Job name remains the same
	if job.GetOptions().Name != jobName {
		t.Fatalf("expected job name to be %s, got %s",
			jobName, job.GetOptions().Name)
	}

	// ----------------------------
	// Stop CronJob
	// ----------------------------
	if err := job.Stop(ctx); err != nil {
		t.Fatalf("expected no error during CronJob stop, got %v", err)
	}

	// Wait briefly for final updates
	time.Sleep(2 * time.Second)
}
