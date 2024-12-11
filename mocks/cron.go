// Code generated by MockGen. DO NOT EDIT.
// Source: ./cron.go
//
// Generated by this command:
//
//	mockgen -source=./cron.go -destination=./mocks/cron.go -package=mocks
//

// Package mocks is a generated GoMock package.
package mocks

import (
	context "context"
	cronlite "cronlite"
	reflect "reflect"

	gomock "go.uber.org/mock/gomock"
)

// MockICronJob is a mock of ICronJob interface.
type MockICronJob struct {
	ctrl     *gomock.Controller
	recorder *MockICronJobMockRecorder
	isgomock struct{}
}

// MockICronJobMockRecorder is the mock recorder for MockICronJob.
type MockICronJobMockRecorder struct {
	mock *MockICronJob
}

// NewMockICronJob creates a new mock instance.
func NewMockICronJob(ctrl *gomock.Controller) *MockICronJob {
	mock := &MockICronJob{ctrl: ctrl}
	mock.recorder = &MockICronJobMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockICronJob) EXPECT() *MockICronJobMockRecorder {
	return m.recorder
}

// GetJobState mocks base method.
func (m *MockICronJob) GetJobState(ctx context.Context) (*cronlite.JobState, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "GetJobState", ctx)
	ret0, _ := ret[0].(*cronlite.JobState)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// GetJobState indicates an expected call of GetJobState.
func (mr *MockICronJobMockRecorder) GetJobState(ctx any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "GetJobState", reflect.TypeOf((*MockICronJob)(nil).GetJobState), ctx)
}

// SaveJobState mocks base method.
func (m *MockICronJob) SaveJobState(ctx context.Context, state *cronlite.JobState) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "SaveJobState", ctx, state)
	ret0, _ := ret[0].(error)
	return ret0
}

// SaveJobState indicates an expected call of SaveJobState.
func (mr *MockICronJobMockRecorder) SaveJobState(ctx, state any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "SaveJobState", reflect.TypeOf((*MockICronJob)(nil).SaveJobState), ctx, state)
}

// Start mocks base method.
func (m *MockICronJob) Start(ctx context.Context) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Start", ctx)
	ret0, _ := ret[0].(error)
	return ret0
}

// Start indicates an expected call of Start.
func (mr *MockICronJobMockRecorder) Start(ctx any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Start", reflect.TypeOf((*MockICronJob)(nil).Start), ctx)
}

// Stop mocks base method.
func (m *MockICronJob) Stop() error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Stop")
	ret0, _ := ret[0].(error)
	return ret0
}

// Stop indicates an expected call of Stop.
func (mr *MockICronJobMockRecorder) Stop() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Stop", reflect.TypeOf((*MockICronJob)(nil).Stop))
}
