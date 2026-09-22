package cmd

import (
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/nyaruka/helpsites/core/certs"
	"github.com/nyaruka/helpsites/runtime"
	"github.com/nyaruka/helpsites/web"
)

// shutdownTimeout is how long we allow for a graceful shutdown before exiting hard. Past this budget something is
// genuinely wedged, and it's better to exit with a record of why than be killed by the orchestrator, whose stop
// timeout should be set a bit above this so the watchdog fires first.
const shutdownTimeout = 60 * time.Second

// Service starts the helpsites service, blocks until a termination signal is received, then stops it. The config
// must already be loaded, e.g. with LoadConfig. All logging is sent to the given handler, e.g. LogHandler(), whose
// level is set from the config.
func Service(cfg *runtime.Config, version, date string, logHandler slog.Handler) error {
	cfg.Version = version

	// configure our logger
	logLevel.Set(cfg.LogLevel)
	slog.SetDefault(slog.New(logHandler))

	log := slog.With("comp", "main")
	log.Info("starting helpsites", "version", version, "released", date)

	rt, err := runtime.NewRuntime(cfg)
	if err != nil {
		return err
	}

	// log what we can and can't reach before we start doing anything with it
	testConnections(rt)

	svc, err := startService(rt)
	if err != nil {
		return err
	}

	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM)
	log.Info("stopping", "signal", <-ch)

	watchdog := time.AfterFunc(shutdownTimeout, func() {
		log.Error("shutdown timed out, exiting", "timeout", shutdownTimeout)
		os.Exit(1)
	})
	defer watchdog.Stop()

	svc.stop()

	return nil
}

// service is the set of components this process runs, in the order they're started
type service struct {
	rt     *runtime.Runtime
	certs  *certs.Manager
	server *web.Server
}

// startService starts each component in turn, unwinding whatever is already running if one of them fails, so that a
// failure part way through doesn't leave the process with half a runtime
func startService(rt *runtime.Runtime) (*service, error) {
	s := &service{rt: rt}

	manager, err := certs.NewManager(rt)
	if err != nil {
		rt.Stop()
		return nil, fmt.Errorf("error creating certificate manager: %w", err)
	}
	if err := manager.Start(); err != nil {
		rt.Stop()
		return nil, fmt.Errorf("error starting certificate manager: %w", err)
	}
	s.certs = manager

	server := web.NewServer(rt, manager)
	if err := server.Start(); err != nil {
		s.stop()
		return nil, err
	}
	s.server = server

	return s, nil
}

// stop stops each component in the reverse of the order it was started, skipping those which never started
func (s *service) stop() {
	if s.server != nil {
		if err := s.server.Stop(); err != nil {
			slog.Error("error stopping server", "error", err)
		}
	}
	if s.certs != nil {
		s.certs.Stop()
	}

	s.rt.Stop()
}
