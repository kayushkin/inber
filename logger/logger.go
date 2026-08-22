// Package logger provides structured JSON logging for inber with context support.
package logger

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"time"
)

// Level represents log severity levels.
type Level int

const (
	DebugLevel Level = iota
	InfoLevel
	WarnLevel
	ErrorLevel
)

func (l Level) String() string {
	switch l {
	case DebugLevel:
		return "debug"
	case InfoLevel:
		return "info"
	case WarnLevel:
		return "warn"
	case ErrorLevel:
		return "error"
	default:
		return "unknown"
	}
}

// Logger provides structured logging with JSON output.
type Logger struct {
	writer io.Writer
	level  Level
}

// New creates a new logger with the specified minimum level.
func New(level Level) *Logger {
	return &Logger{
		writer: os.Stdout,
		level:  level,
	}
}

// NewWithWriter creates a logger with a custom writer.
func NewWithWriter(writer io.Writer, level Level) *Logger {
	return &Logger{
		writer: writer,
		level:  level,
	}
}

// LogEntry represents a structured log entry.
type LogEntry struct {
	Timestamp string                 `json:"timestamp"`
	Level     string                 `json:"level"`
	Message   string                 `json:"message"`
	Component string                 `json:"component,omitempty"`
	SessionID string                 `json:"session_id,omitempty"`
	UserID    string                 `json:"user_id,omitempty"`
	Error     string                 `json:"error,omitempty"`
	Fields    map[string]interface{} `json:"fields,omitempty"`
}

// log writes a log entry if it meets the minimum level requirement.
func (l *Logger) log(level Level, msg string, fields map[string]interface{}) {
	if level < l.level {
		return
	}

	entry := LogEntry{
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Level:     level.String(),
		Message:   msg,
		Fields:    fields,
	}

	// Extract common fields from context
	if ctx, ok := fields["context"].(context.Context); ok {
		delete(fields, "context")
		
		if sessionID := ctx.Value("session_id"); sessionID != nil {
			if s, ok := sessionID.(string); ok {
				entry.SessionID = s
			}
		}
		
		if userID := ctx.Value("user_id"); userID != nil {
			if u, ok := userID.(string); ok {
				entry.UserID = u
			}
		}
		
		if component := ctx.Value("component"); component != nil {
			if c, ok := component.(string); ok {
				entry.Component = c
			}
		}
	}

	// Extract error if present
	if err, ok := fields["error"].(error); ok {
		entry.Error = err.Error()
		delete(fields, "error")
	}

	// Extract component if present
	if component, ok := fields["component"].(string); ok {
		entry.Component = component
		delete(fields, "component")
	}

	// Only include fields if not empty
	if len(fields) == 0 {
		entry.Fields = nil
	}

	data, _ := json.Marshal(entry)
	fmt.Fprintln(l.writer, string(data))
}

// Debug logs a debug message.
func (l *Logger) Debug(msg string, fields ...map[string]interface{}) {
	f := mergeFields(fields...)
	l.log(DebugLevel, msg, f)
}

// Info logs an info message.
func (l *Logger) Info(msg string, fields ...map[string]interface{}) {
	f := mergeFields(fields...)
	l.log(InfoLevel, msg, f)
}

// Warn logs a warning message.
func (l *Logger) Warn(msg string, fields ...map[string]interface{}) {
	f := mergeFields(fields...)
	l.log(WarnLevel, msg, f)
}

// Error logs an error message.
func (l *Logger) Error(msg string, fields ...map[string]interface{}) {
	f := mergeFields(fields...)
	l.log(ErrorLevel, msg, f)
}

// WithContext creates a logger that will extract context information.
func (l *Logger) WithContext(ctx context.Context) *ContextLogger {
	return &ContextLogger{
		logger: l,
		ctx:    ctx,
	}
}

// WithComponent creates a logger that will include the component name.
func (l *Logger) WithComponent(component string) *ComponentLogger {
	return &ComponentLogger{
		logger:    l,
		component: component,
	}
}

// ContextLogger wraps a logger with context information.
type ContextLogger struct {
	logger *Logger
	ctx    context.Context
}

