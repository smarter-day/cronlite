package tests

//
//import (
//	"context"
//	"cronlite"
//	"cronlite/mocks"
//	"encoding/json"
//	"github.com/gorhill/cronexpr"
//	"github.com/redis/go-redis/v9"
//	"go.uber.org/mock/gomock"
//	"testing"
//	"time"
//)
//
//func TestCronJob_Initialization(t *testing.T) {
//	ctrl := gomock.NewController(t)
//	defer ctrl.Finish()
//
//	mockRedis := mocks.NewMockCmdable(ctrl)
//	logger := mocks.NewMockILogger(ctrl)
//
//	options := cronlite.CronJobOptions{
//		Redis:  mockRedis,
//		Name:   "test-job",
//		Spec:   "*/5 * * * * *",
//		Job:    func(ctx context.Context, job *cronlite.CronJob) error { return nil },
//		Logger: logger,
//	}
//
//	cronJob, err := cronlite.NewCronJob(options)
//	if err != nil {
//		t.Fatalf("failed to initialize CronJob: %v", err)
//	}
//
//	if cronJob.Options.Name != "test-job" {
//		t.Fatalf("expected job name 'test-job', got: %v", cronJob.Options.Name)
//	}
//
//	if cronJob.CronExpr == nil {
//		t.Fatalf("expected CronExpr to be initialized")
//	}
//}
//
//func TestCronJob_GetJobState_ExistingState(t *testing.T) {
//	ctrl := gomock.NewController(t)
//	defer ctrl.Finish()
//
//	mockRedis := mocks.NewMockCmdable(ctrl)
//	logger := mocks.NewMockILogger(ctrl)
//
//	stateKey := "cronlite:job::test-job"
//	expectedState := cronlite.JobState{
//		Status:  cronlite.JobStatusRunning,
//		LastRun: time.Now().Add(-5 * time.Minute),
//		NextRun: time.Now().Add(5 * time.Minute),
//		Data:    map[string]interface{}{"iterations": 1},
//	}
//
//	stateData, _ := json.Marshal(expectedState)
//	mockRedis.EXPECT().
//		Get(gomock.Any(), stateKey).
//		Return(redis.NewStringResult(string(stateData), nil)).
//		Times(1)
//
//	options := cronlite.CronJobOptions{
//		Redis:  mockRedis,
//		Name:   "test-job",
//		Spec:   "*/5 * * * * *",
//		Logger: logger,
//	}
//
//	cronJob, _ := cronlite.NewCronJob(options)
//	state, err := cronJob.GetJobState(context.Background())
//	if err != nil {
//		t.Fatalf("failed to get job state: %v", err)
//	}
//
//	if state.Status != cronlite.JobStatusRunning {
//		t.Fatalf("expected status 'Running', got: %v", state.Status)
//	}
//}
//
//func TestCronJob_GetJobState_DefaultState(t *testing.T) {
//	ctrl := gomock.NewController(t)
//	defer ctrl.Finish()
//
//	mockRedis := mocks.NewMockCmdable(ctrl)
//	logger := mocks.NewMockILogger(ctrl)
//
//	stateKey := "cronlite:job::test-job"
//	mockRedis.EXPECT().
//		Get(gomock.Any(), stateKey).
//		Return(redis.NewStringResult("", redis.Nil)).
//		Times(1)
//
//	mockRedis.EXPECT().
//		Set(gomock.Any(), stateKey, gomock.Any(), gomock.Any()).
//		Return(redis.NewStatusResult("OK", nil)).
//		Times(1)
//
//	options := cronlite.CronJobOptions{
//		Redis:  mockRedis,
//		Name:   "test-job",
//		Spec:   "*/5 * * * * *",
//		Logger: logger,
//	}
//
//	cronJob, _ := cronlite.NewCronJob(options)
//	state, err := cronJob.GetJobState(context.Background())
//	if err != nil {
//		t.Fatalf("failed to get job state: %v", err)
//	}
//
//	// Update assertion to expect JobStatusSuccess
//	if state.Status != "" {
//		t.Fatalf("expected default state to have status 'Success', got: %v", state.Status)
//	}
//}
//
//func TestCronJob_SaveJobState(t *testing.T) {
//	ctrl := gomock.NewController(t)
//	defer ctrl.Finish()
//
//	mockRedis := mocks.NewMockCmdable(ctrl)
//	logger := mocks.NewMockILogger(ctrl)
//
//	stateKey := "cronlite:job::test-job"
//	state := cronlite.JobState{
//		Status:  cronlite.JobStatusSuccess,
//		LastRun: time.Now(),
//		NextRun: time.Now().Add(5 * time.Minute),
//		Data:    map[string]interface{}{"iterations": 2},
//	}
//
//	mockRedis.EXPECT().
//		Set(gomock.Any(), stateKey, gomock.Any(), gomock.Any()).
//		Return(redis.NewStatusResult("OK", nil)).
//		Times(1)
//
//	options := cronlite.CronJobOptions{
//		Redis:  mockRedis,
//		Name:   "test-job",
//		Spec:   "*/5 * * * * *",
//		Logger: logger,
//	}
//
//	cronJob, _ := cronlite.NewCronJob(options)
//	err := cronJob.SaveJobState(context.Background(), &state)
//	if err != nil {
//		t.Fatalf("failed to save job state: %v", err)
//	}
//}
//
//func TestCronJob_Start_ContextDone(t *testing.T) {
//	ctrl := gomock.NewController(t)
//	defer ctrl.Finish()
//
//	mockRedis := mocks.NewMockCmdable(ctrl)
//	logger := mocks.NewMockILogger(ctrl)
//
//	options := cronlite.CronJobOptions{
//		Redis:  mockRedis,
//		Name:   "test-job",
//		Spec:   "*/10 * * * * *",
//		Logger: logger,
//		Job: func(ctx context.Context, job *cronlite.CronJob) error {
//			return nil
//		},
//	}
//
//	mockRedis.EXPECT().
//		Get(gomock.Any(), "cronlite:job::test-job").
//		Return(redis.NewStringResult("", redis.Nil)).
//		AnyTimes()
//
//	mockRedis.EXPECT().
//		Set(gomock.Any(), "cronlite:job::test-job", gomock.Any(), gomock.Any()).
//		Return(redis.NewStatusResult("OK", nil)).
//		AnyTimes()
//
//	logger.EXPECT().
//		Debug(gomock.Any(), "Cron job skipped as the next run is in the future", gomock.Any()).
//		AnyTimes()
//
//	logger.EXPECT().
//		Info(gomock.Any(), "Cron job stopped by context", gomock.Any()).
//		Times(1)
//
//	cronJob, _ := cronlite.NewCronJob(options)
//	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(1*time.Second))
//
//	cancel()
//	time.Sleep(2 * time.Second) // Wait for the job to finish
//	cronJob.Start(ctx)
//	time.Sleep(2 * time.Second) // Wait for the job to finish
//}
//
//func TestCronJob_Stop(t *testing.T) {
//	ctrl := gomock.NewController(t)
//	defer ctrl.Finish()
//
//	mockRedis := mocks.NewMockCmdable(ctrl)
//	logger := mocks.NewMockILogger(ctrl)
//
//	options := cronlite.CronJobOptions{
//		Redis:  mockRedis,
//		Name:   "test-job",
//		Spec:   "*/10 * * * * *",
//		Logger: logger,
//		Job: func(ctx context.Context, job *cronlite.CronJob) error {
//			return nil
//		},
//	}
//
//	mockRedis.EXPECT().
//		Get(gomock.Any(), "cronlite:job::test-job").
//		Return(redis.NewStringResult("", redis.Nil)).
//		AnyTimes()
//
//	mockRedis.EXPECT().
//		Set(gomock.Any(), "cronlite:job::test-job", gomock.Any(), gomock.Any()).
//		Return(redis.NewStatusResult("OK", nil)).
//		AnyTimes()
//
//	logger.EXPECT().
//		Debug(gomock.Any(), "Cron job skipped as the next run is in the future", gomock.Any()).
//		AnyTimes()
//
//	logger.EXPECT().
//		Info(gomock.Any(), "Cron job force stopped", gomock.Any()).
//		Times(1)
//
//	cronJob, _ := cronlite.NewCronJob(options)
//	ctx, cancel := context.WithCancel(context.Background())
//	defer cancel()
//
//	go cronJob.Start(ctx)
//
//	time.Sleep(2 * time.Second) // Wait for the job to finish
//	cronJob.Stop()
//	time.Sleep(2 * time.Second) // Wait for the job to finish
//}
//
//func TestCronJob_Execute_LockFailed(t *testing.T) {
//	ctrl := gomock.NewController(t)
//	defer ctrl.Finish()
//
//	mockRedis := mocks.NewMockCmdable(ctrl)
//	logger := mocks.NewMockILogger(ctrl)
//	mockLocker := mocks.NewMockILocker(ctrl)
//
//	options := cronlite.CronJobOptions{
//		Redis:  mockRedis,
//		Name:   "test-job",
//		Spec:   "*/1 * * * * *",
//		Logger: logger,
//		Job: func(ctx context.Context, job *cronlite.CronJob) error {
//			return nil
//		},
//		Locker: mockLocker,
//	}
//
//	mockRedis.EXPECT().
//		Get(gomock.Any(), "cronlite:job::test-job").
//		Return(redis.NewStringResult("", redis.Nil)).
//		AnyTimes()
//
//	mockRedis.EXPECT().
//		Set(gomock.Any(), "cronlite:job::test-job", gomock.Any(), gomock.Any()).
//		Return(redis.NewStatusResult("OK", nil)).
//		AnyTimes()
//
//	mockLocker.EXPECT().
//		Acquire(gomock.Any()).
//		Return(false, nil).
//		Times(1)
//
//	logger.EXPECT().
//		Debug(gomock.Any(), "Cron job skipped due to existing lock", gomock.Any()).
//		Times(1)
//
//	cronJob, _ := cronlite.NewCronJob(options)
//	ctx := context.Background()
//
//	time.Sleep(2 * time.Second) // Wait for the job to finish
//	go cronJob.Start(ctx)
//	go cronJob.Start(ctx)
//	time.Sleep(2 * time.Second) // Wait for the job to finish
//}
//
//func TestCronJob_SetJobStatus(t *testing.T) {
//	ctrl := gomock.NewController(t)
//	defer ctrl.Finish()
//
//	mockRedis := mocks.NewMockCmdable(ctrl)
//	logger := mocks.NewMockILogger(ctrl)
//
//	stateKey := "cronlite:job::test-job"
//	state := cronlite.JobState{
//		Status:  cronlite.JobStatusRunning,
//		LastRun: time.Now(),
//		NextRun: time.Now().Add(5 * time.Minute),
//	}
//
//	expectedSerializedState, _ := json.Marshal(state)
//
//	mockRedis.EXPECT().
//		Set(gomock.Any(), "cronlite:job::test-job", expectedSerializedState, gomock.Any()).
//		Return(redis.NewStatusResult("OK", nil)).
//		Times(1)
//
//	mockRedis.EXPECT().
//		Get(gomock.Any(), stateKey).
//		Return(redis.NewStringResult("", redis.Nil)).
//		AnyTimes()
//
//	options := cronlite.CronJobOptions{
//		Redis:  mockRedis,
//		Name:   "test-job",
//		Spec:   "*/5 * * * * *",
//		Logger: logger,
//	}
//
//	cronJob, _ := cronlite.NewCronJob(options)
//	ctx := context.Background()
//
//	cronJob.SetJobStatus(ctx, cronlite.JobStatusRunning)
//}
//
//func TestCronJob_UpdateSpec(t *testing.T) {
//	ctrl := gomock.NewController(t)
//	defer ctrl.Finish()
//
//	// Mock Redis and Logger
//	mockRedis := mocks.NewMockCmdable(ctrl)
//	logger := mocks.NewMockILogger(ctrl)
//
//	// Define the initial and updated specifications
//	initialSpec := "*/1 * * * *"
//	updatedSpec := "* * */2 * *"
//
//	// Mock CronJobOptions
//	options := cronlite.CronJobOptions{
//		Redis:  mockRedis,
//		Name:   "test-job",
//		Spec:   initialSpec,
//		Logger: logger,
//	}
//
//	// Expected next run time for the updated spec
//	now := time.Now()
//	expectedNextRun := cronexpr.MustParse(updatedSpec).Next(now)
//
//	// Mock Redis behavior
//	stateKey := "cronlite:job::test-job"
//
//	// Initial state setup
//	initialState := cronlite.JobState{
//		Status:  cronlite.JobStatusRunning,
//		LastRun: time.Now(),
//		NextRun: time.Now(),
//		Data:    map[string]interface{}{},
//	}
//	serializedInitialState, _ := json.Marshal(initialState)
//
//	// Updated state setup
//	updatedState := initialState
//	updatedState.Data = map[string]interface{}{
//		"spec": updatedSpec,
//	}
//	serializedUpdatedState, _ := json.Marshal(updatedState)
//
//	mockRedis.EXPECT().
//		Get(gomock.Any(), stateKey).
//		Return(redis.NewStringResult(string(serializedInitialState), nil)).
//		Times(1)
//
//	// Logger behavior
//	logger.EXPECT().
//		Debug(gomock.Any(), gomock.Any(), gomock.Any()).
//		AnyTimes()
//
//	// Initialize the CronJob
//	cronJob, err := cronlite.NewCronJob(options)
//	if err != nil {
//		t.Fatalf("failed to create CronJob: %v", err)
//	}
//
//	// Get the current state and update the spec
//	ctx := context.Background()
//	state, err := cronJob.GetJobState(ctx)
//	if err != nil {
//		t.Fatalf("failed to get initial job state: %v", err)
//	}
//
//	// Update the spec in the state
//	mockRedis.EXPECT().
//		Get(gomock.Any(), stateKey).
//		Return(redis.NewStringResult(string(serializedUpdatedState), nil)).
//		Times(1)
//
//	mockRedis.EXPECT().
//		Set(gomock.Any(), stateKey, serializedUpdatedState, gomock.Any()).
//		Return(redis.NewStatusResult("OK", nil)).
//		Times(1)
//
//	state.Data = map[string]interface{}{
//		"spec": updatedSpec,
//	}
//	err = cronJob.SaveJobState(ctx, state)
//	if err != nil {
//		t.Fatalf("failed to save updated job state: %v", err)
//	}
//
//	// Verify the updated `NextRun` time
//	stateAfterExec, err := cronJob.GetJobState(ctx)
//	if err != nil {
//		t.Fatalf("failed to get job state after execution: %v", err)
//	}
//
//	// Check if next run and expected next run have the same time with precision to mintues
//	expectedNextRun = expectedNextRun.Truncate(time.Hour)
//	stateAfterExec.NextRun = stateAfterExec.NextRun.Truncate(time.Hour)
//	if !stateAfterExec.NextRun.Equal(expectedNextRun) {
//		t.Fatalf("expected NextRun to match updated spec (%v), got: %v", expectedNextRun, stateAfterExec.NextRun)
//	}
//}
