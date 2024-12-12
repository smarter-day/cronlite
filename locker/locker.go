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
	SetTTL(duration time.Duration)
}

// Options defines options for configuring a Locker
type Options struct {
	Name    string
	Redis   redis.Cmdable
	Logger  logger.ILogger
	LockTTL time.Duration
}

// Locker implements ILocker
type Locker struct {
	mu      sync.Mutex
	options Options
}

// NewLocker initializes a new locker
func NewLocker(options Options) ILocker {
	if options.Logger == nil {
		options.Logger = &logger.LogLogger{}
	}
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

func (l *Locker) Acquire(ctx context.Context) (bool, error) {
	if l.options.Name == "" || l.options.Redis == nil {
		return false, ErrRedisUnavailable
	}

	success, err := l.options.Redis.SetNX(ctx, l.getLockName(), 1, l.options.LockTTL).Result()
	l.options.Logger.Debug(ctx, "Acquiring lock", map[string]interface{}{"name": l.options.Name, "success": success, "error": err})
	return success, err
}

func (l *Locker) Release(ctx context.Context) error {
	if l.options.Redis == nil {
		return ErrRedisUnavailable
	}

	err := l.options.Redis.Del(ctx, l.getLockName()).Err()
	if err != nil {
		l.options.Logger.Error(ctx, "Failed to release lock", map[string]interface{}{"name": l.options.Name, "error": err})
		return ErrLockReleaseFailed
	}
	l.options.Logger.Debug(ctx, "Released lock", map[string]interface{}{"name": l.options.Name})
	return nil
}

func (l *Locker) Extend(ctx context.Context) error {
	if l.options.Redis == nil {
		return ErrRedisUnavailable
	}

	success, err := l.options.Redis.Expire(ctx, l.getLockName(), l.options.LockTTL).Result()
	l.options.Logger.Debug(ctx, "Extended lock", map[string]interface{}{"name": l.options.Name, "success": success, "error": err})
	return err
}

func (l *Locker) GetLockTTL() time.Duration {
	return l.options.LockTTL
}

func (l *Locker) SetTTL(duration time.Duration) {
	l.options.LockTTL = duration
}
