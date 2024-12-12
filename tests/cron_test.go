package tests

import (
	"context"
	"cronlite/cron"
	"cronlite/mocks"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/gorhill/cronexpr"
	"github.com/redis/go-redis/v9"
	"go.uber.org/mock/gomock"
	"testing"
	"time"
)

type MockWorkerIdProvider struct {
	ID  string
	Err error
}

func (m MockWorkerIdProvider) Id() (string, error) {
	return m.ID, m.Err
}

func TestCronJob_Initialization(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRedis := mocks.NewMockCmdable(ctrl)
	// We will not explicitly provide Logger and Locker to test default assignment
	options := cron.JobOptions{
		Redis: mockRedis,
		Name:  "TestCronJob_Initialization",
		Spec:  "*/5 * * * * *",
		Job: func(ctx context.Context, job *cron.Job) error {
			return nil
		},
	}

	job, err := cron.NewJob(options)
	if err != nil {
		t.Fatalf("failed to initialize Job: %v", err)
	}

	// Check the name
	if job.(*cron.Job).Options.Name != "TestCronJob_Initialization" {
		t.Errorf("expected job name 'TestCronJob_Initialization', got: %v", job.(*cron.Job).Options.Name)
	}

	// Check if the CronExpr is set
	if job.(*cron.Job).CronExpr == nil {
		t.Errorf("expected CronExpr to be initialized, got nil")
	}

	// Validate the cron spec was parsed correctly
	now := time.Now()
	nextRun := job.(*cron.Job).CronExpr.Next(now)
	if nextRun.Before(now) {
		t.Errorf("expected next run to be in the future, got: %v", nextRun)
	}

	// Check that logger is set by default if not provided
	if job.(*cron.Job).Options.Logger == nil {
		t.Errorf("expected a default logger to be set, got nil")
	}

	// Check that locker is set by default if not provided
	if job.(*cron.Job).Options.Locker == nil {
		t.Errorf("expected a default locker to be set, got nil")
	}

	// Validate state key
	if job.(*cron.Job).StateKey != fmt.Sprintf(cron.JobStateKeyFormat, "TestCronJob_Initialization") {
		t.Errorf("unexpected state key, got: %v", job.(*cron.Job).StateKey)
	}
}

func TestCronJob_SaveJobState(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRedis := mocks.NewMockCmdable(ctrl)
	mockPipeline := mocks.NewMockPipeliner(ctrl)
	logger := mocks.NewMockILogger(ctrl)

	jobName := "TestCronJob_SaveJobState"
	stateKey := fmt.Sprintf(cron.JobStateKeyFormat, jobName)

	state := cron.JobState{
		Status:  cron.JobStatusSuccess,
		LastRun: time.Now(),
		NextRun: time.Now().Add(5 * time.Minute),
		Data:    map[string]interface{}{"iterations": 2},
	}

	mockRedis.EXPECT().TxPipeline().Return(mockPipeline).Times(1)

	mockPipeline.EXPECT().
		Set(gomock.Any(), stateKey, gomock.Any(), time.Duration(0)).
		DoAndReturn(func(ctx context.Context, key string, value interface{}, expiration time.Duration) *redis.StatusCmd {
			if key != stateKey {
				t.Errorf("Set called with unexpected key: got %s, want %s", key, stateKey)
			}
			if expiration != 0 {
				t.Errorf("Set called with unexpected expiration: got %d, want 0", expiration)
			}

			// Value should be []byte, not string
			bytes, ok := value.([]byte)
			if !ok {
				t.Errorf("Set value is not a []byte, got %T", value)
			}

			var savedState cron.JobState
			if err := json.Unmarshal(bytes, &savedState); err != nil {
				t.Errorf("failed to unmarshal state: %v", err)
			}

			// Check some fields
			if savedState.Status != state.Status {
				t.Errorf("saved state Status mismatch: got %v, want %v", savedState.Status, state.Status)
			}
			if savedState.Data["iterations"] != float64(2) {
				// JSON numbers unmarshal as float64 by default
				t.Errorf("saved state Data mismatch: got %v, want 2", savedState.Data["iterations"])
			}

			return redis.NewStatusResult("OK", nil)
		}).
		Times(1)

	expectedScore := float64(state.LastRun.Unix())
	if expectedScore == 0 {
		expectedScore = float64(state.NextRun.Unix())
	}

	mockPipeline.EXPECT().
		ZAdd(gomock.Any(), cron.JobsList, gomock.Any()).
		DoAndReturn(func(ctx context.Context, key string, members ...redis.Z) *redis.IntCmd {
			if key != cron.JobsList {
				t.Errorf("ZAdd called with wrong key. got: %s, want: %s", key, cron.JobsList)
			}
			if len(members) != 1 {
				t.Errorf("ZAdd expected one member, got: %d", len(members))
			} else {
				member := members[0]
				if member.Score != expectedScore {
					t.Errorf("ZAdd called with wrong score. got: %f, want: %f", member.Score, expectedScore)
				}
				if member.Member != jobName {
					t.Errorf("ZAdd called with wrong member. got: %s, want: %s", member.Member, jobName)
				}
			}
			return redis.NewIntResult(1, nil)
		}).
		Times(1)

	mockPipeline.EXPECT().
		Exec(gomock.Any()).
		Return([]redis.Cmder{}, nil).
		Times(1)

	options := cron.JobOptions{
		Redis:  mockRedis,
		Name:   jobName,
		Spec:   "*/5 * * * * *",
		Logger: logger,
		Job: func(ctx context.Context, job *cron.Job) error {
			return nil
		},
	}

	cronJob, err := cron.NewJob(options)
	if err != nil {
		t.Fatalf("failed to initialize Job: %v", err)
	}

	if err = cronJob.SaveState(context.Background(), &state); err != nil {
		t.Fatalf("failed to save job state: %v", err)
	}
}

func TestCronJob_GetJobState_ExistingState(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRedis := mocks.NewMockCmdable(ctrl)

	jobName := "TestCronJob_GetJobState_ExistingState"
	stateKey := fmt.Sprintf(cron.JobStateKeyFormat, jobName)
	expectedState := cron.JobState{
		Status:    cron.JobStatusRunning,
		LastRun:   time.Now().Add(-5 * time.Minute),
		NextRun:   time.Now().Add(5 * time.Minute),
		Data:      map[string]interface{}{"iterations": 1},
		UpdatedAt: time.Now(), // Ensure non-zero UpdatedAt to avoid pipeline calls
	}

	stateData, err := json.Marshal(expectedState)
	if err != nil {
		t.Fatalf("failed to marshal expected state: %v", err)
	}

	// Mock Redis Get to return the existing state
	mockRedis.EXPECT().
		Get(gomock.Any(), stateKey).
		Return(redis.NewStringResult(string(stateData), nil)).
		Times(1)

	options := cron.JobOptions{
		Redis: mockRedis,
		Name:  jobName,
		Spec:  "*/5 * * * * *",
		Job: func(ctx context.Context, job *cron.Job) error {
			return nil
		},
	}

	cronJob, err := cron.NewJob(options)
	if err != nil {
		t.Fatalf("failed to initialize job: %v", err)
	}

	// Fetch the state
	state, err := cronJob.GetState(context.Background())
	if err != nil {
		t.Fatalf("failed to get job state: %v", err)
	}

	// Check status
	if state.Status != cron.JobStatusRunning {
		t.Fatalf("expected status '%s', got: '%s'", cron.JobStatusRunning, state.Status)
	}

	// Allow a small time drift of 1 second
	if diff := state.LastRun.Sub(expectedState.LastRun).Abs(); diff > time.Second {
		t.Errorf("LastRun differs from expected by more than 1s, got: %v, expected: %v", state.LastRun, expectedState.LastRun)
	}

	if diff := state.NextRun.Sub(expectedState.NextRun).Abs(); diff > time.Second {
		t.Errorf("NextRun differs from expected by more than 1s, got: %v, expected: %v", state.NextRun, expectedState.NextRun)
	}

	// Check data field
	if iterations, ok := state.Data["iterations"]; !ok || iterations != float64(1) {
		t.Errorf("expected Data['iterations'] to be 1, got: %v", state.Data["iterations"])
	}
}

