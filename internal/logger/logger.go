package logger

import (
	"context"
	"sync"

	"github.com/fdogov/trading/internal/config"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

const (
	// TracingKey key for tracing
	TracingKey          = "trace_id"
	tracingIDContextKey = "trace_id_ctx_key"
	spanIDContextKey    = "span_id_ctx_key"
	userIDContextKey    = "user_id_ctx_key"
)

var (
	log      *zap.Logger
	logMutex sync.Mutex // For safe lazy initialization
)

// InitLogger initializes the global logger
func InitLogger(cfg config.Logger) (*zap.Logger, error) {
	var level zapcore.Level
	switch cfg.Level {
	case "debug":
		level = zapcore.DebugLevel
	case "info":
		level = zapcore.InfoLevel
	case "warn":
		level = zapcore.WarnLevel
	case "error":
		level = zapcore.ErrorLevel
	default:
		level = zapcore.InfoLevel
	}

	encoderConfig := zap.NewProductionEncoderConfig()
	encoderConfig.TimeKey = "timestamp"
	encoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder

	var encoding string
	if cfg.Encoding == "console" {
		encoding = "console"
	} else {
		encoding = "json"
	}

	config := zap.Config{
		Level:       zap.NewAtomicLevelAt(level),
		Development: false,
		Sampling: &zap.SamplingConfig{
			Initial:    100,
			Thereafter: 100,
		},
		Encoding:         encoding,
		EncoderConfig:    encoderConfig,
		OutputPaths:      []string{cfg.OutputPath},
		ErrorOutputPaths: []string{cfg.OutputPath},
	}

	logger, err := config.Build()
	if err != nil {
		return nil, err
	}

	logMutex.Lock()
	log = logger
	logMutex.Unlock()

	return logger, nil
}

// SetLogger sets an external logger instance (useful for tests)
func SetLogger(logger *zap.Logger) {
	logMutex.Lock()
	defer logMutex.Unlock()
	log = logger
}

// ensureLogger ensures the logger is initialized
func ensureLogger() {
	logMutex.Lock()
	defer logMutex.Unlock()

	if log == nil {
		// Create a simple logger for tests that outputs to console
		config := zap.NewDevelopmentConfig()
		config.Level = zap.NewAtomicLevelAt(zapcore.InfoLevel)
		logger, _ := config.Build()
		log = logger
	}
}

// Get returns the global logger
func Get() *zap.Logger {
	ensureLogger()
	return log
}

// With creates a new logger with the specified fields
func With(fields ...zap.Field) *zap.Logger {
	ensureLogger()
	return log.With(fields...)
}

// FromContext returns a logger with information from the context (trace ID, span ID, etc.)
func FromContext(ctx context.Context) *zap.Logger {
	ensureLogger()

	logger := log

	if ctx == nil {
		return logger
	}

	// Add trace ID if it exists in the context
	if traceID := TraceIDFromContext(ctx); traceID != "" {
		logger = logger.With(zap.String(TracingKey, traceID))
	}

	// Add span ID if it exists in the context
	if spanID := SpanIDFromContext(ctx); spanID != "" {
		logger = logger.With(zap.String("span_id", spanID))
	}

	// Add user ID if it exists in the context
	if userID := UserIDFromContext(ctx); userID != "" {
		logger = logger.With(zap.String("user_id", userID))
	}

	return logger
}

// ContextWithTraceID adds trace ID to the context
func ContextWithTraceID(ctx context.Context, traceID string) context.Context {
	return context.WithValue(ctx, tracingIDContextKey, traceID)
}

// TraceIDFromContext extracts trace ID from the context
func TraceIDFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	if traceID, ok := ctx.Value(tracingIDContextKey).(string); ok {
		return traceID
	}
	return ""
}

// ContextWithSpanID adds span ID to the context
func ContextWithSpanID(ctx context.Context, spanID string) context.Context {
	return context.WithValue(ctx, spanIDContextKey, spanID)
}

// SpanIDFromContext extracts span ID from the context
func SpanIDFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	if spanID, ok := ctx.Value(spanIDContextKey).(string); ok {
		return spanID
	}
	return ""
}

// ContextWithUserID adds user ID to the context
func ContextWithUserID(ctx context.Context, userID string) context.Context {
	return context.WithValue(ctx, userIDContextKey, userID)
}

// UserIDFromContext extracts user ID from the context
func UserIDFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	if userID, ok := ctx.Value(userIDContextKey).(string); ok {
		return userID
	}
	return ""
}

// Debug logs a message with Debug level, using context
func Debug(ctx context.Context, msg string, fields ...zap.Field) {
	FromContext(ctx).Debug(msg, fields...)
}

// Info logs a message with Info level, using context
func Info(ctx context.Context, msg string, fields ...zap.Field) {
	FromContext(ctx).Info(msg, fields...)
}

// Warn logs a message with Warn level, using context
func Warn(ctx context.Context, msg string, fields ...zap.Field) {
	FromContext(ctx).Warn(msg, fields...)
}

// Error logs a message with Error level, using context
func Error(ctx context.Context, msg string, fields ...zap.Field) {
	FromContext(ctx).Error(msg, fields...)
}

// Fatal logs a message with Fatal level, using context
func Fatal(ctx context.Context, msg string, fields ...zap.Field) {
	FromContext(ctx).Fatal(msg, fields...)
}
