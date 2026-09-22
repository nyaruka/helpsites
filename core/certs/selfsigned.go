package certs

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"time"

	"github.com/caddyserver/certmagic"
)

// selfSignedIssuer is a CertMagic issuer which signs certificates with an in-process CA, for local development
// where no public CA can reach us. Only the CA is faked: what it signs is kept, locked and reloaded from storage
// like any other certificate, so a local run exercises the same path as a deployed one. Nothing it signs is trusted
// by anything, so clients have to be told to skip verification - which also covers certificates reloaded after a
// restart, which chain to a CA that no longer exists.
type selfSignedIssuer struct {
	ca  *x509.Certificate
	key *ecdsa.PrivateKey
}

func newSelfSignedIssuer() (*selfSignedIssuer, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}

	template := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "helpsites development CA"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().AddDate(1, 0, 0),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		return nil, err
	}
	ca, err := x509.ParseCertificate(der)
	if err != nil {
		return nil, err
	}

	return &selfSignedIssuer{ca: ca, key: key}, nil
}

func (s *selfSignedIssuer) Issue(
	ctx context.Context, csr *x509.CertificateRequest,
) (*certmagic.IssuedCertificate, error) {
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return nil, err
	}

	subject := csr.Subject
	if subject.CommonName == "" && len(csr.DNSNames) > 0 {
		subject.CommonName = csr.DNSNames[0]
	}

	template := &x509.Certificate{
		SerialNumber: serial,
		Subject:      subject,
		DNSNames:     csr.DNSNames,
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().AddDate(1, 0, 0),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, template, s.ca, csr.PublicKey, s.key)
	if err != nil {
		return nil, fmt.Errorf("error signing certificate for %v: %w", csr.DNSNames, err)
	}

	var chain bytes.Buffer
	for _, c := range [][]byte{der, s.ca.Raw} {
		if err := pem.Encode(&chain, &pem.Block{Type: "CERTIFICATE", Bytes: c}); err != nil {
			return nil, err
		}
	}
	return &certmagic.IssuedCertificate{Certificate: chain.Bytes()}, nil
}

func (s *selfSignedIssuer) IssuerKey() string { return "selfsigned" }

var _ certmagic.Issuer = (*selfSignedIssuer)(nil)
