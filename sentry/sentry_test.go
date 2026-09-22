package sentry_test

import (
	"io"
	"log/slog"
	"reflect"
	"testing"

	"github.com/nyaruka/helpsites/runtime"
	"github.com/nyaruka/helpsites/sentry"
	"github.com/stretchr/testify/assert"
)

func TestInit(t *testing.T) {
	base := slog.NewTextHandler(io.Discard, nil)
	panicHandler := reflect.ValueOf(runtime.PanicHandler).Pointer()

	handler, err := sentry.Init("", base, "1.2.3")
	assert.NoError(t, err)
	assert.Same(t, base, handler, "base handler should be returned unchanged")
	assert.Equal(t, panicHandler, reflect.ValueOf(runtime.PanicHandler).Pointer(), "panic handler should be untouched")
}

func TestFlush(t *testing.T) {
	sentry.Flush() // should be a no-op when reporting isn't configured
}
