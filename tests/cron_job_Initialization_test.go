package tests

//import (
//	"context"
//	"cronlite/cron"
//	"cronlite/mocks" // Ensure this path is correct based on your project structure
//	"fmt"
//	"github.com/gorhill/cronexpr"
//	"github.com/redis/go-redis/v9"
//	"go.uber.org/mock/gomock"
//	"sync"
//	"testing"
//	"time"
//)
//
//func TestCronJob_Initialization(t *testing.T) {
//	// Initialize mock controller
//	ctrl := gomock.NewController(t)
//	defer ctrl.Finish()
//
//	// Mock dependencies
//	mockLocker := mocks.NewMockILocker(ctrl)
//	mockRedis := mocks.NewMockCmdable(ctrl)
//	mockPipeline := mocks.NewMockPipeliner(ctrl) // Newly created mock for Pipeliner
//
//	// Define test variables
//	jobName := "test-job"
//	expression := "*/5 * * * *" // Every 5 minutes
//	now := time.Now()
//
//	// Calculate the expected next run time based on the test's 'now'
//	parsedExpr, err := cron.SpecParser.Parse(expression)
//	if err != nil {
//		t.Fatalf("failed to parse cron expression: %v", err)
//	}
//	nextRun := parsedExpr.Next(now)
//
//	// Prepare the state key format (assuming it's "cronlite:job:state:%s")
//	stateKey := fmt.Sprintf(cron.JobStateKeyFormat, jobName)
//
//	// ----------------------------
//	// Mock Redis.Get Call
//	// ----------------------------
//	// Simulate that the job state does not exist in Redis initially (redis.Nil)
//	cmdGet := redis.NewStringCmd(context.Background(), "GET", stateKey)
//	cmdGet.SetErr(redis.Nil) // Simulate key does not exist
//	mockRedis.EXPECT().Get(gomock.Any(), stateKey).
//		Return(cmdGet).
//		Times(1)
//
//	// ----------------------------
//	// Mock Redis.TxPipeline Call
//	// ----------------------------
//	mockRedis.EXPECT().TxPipeline().
//		Return(mockPipeline).
//		Times(1)
//
//	// ----------------------------
//	// Mock Pipeline.Set Call
//	// ----------------------------
//	// Simulate Set call to store the initial state
//	cmdSet := redis.NewStatusCmd(context.Background(), "SET", stateKey, gomock.Any(), gomock.Any())
//	cmdSet.SetVal("OK") // Simulate successful SET
//	mockPipeline.EXPECT().Set(gomock.Any(), stateKey, gomock.Any(), gomock.Any()).
//		Return(cmdSet).
//		Times(1)
//
//	// ----------------------------
//	// Mock Pipeline.ZAdd Call
//	// ----------------------------
//	// Simulate ZAdd call to update the sorted set with job recency
//	cmdZAdd := redis.NewIntCmd(context.Background(), "ZADD", cron.JobsList, gomock.Any())
//	cmdZAdd.SetVal(1) // Simulate successful ZADD
//	mockPipeline.EXPECT().ZAdd(gomock.Any(), cron.JobsList, gomock.Any()).
//		Return(cmdZAdd).
//		Times(1)
//
//	// ----------------------------
//	// Mock Pipeline.Exec Call
//	// ----------------------------
//	// Simulate successful execution of the pipeline
//	mockPipeline.EXPECT().Exec(gomock.Any()).
//		Return([]redis.Cmder{cmdSet, cmdZAdd}, nil).
//		Times(1)
//
//	// ----------------------------
//	// Create CronJobOptions
//	// ----------------------------
//	options := cron.CronJobOptions{
//		Name:   jobName,
//		Spec:   expression,
//		Locker: mockLocker,
//		Redis:  mockRedis,
//		ExecuteFunc: func(ctx context.Context, job cron.ICronJob) error {
//			// Mock execution function; does nothing
//			return nil
//		},
//	}
//
//	// ----------------------------
//	// Create a new CronJob instance
//	// ----------------------------
//	job, err := cron.NewCronJob(options)
//	if err != nil {
//		t.Fatalf("expected no error during initialization, got %v", err)
//	}
//
//	// ----------------------------
//	// Start the CronJob
//	// ----------------------------
//	ctx, cancel := context.WithCancel(context.Background())
//	defer cancel()
//
//	err = job.Start(ctx)
//	if err != nil {
//		t.Fatalf("expected no error during job start, got %v", err)
//	}
//
//	// Allow some time for the goroutine to potentially execute
//	time.Sleep(100 * time.Millisecond)
//
//	// ----------------------------
//	// Retrieve and Verify the State
//	// ----------------------------
//	state, err := job.GetState().Get(context.Background(), false)
//	if err != nil {
//		t.Fatalf("expected no error during state retrieval, got %v", err)
//	}
//
//	// Ensure that the state has been initialized
//	if state == nil {
//		t.Fatal("expected state to be initialized, got nil")
//	}
//
//	// Verify that NextRun is correctly set
//	if state.NextRun.Before(now) || state.NextRun.After(nextRun.Add(1*time.Second)) {
//		t.Fatalf("expected NextRun to be within the calculated range, got %v", state.NextRun)
//	}
//
//	// Verify that the job name is correctly set
//	if job.GetOptions().Name != jobName {
//		t.Fatalf("expected job name to be %s, got %s", jobName, job.GetOptions().Name)
//	}
//
//	// ----------------------------
//	// Stop the CronJob
//	// ----------------------------
//	err = job.Stop(ctx)
//	if err != nil {
//		t.Fatalf("expected no error during job stop, got %v", err)
//	}
//
//	// Optionally, verify that StopSignal was handled correctly
//	// This might require additional state checks or mock interactions
//}
//
//func TestCronJob_InvalidCronExpression(t *testing.T) {
//	ctrl := gomock.NewController(t)
//	defer ctrl.Finish()
//
//	mockLocker := mocks.NewMockILocker(ctrl)
//	mockRedis := mocks.NewMockCmdable(ctrl)
//
//	// Define an invalid cron expression
//	invalidExpression := "invalid cron"
//
//	options := cron.CronJobOptions{
//		Name:   "test-job-invalid-cron",
//		Spec:   invalidExpression,
//		Locker: mockLocker,
//		Redis:  mockRedis,
//		ExecuteFunc: func(ctx context.Context, job cron.ICronJob) error {
//			// This should not be called
//			t.Fatal("ExecuteFunc should not be called with invalid cron expression")
//			return nil
//		},
//	}
//
//	// Attempt to create a new CronJob with invalid cron expression
//	_, err := cron.NewCronJob(options)
//	if err == nil {
//		t.Fatalf("expected error during CronJob initialization with invalid cron expression, got nil")
//	}
//}
//
//func TestCronJob_ContextCancellation(t *testing.T) {
//	ctrl := gomock.NewController(t)
//	defer ctrl.Finish()
//
//	mockLocker := mocks.NewMockILocker(ctrl)
//	mockRedis := mocks.NewMockCmdable(ctrl)
//	mockPipeline := mocks.NewMockPipeliner(ctrl)
//
//	jobName := "test-job-context-cancellation"
//	expression := "*/2 * * * * *" // Every 2 seconds for testing
//	now := time.Now()
//
//	parsedExpr, err := cron.SpecParser.Parse(expression)
//	if err != nil {
//		t.Fatalf("failed to parse cron expression: %v", err)
//	}
//	nextRun := parsedExpr.Next(now)
//
//	stateKey := fmt.Sprintf(cron.JobStateKeyFormat, jobName)
//
//	// Mock Locker to acquire lock successfully
//	mockLocker.EXPECT().Acquire(gomock.Any()).Return(true, nil).AnyTimes()
//	mockLocker.EXPECT().GetLockTTL().Return(5 * time.Second).Times(1)
//	// Simulate lock release success
//	mockLocker.EXPECT().Release(gomock.Any()).Return(nil).Times(1)
//
//	// Mock Redis GET calls
//	initialState := &cron.CronJobState{
//		RunningBy:  "",
//		Status:     "succeeded",
//		Iterations: 0,
//		Spec:       expression,
//		Data:       map[string]interface{}{},
//		LastRun:    time.Time{},
//		NextRun:    nextRun,
//		CreatedAt:  now,
//		UpdatedAt:  now,
//	}
//
//	serializedState, err := SerializeJobState(initialState)
//	if err != nil {
//		t.Fatalf("failed to serialize job state: %v", err)
//	}
//
//	cmdGet1 := redis.NewStringCmd(context.Background(), "GET", stateKey)
//	cmdGet1.SetVal(serializedState)
//	mockRedis.EXPECT().Get(gomock.Any(), stateKey).
//		Return(cmdGet1).
//		AnyTimes()
//
//	// After lock acquisition, updated state
//	updatedState := *initialState
//	updatedState.RunningBy = "worker-id"
//	updatedState.Status = "Running"
//	updatedState.UpdatedAt = now.Add(1 * time.Second)
//
//	serializedUpdatedState, err := SerializeJobState(&updatedState)
//	if err != nil {
//		t.Fatalf("failed to serialize updated job state: %v", err)
//	}
//
//	cmdGet2 := redis.NewStringCmd(context.Background(), "GET", stateKey)
//	cmdGet2.SetVal(serializedUpdatedState)
//	mockRedis.EXPECT().Get(gomock.Any(), stateKey).
//		Return(cmdGet2).
//		AnyTimes()
//
//	// Mock Redis TxPipeline for execution
//	mockRedis.EXPECT().TxPipeline().
//		Return(mockPipeline).
//		Times(1)
//
//	// Mock Pipeline SET and ZADD
//	mockPipeline.EXPECT().Set(gomock.Any(), stateKey, gomock.Any(), time.Duration(0)).
//		Return(redis.NewStatusCmd(context.Background(), "SET", stateKey, "", time.Duration(0))).
//		Times(1)
//	cmdZAdd := redis.NewIntCmd(context.Background(), "ZADD", cron.JobsList, 0)
//	cmdZAdd.SetVal(1)
//	mockPipeline.EXPECT().ZAdd(gomock.Any(), cron.JobsList, gomock.Any()).
//		Return(cmdZAdd).
//		Times(1)
//	mockPipeline.EXPECT().Exec(gomock.Any()).
//		Return([]redis.Cmder{redis.NewStatusCmd(context.Background(), "SET", stateKey, "", time.Duration(0)), cmdZAdd}, nil).
//		Times(1)
//
//	// Mock Redis TxPipeline for stopping
//	mockRedis.EXPECT().TxPipeline().
//		Return(mockPipeline).
//		Times(1)
//
//	// Mock Pipeline SET and ZADD for stopping
//	mockPipeline.EXPECT().Set(gomock.Any(), stateKey, gomock.Any(), time.Duration(0)).
//		Return(redis.NewStatusCmd(context.Background(), "SET", stateKey, "", time.Duration(0))).
//		Times(1)
//	cmdZAddStop := redis.NewIntCmd(context.Background(), "ZADD", cron.JobsList, 0)
//	cmdZAddStop.SetVal(1)
//	mockPipeline.EXPECT().ZAdd(gomock.Any(), cron.JobsList, gomock.Any()).
//		Return(cmdZAddStop).
//		Times(1)
//	mockPipeline.EXPECT().Exec(gomock.Any()).
//		Return([]redis.Cmder{redis.NewStatusCmd(context.Background(), "SET", stateKey, "", time.Duration(0)), cmdZAddStop}, nil).
//		Times(1)
//
//	// Setup synchronization
//	var wg sync.WaitGroup
//	wg.Add(2)
//
//	executionTime := time.Time{}
//
//	options := cron.CronJobOptions{
//		Name:   jobName,
//		Spec:   expression,
//		Locker: mockLocker,
//		Redis:  mockRedis,
//		ExecuteFunc: func(ctx context.Context, job cron.ICronJob) error {
//			executionTime = time.Now()
//			wg.Done()
//			// Simulate long-running execution
//			select {
//			case <-ctx.Done():
//				return ctx.Err()
//			case <-time.After(5 * time.Second):
//				return nil
//			}
//		},
//	}
//
//	job, err := cron.NewCronJob(options)
//	if err != nil {
//		t.Fatalf("expected no error during CronJob initialization, got %v", err)
//	}
//
//	// Start the CronJob
//	jobCtx, jobCancel := context.WithCancel(context.Background())
//	defer jobCancel()
//
//	err = job.Start(jobCtx)
//	if err != nil {
//		t.Fatalf("expected no error during CronJob start, got %v", err)
//	}
//
//	// Wait for ExecuteFunc to be called
//	done := make(chan struct{})
//	go func() {
//		wg.Wait()
//		close(done)
//	}()
//
//	jobCancel()
//
//	// Wait briefly to allow CronJob to process cancellation
//	time.Sleep(2 * time.Second)
//
//	// Retrieve and verify the updated state
//	state, err := job.GetState().Get(context.Background(), false)
//	if err != nil {
//		t.Fatalf("expected no error during state retrieval, got %v", err)
//	}
//
//	// Verify state updates
//	if state == nil {
//		t.Fatal("expected state to be initialized, got nil")
//	}
//
//	// Iterations should be incremented
//	if state.Iterations != initialState.Iterations+1 {
//		t.Fatalf("expected Iterations to be %d, got %d", initialState.Iterations+1, state.Iterations)
//	}
//
//	// LastRun should be updated
//	if !isWithinDelta(state.LastRun, executionTime, 5*time.Second) {
//		t.Fatalf("expected LastRun to be around %v, got %v", executionTime, state.LastRun)
//	}
//
//	// NextRun should be updated based on the cron expression
//	expectedNextRun := parsedExpr.Next(executionTime)
//	if !isWithinDelta(state.NextRun, expectedNextRun, 5*time.Second) {
//		t.Fatalf("expected NextRun to be around %v, got %v", expectedNextRun, state.NextRun)
//	}
//
//	// Status should be "Success" if ExecuteFunc completed without error
//	if state.Status != cron.JobStatusFailed {
//		t.Fatalf("expected Status to be '%s', got '%s'", cron.JobStatusFailed, state.Status)
//	}
//
//	// Data should remain unchanged
//	if len(state.Data) == 0 {
//		t.Fatalf("expected Data length to be %d, got %d", len(initialState.Data), len(state.Data))
//	}
//	for key, value := range initialState.Data {
//		if state.Data[key] != value {
//			t.Fatalf("expected Data[%s] to be '%v', got '%v'", key, value, state.Data[key])
//		}
//	}
//
//	// Job name should remain unchanged
//	if job.GetOptions().Name != jobName {
//		t.Fatalf("expected job name to be %s, got %s", jobName, job.GetOptions().Name)
//	}
//
//	// Stop the CronJob (already canceled)
//	err = job.Stop(jobCtx)
//	if err != nil {
//		t.Fatalf("expected no error during CronJob stop, got %v", err)
//	}
//
//	time.Sleep(2 * time.Second)
//}