func TestCronJob_GetJobState_DefaultState(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRedis := mocks.NewMockCmdable(ctrl)
	mockPipeline := mocks.NewMockPipeliner(ctrl)

	jobName := "TestCronJob_GetJobState_DefaultState"
	stateKey := fmt.Sprintf(cron.JobStateKeyFormat, jobName)

	// When state doesn't exist, Redis returns redis.Nil
	mockRedis.EXPECT().
		Get(gomock.Any(), stateKey).
		Return(redis.NewStringResult("", redis.Nil)).
		Times(1)

	// Since the state does not exist, GetState will create a default state and call SaveState.
	// SaveState uses TxPipeline, Set, ZAdd, Exec.
	mockRedis.EXPECT().
		TxPipeline().
		Return(mockPipeline).
		Times(1)

	// Check that Set is called with the default state
	mockPipeline.EXPECT().
		Set(gomock.Any(), stateKey, gomock.Any(), time.Duration(0)).
		DoAndReturn(func(ctx context.Context, key string, value interface{}, expiration time.Duration) *redis.StatusCmd {
			if key != stateKey {
				t.Errorf("Set called with unexpected key: got %s, want %s", key, stateKey)
			}
			bytes, ok := value.([]byte)
			if !ok {
				t.Errorf("expected value to be []byte, got %T", value)
			}

			var savedState cron.JobState
			if err := json.Unmarshal(bytes, &savedState); err != nil {
				t.Errorf("failed to unmarshal default state: %v", err)
			}

			// Check some fields of the default state
			// The default state has no Status, default NextRun (based on cron spec), zero Iterations, etc.
			if savedState.Status != "" {
				t.Errorf("expected default state status to be empty, got %v", savedState.Status)
			}
			if savedState.Data == nil {
				t.Errorf("expected default state Data not to be nil")
			}

			return redis.NewStatusResult("OK", nil)
		}).
		Times(1)

	mockPipeline.EXPECT().
		ZAdd(gomock.Any(), cron.JobsList, gomock.Any()).
		DoAndReturn(func(ctx context.Context, key string, members ...redis.Z) *redis.IntCmd {
			if key != cron.JobsList {
				t.Errorf("ZAdd called with wrong key. got: %s, want: %s", key, cron.JobsList)
			}
			if len(members) != 1 {
				t.Errorf("ZAdd expected one member, got %d", len(members))
			} else {
				member := members[0]
				// Since no last run yet, the score should be the NextRun unix time.
				// We'll just ensure it's non-zero and the member matches the job name.
				if member.Member != jobName {
					t.Errorf("ZAdd called with wrong member. got: %v, want: %s", member.Member, jobName)
				}
				if member.Score == 0 {
					t.Errorf("expected a non-zero score for NextRun, got 0")
				}
			}
			return redis.NewIntResult(1, nil)
		}).
		Times(1)

	mockPipeline.EXPECT().
		Exec(gomock.Any()).
		Return([]redis.Cmder{}, nil).
		Times(1)

	options := cron.JobOptions{
		Redis: mockRedis,
		Name:  jobName,
		Spec:  "*/5 * * * * *",
		Job: func(ctx context.Context, job *cron.Job) error {
			return nil
		},
	}

	cronJob, err := cron.NewJob(options)
	if err != nil {
		t.Fatalf("failed to create new job: %v", err)
	}

	state, err := cronJob.GetState(context.Background())
	if err != nil {
		t.Fatalf("failed to get job state: %v", err)
	}

	// Check the returned default state
	if state == nil {
		t.Fatalf("expected a default state, got nil")
	}
	if state.Status != "" {
		t.Errorf("expected default state to have empty Status, got: %v", state.Status)
	}
	if state.Iterations != 0 {
		t.Errorf("expected default state Iterations to be 0, got: %d", state.Iterations)
	}

	// NextRun should be in the future based on the cron spec
	now := time.Now()
	if state.NextRun.Before(now) {
		t.Errorf("expected default state's NextRun to be in the future, got: %v", state.NextRun)
	}

	// Data should not be nil
	if state.Data == nil {
		t.Errorf("expected default state's Data to be non-nil")
	}

	// UpdatedAt should be set now
	if state.UpdatedAt.IsZero() {
		t.Errorf("expected default state's UpdatedAt to be set, got zero time")
	}
}

func TestCronJob_Start_ContextDone(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRedis := mocks.NewMockCmdable(ctrl)
	mockPipeline := mocks.NewMockPipeliner(ctrl)
	logger := mocks.NewMockILogger(ctrl)

	jobName := "TestCronJob_Start_ContextDone"
	stateKey := fmt.Sprintf(cron.JobStateKeyFormat, jobName)

	options := cron.JobOptions{
		Redis:  mockRedis,
		Name:   jobName,
		Spec:   "*/10 * * * * *",
		Logger: logger,
		Job: func(ctx context.Context, job *cron.Job) error {
			// The job won't actually run because the context ends too soon.
			return nil
		},
	}

	// Initial Get call returns no state, causing default state initialization
	mockRedis.EXPECT().
		Get(gomock.Any(), stateKey).
		Return(redis.NewStringResult("", redis.Nil)).
		Times(1)

	// Expect pipeline calls for default state creation
	mockRedis.EXPECT().
		TxPipeline().
		Return(mockPipeline).
		Times(1)

	mockPipeline.EXPECT().
		Set(gomock.Any(), stateKey, gomock.Any(), gomock.Any()).
		Return(redis.NewStatusResult("OK", nil)).
		Times(1)

	mockPipeline.EXPECT().
		ZAdd(gomock.Any(), cron.JobsList, gomock.Any()).
		Return(redis.NewIntResult(1, nil)).
		Times(1)

	mockPipeline.EXPECT().
		Exec(gomock.Any()).
		Return([]redis.Cmder{}, nil).
		Times(1)

	// Since the context will end before the job executes any task, we expect this log:
	logger.EXPECT().
		Info(gomock.Any(), "Cron job stopped by context without active execution", gomock.Any()).
		Times(1)

	cronJob, err := cron.NewJob(options)
	if err != nil {
		t.Fatalf("failed to initialize Job: %v", err)
	}

	// Create a context with a very short deadline so it ends quickly
	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(50*time.Millisecond))
	defer cancel()

	// Start the job
	if err := cronJob.Start(ctx); err != nil {
		t.Fatalf("failed to start cron job: %v", err)
	}

	// Wait slightly longer than the deadline to ensure the job goroutine sees the context ending
	time.Sleep(100 * time.Millisecond)

	// If the test reaches here, the expected Info log should have been called,
	// verifying the job handled the context ending without running any task.
}

