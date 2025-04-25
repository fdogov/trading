package logger

import (
	"context"
	"sync"

	"github.com/fdogov/trading/internal/config"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

const (
	// TracingKey ключ для трейсинга
	TracingKey          = "trace_id"
	tracingIDContextKey = "trace_id_ctx_key"
	spanIDContextKey    = "span_id_ctx_key"
	userIDContextKey    = "user_id_ctx_key"
)

var (
	log      *zap.Logger
	logMutex sync.Mutex // Для безопасной ленивой инициализации
)

// InitLogger инициализирует глобальный логгер
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

// SetLogger устанавливает внешний экземпляр логгера (полезно для тестов)
func SetLogger(logger *zap.Logger) {
	logMutex.Lock()
	defer logMutex.Unlock()
	log = logger
}

// ensureLogger гарантирует, что логгер инициализирован
func ensureLogger() {
	logMutex.Lock()
	defer logMutex.Unlock()

	if log == nil {
		// Создаем простой логгер для тестов, который выводит в консоль
		config := zap.NewDevelopmentConfig()
		config.Level = zap.NewAtomicLevelAt(zapcore.InfoLevel)
		logger, _ := config.Build()
		log = logger
	}
}

// Get возвращает глобальный логгер
func Get() *zap.Logger {
	ensureLogger()
	return log
}

// With создаёт новый логгер с указанными полями
func With(fields ...zap.Field) *zap.Logger {
	ensureLogger()
	return log.With(fields...)
}

// FromContext возвращает логгер с информацией из контекста (trace ID, span ID и т.д.)
func FromContext(ctx context.Context) *zap.Logger {
	ensureLogger()

	logger := log

	if ctx == nil {
		return logger
	}

	// Добавляем trace ID, если он есть в контексте
	if traceID := TraceIDFromContext(ctx); traceID != "" {
		logger = logger.With(zap.String(TracingKey, traceID))
	}

	// Добавляем span ID, если он есть в контексте
	if spanID := SpanIDFromContext(ctx); spanID != "" {
		logger = logger.With(zap.String("span_id", spanID))
	}

	// Добавляем user ID, если он есть в контексте
	if userID := UserIDFromContext(ctx); userID != "" {
		logger = logger.With(zap.String("user_id", userID))
	}

	return logger
}

// ContextWithTraceID добавляет trace ID в контекст
func ContextWithTraceID(ctx context.Context, traceID string) context.Context {
	return context.WithValue(ctx, tracingIDContextKey, traceID)
}

// TraceIDFromContext извлекает trace ID из контекста
func TraceIDFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	if traceID, ok := ctx.Value(tracingIDContextKey).(string); ok {
		return traceID
	}
	return ""
}

// ContextWithSpanID добавляет span ID в контекст
func ContextWithSpanID(ctx context.Context, spanID string) context.Context {
	return context.WithValue(ctx, spanIDContextKey, spanID)
}

// SpanIDFromContext извлекает span ID из контекста
func SpanIDFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	if spanID, ok := ctx.Value(spanIDContextKey).(string); ok {
		return spanID
	}
	return ""
}

// ContextWithUserID добавляет user ID в контекст
func ContextWithUserID(ctx context.Context, userID string) context.Context {
	return context.WithValue(ctx, userIDContextKey, userID)
}

// UserIDFromContext извлекает user ID из контекста
func UserIDFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	if userID, ok := ctx.Value(userIDContextKey).(string); ok {
		return userID
	}
	return ""
}

// Debug логирует сообщение с уровнем Debug, используя контекст
func Debug(ctx context.Context, msg string, fields ...zap.Field) {
	FromContext(ctx).Debug(msg, fields...)
}

// Info логирует сообщение с уровнем Info, используя контекст
func Info(ctx context.Context, msg string, fields ...zap.Field) {
	FromContext(ctx).Info(msg, fields...)
}

// Warn логирует сообщение с уровнем Warn, используя контекст
func Warn(ctx context.Context, msg string, fields ...zap.Field) {
	FromContext(ctx).Warn(msg, fields...)
}

// Error логирует сообщение с уровнем Error, используя контекст
func Error(ctx context.Context, msg string, fields ...zap.Field) {
	FromContext(ctx).Error(msg, fields...)
}

// Fatal логирует сообщение с уровнем Fatal, используя контекст
func Fatal(ctx context.Context, msg string, fields ...zap.Field) {
	FromContext(ctx).Fatal(msg, fields...)
}
