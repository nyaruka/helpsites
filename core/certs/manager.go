package certs

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"slices"
	"sync"
	"sync/atomic"
	"time"

	"github.com/caddyserver/certmagic"
	"github.com/nyaruka/helpsites/core/models"
	"github.com/nyaruka/helpsites/runtime"
)

// ErrNotAllowed is the error a handshake gets for a name that isn't a verified site's
var ErrNotAllowed = errors.New("not a verified help site domain")

// Manager provides the certificates the HTTPS listener serves with: one per verified site domain, obtained on demand
// the first time a request for the domain arrives and kept in shared storage. Which names it will obtain a
// certificate for is decided by the set of verified domains, reloaded from the database on an interval, so that
// nobody can make us ask a CA for a name that isn't a site's.
type Manager struct {
	rt      *runtime.Runtime
	domains atomic.Pointer[map[string]bool]

	// for ACME
	config *certmagic.Config
	issuer *certmagic.ACMEIssuer

	// for local development
	selfSigner *selfSigner

	stop chan struct{}
	wg   sync.WaitGroup
}

func NewManager(rt *runtime.Runtime) (*Manager, error) {
	m := &Manager{rt: rt, stop: make(chan struct{})}
	m.domains.Store(&map[string]bool{})

	cfg := rt.Config

	switch cfg.TLSMode {
	case runtime.TLSModeSelfSigned:
		signer, err := newSelfSigner()
		if err != nil {
			return nil, fmt.Errorf("error creating self signer: %w", err)
		}
		m.selfSigner = signer

	case runtime.TLSModeACME:
		storage := NewDynamoStorage(rt.Dynamo, cfg.CertsTable())

		ca := certmagic.LetsEncryptProductionCA
		if cfg.ACMECA == runtime.ACMECAStaging {
			ca = certmagic.LetsEncryptStagingCA
		}

		logger := newZapLogger(slog.Default())

		cache := certmagic.NewCache(certmagic.CacheOptions{
			GetConfigForCert: func(certmagic.Certificate) (*certmagic.Config, error) { return m.config, nil },
			Logger:           logger,
		})
		m.config = certmagic.New(cache, certmagic.Config{
			Storage:  storage,
			OnDemand: &certmagic.OnDemandConfig{DecisionFunc: m.decide},
			Logger:   logger,
		})
		m.issuer = certmagic.NewACMEIssuer(m.config, certmagic.ACMEIssuer{
			CA:     ca,
			Email:  cfg.ACMEEmail,
			Agreed: true,
			Logger: logger,
		})
		m.config.Issuers = []certmagic.Issuer{m.issuer}

	default:
		return nil, fmt.Errorf("unknown TLS mode %q", cfg.TLSMode)
	}

	return m, nil
}

// Start loads the verified domains and starts reloading them on the configured interval
func (m *Manager) Start() error {
	if err := m.Refresh(context.Background()); err != nil {
		return err
	}

	interval := time.Duration(m.rt.Config.DomainsRefresh) * time.Second

	m.wg.Add(1)
	go func() {
		defer m.wg.Done()

		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-m.stop:
				return
			case <-ticker.C:
				if err := m.Refresh(context.Background()); err != nil {
					slog.Error("error refreshing verified domains", "comp", "certs", "error", err)
				}
			}
		}
	}()

	return nil
}

func (m *Manager) Stop() {
	close(m.stop)
	m.wg.Wait()
}

// Refresh reloads the set of verified domains from the database
func (m *Manager) Refresh(ctx context.Context) error {
	domains, err := models.LoadVerifiedDomains(ctx, m.rt.DB)
	if err != nil {
		return err
	}

	set := make(map[string]bool, len(domains))
	for _, d := range domains {
		set[models.NormalizeDomain(d)] = true
	}
	m.domains.Store(&set)
	return nil
}

// Allowed returns whether the given name is one we'll obtain a certificate for - a verified site's domain, or its
// www. form
func (m *Manager) Allowed(name string) bool {
	return (*m.domains.Load())[models.NormalizeDomain(name)]
}

// Domains returns the verified domains, sorted
func (m *Manager) Domains() []string {
	set := *m.domains.Load()
	domains := make([]string, 0, len(set))
	for d := range set {
		domains = append(domains, d)
	}
	slices.Sort(domains)
	return domains
}

// decide is CertMagic's on-demand decision: whether to obtain a certificate for the name in a handshake
func (m *Manager) decide(ctx context.Context, name string) error {
	if !m.Allowed(name) {
		return ErrNotAllowed
	}
	return nil
}

// TLSConfig returns the TLS configuration for the HTTPS listener
func (m *Manager) TLSConfig() *tls.Config {
	var tc *tls.Config

	if m.selfSigner != nil {
		tc = &tls.Config{
			GetCertificate: func(hello *tls.ClientHelloInfo) (*tls.Certificate, error) {
				if !m.Allowed(hello.ServerName) {
					return nil, ErrNotAllowed
				}
				return m.selfSigner.certificate(hello.ServerName)
			},
		}
	} else {
		tc = m.config.TLSConfig()
	}

	tc.MinVersion = tls.VersionTLS12

	// serve HTTP/2 as well as 1.1 - CertMagic's config only asks for its ACME ALPN
	for _, proto := range []string{"http/1.1", "h2"} {
		if !slices.Contains(tc.NextProtos, proto) {
			tc.NextProtos = append([]string{proto}, tc.NextProtos...)
		}
	}

	return tc
}

// HTTPChallengeHandler wraps the given handler in one which answers ACME HTTP-01 challenges, which is nothing at all
// when there's no CA to answer to
func (m *Manager) HTTPChallengeHandler(next http.Handler) http.Handler {
	if m.issuer == nil {
		return next
	}
	return m.issuer.HTTPChallengeHandler(next)
}