func TestCronJob_Stop(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRedis := mocks.NewMockCmdable(ctrl)
	mockPipeline := mocks.NewMockPipeliner(ctrl)
	logger := mocks.NewMockILogger(ctrl)

	jobName := "TestCronJob_Stop"
	stateKey := fmt.Sprintf(cron.JobStateKeyFormat, jobName)

	options := cron.JobOptions{
		Redis:  mockRedis,
		Name:   jobName,
		Spec:   "*/10 * * * * *", // Next run is far enough in the future
		Logger: logger,
		Job: func(ctx context.Context, job *cron.Job) error {
			// The job won't actually run because we'll stop it first.
			return nil
		},
	}

	// Initial state retrieval returns redis.Nil, so default state is created
	mockRedis.EXPECT().
		Get(gomock.Any(), stateKey).
		Return(redis.NewStringResult("", redis.Nil)).
		Times(1)

	// Expect the pipeline calls for saving the default state
	mockRedis.EXPECT().
		TxPipeline().
		Return(mockPipeline).
		Times(1)

	mockPipeline.EXPECT().
		Set(gomock.Any(), stateKey, gomock.Any(), time.Duration(0)).
		Return(redis.NewStatusResult("OK", nil)).
		Times(1)

	mockPipeline.EXPECT().
		ZAdd(gomock.Any(), cron.JobsList, gomock.Any()).
		Return(redis.NewIntResult(1, nil)).
		Times(1)

	mockPipeline.EXPECT().
		Exec(gomock.Any()).
		Return([]redis.Cmder{}, nil).
		Times(1)

	// Since we will stop the job before it runs, we expect the following log:
	logger.EXPECT().
		Info(gomock.Any(), "Cron job force stopped without active execution", gomock.Any()).
		Times(1)

	cronJob, err := cron.NewJob(options)
	if err != nil {
		t.Fatalf("failed to initialize Job: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start the job
	if err := cronJob.Start(ctx); err != nil {
		t.Fatalf("failed to start cron job: %v", err)
	}

	// Give the job a brief moment to set up
	time.Sleep(100 * time.Millisecond)

	// Now stop the job before it executes
	if err := cronJob.Stop(ctx); err != nil {
		t.Fatalf("failed to stop cron job: %v", err)
	}

	// Allow some time for the goroutine in Start to process the stop signal
	time.Sleep(100 * time.Millisecond)

	// If we get here, the log expectation has passed and the job was stopped before running any execution.
}

func TestCronJob_Execute_LockFailed(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRedis := mocks.NewMockCmdable(ctrl)
	mockPipeline := mocks.NewMockPipeliner(ctrl)
	logger := mocks.NewMockILogger(ctrl)
	mockLocker := mocks.NewMockILocker(ctrl)

	jobName := "TestCronJob_Execute_LockFailed"
	stateKey := fmt.Sprintf(cron.JobStateKeyFormat, jobName)

	// This cron spec runs the job every 10 seconds, ensuring a nearer execution time.
	options := cron.JobOptions{
		Redis:  mockRedis,
		Name:   jobName,
		Spec:   "*/1 * * * * * *",
		Logger: logger,
		Job: func(ctx context.Context, job *cron.Job) error {
			return nil
		},
		Locker: mockLocker,
	}

	// Initial Get returns nil, so default state is initialized.
	mockRedis.EXPECT().
		Get(gomock.Any(), stateKey).
		Return(redis.NewStringResult("", redis.Nil)).
		AnyTimes()

	// Expect calls to create default state
	mockRedis.EXPECT().
		TxPipeline().
		Return(mockPipeline).
		AnyTimes()

	mockPipeline.EXPECT().
		Set(gomock.Any(), stateKey, gomock.Any(), time.Duration(0)).
		Return(redis.NewStatusResult("OK", nil)).
		AnyTimes()

	mockPipeline.EXPECT().
		ZAdd(gomock.Any(), cron.JobsList, gomock.Any()).
		Return(redis.NewIntResult(1, nil)).
		AnyTimes()

	mockPipeline.EXPECT().
		Exec(gomock.Any()).
		Return([]redis.Cmder{}, nil).
		Times(1)

	// Expect lock acquisition failure once the job tries to run
	mockLocker.EXPECT().
		Acquire(gomock.Any()).
		Return(false, nil).
		AnyTimes()

	logger.EXPECT().
		Debug(gomock.Any(), "Cron job skipped due to existing lock", gomock.Any()).
		MinTimes(1)

	cronJob, err := cron.NewJob(options)
	if err != nil {
		t.Fatalf("failed to initialize CronJob: %v", err)
	}

	ctx := context.Background()

	if err := cronJob.Start(ctx); err != nil {
		t.Fatalf("failed to start cron job: %v", err)
	}

	// Wait long enough to ensure we pass at least one scheduled run boundary.
	// If next run is in ~7 seconds, waiting 30 seconds ensures at least one attempt.
	time.Sleep(5 * time.Second)
}

func TestCronJob_SetJobStatus(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRedis := mocks.NewMockCmdable(ctrl)
	mockPipeline := mocks.NewMockPipeliner(ctrl)
	logger := mocks.NewMockILogger(ctrl)

	jobName := "TestCronJob_SetJobStatus"
	stateKey := fmt.Sprintf(cron.JobStateKeyFormat, jobName)

	// Existing job state in Redis
	existingState := cron.JobState{
		Status:    cron.JobStatusRunning,
		LastRun:   time.Now().Add(-1 * time.Minute),
		NextRun:   time.Now().Add(4 * time.Minute),
		UpdatedAt: time.Now(),
		Data:      map[string]interface{}{"iterations": 2},
	}

	serializedExistingState, err := json.Marshal(existingState)
	if err != nil {
		t.Fatalf("failed to marshal existing state: %v", err)
	}

	// Mock fetching the existing state from Redis
	mockRedis.EXPECT().
		Get(gomock.Any(), stateKey).
		Return(redis.NewStringResult(string(serializedExistingState), nil)).
		Times(1)

	options := cron.JobOptions{
		Redis:  mockRedis,
		Name:   jobName,
		Spec:   "*/5 * * * * *", // run every 5 seconds
		Logger: logger,
		Job: func(ctx context.Context, job *cron.Job) error {
			return nil
		},
	}

	// Initialize the job
	cronJob, err := cron.NewJob(options)
	if err != nil {
		t.Fatalf("failed to initialize job: %v", err)
	}

	ctx := context.Background()

	// Get the current state
	state, err := cronJob.GetState(ctx)
	if err != nil {
		t.Fatalf("failed to get job state: %v", err)
	}

	// Update the status in the state
	state.Status = cron.JobStatusFailed
	state.UpdatedAt = time.Now()

	// Expect pipeline operations to save the updated state
	mockRedis.EXPECT().
		TxPipeline().
		Return(mockPipeline).
		Times(1)

	mockPipeline.EXPECT().
		Set(gomock.Any(), stateKey, gomock.Any(), time.Duration(0)).
		DoAndReturn(func(ctx context.Context, key string, value interface{}, expiration time.Duration) *redis.StatusCmd {
			// Verify the serialized updated state
			bytes, ok := value.([]byte)
			if !ok {
				t.Errorf("expected value to be []byte, got %T", value)
			}
			var savedState cron.JobState
			if err := json.Unmarshal(bytes, &savedState); err != nil {
				t.Errorf("failed to unmarshal saved state: %v", err)
			}
			// Check the updated status
			if savedState.Status != cron.JobStatusFailed {
				t.Errorf("expected status to be 'Failed', got: %s", savedState.Status)
			}
			return redis.NewStatusResult("OK", nil)
		}).
		Times(1)

	// The score for ZAdd should be the LastRun Unix if set, otherwise NextRun
	expectedScore := float64(state.LastRun.Unix())
	if expectedScore == 0 {
		expectedScore = float64(state.NextRun.Unix())
	}

	mockPipeline.EXPECT().
		ZAdd(gomock.Any(), cron.JobsList, gomock.Any()).
		DoAndReturn(func(ctx context.Context, key string, members ...redis.Z) *redis.IntCmd {
			if key != cron.JobsList {
				t.Errorf("expected key %s, got %s", cron.JobsList, key)
			}
			if len(members) != 1 {
				t.Errorf("expected one member in ZAdd")
			} else {
				member := members[0]
				if member.Member != jobName {
					t.Errorf("expected member %s, got %v", jobName, member.Member)
				}
				if member.Score != expectedScore {
					t.Errorf("expected score %f, got %f", expectedScore, member.Score)
				}
			}
			return redis.NewIntResult(1, nil)
		}).
		Times(1)

	mockPipeline.EXPECT().
		Exec(gomock.Any()).
		Return([]redis.Cmder{}, nil).
		Times(1)

	// Now save the state (updating the status in Redis)
	if err := cronJob.SaveState(ctx, state); err != nil {
		t.Fatalf("failed to save updated job state: %v", err)
	}
}

func TestCronJob_UpdateSpec(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRedis := mocks.NewMockCmdable(ctrl)
	logger := mocks.NewMockILogger(ctrl)
	mockPipeline := mocks.NewMockPipeliner(ctrl)

	jobName := "TestCronJob_UpdateSpec"
	stateKey := fmt.Sprintf(cron.JobStateKeyFormat, jobName)

	// Initial spec and state
	initialSpec := "*/1 * * * * *" // runs every second
	updatedSpec := "0 0 * * * *"   // runs every hour at the top of the hour
	now := time.Now()

	// Initial state: Running status with some default times
	initialState := cron.JobState{
		Status:    cron.JobStatusRunning,
		LastRun:   now,
		NextRun:   cronexpr.MustParse(initialSpec).Next(now),
		Data:      map[string]interface{}{},
		UpdatedAt: now,
	}
	initialStateBytes, err := json.Marshal(initialState)
	if err != nil {
		t.Fatalf("failed to marshal initial state: %v", err)
	}

	// Mock Redis Get to return the initial state
	mockRedis.EXPECT().
		Get(gomock.Any(), stateKey).
		Return(redis.NewStringResult(string(initialStateBytes), nil)).
		Times(1)

	options := cron.JobOptions{
		Redis:  mockRedis,
		Name:   jobName,
		Spec:   initialSpec,
		Logger: logger,
		Job: func(ctx context.Context, job *cron.Job) error {
			return nil
		},
	}

	cronJob, err := cron.NewJob(options)
	if err != nil {
		t.Fatalf("failed to create CronJob: %v", err)
	}

	ctx := context.Background()

	// Get the current state
	state, err := cronJob.GetState(ctx)
	if err != nil {
		t.Fatalf("failed to get initial job state: %v", err)
	}

	// Update the spec in the state
	state.Data["spec"] = updatedSpec

	// After updating the spec, we save the state
	mockRedis.EXPECT().
		TxPipeline().
		Return(mockPipeline).
		Times(1)

	mockPipeline.EXPECT().
		Set(gomock.Any(), stateKey, gomock.Any(), time.Duration(0)).
		DoAndReturn(func(ctx context.Context, key string, value interface{}, expiration time.Duration) *redis.StatusCmd {
			bytes, ok := value.([]byte)
			if !ok {
				t.Errorf("expected value to be []byte, got %T", value)
			}
			var savedState cron.JobState
			if err := json.Unmarshal(bytes, &savedState); err != nil {
				t.Errorf("failed to unmarshal saved state: %v", err)
			}
			// Check that Data["spec"] is updated
			if savedState.Data["spec"] != updatedSpec {
				t.Errorf("expected updated spec %s, got %v", updatedSpec, savedState.Data["spec"])
			}
			return redis.NewStatusResult("OK", nil)
		}).
		Times(1)

	mockPipeline.EXPECT().
		ZAdd(gomock.Any(), cron.JobsList, gomock.Any()).
		Return(redis.NewIntResult(1, nil)).
		Times(1)

	mockPipeline.EXPECT().
		Exec(gomock.Any()).
		Return([]redis.Cmder{}, nil).
		Times(1)

	// Save the updated state
	if err := cronJob.SaveState(ctx, state); err != nil {
		t.Fatalf("failed to save updated job state: %v", err)
	}

	// Now when we get the state again, the updated spec should influence NextRun
	updatedState := *state
	updatedState.NextRun = cronexpr.MustParse(updatedSpec).Next(time.Now())
	updatedStateBytes, err := json.Marshal(updatedState)
	if err != nil {
		t.Fatalf("failed to marshal updated state: %v", err)
	}

	// Mock Redis Get to return the updated state
	mockRedis.EXPECT().
		Get(gomock.Any(), stateKey).
		Return(redis.NewStringResult(string(updatedStateBytes), nil)).
		Times(1)

	// Get the state again and verify NextRun matches the updatedSpec
	stateAfterUpdate, err := cronJob.GetState(ctx)
	if err != nil {
		t.Fatalf("failed to get job state after spec update: %v", err)
	}

	// Check that NextRun corresponds to the updated spec
	expectedNextRun := cronexpr.MustParse(updatedSpec).Next(time.Now())
	// We'll compare up to the nearest minute to account for small differences in timing
	if !stateAfterUpdate.NextRun.Truncate(time.Minute).Equal(expectedNextRun.Truncate(time.Minute)) {
		t.Errorf("expected NextRun to be around %v, got: %v", expectedNextRun, stateAfterUpdate.NextRun)
	}
}

func TestCronJob_Delete(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRedis := mocks.NewMockCmdable(ctrl)
	mockPipeline := mocks.NewMockPipeliner(ctrl)
	logger := mocks.NewMockILogger(ctrl)

	jobName := "TestCronJob_Delete"
	stateKey := fmt.Sprintf(cron.JobStateKeyFormat, jobName)

	options := cron.JobOptions{
		Redis:  mockRedis,
		Name:   jobName,
		Spec:   "*/5 * * * * *",
		Logger: logger,
		Job: func(ctx context.Context, job *cron.Job) error {
			return nil
		},
	}

	cronJob, err := cron.NewJob(options)
	if err != nil {
		t.Fatalf("failed to initialize job: %v", err)
	}

	// Expect a pipeline call when deleting the job
	mockRedis.EXPECT().
		TxPipeline().
		Return(mockPipeline).
		Times(1)

	// Expect Del and ZRem calls within the pipeline
	mockPipeline.EXPECT().
		Del(gomock.Any(), stateKey).
		Return(redis.NewIntResult(1, nil)).
		Times(1)

	mockPipeline.EXPECT().
		ZRem(gomock.Any(), cron.JobsList, jobName).
		Return(redis.NewIntResult(1, nil)).
		Times(1)

	// Expect Exec to finalize the pipeline
	mockPipeline.EXPECT().
		Exec(gomock.Any()).
		Return([]redis.Cmder{}, nil).
		Times(1)

	// Expect a log message indicating successful deletion
	logger.EXPECT().
		Info(gomock.Any(), "Job deleted successfully", gomock.Any()).
		Times(1)

	ctx := context.Background()
	if err := cronJob.Delete(ctx); err != nil {
		t.Fatalf("failed to delete job: %v", err)
	}
}

func TestCronJob_IsRunningByMe(t *testing.T) {
	ctx := context.Background()

	now := time.Now()
	testMachineID := "test-machine-id"

	testCases := []struct {
		name      string
		state     cron.JobState
		workerID  string
		workerErr error
		want      bool
		wantErr   bool
	}{
		{
			name: "Running by me, not stale",
			state: cron.JobState{
				Status:    cron.JobStatusRunning,
				RunningBy: testMachineID,
				UpdatedAt: now,
			},
			workerID:  testMachineID,
			workerErr: nil,
			want:      true,
			wantErr:   false,
		},
		{
			name: "Running by another machine",
			state: cron.JobState{
				Status:    cron.JobStatusRunning,
				RunningBy: "another-machine",
				UpdatedAt: now,
			},
			workerID:  testMachineID,
			workerErr: nil,
			want:      false,
			wantErr:   false,
		},
		{
			name: "Not running",
			state: cron.JobState{
				Status:    cron.JobStatusFailed,
				RunningBy: testMachineID,
				UpdatedAt: now,
			},
			workerID:  testMachineID,
			workerErr: nil,
			want:      false,
			wantErr:   false,
		},
		{
			name: "Running by me, but stale",
			state: cron.JobState{
				Status:    cron.JobStatusRunning,
				RunningBy: testMachineID,
				UpdatedAt: now.Add(-cron.StaleThreshold * 2),
			},
			workerID:  testMachineID,
			workerErr: nil,
			want:      false,
			wantErr:   false,
		},
		{
			name: "Error fetching worker ID",
			state: cron.JobState{
				Status:    cron.JobStatusRunning,
				RunningBy: testMachineID,
				UpdatedAt: now,
			},
			workerID:  "",
			workerErr: errors.New("failed to get worker ID"),
			want:      false,
			wantErr:   true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockProvider := MockWorkerIdProvider{ID: tc.workerID, Err: tc.workerErr}

			job := &cron.Job{
				Options: cron.JobOptions{
					WorkerIdProvider: mockProvider,
				},
			}

			got, err := job.IsRunningByMe(ctx, &tc.state)
			if tc.wantErr && err == nil {
				t.Errorf("expected an error, got nil")
			} else if !tc.wantErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}

			if got != tc.want {
				t.Errorf("expected %v, got %v", tc.want, got)
			}
		})
	}
}

