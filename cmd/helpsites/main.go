package main

import (
	"github.com/nyaruka/helpsites/v26/cmd"
	"github.com/nyaruka/helpsites/v26/runtime"

	_ "github.com/nyaruka/helpsites/v26/web/site" // the site's pages register their routes on load
)

var (
	// overridden at build time via -ldflags "-X main.version=... -X main.date=..."
	version = "dev"
	date    = "unknown"
)

func main() {
	cfg := runtime.NewDefaultConfig()
	cmd.LoadConfig(cfg)
	cmd.Run(cmd.Service(cfg, version, date, cmd.LogHandler()))
}
