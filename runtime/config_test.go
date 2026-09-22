package runtime_test

import (
	"testing"

	"github.com/nyaruka/helpsites/v26/runtime"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConfig(t *testing.T) {
	cfg := runtime.NewDefaultConfig()
	assert.ErrorContains(t, cfg.Parse(), "StorageURL")

	cfg.StorageURL = "https://storage.example.com/bucket/"
	require.NoError(t, cfg.Parse())

	assert.Equal(t, "TembaCerts", cfg.CertsTable())

	cfg.DynamoTablePrefix = "Test"
	assert.Equal(t, "TestCerts", cfg.CertsTable())
}