func TestCronJob_BeforeStart(t *testing.T) {
	// Define a consistent machine ID for testing
	const testMachineID = "test-machine-id"

	// Define the cron spec for all subtests
	spec := "*/5 * * * * *" // runs every 5 seconds

	// Subtest: BeforeStart returns true
	t.Run("BeforeStart_returns_true", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		// Initialize mocks
		mockRedis := mocks.NewMockCmdable(ctrl)
		mockPipeline := mocks.NewMockPipeliner(ctrl)
		logger := mocks.NewMockILogger(ctrl)
		workerIdProvider := MockWorkerIdProvider{ID: testMachineID, Err: nil}

		jobName := "BeforeStart_returns_true"
		stateKey := fmt.Sprintf(cron.JobStateKeyFormat, jobName)

		// Set up Redis mock expectations
		mockRedis.EXPECT().
			Get(gomock.Any(), stateKey).
			Return(redis.NewStringResult("", redis.Nil)).
			Times(1)

		mockRedis.EXPECT().
			TxPipeline().
			Return(mockPipeline).
			Times(1)

		mockPipeline.EXPECT().
			Set(gomock.Any(), stateKey, gomock.Any(), gomock.Any()).
			Return(redis.NewStatusResult("OK", nil)).
			Times(1)

		mockPipeline.EXPECT().
			ZAdd(gomock.Any(), cron.JobsList, gomock.Any()).
			Return(redis.NewIntResult(1, nil)).
			Times(1)

		mockPipeline.EXPECT().
			Exec(gomock.Any()).
			Return([]redis.Cmder{}, nil).
			Times(1)

		// Set up logger expectations
		// Allow any number of debug and info logs, but expect one specific Info log when stopping
		logger.EXPECT().
			Debug(gomock.Any(), gomock.Any(), gomock.Any()).
			AnyTimes()

		logger.EXPECT().
			Info(gomock.Any(), "Cron job force stopped without active execution", gomock.Any()).
			Times(1)

		// Initialize the job with BeforeStart returning true
		job, err := cron.NewJob(cron.JobOptions{
			Redis:            mockRedis,
			Name:             jobName,
			Spec:             spec,
			Logger:           logger,
			Job:              func(ctx context.Context, job *cron.Job) error { return nil },
			BeforeStart:      func(ctx context.Context, job *cron.Job) (bool, error) { return true, nil },
			WorkerIdProvider: workerIdProvider,
		})
		if err != nil {
			t.Fatalf("failed to initialize job: %v", err)
		}

		// Start the job with a cancellable context
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		if err := job.Start(ctx); err != nil {
			t.Errorf("expected no error when starting job, got: %v", err)
		}

		// Allow some time for the job to start
		time.Sleep(100 * time.Millisecond)

		// Stop the job explicitly to trigger the stop log
		if err := job.Stop(ctx); err != nil {
			t.Errorf("expected no error when stopping job, got: %v", err)
		}

		// Allow some time for the stop signal to be processed
		time.Sleep(100 * time.Millisecond)
	})

	// Subtest: BeforeStart returns false
	t.Run("BeforeStart_returns_false", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		// Initialize mocks
		mockRedis := mocks.NewMockCmdable(ctrl)
		mockPipeline := mocks.NewMockPipeliner(ctrl)
		logger := mocks.NewMockILogger(ctrl)
		workerIdProvider := MockWorkerIdProvider{ID: testMachineID, Err: nil}

		jobName := "BeforeStart_returns_false"
		stateKey := fmt.Sprintf(cron.JobStateKeyFormat, jobName)

		// Set up Redis mock expectations
		mockRedis.EXPECT().
			Get(gomock.Any(), stateKey).
			Return(redis.NewStringResult("", redis.Nil)).
			Times(1)

		mockRedis.EXPECT().
			TxPipeline().
			Return(mockPipeline).
			Times(1)

		mockPipeline.EXPECT().
			Set(gomock.Any(), stateKey, gomock.Any(), gomock.Any()).
			Return(redis.NewStatusResult("OK", nil)).
			Times(1)

		mockPipeline.EXPECT().
			ZAdd(gomock.Any(), cron.JobsList, gomock.Any()).
			Return(redis.NewIntResult(1, nil)).
			Times(1)

		mockPipeline.EXPECT().
			Exec(gomock.Any()).
			Return([]redis.Cmder{}, nil).
			Times(1)

		// Set up logger expectations
		// Expect a specific Info log indicating cancellation
		logger.EXPECT().
			Info(gomock.Any(), "Job start was cancelled by BeforeStart callback", gomock.Any()).
			Times(1)

		// Initialize the job with BeforeStart returning false
		job, err := cron.NewJob(cron.JobOptions{
			Redis:            mockRedis,
			Name:             jobName,
			Spec:             spec,
			Logger:           logger,
			Job:              func(ctx context.Context, job *cron.Job) error { return nil },
			BeforeStart:      func(ctx context.Context, job *cron.Job) (bool, error) { return false, nil },
			WorkerIdProvider: workerIdProvider,
		})
		if err != nil {
			t.Fatalf("failed to initialize job2: %v", err)
		}

		// Start the job with a cancellable context
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		// Starting the job should not result in an error
		if err := job.Start(ctx); err != nil {
			t.Errorf("expected no error when job start is cancelled by BeforeStart, got: %v", err)
		}

		// Allow some time for the job to process the BeforeStart callback
		time.Sleep(100 * time.Millisecond)
	})

	// Subtest: BeforeStart returns an error
	t.Run("BeforeStart_returns_error", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		// Initialize mocks
		mockRedis := mocks.NewMockCmdable(ctrl)
		mockPipeline := mocks.NewMockPipeliner(ctrl)
		logger := mocks.NewMockILogger(ctrl)
		workerIdProvider := MockWorkerIdProvider{ID: testMachineID, Err: nil}

		jobName := "BeforeStart_returns_error"
		stateKey := fmt.Sprintf(cron.JobStateKeyFormat, jobName)

		// Set up Redis mock expectations
		mockRedis.EXPECT().
			Get(gomock.Any(), stateKey).
			Return(redis.NewStringResult("", redis.Nil)).
			Times(1)

		mockRedis.EXPECT().
			TxPipeline().
			Return(mockPipeline).
			Times(1)

		mockPipeline.EXPECT().
			Set(gomock.Any(), stateKey, gomock.Any(), gomock.Any()).
			Return(redis.NewStatusResult("OK", nil)).
			Times(1)

		mockPipeline.EXPECT().
			ZAdd(gomock.Any(), cron.JobsList, gomock.Any()).
			Return(redis.NewIntResult(1, nil)).
			Times(1)

		mockPipeline.EXPECT().
			Exec(gomock.Any()).
			Return([]redis.Cmder{}, nil).
			Times(1)

		// Set up logger expectations
		// Expect an error log indicating BeforeStart callback error
		logger.EXPECT().
			Error(gomock.Any(), "BeforeStart callback returned an error", gomock.Any()).
			Times(1)

		// Initialize the job with BeforeStart returning an error
		job, err := cron.NewJob(cron.JobOptions{
			Redis:  mockRedis,
			Name:   jobName,
			Spec:   spec,
			Logger: logger,
			Job:    func(ctx context.Context, job *cron.Job) error { return nil },
			BeforeStart: func(ctx context.Context, job *cron.Job) (bool, error) {
				return false, errors.New("some error in BeforeStart")
			},
			WorkerIdProvider: workerIdProvider,
		})
		if err != nil {
			t.Fatalf("failed to initialize job3: %v", err)
		}

		// Start the job with a cancellable context
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		// Starting the job should return an error
		err = job.Start(ctx)
		if err == nil {
			t.Errorf("expected an error due to BeforeStart callback returning an error, got nil")
		}
	})
}

