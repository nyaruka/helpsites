package main

import (
	"github.com/nyaruka/helpsites/cmd"
	"github.com/nyaruka/helpsites/runtime"
	"github.com/nyaruka/helpsites/sentry"

	_ "github.com/nyaruka/helpsites/web/site" // the site's pages register their routes on load
)

var (
	// https://goreleaser.com/cookbooks/using-main.version
	version = "dev"
	date    = "unknown"
)

func main() {
	cmd.Run(run())
}

// run loads the config and starts the service, returning any error to main. It's separate from main because cmd.Run
// exits the process, so a defer there would never fire.
func run() error {
	cfg := runtime.NewDefaultConfig()
	cmd.LoadConfig(cfg)

	// add error reporting to our logging, if we have a DSN for it
	logHandler, err := sentry.Init(cfg.SentryDSN, cmd.LogHandler(), version)
	if err != nil {
		return err
	}
	defer sentry.Flush()

	return cmd.Service(cfg, version, date, logHandler)
}
