package runtime

import (
	"crypto/tls"
	"net/http"
	"time"
)

// HTTP holds the http.Clients used for outbound calls
type HTTP struct {
	Mailroom *http.Client
}

func newHTTP(cfg *Config) *HTTP {
	transport := http.DefaultTransport.(*http.Transport).Clone()

	// mailroom is reached by the address of a load balancer whose certificate is for the platform's own name
	if cfg.MailroomTLSName != "" {
		transport.TLSClientConfig = &tls.Config{ServerName: cfg.MailroomTLSName}
	}

	return &HTTP{
		Mailroom: &http.Client{Transport: transport, Timeout: 15 * time.Second},
	}
}