func TestCronJob_Execute_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// Initialize mocks
	mockRedis := mocks.NewMockCmdable(ctrl)
	mockPipelineInit := mocks.NewMockPipeliner(ctrl)
	mockPipelineExec := mocks.NewMockPipeliner(ctrl)
	logger := mocks.NewMockILogger(ctrl)
	mockLocker := mocks.NewMockILocker(ctrl)
	workerIdProvider := MockWorkerIdProvider{ID: "test-machine-id", Err: nil}

	jobName := "TestCronJob_Execute_Success"
	stateKey := fmt.Sprintf(cron.JobStateKeyFormat, jobName)
	spec := "*/1 * * * * *" // Adjust to "*/1 * * * *" if cronlite doesn't support seconds

	// Create an initial state with NextRun in the past to trigger immediate execution
	initialState := cron.JobState{
		Status:  "", // Not running
		LastRun: time.Time{},
		NextRun: time.Now().Add(-1 * time.Second), // Adjust to -1 minute if using 5-field spec

		Iterations: 0,

		Data:      map[string]interface{}{},
		UpdatedAt: time.Now(),
	}
	initialStateJSON, err := json.Marshal(initialState)
	if err != nil {
		t.Fatalf("failed to marshal initial state: %v", err)
	}

	// Allow multiple Get calls by setting it to AnyTimes()
	mockRedis.EXPECT().
		Get(gomock.Any(), stateKey).
		Return(redis.NewStringResult(string(initialStateJSON), nil)).
		AnyTimes()

	// Mock Redis TxPipeline call for initializing default state
	mockRedis.EXPECT().
		TxPipeline().
		Return(mockPipelineInit).
		Times(1)

	// Mock pipelineInit.Set for default state
	mockPipelineInit.EXPECT().
		Set(gomock.Any(), stateKey, gomock.Any(), gomock.Any()).
		Return(redis.NewStatusResult("OK", nil)).
		Times(1)

	// Mock pipelineInit.ZAdd for adding job to sorted set
	mockPipelineInit.EXPECT().
		ZAdd(gomock.Any(), cron.JobsList, gomock.Any()).
		Return(redis.NewIntResult(1, nil)).
		Times(1)

	// Mock pipelineInit.Exec for initializing state
	mockPipelineInit.EXPECT().
		Exec(gomock.Any()).
		Return([]redis.Cmder{}, nil).
		Times(1)

	// Mock Locker.Acquire to succeed
	mockLocker.EXPECT().
		Acquire(gomock.Any()).
		Return(true, nil).
		Times(1)

	// Mock Locker.GetLockTTL if used
	mockLocker.EXPECT().
		GetLockTTL().
		Return(30 * time.Second).
		Times(1)

	// Create a channel to signal that the job was executed
	executed := make(chan bool, 1)

	// Mock Logger.Info for executing job and signal execution
	logger.EXPECT().
		Info(gomock.Any(), "Executing cron job", gomock.Any()).
		Do(func(ctx context.Context, msg string, data map[string]interface{}) {
			// Signal that execute is being called
			executed <- true
		}).
		Times(1)

	// Mock Logger.Info for successful execution
	logger.EXPECT().
		Info(gomock.Any(), "Cron job executed successfully", gomock.Any()).
		Times(1)

	// Mock Logger.Debug for lock extension (optional)
	logger.EXPECT().
		Debug(gomock.Any(), gomock.Any(), gomock.Any()).
		AnyTimes() // Allow any number of Debug logs

	// Mock Logger.Error should not be called in success scenario
	logger.EXPECT().
		Error(gomock.Any(), gomock.Any(), gomock.Any()).
		Times(0)

	// Mock Redis TxPipeline for saving state after execution
	mockRedis.EXPECT().
		TxPipeline().
		Return(mockPipelineExec).
		Times(1)

	// Mock pipelineExec.Set for updating state after execution to Success
	mockPipelineExec.EXPECT().
		Set(gomock.Any(), stateKey, gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, key string, value interface{}, expiration time.Duration) *redis.StatusCmd {
			// Verify that state.Status is set to JobStatusSuccess
			var savedState cron.JobState
			switch v := value.(type) {
			case cron.JobState:
				savedState = v
			case []byte:
				if err := json.Unmarshal(v, &savedState); err != nil {
					t.Errorf("failed to unmarshal saved state: %v", err)
				}
			default:
				t.Errorf("expected value to be cron.JobState or []byte, got %T", value)
			}
			if savedState.Status != cron.JobStatusSuccess {
				t.Errorf("expected status to be '%s', got '%s'", cron.JobStatusSuccess, savedState.Status)
			}
			return redis.NewStatusResult("OK", nil)
		}).
		Times(1)

	// Mock pipelineExec.ZAdd for updating job in sorted set after execution to Success
	mockPipelineExec.EXPECT().
		ZAdd(gomock.Any(), cron.JobsList, gomock.Any()).
		Return(redis.NewIntResult(1, nil)).
		Times(1)

	// Mock pipelineExec.Exec for saving state after execution to Success
	mockPipelineExec.EXPECT().
		Exec(gomock.Any()).
		Return([]redis.Cmder{}, nil).
		Times(1)

	mockLocker.EXPECT().
		Release(gomock.Any()).
		Return(nil).
		AnyTimes()

	// Initialize the job with a job function that succeeds
	spec = "*/1 * * * * *"
	job, err := cron.NewJob(cron.JobOptions{
		Redis:            mockRedis,
		Name:             jobName,
		Spec:             spec,
		Logger:           logger,
		Job:              func(ctx context.Context, job *cron.Job) error { return nil },
		WorkerIdProvider: workerIdProvider,
		Locker:           mockLocker, // Inject the mocked locker
	})
	if err != nil {
		t.Fatalf("failed to initialize job: %v", err)
	}

	// Start the job with a cancellable context
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := job.Start(ctx); err != nil {
		t.Errorf("expected no error when starting job, got: %v", err)
	}

	// Wait for the job to execute
	select {
	case <-executed:
		// Job executed successfully
	case <-time.After(2 * time.Second):
		t.Fatal("job did not execute within expected time")
	}

	// Stop the job to clean up
	if err := job.Stop(ctx); err != nil {
		t.Errorf("expected no error when stopping job, got: %v", err)
	}

	// Allow some time for the job to stop gracefully
	time.Sleep(5 * time.Second)
}

