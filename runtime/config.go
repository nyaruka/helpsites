package runtime

import (
	"log/slog"

	"github.com/nyaruka/helpsites/v26/utils"
)

// TLS modes
const (
	TLSModeACME       = "acme"       // certificates from an ACME CA, obtained on demand
	TLSModeSelfSigned = "selfsigned" // certificates signed by an in-process CA, for local development
)

// ACME CAs
const (
	ACMECAProduction = "production"
	ACMECAStaging    = "staging"
)

// Config is our top level configuration object
type Config struct {
	DB     string `validate:"url,startswith=postgres:"                    help:"URL for the platform's Postgres database"`
	Valkey string `validate:"url,startswith=valkey:|startswith=valkeys:" help:"URL for the Valkey instance, valkeys:// for TLS"`

	HTTPSAddress    string `help:"the address the HTTPS listener will bind to, empty means all interfaces"`
	HTTPSPort       int    `help:"the port the HTTPS listener will listen on"`
	HTTPAddress     string `help:"the address the HTTP listener will bind to, empty means all interfaces"`
	HTTPPort        int    `help:"the port the HTTP listener will listen on"`
	InternalAddress string `help:"the address the internal listener will bind to, empty means all interfaces"`
	InternalPort    int    `help:"the port the internal listener will listen on"`

	TLSMode           string `validate:"eq=acme|eq=selfsigned"      help:"how certificates are obtained: acme, or selfsigned for local development"`
	ACMECA            string `validate:"eq=production|eq=staging"   help:"which Let's Encrypt CA to use: production, or staging while testing a deployment"`
	ACMEEmail         string `validate:"omitempty,email"            help:"the contact email for the ACME account, where the CA writes about certificates it couldn't renew"`
	DynamoTablePrefix string `help:"the prefix of the DynamoDB table names, e.g. Temba keeps certificates in TembaCerts"`
	DynamoEndpoint    string `help:"DynamoDB service endpoint, empty for the SDK default"`
	DomainsRefresh    int    `validate:"min=1"                      help:"how often, in seconds, the set of verified site domains is reloaded from the database"`

	AppHost           string `help:"the host of the platform itself, which the chat widget on a site is loaded from and talks to"`
	MailroomURL       string `validate:"omitempty,url" help:"the base URL of mailroom, for semantic search; empty disables it"`
	MailroomTLSName   string `help:"the name mailroom's certificate is verified against, when its URL doesn't carry it"`
	MailroomAuthToken string `help:"the authentication token for mailroom's internal endpoints"`
	AuthToken         string `help:"the authentication token the platform sends on requests to the internal listener"`

	DeploymentID string     `help:"the deployment identifier to use for metrics"`
	LogLevel     slog.Level `help:"the logging level helpsites should use"`
	Version      string     `help:"the version that will be used in request and response headers"`
}

// NewDefaultConfig returns a new default configuration object
func NewDefaultConfig() *Config {
	return &Config{
		DB:     "postgres://temba:temba@postgres/temba?sslmode=disable",
		Valkey: "valkey://valkey:6379/15",

		HTTPSAddress:    "",
		HTTPSPort:       443,
		HTTPAddress:     "",
		HTTPPort:        80,
		InternalAddress: "",
		InternalPort:    8031,

		TLSMode:           TLSModeACME,
		ACMECA:            ACMECAProduction,
		DynamoTablePrefix: "Temba",
		DomainsRefresh:    30,

		AppHost: "localhost.textit.com",

		DeploymentID: "dev",
		LogLevel:     slog.LevelWarn,
		Version:      "Dev",
	}
}

// Parse validates the config. It's called by cmd.LoadConfig, and a config built by other means (e.g. NewDefaultConfig
// in a test) should be parsed before being handed to NewRuntime.
func (c *Config) Parse() error {
	return utils.Validate(c)
}

// CertsTable returns the name of the DynamoDB table certificates are kept in
func (c *Config) CertsTable() string {
	return c.DynamoTablePrefix + "Certs"
}
