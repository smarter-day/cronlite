// Code generated by MockGen. DO NOT EDIT.
// Source: ./logger/logger.go
//
// Generated by this command:
//
//	mockgen -source=./logger/logger.go -destination=./mocks/logger_mock.go -package=mocks
//

// Package mocks is a generated GoMock package.
package mocks

import (
	logger "github.com/smarter-day/logger"
	reflect "reflect"

	gomock "go.uber.org/mock/gomock"
)

// MockILogger is a mock of ILogger interface.
type MockILogger struct {
	ctrl     *gomock.Controller
	recorder *MockILoggerMockRecorder
	isgomock struct{}
}

// MockILoggerMockRecorder is the mock recorder for MockILogger.
type MockILoggerMockRecorder struct {
	mock *MockILogger
}

// NewMockILogger creates a new mock instance.
func NewMockILogger(ctrl *gomock.Controller) *MockILogger {
	mock := &MockILogger{ctrl: ctrl}
	mock.recorder = &MockILoggerMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockILogger) EXPECT() *MockILoggerMockRecorder {
	return m.recorder
}

// Debug mocks base method.
func (m *MockILogger) Debug(msg string, keysAndValues ...any) {
	m.ctrl.T.Helper()
	varargs := []any{msg}
	for _, a := range keysAndValues {
		varargs = append(varargs, a)
	}
	m.ctrl.Call(m, "Debug", varargs...)
}

// Debug indicates an expected call of Debug.
func (mr *MockILoggerMockRecorder) Debug(msg any, keysAndValues ...any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	varargs := append([]any{msg}, keysAndValues...)
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Debug", reflect.TypeOf((*MockILogger)(nil).Debug), varargs...)
}

// Error mocks base method.
func (m *MockILogger) Error(msg string, keysAndValues ...any) {
	m.ctrl.T.Helper()
	varargs := []any{msg}
	for _, a := range keysAndValues {
		varargs = append(varargs, a)
	}
	m.ctrl.Call(m, "Error", varargs...)
}

// Error indicates an expected call of Error.
func (mr *MockILoggerMockRecorder) Error(msg any, keysAndValues ...any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	varargs := append([]any{msg}, keysAndValues...)
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Error", reflect.TypeOf((*MockILogger)(nil).Error), varargs...)
}

// Fatal mocks base method.
func (m *MockILogger) Fatal(msg string, keysAndValues ...any) {
	m.ctrl.T.Helper()
	varargs := []any{msg}
	for _, a := range keysAndValues {
		varargs = append(varargs, a)
	}
	m.ctrl.Call(m, "Fatal", varargs...)
}

// Fatal indicates an expected call of Fatal.
func (mr *MockILoggerMockRecorder) Fatal(msg any, keysAndValues ...any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	varargs := append([]any{msg}, keysAndValues...)
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Fatal", reflect.TypeOf((*MockILogger)(nil).Fatal), varargs...)
}

// Info mocks base method.
func (m *MockILogger) Info(msg string, keysAndValues ...any) {
	m.ctrl.T.Helper()
	varargs := []any{msg}
	for _, a := range keysAndValues {
		varargs = append(varargs, a)
	}
	m.ctrl.Call(m, "Info", varargs...)
}

// Info indicates an expected call of Info.
func (mr *MockILoggerMockRecorder) Info(msg any, keysAndValues ...any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	varargs := append([]any{msg}, keysAndValues...)
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Info", reflect.TypeOf((*MockILogger)(nil).Info), varargs...)
}

// Panic mocks base method.
func (m *MockILogger) Panic(msg string, keysAndValues ...any) {
	m.ctrl.T.Helper()
	varargs := []any{msg}
	for _, a := range keysAndValues {
		varargs = append(varargs, a)
	}
	m.ctrl.Call(m, "Panic", varargs...)
}

// Panic indicates an expected call of Panic.
func (mr *MockILoggerMockRecorder) Panic(msg any, keysAndValues ...any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	varargs := append([]any{msg}, keysAndValues...)
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Panic", reflect.TypeOf((*MockILogger)(nil).Panic), varargs...)
}

// SetLevel mocks base method.
func (m *MockILogger) SetLevel(level logger.Level) {
	m.ctrl.T.Helper()
	m.ctrl.Call(m, "SetLevel", level)
}

// SetLevel indicates an expected call of SetLevel.
func (mr *MockILoggerMockRecorder) SetLevel(level any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "SetLevel", reflect.TypeOf((*MockILogger)(nil).SetLevel), level)
}

// Warn mocks base method.
func (m *MockILogger) Warn(msg string, keysAndValues ...any) {
	m.ctrl.T.Helper()
	varargs := []any{msg}
	for _, a := range keysAndValues {
		varargs = append(varargs, a)
	}
	m.ctrl.Call(m, "Warn", varargs...)
}

// Warn indicates an expected call of Warn.
func (mr *MockILoggerMockRecorder) Warn(msg any, keysAndValues ...any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	varargs := append([]any{msg}, keysAndValues...)
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Warn", reflect.TypeOf((*MockILogger)(nil).Warn), varargs...)
}

// WithError mocks base method.
func (m *MockILogger) WithError(err error) logger.ILogger {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "WithError", err)
	ret0, _ := ret[0].(logger.ILogger)
	return ret0
}

// WithError indicates an expected call of WithError.
func (mr *MockILoggerMockRecorder) WithError(err any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "WithError", reflect.TypeOf((*MockILogger)(nil).WithError), err)
}

// WithValues mocks base method.
func (m *MockILogger) WithValues(keysAndValues ...any) logger.ILogger {
	m.ctrl.T.Helper()
	varargs := []any{}
	for _, a := range keysAndValues {
		varargs = append(varargs, a)
	}
	ret := m.ctrl.Call(m, "WithValues", varargs...)
	ret0, _ := ret[0].(logger.ILogger)
	return ret0
}

// WithValues indicates an expected call of WithValues.
func (mr *MockILoggerMockRecorder) WithValues(keysAndValues ...any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "WithValues", reflect.TypeOf((*MockILogger)(nil).WithValues), keysAndValues...)
}
