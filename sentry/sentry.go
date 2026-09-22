// Package sentry configures reporting of errors and panics to Sentry.
package sentry

import (
	"log/slog"
	"maps"
	"runtime/debug"
	"slices"
	"time"

	sentrygo "github.com/getsentry/sentry-go"
	"github.com/nyaruka/helpsites/runtime"
	slogmulti "github.com/samber/slog-multi"
	slogsentry "github.com/samber/slog-sentry/v2"
)

// Init configures error reporting to the given DSN, returning a log handler which fans out error level logging to
// Sentry as well as the given base handler. If the DSN is empty then reporting is disabled and the base handler is
// returned unchanged.
func Init(dsn string, base slog.Handler, version string) (slog.Handler, error) {
	if dsn == "" {
		return base, nil
	}

	if err := sentrygo.Init(sentrygo.ClientOptions{Dsn: dsn, Release: version, AttachStacktrace: true}); err != nil {
		return nil, err
	}

	// panics are recovered where they happen and reach us via the panic handler rather than the logger - log them
	// via the base handler only so the fanout below doesn't turn the log record into a second Sentry event
	logger := slog.New(base)
	runtime.PanicHandler = func(val any, tags map[string]string) { report(logger, val, tags) }

	return slogmulti.Fanout(base, slogsentry.Option{Level: slog.LevelError}.NewSentryHandler()), nil
}

// reports a recovered panic to Sentry, with its tags as Sentry tags, as well as logging it via the given logger
func report(logger *slog.Logger, val any, tags map[string]string) {
	attrs := make([]any, 0, len(tags)*2+4)
	for _, k := range slices.Sorted(maps.Keys(tags)) {
		attrs = append(attrs, k, tags[k])
	}
	attrs = append(attrs, "panic", val, "stack", string(debug.Stack()))

	logger.Error("recovered from panic", attrs...)

	// clone the hub so that concurrent panics don't share a scope stack
	hub := sentrygo.CurrentHub().Clone()
	hub.WithScope(func(scope *sentrygo.Scope) {
		scope.SetTags(tags)
		hub.Recover(val)
	})
}

// Flush sends any pending events before exiting. It's a no-op if error reporting isn't configured.
func Flush() {
	sentrygo.Flush(2 * time.Second)
}
