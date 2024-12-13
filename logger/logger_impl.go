package logger

import (
	"context"
	"os"

	"github.com/go-logr/logr"
	"github.com/go-logr/zerologr"
	"github.com/rs/zerolog"
	"go.opentelemetry.io/otel/trace"
)

const (
	ErrorKey          = "error"
	SpanIdLogKeyName  = "spanID"
	TraceIdLogKeyName = "traceID"
)

var logger ILogger

// init initializes the logger configuration and sets up the global logger instance.
// It configures the zerolog to use Unix milliseconds for time formatting and sets
// the name field and separator for the zerologr logger. It also creates a new
// zerolog logger that writes to standard error, includes caller information, and
// timestamps each log entry. The logger instance is then wrapped in a logr.Logger
// and assigned to the global logger variable.
func init() {
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnixMs
	zerologr.NameFieldName = "logger"
	zerologr.NameSeparator = "/"

	zl := zerolog.New(os.Stderr)
	zl = zl.With().Caller().Timestamp().Logger()

	zerologr.VerbosityFieldName = ""
	ls := zerologr.NewLogSink(&zl)
	loggerInstance := logr.New(ls)
	logger = &Logger{&loggerInstance}
}

// Log returns a logger instance that is enriched with tracing information
// if a valid context is provided.
//
// The function extracts the SpanContext from the given context and, if valid,
// adds the span and trace IDs to the logger's context, allowing for enhanced
// traceability in distributed systems.
//
// Parameters:
//
//	ctx - A context.Context from which the SpanContext is extracted. If the
//	      context is nil or does not contain a valid SpanContext, the logger
//	      is returned without additional trace information.
//
// Returns:
//
//	ILogger - A logger instance that may include span and trace IDs if the
//	          provided context contains a valid SpanContext.
func Log(ctx context.Context) ILogger {
	newLogger := logger
	if ctx != nil {
		sc := trace.SpanContextFromContext(ctx)
		if sc.IsValid() {
			newLogger = newLogger.WithValues(
				SpanIdLogKeyName, sc.SpanID().String(),
				TraceIdLogKeyName, sc.TraceID().String())
		}
	}
	return newLogger
}

type Logger struct {
	Logr *logr.Logger
}

func (l *Logger) SetLevel(level Level) {
	zerolog.SetGlobalLevel(zerolog.Level(int(level)))
}

// Debug logs a message at the debug level with optional key-value pairs for additional context.
//
// This method retrieves the current log sink and, if it supports call stack
// helpers, invokes the helper to capture the call stack. It then logs the
// provided message and key-value pairs at the debug level.
//
// Parameters:
//
//	msg - The message to be logged.
//	keysAndValues - Optional key-value pairs that provide additional context
//	                for the log entry. These are variadic and can be omitted.
//
// The function does not return any value.
func (l *Logger) Debug(msg string, keysAndValues ...interface{}) {
	sink := l.Logr.GetSink()
	if withHelper, ok := sink.(logr.CallStackHelperLogSink); ok {
		withHelper.GetCallStackHelper()()
	}
	sink.Info(int(LogDebugLevel), msg, keysAndValues...)
}

// Info logs a message at the info level with optional key-value pairs for additional context.
//
// This method retrieves the current log sink and, if it supports call stack
// helpers, invokes the helper to capture the call stack. It then logs the
// provided message and key-value pairs at the info level.
//
// Parameters:
//
//	msg - The message to be logged.
//	keysAndValues - Optional key-value pairs that provide additional context
//	                for the log entry. These are variadic and can be omitted.
//
// The function does not return any value.
func (l *Logger) Info(msg string, keysAndValues ...interface{}) {
	sink := l.Logr.GetSink()
	if withHelper, ok := sink.(logr.CallStackHelperLogSink); ok {
		withHelper.GetCallStackHelper()()
	}
	sink.Info(int(LogInfoLevel), msg, keysAndValues...)
}

