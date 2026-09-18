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

	cfg.DynamoTablePrefix = "Test"
	assert.Equal(t, "TestCerts", cfg.CertsTable())
}