// Debug logs a debug message with context.
func (cl *ContextLogger) Debug(msg string, fields ...map[string]interface{}) {
	f := mergeFields(fields...)
	f["context"] = cl.ctx
	cl.logger.log(DebugLevel, msg, f)
}

// Info logs an info message with context.
func (cl *ContextLogger) Info(msg string, fields ...map[string]interface{}) {
	f := mergeFields(fields...)
	f["context"] = cl.ctx
	cl.logger.log(InfoLevel, msg, f)
}

// Warn logs a warning message with context.
func (cl *ContextLogger) Warn(msg string, fields ...map[string]interface{}) {
	f := mergeFields(fields...)
	f["context"] = cl.ctx
	cl.logger.log(WarnLevel, msg, f)
}

// Error logs an error message with context.
func (cl *ContextLogger) Error(msg string, fields ...map[string]interface{}) {
	f := mergeFields(fields...)
	f["context"] = cl.ctx
	cl.logger.log(ErrorLevel, msg, f)
}

// ComponentLogger wraps a logger with component information.
type ComponentLogger struct {
	logger    *Logger
	component string
}

// Debug logs a debug message with component.
func (cl *ComponentLogger) Debug(msg string, fields ...map[string]interface{}) {
	f := mergeFields(fields...)
	f["component"] = cl.component
	cl.logger.log(DebugLevel, msg, f)
}

// Info logs an info message with component.
func (cl *ComponentLogger) Info(msg string, fields ...map[string]interface{}) {
	f := mergeFields(fields...)
	f["component"] = cl.component
	cl.logger.log(InfoLevel, msg, f)
}

// Warn logs a warning message with component.
func (cl *ComponentLogger) Warn(msg string, fields ...map[string]interface{}) {
	f := mergeFields(fields...)
	f["component"] = cl.component
	cl.logger.log(WarnLevel, msg, f)
}

// Error logs an error message with component.
func (cl *ComponentLogger) Error(msg string, fields ...map[string]interface{}) {
	f := mergeFields(fields...)
	f["component"] = cl.component
	cl.logger.log(ErrorLevel, msg, f)
}

// mergeFields combines multiple field maps.
func mergeFields(fields ...map[string]interface{}) map[string]interface{} {
	result := make(map[string]interface{})
	for _, f := range fields {
		for k, v := range f {
			result[k] = v
		}
	}
	return result
}

// Global logger instance
var defaultLogger = New(InfoLevel)

// SetDefaultLogger installs l as the logger behind the package-level Debug,
// Info, Warn, Error, WithContext and WithComponent functions, and returns the
// call that puts the previous one back.
//
// It exists so that a caller in another package can read what it logged. A log
// line is the whole repair for a class of defect on this box — a divergence, a
// discarded value, a failure that was survivable in silence — and a repair
// nothing can observe is one no test holds. Pair it with NewWithWriter:
//
//	var buf bytes.Buffer
//	defer logger.SetDefaultLogger(logger.NewWithWriter(&buf, logger.InfoLevel))()
//
// It swaps a package-level variable, so a test using it must not run in
// parallel with another that logs.
func SetDefaultLogger(l *Logger) (restore func()) {
	previous := defaultLogger
	defaultLogger = l
	return func() { defaultLogger = previous }
}

// Package-level convenience functions that use the default logger

// SetLevel sets the global logger level.
func SetLevel(level Level) {
	defaultLogger.level = level
}

// Debug logs a debug message using the default logger.
func Debug(msg string, fields ...map[string]interface{}) {
	defaultLogger.Debug(msg, fields...)
}

// Info logs an info message using the default logger.
func Info(msg string, fields ...map[string]interface{}) {
	defaultLogger.Info(msg, fields...)
}

// Warn logs a warning message using the default logger.
func Warn(msg string, fields ...map[string]interface{}) {
	defaultLogger.Warn(msg, fields...)
}

// Error logs an error message using the default logger.
func Error(msg string, fields ...map[string]interface{}) {
	defaultLogger.Error(msg, fields...)
}

// WithContext creates a context logger using the default logger.
func WithContext(ctx context.Context) *ContextLogger {
	return defaultLogger.WithContext(ctx)
}

// WithComponent creates a component logger using the default logger.
func WithComponent(component string) *ComponentLogger {
	return defaultLogger.WithComponent(component)
}