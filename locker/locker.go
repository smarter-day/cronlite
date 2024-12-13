package locker

import (
	"context"
	"cronlite/logger"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

var (
	DefaultLockTTL       = 10 * time.Minute
	ErrRedisUnavailable  = errors.New("redis client unavailable")
	ErrLockReleaseFailed = errors.New("lock release failed")
)

// ILocker defines the interface for a lock manager
type ILocker interface {
	Acquire(ctx context.Context) (bool, error)
	Release(ctx context.Context) error
	Extend(ctx context.Context) error
	GetLockTTL() time.Duration
	SetTTL(duration time.Duration) ILocker
}

// Options defines options for configuring a Locker
type Options struct {
	Name    string
	Redis   redis.Cmdable
	LockTTL time.Duration
}

// Locker implements ILocker
type Locker struct {
	mu      sync.Mutex
	options Options
}

// NewLocker initializes a new locker
func NewLocker(options Options) ILocker {
	if options.LockTTL == 0 {
		options.LockTTL = DefaultLockTTL
	}

	return &Locker{
		options: options,
	}
}

func (l *Locker) getLockName() string {
	return fmt.Sprintf("cronlite:lock::%s", l.options.Name)
}

// Acquire attempts to acquire a lock with the specified name and TTL.
// It returns true if the lock was successfully acquired, or false if it was not.
//
// Parameters:
//
//	ctx - The context for managing request-scoped values, cancellation signals, and deadlines.
//
// Returns:
//
//	bool - True if the lock was successfully acquired, false otherwise.
//	error - An error if the Redis client is unavailable or if there was an issue acquiring the lock.
func (l *Locker) Acquire(ctx context.Context) (bool, error) {
	if l.options.Name == "" || l.options.Redis == nil {
		return false, ErrRedisUnavailable
	}

	success, err := l.options.Redis.SetNX(ctx, l.getLockName(), 1, l.options.LockTTL).Result()
	logger.Log(ctx).WithValues("name", l.options.Name, "success", success).Debug("Acquiring lock")
	return success, err
}

// Release attempts to release the lock associated with the specified name.
// It logs the operation and returns an error if the Redis client is unavailable
// or if there was an issue releasing the lock.
//
// Parameters:
//
//	ctx - The context for managing request-scoped values, cancellation signals, and deadlines.
//
// Returns:
//
//	error - An error if the Redis client is unavailable or if there was an issue releasing the lock.
func (l *Locker) Release(ctx context.Context) error {
	if l.options.Redis == nil {
		return ErrRedisUnavailable
	}

	log := logger.Log(ctx).WithValues("name", l.options.Name)
	err := l.options.Redis.Del(ctx, l.getLockName()).Err()
	if err != nil {
		log.WithError(err).Error("Failed to release lock")
		return ErrLockReleaseFailed
	}
	log.Debug("Released lock")
	return nil
}

// Extend attempts to extend the TTL of the lock associated with the specified name.
// It logs the operation and returns an error if the Redis client is unavailable
// or if there was an issue extending the lock's TTL.
//
// Parameters:
//
//	ctx - The context for managing request-scoped values, cancellation signals, and deadlines.
//
// Returns:
//
//	error - An error if the Redis client is unavailable or if there was an issue extending the lock's TTL.
func (l *Locker) Extend(ctx context.Context) error {
	if l.options.Redis == nil {
		return ErrRedisUnavailable
	}

	success, err := l.options.Redis.Expire(ctx, l.getLockName(), l.options.LockTTL).Result()
	logger.Log(ctx).WithValues("name", l.options.Name, "success", success).
		Debug("Extending lock TTL")
	return err
}

func (l *Locker) GetLockTTL() time.Duration {
	return l.options.LockTTL
}

// SetTTL sets the time-to-live (TTL) duration for the lock.
//
// Parameters:
//
//	duration - The new TTL duration to be set for the lock.
//
// Returns:
//
//	ILocker - The locker instance with the updated TTL.
func (l *Locker) SetTTL(duration time.Duration) ILocker {
	l.options.LockTTL = duration
	return l
}
