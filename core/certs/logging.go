package certs

import (
	"context"
	"log/slog"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// CertMagic logs through zap, so this is a zap core which forwards to slog, so that its lines land in the same
// stream, in the same format, as everything else.
type slogCore struct {
	logger *slog.Logger
	fields []zapcore.Field
}

func newZapLogger(logger *slog.Logger) *zap.Logger {
	return zap.New(&slogCore{logger: logger})
}

func (c *slogCore) Enabled(level zapcore.Level) bool {
	return c.logger.Enabled(context.Background(), slogLevel(level))
}

func (c *slogCore) With(fields []zapcore.Field) zapcore.Core {
	return &slogCore{logger: c.logger, fields: append(append([]zapcore.Field{}, c.fields...), fields...)}
}

func (c *slogCore) Check(entry zapcore.Entry, checked *zapcore.CheckedEntry) *zapcore.CheckedEntry {
	if c.Enabled(entry.Level) {
		return checked.AddCore(entry, c)
	}
	return checked
}

func (c *slogCore) Write(entry zapcore.Entry, fields []zapcore.Field) error {
	enc := zapcore.NewMapObjectEncoder()
	for _, f := range c.fields {
		f.AddTo(enc)
	}
	for _, f := range fields {
		f.AddTo(enc)
	}

	attrs := make([]any, 0, len(enc.Fields)*2+2)
	attrs = append(attrs, "comp", "certmagic")
	for k, v := range enc.Fields {
		attrs = append(attrs, k, v)
	}

	c.logger.Log(context.Background(), slogLevel(entry.Level), entry.Message, attrs...)
	return nil
}

func (c *slogCore) Sync() error { return nil }

func slogLevel(level zapcore.Level) slog.Level {
	switch {
	case level >= zapcore.ErrorLevel:
		return slog.LevelError
	case level == zapcore.WarnLevel:
		return slog.LevelWarn
	case level == zapcore.InfoLevel:
		return slog.LevelInfo
	default:
		return slog.LevelDebug
	}
}
