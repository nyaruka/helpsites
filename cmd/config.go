package cmd

import (
	"errors"
	"fmt"
	"os"

	"github.com/nyaruka/ezconf"
)

// LoadConfig loads configuration from a config file, environment variables and command line args, on top of the
// given base config, e.g. runtime.NewDefaultConfig(). If the config can't be loaded, the error is logged and the
// process exits. If usage was requested with -help, it's shown and the process exits cleanly.
func LoadConfig(cfg interface{ Parse() error }) {
	Run(loadConfig(cfg, os.Args[1:]))
}

// loadConfig is LoadConfig with the args passed explicitly and the outcome returned, so that it can be tested
func loadConfig(cfg interface{ Parse() error }, args []string) error {
	loader := ezconf.NewLoader(cfg, "helpsites", "Helpsites - serves the platform's help sites on their own domains", []string{"helpsites.toml"})
	loader.SetArgs(args...)

	if err := loader.Load(); err != nil {
		// Load never writes to stdout or stderr itself, so a request for usage comes back as ErrHelp for us to
		// act on here, where we still have the loader to show it with.
		if errors.Is(err, ezconf.ErrHelp) {
			loader.Usage()
		}
		return err
	}

	if err := cfg.Parse(); err != nil {
		return fmt.Errorf("invalid config: %w", err)
	}

	return nil
}
