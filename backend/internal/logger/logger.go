// Package logger provides structured logging for the application using Go's slog package.
// It supports request ID correlation, log levels, and JSON output for production.
package logger

import (
	"context"
	"log/slog"
	"os"
	"sync"
)

// contextKey is used to store logger in context
type contextKey string

const (
	// LoggerContextKey is the key used to store the logger in context
	LoggerContextKey contextKey = "logger"
	// RequestIDContextKey is the key used to store the request ID in context
	RequestIDContextKey contextKey = "request_id"
)

var (
	defaultLogger *slog.Logger
	once          sync.Once
)

// Init initializes the global logger with the specified configuration.
// If json is true, logs are output in JSON format (recommended for production).
// If debug is true, DEBUG level logs are included.
func Init(json bool, debug bool) {
	once.Do(func() {
		var handler slog.Handler
		level := slog.LevelInfo
		if debug {
			level = slog.LevelDebug
		}

		opts := &slog.HandlerOptions{
			Level:     level,
			AddSource: true,
		}

		if json {
			handler = slog.NewJSONHandler(os.Stdout, opts)
		} else {
			handler = slog.NewTextHandler(os.Stdout, opts)
		}

		defaultLogger = slog.New(handler)
		slog.SetDefault(defaultLogger)
	})
}

// Default returns the default logger instance.
// If Init hasn't been called, it initializes with development defaults.
func Default() *slog.Logger {
	if defaultLogger == nil {
		Init(false, true) // Default to text output with debug enabled
	}
	return defaultLogger
}

// WithRequestID returns a new logger with the request ID attached.
func WithRequestID(logger *slog.Logger, requestID string) *slog.Logger {
	return logger.With(slog.String("request_id", requestID))
}

// NewContext returns a new context with the logger attached.
func NewContext(ctx context.Context, logger *slog.Logger) context.Context {
	return context.WithValue(ctx, LoggerContextKey, logger)
}

// FromContext returns the logger from the context.
// If no logger is found, returns the default logger.
func FromContext(ctx context.Context) *slog.Logger {
	if logger, ok := ctx.Value(LoggerContextKey).(*slog.Logger); ok {
		return logger
	}
	return Default()
}

// SetRequestID stores the request ID in the context.
func SetRequestID(ctx context.Context, requestID string) context.Context {
	return context.WithValue(ctx, RequestIDContextKey, requestID)
}

// GetRequestID retrieves the request ID from the context.
func GetRequestID(ctx context.Context) string {
	if id, ok := ctx.Value(RequestIDContextKey).(string); ok {
		return id
	}
	return ""
}

// Auth-specific logging helpers

// AuthInfo logs an authentication event at INFO level.
func AuthInfo(ctx context.Context, msg string, attrs ...any) {
	logger := FromContext(ctx)
	requestID := GetRequestID(ctx)
	if requestID != "" {
		attrs = append(attrs, slog.String("request_id", requestID))
	}
	logger.Info(msg, attrs...)
}

// AuthWarn logs an authentication warning at WARN level.
func AuthWarn(ctx context.Context, msg string, attrs ...any) {
	logger := FromContext(ctx)
	requestID := GetRequestID(ctx)
	if requestID != "" {
		attrs = append(attrs, slog.String("request_id", requestID))
	}
	logger.Warn(msg, attrs...)
}

// AuthError logs an authentication error at ERROR level.
func AuthError(ctx context.Context, msg string, err error, attrs ...any) {
	logger := FromContext(ctx)
	requestID := GetRequestID(ctx)
	attrs = append(attrs, slog.Any("error", err))
	if requestID != "" {
		attrs = append(attrs, slog.String("request_id", requestID))
	}
	logger.Error(msg, attrs...)
}

// AuthDebug logs an authentication debug message at DEBUG level.
func AuthDebug(ctx context.Context, msg string, attrs ...any) {
	logger := FromContext(ctx)
	requestID := GetRequestID(ctx)
	if requestID != "" {
		attrs = append(attrs, slog.String("request_id", requestID))
	}
	logger.Debug(msg, attrs...)
}

// HashEmail creates a simple hash representation of an email for logging.
// This avoids logging the full email while still allowing correlation.
func HashEmail(email string) string {
	if len(email) < 3 {
		return "***"
	}
	// Show first 2 chars and domain
	atIndex := -1
	for i, c := range email {
		if c == '@' {
			atIndex = i
			break
		}
	}
	if atIndex > 2 {
		return email[:2] + "***" + email[atIndex:]
	}
	return email[:2] + "***"
}
