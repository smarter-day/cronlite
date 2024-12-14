package tests

import (
	"context"
	"cronlite/locker"
	"cronlite/mocks"
	"github.com/redis/go-redis/v9"
	"go.uber.org/mock/gomock"
	"testing"
	"time"
)

func TestLocker_Acquire(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRedis := mocks.NewMockCmdable(ctrl)
	//logger := mocks.NewMockILogger(ctrl)
	lockName := "cronlite:lock::test-lock"

	ttl := 10 * time.Second
	mockRedis.EXPECT().
		SetNX(gomock.Any(), lockName, 1, ttl).
		Return(redis.NewBoolResult(true, nil)).
		Times(1)

	l := locker.NewLocker(locker.Options{
		Name:    "test-lock",
		Redis:   mockRedis,
		LockTTL: ttl,
	})

	success, err := l.Acquire(context.Background())
	if err != nil || !success {
		t.Fatalf("expected lock to be acquired, got err: %v", err)
	}
}

func TestLocker_Acquire_AlreadyLocked(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRedis := mocks.NewMockCmdable(ctrl)
	lockName := "cronlite:lock::test-lock"

	// Expect SetNX to be called and return false, indicating the lock is already held
	mockRedis.EXPECT().
		SetNX(gomock.Any(), lockName, 1, gomock.Any()).
		Return(redis.NewBoolResult(false, nil)).
		Times(1)

	// Initialize the Locker with the mocked ILogger
	l := locker.NewLocker(locker.Options{
		Name:  "test-lock",
		Redis: mockRedis,
	})

	success, err := l.Acquire(context.Background())
	if err != nil || success {
		t.Fatalf("expected lock to not be acquired, got success: %v, err: %v", success, err)
	}
}

func TestLocker_Release(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRedis := mocks.NewMockCmdable(ctrl)
	lockName := "cronlite:lock::test-lock"

	// Expect Del to be called and return 1, indicating the lock was deleted
	mockRedis.EXPECT().
		Del(gomock.Any(), lockName).
		Return(redis.NewIntResult(1, nil)).
		Times(1)

	// Initialize the Locker with the mocked ILogger
	l := locker.NewLocker(locker.Options{
		Name:  "test-lock",
		Redis: mockRedis,
	})

	err := l.Release(context.Background())
	if err != nil {
		t.Fatalf("expected lock to be released, got err: %v", err)
	}
}

func TestLocker_Extend(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRedis := mocks.NewMockCmdable(ctrl)
	lockName := "cronlite:lock::test-lock"
	ttl := 10 * time.Second

	// Expect Expire to be called and return true, indicating the TTL was updated
	mockRedis.EXPECT().
		Expire(gomock.Any(), lockName, ttl).
		Return(redis.NewBoolResult(true, nil)).
		Times(1)

	// Initialize the Locker with the mocked ILogger
	l := locker.NewLocker(locker.Options{
		Name:    "test-lock",
		Redis:   mockRedis,
		LockTTL: ttl,
	})

	err := l.Extend(context.Background())
	if err != nil {
		t.Fatalf("expected lock to be extended, got err: %v", err)
	}
}

func TestLocker_GetLockTTL(t *testing.T) {
	l := locker.NewLocker(locker.Options{
		Name:    "test-lock",
		LockTTL: 10 * time.Second,
	})

	ttl := l.GetLockTTL()
	if ttl != 10*time.Second {
		t.Fatalf("expected lock TTL to be 10 seconds, got: %v", ttl)
	}
}