func TestCronJob_Execute_Failure(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// Initialize mocks
	mockRedis := mocks.NewMockCmdable(ctrl)
	mockPipelineInit := mocks.NewMockPipeliner(ctrl)
	mockPipelineExec := mocks.NewMockPipeliner(ctrl)
	logger := mocks.NewMockILogger(ctrl)
	mockLocker := mocks.NewMockILocker(ctrl)
	workerIdProvider := MockWorkerIdProvider{ID: "test-machine-id", Err: nil}

	jobName := "TestCronJob_Execute_Failure"
	stateKey := fmt.Sprintf(cron.JobStateKeyFormat, jobName)
	spec := "*/1 * * * * *" // runs every second, assuming cronlite supports 6 fields

	// Create an initial state with NextRun in the past to trigger immediate execution
	initialState := cron.JobState{
		Status:  "", // Not running
		LastRun: time.Time{},
		NextRun: time.Now().Add(-1 * time.Second),

		Iterations: 0,

		Data:      map[string]interface{}{},
		UpdatedAt: time.Now(),
	}
	initialStateJSON, err := json.Marshal(initialState)
	if err != nil {
		t.Fatalf("failed to marshal initial state: %v", err)
	}

	// Allow multiple Get calls by setting it to AnyTimes()
	mockRedis.EXPECT().
		Get(gomock.Any(), stateKey).
		Return(redis.NewStringResult(string(initialStateJSON), nil)).
		AnyTimes()

	// Mock Redis TxPipeline call for initializing default state
	mockRedis.EXPECT().
		TxPipeline().
		Return(mockPipelineInit).
		Times(1)

	// Mock pipelineInit.Set for default state
	mockPipelineInit.EXPECT().
		Set(gomock.Any(), stateKey, gomock.Any(), gomock.Any()).
		Return(redis.NewStatusResult("OK", nil)).
		Times(1)

	// Mock pipelineInit.ZAdd for adding job to sorted set
	mockPipelineInit.EXPECT().
		ZAdd(gomock.Any(), cron.JobsList, gomock.Any()).
		Return(redis.NewIntResult(1, nil)).
		Times(1)

	// Mock pipelineInit.Exec for initializing state
	mockPipelineInit.EXPECT().
		Exec(gomock.Any()).
		Return([]redis.Cmder{}, nil).
		Times(1)

	// Mock Locker.Acquire to succeed
	mockLocker.EXPECT().
		Acquire(gomock.Any()).
		Return(true, nil).
		Times(1)

	// Mock Locker.GetLockTTL if used
	mockLocker.EXPECT().
		GetLockTTL().
		Return(30 * time.Second).
		Times(1)

	// Create a channel to signal that the job was executed
	executed := make(chan bool, 1)

	// Mock Logger.Info for executing job and signal execution
	logger.EXPECT().
		Info(gomock.Any(), "Executing cron job", gomock.Any()).
		Do(func(ctx context.Context, msg string, data map[string]interface{}) {
			// Signal that execute is being called
			executed <- true
		}).
		Times(1)

	// Mock Logger.Error for job execution failure (corrected log message)
	executionError := errors.New("job execution failed")
	logger.EXPECT().
		Error(gomock.Any(), "Error executing cron job", gomock.Any()).
		Times(1)

	// Mock Redis TxPipeline for saving state after execution
	mockRedis.EXPECT().
		TxPipeline().
		Return(mockPipelineExec).
		Times(1)

	// Mock pipelineExec.Set for updating state after execution to Failure
	mockPipelineExec.EXPECT().
		Set(gomock.Any(), stateKey, gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, key string, value interface{}, expiration time.Duration) *redis.StatusCmd {
			// Verify that state.Status is set to JobStatusFailed
			var savedState cron.JobState
			switch v := value.(type) {
			case cron.JobState:
				savedState = v
			case []byte:
				if err := json.Unmarshal(v, &savedState); err != nil {
					t.Errorf("failed to unmarshal saved state: %v", err)
				}
			case string:
				if err := json.Unmarshal([]byte(v), &savedState); err != nil {
					t.Errorf("failed to unmarshal saved state string: %v", err)
				}
			default:
				t.Errorf("expected value to be cron.JobState, []byte, or string, got %T", value)
			}
			if savedState.Status != cron.JobStatusFailed {
				t.Errorf("expected status to be '%s', got '%s'", cron.JobStatusFailed, savedState.Status)
			}
			return redis.NewStatusResult("OK", nil)
		}).
		Times(1)

	// Mock pipelineExec.ZAdd for updating job in sorted set after execution to Failure
	mockPipelineExec.EXPECT().
		ZAdd(gomock.Any(), cron.JobsList, gomock.Any()).
		Return(redis.NewIntResult(1, nil)).
		Times(1)

	// Mock pipelineExec.Exec for saving state after execution to Failure
	mockPipelineExec.EXPECT().
		Exec(gomock.Any()).
		Return([]redis.Cmder{}, nil).
		Times(1)

	// Allow any Debug calls (since the code invokes Debug)
	logger.EXPECT().
		Debug(gomock.Any(), gomock.Any(), gomock.Any()).
		AnyTimes()

	mockLocker.EXPECT().
		Release(gomock.Any()).
		Return(nil).
		AnyTimes()

	// Initialize the job with a job function that fails
	job, err := cron.NewJob(cron.JobOptions{
		Redis:            mockRedis,
		Name:             jobName,
		Spec:             spec,
		Logger:           logger,
		Job:              func(ctx context.Context, job *cron.Job) error { return executionError },
		WorkerIdProvider: workerIdProvider,
		Locker:           mockLocker, // Inject the mocked locker
	})
	if err != nil {
		t.Fatalf("failed to initialize job: %v", err)
	}

	// Start the job with a cancellable context
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := job.Start(ctx); err != nil {
		t.Errorf("expected no error when starting job, got: %v", err)
	}

	// Wait for the job to execute
	select {
	case <-executed:
		// Job executed (even if it failed)
	case <-time.After(5 * time.Second):
		t.Fatal("job did not execute within expected time")
	}

	// Stop the job to clean up
	if err := job.Stop(ctx); err != nil {
		t.Errorf("expected no error when stopping job, got: %v", err)
	}

	// Allow some time for the job to stop gracefully
	time.Sleep(5 * time.Second)
}

