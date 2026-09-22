package testsuite

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log/slog"
	"os"
	"path"
	goruntime "runtime"
	"sync"
	"testing"

	"github.com/nyaruka/helpsites/runtime"
	"github.com/stretchr/testify/require"
)

// testdataPath returns the absolute path of a file in this package's testdata directory. Paths are resolved relative
// to this source file rather than the module being tested, so that they also work from modules importing this package.
func testdataPath(file string) string {
	_, thisFile, _, _ := goruntime.Caller(0)
	return path.Join(path.Dir(thisFile), "testdata", file)
}

// random per-binary identifier used in per-binary resource names. Can't use the pid because binaries run in
// per-worktree containers which share the backing services, so pids are neither unique nor observable across them.
var binProcID = sync.OnceValue(func() string {
	b := make([]byte, 4)
	rand.Read(b)
	return hex.EncodeToString(b)
})

// Runtime returns the various runtime things a test might need
func Runtime(t *testing.T) (context.Context, *runtime.Runtime) {
	// each test gets its own database cloned from a template built from our dump - see dbtemplate.go
	dbName := createTestDB(t)
	t.Cleanup(func() { dropTestDB(t, dbName) })

	// this binary's slot gives it its own valkey database and DynamoDB table - see slot.go
	slot := claimSlot(t)

	cfg := runtime.NewDefaultConfig()
	cfg.DeploymentID = "test"
	cfg.DB = fmt.Sprintf(dbTestDSNFormat, dbName)
	cfg.Valkey = fmt.Sprintf(vkTestDSNFormat, slotVKDB(slot))
	cfg.TLSMode = runtime.TLSModeSelfSigned
	cfg.DynamoTablePrefix = fmt.Sprintf("Test%d", slot)
	cfg.DynamoEndpoint = "http://dynamodb:8000"

	// AWS SDK default chain reads these - used by the DynamoDB client
	t.Setenv("AWS_ACCESS_KEY_ID", "root")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "tembatemba")
	t.Setenv("AWS_REGION", "us-east-1")
	cfg.AppHost = "app.example.com"
	cfg.PreviewToken = "sesame"

	err := cfg.Parse()
	require.NoError(t, err)

	rt, err := runtime.NewRuntime(cfg)
	require.NoError(t, err)

	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo})))

	// so every test starts with empty valkey and certificates table
	require.NoError(t, flushVKDB(slotVKDB(slot)))
	ensureDynamoTable(t, rt)

	t.Cleanup(func() { rt.Stop() })

	return t.Context(), rt
}
