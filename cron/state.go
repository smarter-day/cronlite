package cron

import (
	"context"
	"errors"
	"fmt"
	"github.com/redis/go-redis/v9"
	"sync"
	"time"
)

type IState interface {
	Get(ctx context.Context, force bool) (*CronJobState, error)
	Save(ctx context.Context, state *CronJobState) error
	Delete(ctx context.Context) error
	Exists(ctx context.Context) (bool, error)
}

// State handles state loading and saving with proper synchronization
type State struct {
	mutex          sync.RWMutex
	redis          redis.Cmdable
	stateKey       string
	prevState      *CronJobState
	currentState   *CronJobState
	onStateUpdated func(ctx context.Context, state *CronJobState) error
}

func NewState(redis redis.Cmdable, stateKey string, OnStateUpdated func(ctx context.Context, state *CronJobState) error) IState {
	return &State{
		redis:          redis,
		stateKey:       stateKey,
		onStateUpdated: OnStateUpdated,
		currentState:   &CronJobState{},
	}
}

func (sm *State) Get(ctx context.Context, force bool) (*CronJobState, error) {
	sm.mutex.RLock()
	defer sm.mutex.RUnlock()

	if force || sm.currentState == nil {
		var err error
		newState, err := LoadJobStateFromRedis(ctx, sm.redis, sm.stateKey)
		if err != nil {
			if errors.Is(err, redis.Nil) {
				return nil, nil
			}
			return nil, fmt.Errorf("failed to load job state: %w", err)
		}
		sm.currentState = newState

		err = sm.onStateUpdated(ctx, sm.currentState)
		if err != nil {
			return sm.currentState, err
		}
	}

	return sm.currentState, nil
}

func (sm *State) Save(ctx context.Context, state *CronJobState) error {
	sm.mutex.Lock()
	defer sm.mutex.Unlock()

	state.UpdatedAt = time.Now()
	sm.currentState = state

	err := SaveJobStateToRedis(ctx, sm.redis, sm.stateKey, state)
	if err != nil {
		return fmt.Errorf("failed to save job state: %w", err)
	}

	err = sm.onStateUpdated(ctx, sm.currentState)
	if err != nil {
		return err
	}

	return nil
}

func (sm *State) Delete(ctx context.Context) error {
	sm.mutex.Lock()
	defer sm.mutex.Unlock()

	_, err := sm.redis.Del(ctx, sm.stateKey).Result()
	if err != nil {
		return fmt.Errorf("failed to delete job state: %w", err)
	}
	return nil
}

func (sm *State) Exists(ctx context.Context) (bool, error) {
	sm.mutex.RLock()
	defer sm.mutex.RUnlock()

	exists, err := sm.redis.Exists(ctx, sm.stateKey).Result()
	if err != nil {
		return false, fmt.Errorf("failed to check if job state exists: %w", err)
	}
	return exists == 1, nil
}
