package certs_test

import (
	"crypto/tls"
	"crypto/x509"
	"testing"

	"github.com/nyaruka/helpsites/core/certs"
	"github.com/nyaruka/helpsites/runtime"
	"github.com/nyaruka/helpsites/testsuite"
	"github.com/nyaruka/helpsites/testsuite/testdb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestManager(t *testing.T) {
	ctx, rt := testsuite.Runtime(t)

	testdb.InsertSite(t, rt, testdb.Org1, "Help", testdb.SiteOptions{Domain: "help.nyaruka.com", Verified: true})
	testdb.InsertSite(t, rt, testdb.Org2, "Other", testdb.SiteOptions{Domain: "help.other.com"}) // not verified

	mgr, err := certs.NewManager(rt)
	require.NoError(t, err)
	require.NoError(t, mgr.Start())
	defer mgr.Stop()

	assert.Equal(t, []string{"help.nyaruka.com"}, mgr.Domains())
	assert.True(t, mgr.Allowed("help.nyaruka.com"))
	assert.True(t, mgr.Allowed("WWW.help.nyaruka.com"))
	assert.False(t, mgr.Allowed("help.other.com"))
	assert.False(t, mgr.Allowed("nyaruka.com"))
	assert.False(t, mgr.Allowed(""))

	// in self-signed mode, a certificate is signed for a name we allow and refused for one we don't
	tc := mgr.TLSConfig()
	assert.Contains(t, tc.NextProtos, "h2")
	assert.Contains(t, tc.NextProtos, "http/1.1")

	cert, err := tc.GetCertificate(&tls.ClientHelloInfo{ServerName: "www.help.nyaruka.com"})
	require.NoError(t, err)
	require.NotNil(t, cert)

	x509Cert, err := x509.ParseCertificate(cert.Certificate[0])
	require.NoError(t, err)
	assert.Equal(t, []string{"www.help.nyaruka.com"}, x509Cert.DNSNames)

	// and is the same one next time
	again, err := tc.GetCertificate(&tls.ClientHelloInfo{ServerName: "www.help.nyaruka.com"})
	require.NoError(t, err)
	assert.Same(t, cert, again)

	_, err = tc.GetCertificate(&tls.ClientHelloInfo{ServerName: "help.other.com"})
	assert.ErrorIs(t, err, certs.ErrNotAllowed)

	// a newly verified domain is picked up on refresh
	_, err = rt.DB.ExecContext(ctx, `UPDATE knowledge_helpsite SET domain_verified_on = NOW() WHERE domain = 'help.other.com'`)
	require.NoError(t, err)
	require.NoError(t, mgr.Refresh(ctx))

	assert.Equal(t, []string{"help.nyaruka.com", "help.other.com"}, mgr.Domains())
	assert.True(t, mgr.Allowed("help.other.com"))
}

func TestManagerACME(t *testing.T) {
	_, rt := testsuite.Runtime(t)

	// an ACME manager can be built with certificates on disk, and answers challenges on the HTTP handler
	rt.Config.TLSMode = runtime.TLSModeACME
	rt.Config.CertsStorage = runtime.CertsStorageDisk
	rt.Config.CertsDir = t.TempDir()

	mgr, err := certs.NewManager(rt)
	require.NoError(t, err)
	require.NoError(t, mgr.Start())
	defer mgr.Stop()

	tc := mgr.TLSConfig()
	assert.NotNil(t, tc.GetCertificate)
	assert.Contains(t, tc.NextProtos, "acme-tls/1")
	assert.Contains(t, tc.NextProtos, "h2")

	_, err = tc.GetCertificate(&tls.ClientHelloInfo{ServerName: "nobody.example.com"})
	assert.Error(t, err) // not a site's, so no certificate is obtained for it
}
