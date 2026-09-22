package cmd

import (
	"context"
	"log/slog"
	"sync"
	"testing"

	"github.com/nyaruka/helpsites/v26/runtime"
	"github.com/nyaruka/helpsites/v26/testsuite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTestConnections(t *testing.T) {
	_, rt := testsuite.Runtime(t)

	assert.Equal(t, []string{"db ok", "valkey ok"}, captureLogs(func() { testConnections(rt) }))
}

func TestTestConnectionsUnreachable(t *testing.T) {
	cfg := runtime.NewDefaultConfig()
	cfg.DB = "postgres://temba:temba@127.0.0.1:1/temba?sslmode=disable"
	cfg.Valkey = "valkey://127.0.0.1:1/0"
	require.NoError(t, cfg.Parse())

	rt, err := runtime.NewRuntime(cfg)
	require.NoError(t, err)
	defer rt.Stop()

	assert.Equal(t, []string{"db not reachable", "valkey not reachable"}, captureLogs(func() { testConnections(rt) }))
}

// captureLogs returns the messages logged while fn runs
func captureLogs(fn func()) []string {
	h := &capturingHandler{}

	prev := slog.Default()
	slog.SetDefault(slog.New(h))
	defer slog.SetDefault(prev)

	fn()

	return h.msgs
}

type capturingHandler struct {
	mu   sync.Mutex
	msgs []string
}

func (h *capturingHandler) Enabled(context.Context, slog.Level) bool { return true }

func (h *capturingHandler) Handle(_ context.Context, r slog.Record) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.msgs = append(h.msgs, r.Message)
	return nil
}

func (h *capturingHandler) WithAttrs([]slog.Attr) slog.Handler { return h }
func (h *capturingHandler) WithGroup(string) slog.Handler      { return h }
