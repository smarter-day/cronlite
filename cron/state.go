package cron

import (
	"bytes"
	"context"
	"cronlite/logger"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/redis/go-redis/v9"
	"sync"
)

// State handles state loading and saving with proper synchronization
type State struct {
	mutex          sync.RWMutex
	redis          redis.Cmdable
	stateKey       string
	logger         logger.ILogger
	prevState      JobState
	currentState   *JobState
	onStateUpdated func(ctx context.Context, state *JobState) error
}

func NewState(redis redis.Cmdable, stateKey string, logger logger.ILogger, OnStateUpdated func(ctx context.Context, state *JobState) error) IState {
	return &State{
		redis:          redis,
		stateKey:       stateKey,
		logger:         logger,
		onStateUpdated: OnStateUpdated,
	}
}

func (sm *State) Get(ctx context.Context, force bool) (*JobState, error) {
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

		// Compare current state with the new state, excluding timestamps
		if sm.currentState != nil && !statesAreEqual(sm.currentState, newState) {
			err := sm.onStateUpdated(ctx, newState)
			if err != nil {
				return nil, err
			}
		}

		if sm.currentState != nil {
			sm.prevState = *sm.currentState
			sm.currentState = newState
		} else {
			sm.currentState = newState
			sm.prevState = *sm.currentState
		}
	}
	return sm.currentState, nil
}

func (sm *State) Save(ctx context.Context, state *JobState) error {
	sm.mutex.Lock()
	defer sm.mutex.Unlock()

	err := SaveJobStateToRedis(ctx, sm.redis, sm.stateKey, state)

	if sm.currentState != nil && !statesAreEqual(&sm.prevState, state) {
		sm.prevState = *sm.currentState
		sm.currentState = state
		err := sm.onStateUpdated(ctx, sm.currentState)
		if err != nil {
			return err
		}
	}

	if err != nil {
		return fmt.Errorf("failed to save job state: %w", err)
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

// statesAreEqual compares two JobState structs and returns true if they are equal, excluding timestamp fields.
func statesAreEqual(state1, state2 *JobState) bool {
	// Json serialize both structs to JSON, excluding timestamp fields
	jsonBytes1, err := json.Marshal(struct {
		*JobState
		Timestamps []string `json:"-"`
	}{state1, []string{"CreatedAt", "UpdatedAt"}})
	if err != nil {
		return false
	}

	jsonBytes2, err := json.Marshal(struct {
		*JobState
		Timestamps []string `json:"-"`
	}{state2, []string{"CreatedAt", "UpdatedAt"}})
	if err != nil {
		return false
	}

	// Compare the JSON representations
	return bytes.Equal(jsonBytes1, jsonBytes2)
}
