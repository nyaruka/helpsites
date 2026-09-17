package certs_test

import (
	"context"
	"io/fs"
	"testing"
	"time"

	"github.com/nyaruka/helpsites/core/certs"
	"github.com/nyaruka/helpsites/testsuite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDynamoStorage(t *testing.T) {
	ctx, rt := testsuite.Runtime(t)
	testsuite.EnsureDynamoTable(t, rt)

	s := certs.NewDynamoStorage(rt.Dynamo, rt.Config.DynamoTable)

	// nothing there to start with
	_, err := s.Load(ctx, "certs/example.com/cert.pem")
	assert.ErrorIs(t, err, fs.ErrNotExist)
	assert.False(t, s.Exists(ctx, "certs/example.com/cert.pem"))
	assert.False(t, s.Exists(ctx, "certs"))
	_, err = s.Stat(ctx, "certs")
	assert.ErrorIs(t, err, fs.ErrNotExist)

	require.NoError(t, s.Store(ctx, "certs/example.com/cert.pem", []byte("CERT")))
	require.NoError(t, s.Store(ctx, "certs/example.com/key.pem", []byte("KEY")))
	require.NoError(t, s.Store(ctx, "certs/other.com/cert.pem", []byte("CERT2")))
	require.NoError(t, s.Store(ctx, "acme/account.json", []byte("{}")))

	val, err := s.Load(ctx, "certs/example.com/cert.pem")
	require.NoError(t, err)
	assert.Equal(t, []byte("CERT"), val)

	// overwriting
	require.NoError(t, s.Store(ctx, "certs/example.com/cert.pem", []byte("CERT-NEW")))
	val, err = s.Load(ctx, "certs/example.com/cert.pem")
	require.NoError(t, err)
	assert.Equal(t, []byte("CERT-NEW"), val)

	// files and directories exist
	assert.True(t, s.Exists(ctx, "certs/example.com/cert.pem"))
	assert.True(t, s.Exists(ctx, "certs/example.com"))
	assert.True(t, s.Exists(ctx, "certs"))
	assert.False(t, s.Exists(ctx, "cert"))
	assert.False(t, s.Exists(ctx, "certs/example.co"))

	info, err := s.Stat(ctx, "certs/example.com/cert.pem")
	require.NoError(t, err)
	assert.Equal(t, "certs/example.com/cert.pem", info.Key)
	assert.True(t, info.IsTerminal)
	assert.Equal(t, int64(8), info.Size)
	assert.WithinDuration(t, time.Now(), info.Modified, 10*time.Second)

	info, err = s.Stat(ctx, "certs/example.com")
	require.NoError(t, err)
	assert.False(t, info.IsTerminal)

	// listing, recursive and not
	keys, err := s.List(ctx, "certs", false)
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"certs/example.com", "certs/other.com"}, keys)

	keys, err = s.List(ctx, "certs", true)
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"certs/example.com/cert.pem", "certs/example.com/key.pem", "certs/other.com/cert.pem"}, keys)

	keys, err = s.List(ctx, "nothing", true)
	require.NoError(t, err)
	assert.Empty(t, keys)

	// deleting a file, and a directory
	require.NoError(t, s.Delete(ctx, "certs/other.com/cert.pem"))
	assert.False(t, s.Exists(ctx, "certs/other.com/cert.pem"))
	assert.False(t, s.Exists(ctx, "certs/other.com"))

	require.NoError(t, s.Delete(ctx, "certs/example.com"))
	assert.False(t, s.Exists(ctx, "certs/example.com/cert.pem"))
	assert.False(t, s.Exists(ctx, "certs"))
	assert.True(t, s.Exists(ctx, "acme/account.json"))

	// locking: a second attempt on a held lock waits until it's released or its context is done
	require.NoError(t, s.Lock(ctx, "issue_example.com"))

	waitCtx, cancel := context.WithTimeout(ctx, 1500*time.Millisecond)
	defer cancel()
	assert.ErrorIs(t, s.Lock(waitCtx, "issue_example.com"), context.DeadlineExceeded)

	require.NoError(t, s.Lock(ctx, "issue_other.com")) // a different lock is fine

	require.NoError(t, s.Unlock(ctx, "issue_example.com"))
	require.NoError(t, s.Lock(ctx, "issue_example.com"))
	require.NoError(t, s.Unlock(ctx, "issue_example.com"))
	require.NoError(t, s.Unlock(ctx, "issue_other.com"))
}