// Warn logs a message at the warning level with optional key-value pairs for additional context.
//
// This method retrieves the current log sink and, if it supports call stack
// helpers, invokes the helper to capture the call stack. It then logs the
// provided message and key-value pairs at the warning level.
//
// Parameters:
//
//	msg - The message to be logged.
//	keysAndValues - Optional key-value pairs that provide additional context
//	                for the log entry. These are variadic and can be omitted.
//
// The function does not return any value.
func (l *Logger) Warn(msg string, keysAndValues ...interface{}) {
	sink := l.Logr.GetSink()
	if withHelper, ok := sink.(logr.CallStackHelperLogSink); ok {
		withHelper.GetCallStackHelper()()
	}
	sink.Info(int(LogWarnLevel), msg, keysAndValues...)
}

// Error logs a message at the error level with optional key-value pairs for additional context.
//
// This method retrieves the current log sink and, if it supports call stack
// helpers, invokes the helper to capture the call stack. It then logs the
// provided message and key-value pairs at the error level.
//
// Parameters:
//
//	msg - The message to be logged.
//	keysAndValues - Optional key-value pairs that provide additional context
//	                for the log entry. These are variadic and can be omitted.
//
// The function does not return any value.
func (l *Logger) Error(msg string, keysAndValues ...interface{}) {
	sink := l.Logr.GetSink()
	if withHelper, ok := sink.(logr.CallStackHelperLogSink); ok {
		withHelper.GetCallStackHelper()()
	}
	sink.Info(int(LogErrorLevel), msg, keysAndValues...)
}

// Fatal logs a message at the fatal level with optional key-value pairs for additional context.
// This method retrieves the current log sink and, if it supports call stack
// helpers, invokes the helper to capture the call stack. It then logs the
// provided message and key-value pairs at the fatal level.
//
// Parameters:
//
//	msg - The message to be logged.
//	keysAndValues - Optional key-value pairs that provide additional context
//	                for the log entry. These are variadic and can be omitted.
//
// The function does not return any value.
func (l *Logger) Fatal(msg string, keysAndValues ...interface{}) {
	sink := l.Logr.GetSink()
	if withHelper, ok := sink.(logr.CallStackHelperLogSink); ok {
		withHelper.GetCallStackHelper()()
	}
	sink.Info(int(LogFatalLevel), msg, keysAndValues...)
}

// Panic logs a message at the panic level with optional key-value pairs for additional context.
//
// This method retrieves the current log sink and, if it supports call stack
// helpers, invokes the helper to capture the call stack. It then logs the
// provided message and key-value pairs at the panic level.
//
// Parameters:
//
//	msg - The message to be logged.
//	keysAndValues - Optional key-value pairs that provide additional context
//	                for the log entry. These are variadic and can be omitted.
//
// The function does not return any value.
func (l *Logger) Panic(msg string, keysAndValues ...interface{}) {
	sink := l.Logr.GetSink()
	if withHelper, ok := sink.(logr.CallStackHelperLogSink); ok {
		withHelper.GetCallStackHelper()()
	}
	sink.Info(int(LogPanicLevel), msg, keysAndValues...)
}

// WithValues returns a new ILogger instance with additional context provided
// by key-value pairs. These pairs are added to the logger's context, allowing
// for more detailed and structured logging.
//
// Parameters:
//
//	keysAndValues - Variadic key-value pairs that provide additional context
//	                for the log entry. These pairs are used to enrich the
//	                logger's context and can be omitted if not needed.
//
// Returns:
//
//	ILogger - A new logger instance that includes the provided key-value pairs
//	          in its context, enabling enhanced logging capabilities.
func (l Logger) WithValues(keysAndValues ...interface{}) ILogger {
	withValues := l.Logr.WithValues(keysAndValues...)
	l.Logr = &withValues
	return &l
}

// WithError returns a new ILogger instance with an error context added.
// This method enriches the logger's context with the provided error,
// allowing for more detailed and structured logging of error information.
//
// Parameters:
//
//	err - The error to be added to the logger's context. This error is
//	      associated with the predefined ErrorKey, enabling consistent
//	      error logging across the application.
//
// Returns:
//
//	ILogger - A new logger instance that includes the provided error in
//	          its context, facilitating enhanced error traceability.
func (l Logger) WithError(err error) ILogger {
	withValues := l.Logr.WithValues(ErrorKey, err)
	l.Logr = &withValues
	return &l
}
