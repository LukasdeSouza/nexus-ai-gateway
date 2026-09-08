// Package observability provides structured logging, Prometheus metrics, and OpenTelemetry tracing.
package observability

import (
	"context"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

type loggerContextKey struct{}

var globalLogger *zap.Logger

func init() {
	globalLogger = zap.NewNop()
}

// NewLogger creates a zap.Logger configured for the runtime environment.
func NewLogger(levelStr, env string) (*zap.Logger, error) {
	var level zapcore.Level
	if err := level.UnmarshalText([]byte(levelStr)); err != nil {
		level = zapcore.InfoLevel
	}

	var zapCfg zap.Config
	if env == "local" || env == "development" {
		zapCfg = zap.NewDevelopmentConfig()
		zapCfg.EncoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
	} else {
		zapCfg = zap.NewProductionConfig()
		zapCfg.EncoderConfig.TimeKey = "timestamp"
		zapCfg.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	}

	zapCfg.Level = zap.NewAtomicLevelAt(level)
	zapCfg.DisableStacktrace = true

	logger, err := zapCfg.Build()
	if err != nil {
		return nil, err
	}

	globalLogger = logger
	return logger, nil
}

// WithLogger returns a new context with the logger embedded.
func WithLogger(ctx context.Context, logger *zap.Logger) context.Context {
	return context.WithValue(ctx, loggerContextKey{}, logger)
}

// FromContext extracts the logger from context, or returns globalLogger.
func FromContext(ctx context.Context) *zap.Logger {
	if ctx != nil {
		if l, ok := ctx.Value(loggerContextKey{}).(*zap.Logger); ok && l != nil {
			return l
		}
	}
	return globalLogger
}

// WithRequestID adds request_id field to logger.
func WithRequestID(logger *zap.Logger, requestID string) *zap.Logger {
	return logger.With(zap.String("request_id", requestID))
}

// WithProjectID adds project_id field to logger.
func WithProjectID(logger *zap.Logger, projectID string) *zap.Logger {
	return logger.With(zap.String("project_id", projectID))
}

// WithProvider adds provider field to logger.
func WithProvider(logger *zap.Logger, provider string) *zap.Logger {
	return logger.With(zap.String("provider", provider))
}
