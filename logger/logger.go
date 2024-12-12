package logger

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"

	"github.com/sirupsen/logrus"
)

// ILogger defines a basic logger interface for flexibility
type ILogger interface {
	Info(ctx context.Context, message string, fields map[string]interface{})
	Warning(ctx context.Context, message string, fields map[string]interface{})
	Error(ctx context.Context, message string, fields map[string]interface{})
	Debug(ctx context.Context, message string, fields map[string]interface{})
	SetLogLevel(level string) error
}

// LogLogger is an implementation of ILogger using the standard library's log package with JSON formatting
type LogLogger struct {
	level string
}

func (l *LogLogger) log(level string, message string, fields map[string]interface{}) {
	// Include the log level and message in the fields
	logData := map[string]interface{}{
		"level":   level,
		"message": message,
	}
	for k, v := range fields {
		logData[k] = v
	}

	// Serialize log data to JSON
	logBytes, err := json.Marshal(logData)
	if err != nil {
		log.Printf(`{"level": "error", "message": "Failed to serialize log", "error": "%v"}`, err)
		return
	}

	// Print JSON log
	log.Println(string(logBytes))
}

func (l *LogLogger) Debug(ctx context.Context, message string, fields map[string]interface{}) {
	if l.level == "debug" {
		l.log("debug", message, fields)
	}
}

func (l *LogLogger) Error(ctx context.Context, message string, fields map[string]interface{}) {
	l.log("error", message, fields)
}

func (l *LogLogger) Info(ctx context.Context, message string, fields map[string]interface{}) {
	if l.level == "info" || l.level == "debug" {
		l.log("info", message, fields)
	}
}

func (l *LogLogger) Warning(ctx context.Context, message string, fields map[string]interface{}) {
	if l.level == "warning" || l.level == "info" || l.level == "debug" {
		l.log("warning", message, fields)
	}
}

func (l *LogLogger) SetLogLevel(level string) error {
	level = strings.ToLower(level)
	switch level {
	case "debug", "info", "error":
		l.level = level
		return nil
	default:
		return fmt.Errorf("invalid log level: %s", level)
	}
}

// LogrusLogger is an implementation of ILogger using the logrus package with JSON formatting
type LogrusLogger struct {
	Logger *logrus.Logger
}

func NewLogrusLogger(logger *logrus.Logger) *LogrusLogger {
	if logger == nil {
		logger = logrus.New()
		logger.SetFormatter(&logrus.JSONFormatter{}) // Use JSONFormatter for JSON logs
	}
	return &LogrusLogger{Logger: logger}
}

func (l *LogrusLogger) Debug(ctx context.Context, message string, fields map[string]interface{}) {
	l.Logger.WithFields(logrus.Fields(fields)).Debug(message)
}

func (l *LogrusLogger) Error(ctx context.Context, message string, fields map[string]interface{}) {
	l.Logger.WithFields(logrus.Fields(fields)).Error(message)
}

func (l *LogrusLogger) Info(ctx context.Context, message string, fields map[string]interface{}) {
	l.Logger.WithFields(logrus.Fields(fields)).Info(message)
}

func (l *LogrusLogger) Warning(ctx context.Context, message string, fields map[string]interface{}) {
	l.Logger.WithFields(logrus.Fields(fields)).Warning(message)
}

func (l *LogrusLogger) SetLogLevel(level string) error {
	switch strings.ToLower(level) {
	case "debug":
		l.Logger.SetLevel(logrus.DebugLevel)
	case "info":
		l.Logger.SetLevel(logrus.InfoLevel)
	case "error":
		l.Logger.SetLevel(logrus.ErrorLevel)
	case "warning":
		l.Logger.SetLevel(logrus.WarnLevel)
	default:
		return fmt.Errorf("invalid log level: %s", level)
	}
	return nil
}

// NewDynamicLogger initializes ILogger with a dynamic log level
func NewDynamicLogger(loggerType string, level string) (ILogger, error) {
	switch strings.ToLower(loggerType) {
	case "logrus":
		logrusLogger := NewLogrusLogger(nil)
		if err := logrusLogger.SetLogLevel(level); err != nil {
			return nil, err
		}
		return logrusLogger, nil
	case "log":
		logLogger := &LogLogger{}
		if err := logLogger.SetLogLevel(level); err != nil {
			return nil, err
		}
		return logLogger, nil
	default:
		return nil, fmt.Errorf("invalid logger type: %s", loggerType)
	}
}
