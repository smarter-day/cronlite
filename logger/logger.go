package logger

// Level defines log levels
type Level int8

// Log level constants according to zerologr implementation
const (
	LogPanicLevel = iota - 4
	LogFatalLevel
	LogErrorLevel
	LogWarnLevel
	LogInfoLevel
	LogDebugLevel
)

// ILogger represents a logging interface.
type ILogger interface {
	// SetLevel sets the log level.
	SetLevel(level Level)

	// Debug logs debug message with the given key/value pairs as context.
	Debug(msg string, keysAndValues ...interface{})

	// Info logs info message with the given key/value pairs as context.
	Info(msg string, keysAndValues ...interface{})

	// Warn logs warning message with the given key/value pairs as context.
	Warn(msg string, keysAndValues ...interface{})

	// Error logs an error, with the given message and key/value pairs as context.
	Error(msg string, keysAndValues ...interface{})

	// Fatal logs fatal message with the given key/value pairs as context.
	Fatal(msg string, keysAndValues ...interface{})

	// Panic logs panic message with the given key/value pairs as context.
	Panic(msg string, keysAndValues ...interface{})

	// WithValues returns a new Logger with additional key/value pairs.
	WithValues(keysAndValues ...interface{}) ILogger

	// WithError returns a new Logger with additional error.
	WithError(err error) ILogger
}
