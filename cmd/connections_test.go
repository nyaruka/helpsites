package cmd

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/nyaruka/helpsites/runtime"
	"github.com/nyaruka/helpsites/testsuite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTestConnections(t *testing.T) {
	_, rt := testsuite.Runtime(t)

	mailroom := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Write([]byte(`{"component": "mailroom"}`))
	}))
	defer mailroom.Close()

	broken := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer broken.Close()

	// self signing and no mailroom, so neither is checked
	assert.Equal(t, []string{"db ok", "valkey ok"}, captureLogs(func() { testConnections(rt) }))

	// with ACME we check the certificates table, and we check mailroom whenever it's configured
	testsuite.EnsureDynamoTable(t, rt)
	rt.Config.TLSMode = runtime.TLSModeACME
	rt.Config.MailroomURL = mailroom.URL

	assert.Equal(t, []string{"db ok", "valkey ok", "dynamodb ok", "mailroom ok"},
		captureLogs(func() { testConnections(rt) }))

	// a table we can't reach and a mailroom that won't answer are logged, and we still check everything else
	rt.Config.DynamoTablePrefix = "Nonexistent"
	rt.Config.MailroomURL = broken.URL

	assert.Equal(t, []string{"db ok", "valkey ok", "dynamodb not reachable", "mailroom not reachable"},
		captureLogs(func() { testConnections(rt) }))
}

func TestTestConnectionsUnreachable(t *testing.T) {
	cfg := runtime.NewDefaultConfig()
	cfg.DB = "postgres://temba:temba@127.0.0.1:1/temba?sslmode=disable"
	cfg.Valkey = "valkey://127.0.0.1:1/0"
	cfg.TLSMode = runtime.TLSModeSelfSigned // so we don't wait on the AWS SDK retrying an unreachable endpoint
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
