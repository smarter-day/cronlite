// Code generated by MockGen. DO NOT EDIT.
// Source: ./cron/job.go
//
// Generated by this command:
//
//	mockgen -source=./cron/job.go -destination=./mocks/cron_job_mock.go -package=mocks
//

// Package mocks is a generated GoMock package.
package mocks

import (
	context "context"
	cron "cronlite/cron"
	reflect "reflect"

	redsync "github.com/go-redsync/redsync/v4"
	gomock "go.uber.org/mock/gomock"
)

// MockRedsyncProvider is a mock of RedsyncProvider interface.
type MockRedsyncProvider struct {
	ctrl     *gomock.Controller
	recorder *MockRedsyncProviderMockRecorder
	isgomock struct{}
}

// MockRedsyncProviderMockRecorder is the mock recorder for MockRedsyncProvider.
type MockRedsyncProviderMockRecorder struct {
	mock *MockRedsyncProvider
}

// NewMockRedsyncProvider creates a new mock instance.
func NewMockRedsyncProvider(ctrl *gomock.Controller) *MockRedsyncProvider {
	mock := &MockRedsyncProvider{ctrl: ctrl}
	mock.recorder = &MockRedsyncProviderMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockRedsyncProvider) EXPECT() *MockRedsyncProviderMockRecorder {
	return m.recorder
}

// NewMutex mocks base method.
func (m *MockRedsyncProvider) NewMutex(name string, options ...redsync.Option) *redsync.Mutex {
	m.ctrl.T.Helper()
	varargs := []any{name}
	for _, a := range options {
		varargs = append(varargs, a)
	}
	ret := m.ctrl.Call(m, "NewMutex", varargs...)
	ret0, _ := ret[0].(*redsync.Mutex)
	return ret0
}

// NewMutex indicates an expected call of NewMutex.
func (mr *MockRedsyncProviderMockRecorder) NewMutex(name any, options ...any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	varargs := append([]any{name}, options...)
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "NewMutex", reflect.TypeOf((*MockRedsyncProvider)(nil).NewMutex), varargs...)
}

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

// GetOptions mocks base method.
func (m *MockICronJob) GetOptions() *cron.CronJobOptions {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "GetOptions")
	ret0, _ := ret[0].(*cron.CronJobOptions)
	return ret0
}

// GetOptions indicates an expected call of GetOptions.
func (mr *MockICronJobMockRecorder) GetOptions() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "GetOptions", reflect.TypeOf((*MockICronJob)(nil).GetOptions))
}

// GetState mocks base method.
func (m *MockICronJob) GetState() cron.IState {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "GetState")
	ret0, _ := ret[0].(cron.IState)
	return ret0
}

// GetState indicates an expected call of GetState.
func (mr *MockICronJobMockRecorder) GetState() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "GetState", reflect.TypeOf((*MockICronJob)(nil).GetState))
}

// OnStateUpdated mocks base method.
func (m *MockICronJob) OnStateUpdated(ctx context.Context, state *cron.CronJobState) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "OnStateUpdated", ctx, state)
	ret0, _ := ret[0].(error)
	return ret0
}

// OnStateUpdated indicates an expected call of OnStateUpdated.
func (mr *MockICronJobMockRecorder) OnStateUpdated(ctx, state any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "OnStateUpdated", reflect.TypeOf((*MockICronJob)(nil).OnStateUpdated), ctx, state)
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
func (m *MockICronJob) Stop(ctx context.Context) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Stop", ctx)
	ret0, _ := ret[0].(error)
	return ret0
}

// Stop indicates an expected call of Stop.
func (mr *MockICronJobMockRecorder) Stop(ctx any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Stop", reflect.TypeOf((*MockICronJob)(nil).Stop), ctx)
}
