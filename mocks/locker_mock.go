// Code generated by MockGen. DO NOT EDIT.
// Source: ./locker/locker.go
//
// Generated by this command:
//
//	mockgen -source=./locker/locker.go -destination=./mocks/locker_mock.go -package=mocks
//

// Package mocks is a generated GoMock package.
package mocks

import (
	context "context"
	locker "cronlite/locker"
	reflect "reflect"
	time "time"

	gomock "go.uber.org/mock/gomock"
)

// MockILocker is a mock of ILocker interface.
type MockILocker struct {
	ctrl     *gomock.Controller
	recorder *MockILockerMockRecorder
	isgomock struct{}
}

// MockILockerMockRecorder is the mock recorder for MockILocker.
type MockILockerMockRecorder struct {
	mock *MockILocker
}

// NewMockILocker creates a new mock instance.
func NewMockILocker(ctrl *gomock.Controller) *MockILocker {
	mock := &MockILocker{ctrl: ctrl}
	mock.recorder = &MockILockerMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockILocker) EXPECT() *MockILockerMockRecorder {
	return m.recorder
}

// Acquire mocks base method.
func (m *MockILocker) Acquire(ctx context.Context) (bool, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Acquire", ctx)
	ret0, _ := ret[0].(bool)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// Acquire indicates an expected call of Acquire.
func (mr *MockILockerMockRecorder) Acquire(ctx any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Acquire", reflect.TypeOf((*MockILocker)(nil).Acquire), ctx)
}

// Extend mocks base method.
func (m *MockILocker) Extend(ctx context.Context) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Extend", ctx)
	ret0, _ := ret[0].(error)
	return ret0
}

// Extend indicates an expected call of Extend.
func (mr *MockILockerMockRecorder) Extend(ctx any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Extend", reflect.TypeOf((*MockILocker)(nil).Extend), ctx)
}

// GetLockTTL mocks base method.
func (m *MockILocker) GetLockTTL() time.Duration {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "GetLockTTL")
	ret0, _ := ret[0].(time.Duration)
	return ret0
}

// GetLockTTL indicates an expected call of GetLockTTL.
func (mr *MockILockerMockRecorder) GetLockTTL() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "GetLockTTL", reflect.TypeOf((*MockILocker)(nil).GetLockTTL))
}

// Release mocks base method.
func (m *MockILocker) Release(ctx context.Context) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Release", ctx)
	ret0, _ := ret[0].(error)
	return ret0
}

// Release indicates an expected call of Release.
func (mr *MockILockerMockRecorder) Release(ctx any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Release", reflect.TypeOf((*MockILocker)(nil).Release), ctx)
}

// SetTTL mocks base method.
func (m *MockILocker) SetTTL(duration time.Duration) locker.ILocker {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "SetTTL", duration)
	ret0, _ := ret[0].(locker.ILocker)
	return ret0
}

// SetTTL indicates an expected call of SetTTL.
func (mr *MockILockerMockRecorder) SetTTL(duration any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "SetTTL", reflect.TypeOf((*MockILocker)(nil).SetTTL), duration)
}