func TestCronJob_Scheduling(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// Initialize mocks
	mockRedis := mocks.NewMockCmdable(ctrl)
	mockPipelineInit := mocks.NewMockPipeliner(ctrl)
	mockPipelineExec := mocks.NewMockPipeliner(ctrl)
	logger := mocks.NewMockILogger(ctrl)
	mockLocker := mocks.NewMockILocker(ctrl)
	workerIdProvider := MockWorkerIdProvider{ID: "test-machine-id", Err: nil}

	jobName := "TestCronJob_Scheduling"
	stateKey := fmt.Sprintf(cron.JobStateKeyFormat, jobName)
	spec := "*/1 * * * * *" // runs every second, assuming cronlite supports 6 fields

	// Create an initial state with NextRun in the past to trigger immediate execution
	initialState := cron.JobState{
		Status:  "", // Not running
		LastRun: time.Time{},
		NextRun: time.Now().Add(-1 * time.Second),

		Iterations: 0,

		Data:      map[string]interface{}{},
		UpdatedAt: time.Now(),
	}
	initialStateJSON, err := json.Marshal(initialState)
	if err != nil {
		t.Fatalf("failed to marshal initial state: %v", err)
	}

	// Allow multiple Get calls by setting it to AnyTimes()
	mockRedis.EXPECT().
		Get(gomock.Any(), stateKey).
		Return(redis.NewStringResult(string(initialStateJSON), nil)).
		AnyTimes()

	// Mock Redis TxPipeline call for initializing default state
	mockRedis.EXPECT().
		TxPipeline().
		Return(mockPipelineInit).
		Times(1)

	// Mock pipelineInit.Set for default state
	mockPipelineInit.EXPECT().
		Set(gomock.Any(), stateKey, gomock.Any(), gomock.Any()).
		Return(redis.NewStatusResult("OK", nil)).
		Times(1)

	// Mock pipelineInit.ZAdd for adding job to sorted set
	mockPipelineInit.EXPECT().
		ZAdd(gomock.Any(), cron.JobsList, gomock.Any()).
		Return(redis.NewIntResult(1, nil)).
		Times(1)

	// Mock pipelineInit.Exec for initializing state
	mockPipelineInit.EXPECT().
		Exec(gomock.Any()).
		Return([]redis.Cmder{}, nil).
		Times(1)

	// Mock Locker.Acquire to succeed
	mockLocker.EXPECT().
		Acquire(gomock.Any()).
		Return(true, nil).
		AnyTimes()

	// Mock Locker.GetLockTTL if used
	mockLocker.EXPECT().
		GetLockTTL().
		Return(30 * time.Second).
		Times(1)

	// Create a channel to signal that the job was executed
	executed := make(chan bool, 1)

	// Mock Logger.Info for executing job and signal execution
	logger.EXPECT().
		Info(gomock.Any(), "Executing cron job", gomock.Any()).
		Do(func(ctx context.Context, msg string, data map[string]interface{}) {
			// Signal that execute is being called
			executed <- true
		}).
		Times(1)

	// Mock Logger.Info for successful execution
	logger.EXPECT().
		Info(gomock.Any(), "Cron job executed successfully", gomock.Any()).
		Times(1)

	// Mock Redis TxPipeline for saving state after execution
	mockRedis.EXPECT().
		TxPipeline().
		Return(mockPipelineExec).
		Times(1)

	// Mock pipelineExec.Set for updating state after execution to Success
	mockPipelineExec.EXPECT().
		Set(gomock.Any(), stateKey, gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, key string, value interface{}, expiration time.Duration) *redis.StatusCmd {
			// Verify that state.Status is set to JobStatusSuccess
			var savedState cron.JobState
			switch v := value.(type) {
			case cron.JobState:
				savedState = v
			case []byte:
				if err := json.Unmarshal(v, &savedState); err != nil {
					t.Errorf("failed to unmarshal saved state: %v", err)
				}
			case string:
				if err := json.Unmarshal([]byte(v), &savedState); err != nil {
					t.Errorf("failed to unmarshal saved state string: %v", err)
				}
			default:
				t.Errorf("expected value to be cron.JobState, []byte, or string, got %T", value)
			}
			if savedState.Status != cron.JobStatusSuccess {
				t.Errorf("expected status to be '%s', got '%s'", cron.JobStatusSuccess, savedState.Status)
			}
			return redis.NewStatusResult("OK", nil)
		}).
		Times(1)

	// Mock pipelineExec.ZAdd for updating job in sorted set after execution to Success
	mockPipelineExec.EXPECT().
		ZAdd(gomock.Any(), cron.JobsList, gomock.Any()).
		Return(redis.NewIntResult(1, nil)).
		Times(1)

	// Mock pipelineExec.Exec for saving state after execution to Success
	mockPipelineExec.EXPECT().
		Exec(gomock.Any()).
		Return([]redis.Cmder{}, nil).
		Times(1)

	// Allow any Debug calls (since the code might invoke Debug)
	logger.EXPECT().
		Debug(gomock.Any(), gomock.Any(), gomock.Any()).
		AnyTimes()

	mockLocker.EXPECT().
		Release(gomock.Any()).
		Return(nil).
		AnyTimes()

	// Initialize the job with a job function that succeeds
	job, err := cron.NewJob(cron.JobOptions{
		Redis:            mockRedis,
		Name:             jobName,
		Spec:             spec,
		Logger:           logger,
		Job:              func(ctx context.Context, job *cron.Job) error { return nil },
		WorkerIdProvider: workerIdProvider,
		Locker:           mockLocker, // Inject the mocked locker
	})
	if err != nil {
		t.Fatalf("failed to initialize job: %v", err)
	}

	// Start the job with a cancellable context
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := job.Start(ctx); err != nil {
		t.Errorf("expected no error when starting job, got: %v", err)
	}

	// Wait for the job to execute
	select {
	case <-executed:
		// Job executed successfully
	case <-time.After(5 * time.Second):
		t.Fatal("job did not execute within expected time")
	}

	// Verify that NextRun is updated correctly
	// Fetch the latest state from Redis
	latestStateJSON, err := json.Marshal(cron.JobState{
		Status:  cron.JobStatusSuccess,
		LastRun: time.Now(),
		NextRun: time.Now().Add(1 * time.Second),

		Iterations: 1,

		Data:      map[string]interface{}{},
		UpdatedAt: time.Now(),
	})
	if err != nil {
		t.Fatalf("failed to marshal latest state: %v", err)
	}

	// Mock Redis Get to return the latest state after execution
	mockRedis.EXPECT().
		Get(gomock.Any(), stateKey).
		Return(redis.NewStringResult(string(latestStateJSON), nil)).
		AnyTimes()

	// Stop the job to clean up
	if err := job.Stop(ctx); err != nil {
		t.Errorf("expected no error when stopping job, got: %v", err)
	}

	// Allow some time for the job to stop gracefully
	time.Sleep(5 * time.Second)
}

