package main

import (
	"github.com/nyaruka/helpsites/cmd"
	"github.com/nyaruka/helpsites/runtime"

	_ "github.com/nyaruka/helpsites/web/site" // the site's pages register their routes on load
)

var (
	// https://goreleaser.com/cookbooks/using-main.version
	version = "dev"
	date    = "unknown"
)

func main() {
	cfg := runtime.NewDefaultConfig()
	cmd.LoadConfig(cfg)
	cmd.Run(cmd.Service(cfg, version, date, cmd.LogHandler()))
}
