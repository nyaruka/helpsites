package runtime_test

import (
	"testing"

	"github.com/nyaruka/helpsites/runtime"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConfig(t *testing.T) {
	cfg := runtime.NewDefaultConfig()
	require.NoError(t, cfg.Parse())

	assert.Equal(t, "TembaCerts", cfg.CertsTable())
	assert.True(t, cfg.UsesDynamo())

	cfg.DynamoTablePrefix = "Test"
	assert.Equal(t, "TestCerts", cfg.CertsTable())

	cfg.CertsStorage = runtime.CertsStorageDisk
	require.NoError(t, cfg.Parse())
	assert.False(t, cfg.UsesDynamo())

	// self-signed mode never touches storage
	cfg.CertsStorage = runtime.CertsStorageDynamo
	cfg.TLSMode = runtime.TLSModeSelfSigned
	require.NoError(t, cfg.Parse())
	assert.False(t, cfg.UsesDynamo())

	cfg.CertsStorage = "s3"
	assert.Error(t, cfg.Parse())
}