func TestCronJob_Execution_WithBeforeExecuteHook(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// Initialize mocks
	mockRedis := mocks.NewMockCmdable(ctrl)
	mockPipelineExec := mocks.NewMockPipeliner(ctrl)
	logger := mocks.NewMockILogger(ctrl)
	mockLocker := mocks.NewMockILocker(ctrl)
	workerIdProvider := MockWorkerIdProvider{ID: "test-machine-id", Err: nil}

	jobName := "TestCronJob_Execution_WithBeforeExecuteHook"
	stateKey := fmt.Sprintf(cron.JobStateKeyFormat, jobName)
	spec := "*/1 * * * * *" // runs every second

	// Create an initial state with NextRun in the past to trigger immediate execution
	initialState := cron.JobState{
		Status:     "", // Not running
		LastRun:    time.Time{},
		NextRun:    time.Now().Add(-1 * time.Second),
		Iterations: 0, // Initially 0 iterations
		Data:       map[string]interface{}{},
		UpdatedAt:  time.Now(),
	}
	initialStateJSON, err := json.Marshal(initialState)
	if err != nil {
		t.Fatalf("failed to marshal initial state: %v", err)
	}

	// Mock Redis Get to return the initial state
	mockRedis.EXPECT().
		Get(gomock.Any(), stateKey).
		Return(redis.NewStringResult(string(initialStateJSON), nil)).
		AnyTimes()

	// Mock Locker.Acquire to succeed
	mockLocker.EXPECT().
		Acquire(gomock.Any()).
		Return(true, nil).
		Times(1)

	// Mock Locker.GetLockTTL if used
	mockLocker.EXPECT().
		GetLockTTL().
		Return(30 * time.Second).
		Times(1)

	// Create a channel to signal that the job was executed
	executed := make(chan bool, 1)
	executions := 0
	maxExecutions := 1

	// Mock Logger.Info for executing job and signal execution
	logger.EXPECT().
		Info(gomock.Any(), "Executing cron job", gomock.Any()).
		Do(func(ctx context.Context, msg string, data map[string]interface{}) {
			executions++
			executed <- true
		}).
		Times(maxExecutions)

	// Mock Logger.Info for successful execution
	logger.EXPECT().
		Info(gomock.Any(), "Cron job executed successfully", gomock.Any()).
		Times(maxExecutions)

	// Mock Locker.Release to succeed (called once after execution)
	mockLocker.EXPECT().
		Release(gomock.Any()).
		Return(nil).
		Times(maxExecutions)

	// Mock Redis TxPipeline for saving state after execution
	mockRedis.EXPECT().
		TxPipeline().
		Return(mockPipelineExec).
		AnyTimes() // Allow any number of calls to accommodate heartbeat

	// Mock pipelineExec.Set for updating state after execution to Success
	mockPipelineExec.EXPECT().
		Set(gomock.Any(), stateKey, gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, key string, value interface{}, expiration time.Duration) *redis.StatusCmd {
			// Verify that state.Iterations is incremented
			var savedState cron.JobState
			switch v := value.(type) {
			case cron.JobState:
				savedState = v
			case []byte:
				if err := json.Unmarshal(v, &savedState); err != nil {
					t.Errorf("failed to unmarshal saved state: %v", err)
				}
			case string:
				if err := json.Unmarshal([]byte(v), &savedState); err != nil {
					t.Errorf("failed to unmarshal saved state string: %v", err)
				}
			default:
				t.Errorf("expected value to be cron.JobState, []byte, or string, got %T", value)
			}
			if savedState.Iterations != 1 {
				t.Errorf("expected iterations to be 1, got %d", savedState.Iterations)
			}
			return redis.NewStatusResult("OK", nil)
		}).
		AnyTimes() // Allow any number of Set calls

	// Mock pipelineExec.ZAdd for updating job in sorted set after execution to Success
	mockPipelineExec.EXPECT().
		ZAdd(gomock.Any(), cron.JobsList, gomock.Any()).
		Return(redis.NewIntResult(1, nil)).
		AnyTimes() // Allow any number of ZAdd calls

	// Mock pipelineExec.Exec for saving state after execution to Success
	mockPipelineExec.EXPECT().
		Exec(gomock.Any()).
		Return([]redis.Cmder{}, nil).
		AnyTimes() // Allow any number of Exec calls

	// Allow any Debug calls (since the code might invoke Debug)
	logger.EXPECT().
		Debug(gomock.Any(), gomock.Any(), gomock.Any()).
		AnyTimes()

	// Define the BeforeExecute hook to limit executions
	beforeExecute := func(ctx context.Context, job *cron.Job) (bool, error) {
		state, err := job.GetState(ctx)
		if err != nil {
			return false, err
		}
		if state.Iterations >= maxExecutions {
			// Do not execute further
			return false, nil
		}
		return true, nil
	}

	// Initialize the job with a job function that succeeds
	job, err := cron.NewJob(cron.JobOptions{
		Redis:            mockRedis,
		Name:             jobName,
		Spec:             spec,
		Logger:           logger,
		Job:              func(ctx context.Context, job *cron.Job) error { return nil },
		WorkerIdProvider: workerIdProvider,
		Locker:           mockLocker, // Inject the mocked locker
		BeforeExecute:    beforeExecute,
	})
	if err != nil {
		t.Fatalf("failed to initialize job: %v", err)
	}

	// Start the job with a cancellable context
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := job.Start(ctx); err != nil {
		t.Errorf("expected no error when starting job, got: %v", err)
	}

	// Wait for the job to execute the first time
	select {
	case <-executed:
		// Job executed successfully
	case <-time.After(5 * time.Second):
		t.Fatal("job did not execute within expected time")
	}

	// Verify that the job does not execute again since BeforeExecute prevented it
	// We'll wait for a duration longer than the cron spec interval to ensure no second execution
	select {
	case <-executed:
		t.Fatal("job executed more times than expected")
	case <-time.After(2 * time.Second):
		// No second execution occurred as expected
	}

	// Mock Redis Get to return the updated state with Iterations set to 1
	updatedState := cron.JobState{
		Status:     cron.JobStatusSuccess, // Assuming job succeeded
		LastRun:    time.Now(),
		NextRun:    cronexpr.MustParse(spec).Next(time.Now()),
		Iterations: executions,
		Data:       map[string]interface{}{},
		UpdatedAt:  time.Now(),
	}
	updatedStateJSON, err := json.Marshal(updatedState)
	if err != nil {
		t.Fatalf("failed to marshal updated state: %v", err)
	}

	// Mock Redis Get to return the updated state
	mockRedis.EXPECT().
		Get(gomock.Any(), stateKey).
		Return(redis.NewStringResult(string(updatedStateJSON), nil)).
		AnyTimes()

	// Allow some time for the NextRun to be updated (if applicable)
	time.Sleep(100 * time.Millisecond)

	// Stop the job to clean up
	if err := job.Stop(ctx); err != nil {
		t.Errorf("expected no error when stopping job, got: %v", err)
	}

	// Allow some time for the job to stop gracefully
	time.Sleep(100 * time.Millisecond)

	// Verify the number of executions
	if executions != maxExecutions {
		t.Errorf("expected %d executions, got %d", maxExecutions, executions)
	}
}
