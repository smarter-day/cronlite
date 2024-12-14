package logger

import (
	"context"
	"fmt"
	"path/filepath"
	"runtime"
	"time"

	"github.com/sirupsen/logrus"
	"go.opentelemetry.io/otel/trace"
)

const (
	ErrorKey          = "error"
	SpanIdLogKeyName  = "spanID"
	TraceIdLogKeyName = "traceID"
)

var logger ILogger

// init initializes the logger configuration and sets up the global logger instance.
// It configures Logrus to use JSON formatting with custom timestamp formatting.
// Caller reporting is disabled to allow manual capture of caller information.
// The logger instance is then assigned to the global logger variable.
func init() {
	log := logrus.New()

	// Configure Logrus formatter with JSON format and custom timestamp
	log.SetFormatter(&logrus.JSONFormatter{
		TimestampFormat: time.RFC3339Nano,
	})

	// Disable Logrus's built-in caller reporting
	log.SetReportCaller(false)

	// Set default log level
	log.SetLevel(logrus.DebugLevel)

	// Initialize the global logger with a base entry
	logger = &Logger{
		Entry: log.WithFields(logrus.Fields{}),
	}
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
				TraceIdLogKeyName, sc.TraceID().String(),
			)
		}
	}
	return newLogger
}

// Logger implements the ILogger interface using Logrus.
type Logger struct {
	Entry *logrus.Entry
}

// SetLevel sets the global logging level.
func (l *Logger) SetLevel(level Level) {
	l.Entry.Logger.SetLevel(logrus.Level(level))
}

// Debug logs a message at the debug level with optional key-value pairs for additional context.
func (l *Logger) Debug(msg string, keysAndValues ...interface{}) {
	fields := convertToFields(keysAndValues...)
	callerInfo := getCallerInfo()
	l.Entry.WithFields(fields).WithFields(logrus.Fields{
		"caller": callerInfo,
	}).Debug(msg)
}

// Info logs a message at the info level with optional key-value pairs for additional context.
func (l *Logger) Info(msg string, keysAndValues ...interface{}) {
	fields := convertToFields(keysAndValues...)
	callerInfo := getCallerInfo()
	l.Entry.WithFields(fields).WithFields(logrus.Fields{
		"caller": callerInfo,
	}).Info(msg)
}

// Warn logs a message at the warning level with optional key-value pairs for additional context.
func (l *Logger) Warn(msg string, keysAndValues ...interface{}) {
	fields := convertToFields(keysAndValues...)
	callerInfo := getCallerInfo()
	l.Entry.WithFields(fields).WithFields(logrus.Fields{
		"caller": callerInfo,
	}).Warn(msg)
}

// Error logs a message at the error level with optional key-value pairs for additional context.
func (l *Logger) Error(msg string, keysAndValues ...interface{}) {
	fields := convertToFields(keysAndValues...)
	callerInfo := getCallerInfo()
	l.Entry.WithFields(fields).WithFields(logrus.Fields{
		"caller": callerInfo,
	}).Error(msg)
}

// Fatal logs a message at the fatal level with optional key-value pairs for additional context.
// It then exits the application with status code 1.
func (l *Logger) Fatal(msg string, keysAndValues ...interface{}) {
	fields := convertToFields(keysAndValues...)
	callerInfo := getCallerInfo()
	l.Entry.WithFields(fields).WithFields(logrus.Fields{
		"caller": callerInfo,
	}).Fatal(msg)
}

// Panic logs a message at the panic level with optional key-value pairs for additional context.
// It then panics with the provided message.
func (l *Logger) Panic(msg string, keysAndValues ...interface{}) {
	fields := convertToFields(keysAndValues...)
	callerInfo := getCallerInfo()
	l.Entry.WithFields(fields).WithFields(logrus.Fields{
		"caller": callerInfo,
	}).Panic(msg)
}

// WithValues returns a new ILogger instance with additional context provided
// by key-value pairs. These pairs are added to the logger's context, allowing
// for more detailed and structured logging.
func (l Logger) WithValues(keysAndValues ...interface{}) ILogger {
	fields := convertToFields(keysAndValues...)
	newEntry := l.Entry.WithFields(fields)
	return &Logger{Entry: newEntry}
}

// WithError returns a new ILogger instance with an error context added.
// This method enriches the logger's context with the provided error,
// allowing for more detailed and structured logging of error information.
func (l Logger) WithError(err error) ILogger {
	newEntry := l.Entry.WithField(ErrorKey, err)
	return &Logger{Entry: newEntry}
}

// convertToFields converts variadic key-value pairs into logrus.Fields.
// It ensures that keys are strings and handles any mismatches gracefully.
func convertToFields(keysAndValues ...interface{}) logrus.Fields {
	fields := logrus.Fields{}
	length := len(keysAndValues)
	for i := 0; i < length-1; i += 2 {
		key, ok := keysAndValues[i].(string)
		if !ok {
			key = fmt.Sprintf("invalid_key_%d", i)
		}
		fields[key] = keysAndValues[i+1]
	}
	// Handle odd number of arguments
	if length%2 != 0 {
		key, ok := keysAndValues[length-1].(string)
		if !ok {
			key = fmt.Sprintf("invalid_key_%d", length-1)
		}
		fields[key] = "MISSING_VALUE"
	}
	return fields
}

// getCallerInfo retrieves the file and function name of the caller outside the logger package.
// It skips the frames related to the logger's internal methods.
func getCallerInfo() string {
	// Number of stack frames to skip to reach the actual caller
	const skipFrames = 3
	pc, file, line, ok := runtime.Caller(skipFrames)
	if !ok {
		return "unknown"
	}

	function := "unknown"
	fn := runtime.FuncForPC(pc)
	if fn != nil {
		function = fn.Name()
	}

	// Shorten the file path to just the file name
	shortFile := filepath.Base(file)

	return fmt.Sprintf("%s:%d %s", shortFile, line, function)
}
