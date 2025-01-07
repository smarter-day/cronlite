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
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// Mock dependencies
	mockRedis := mocks.NewMockUniversalClient(ctrl)
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
	// Mock Redis.Get Call (initial load)
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
	mockRedis.EXPECT().Get(gomock.Any(), stateKey).Return(cmdGet1).AnyTimes()

	// ----------------------------
	// Mock Redis.SetNX (lock acquisition)
	// ----------------------------
	// 1) First call => success: lock is acquired
	setNXCmdAcquired := redis.NewBoolCmd(context.Background())
	setNXCmdAcquired.SetVal(true)
	mockRedis.EXPECT().
		SetNX(gomock.Any(), "cron-job-lock:"+jobName, gomock.Any(), gomock.Any()).
		Return(setNXCmdAcquired).
		Times(1)

	// 2) Any subsequent calls => fail
	setNXCmdFail := redis.NewBoolCmd(context.Background())
	setNXCmdFail.SetVal(false)
	mockRedis.EXPECT().
		SetNX(gomock.Any(), "cron-job-lock:"+jobName, gomock.Any(), gomock.Any()).
		Return(setNXCmdFail).
		AnyTimes()

	// ----------------------------
	// Redsync calls EvalSha to Extend/Unlock
	// We'll just mock them as successful
	// ----------------------------
	extendCmdOk := redis.NewCmd(context.Background())
	extendCmdOk.SetVal(int64(1)) // 1 => success
	mockRedis.EXPECT().
		EvalSha(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(extendCmdOk).
		AnyTimes()

	// ----------------------------
	// Mock Redis.TxPipeline Calls
	// ----------------------------
	mockRedis.EXPECT().TxPipeline().Return(mockPipeline).AnyTimes()
	mockRedis.EXPECT().TxPipeline().Return(mockPipeline2).AnyTimes()

	// For the "execution" pipeline:
	mockPipeline.EXPECT().
		Set(gomock.Any(), stateKey, gomock.Any(), time.Duration(0)).
		Return(redis.NewStatusCmd(context.Background())).
		AnyTimes()

	cmdZAdd1 := redis.NewIntCmd(context.Background())
	cmdZAdd1.SetVal(1)
	mockPipeline.EXPECT().
		ZAdd(gomock.Any(), cron.JobsList, gomock.Any()).
		Return(cmdZAdd1).
		AnyTimes()

	cmdSet1 := redis.NewStatusCmd(context.Background())
	cmdSet1.SetVal("OK")
	mockPipeline.EXPECT().Exec(gomock.Any()).
		Return([]redis.Cmder{cmdSet1, cmdZAdd1}, nil).
		AnyTimes()

	// For the "stop" pipeline:
	mockPipeline2.EXPECT().
		Set(gomock.Any(), stateKey, gomock.Any(), time.Duration(0)).
		Return(redis.NewStatusCmd(context.Background())).
		AnyTimes()

	cmdZAdd2 := redis.NewIntCmd(context.Background())
	cmdZAdd2.SetVal(1)
	mockPipeline2.EXPECT().
		ZAdd(gomock.Any(), cron.JobsList, gomock.Any()).
		Return(cmdZAdd2).
		AnyTimes()

	cmdSet2 := redis.NewStatusCmd(context.Background())
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
		Name:  jobName,
		Spec:  expression,
		Redis: mockRedis,
		ExecuteFunc: func(ctx context.Context, job cron.ICronJob) error {
			// This should get called exactly once
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
