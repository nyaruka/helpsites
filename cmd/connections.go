package cmd

import (
	"context"
	"log/slog"
	"time"

	"github.com/nyaruka/helpsites/runtime"
)

// tests our connections to backing services, logging any failures but always moving forward
func testConnections(rt *runtime.Runtime) {
	log := slog.With("comp", "server")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// test Postgres
	if err := rt.DB.PingContext(ctx); err != nil {
		log.Error("db not reachable", "error", err)
	} else {
		log.Info("db ok")
	}

	// test Valkey
	vc := rt.VK.Get()
	defer vc.Close()
	if _, err := vc.Do("PING"); err != nil {
		log.Error("valkey not reachable", "error", err)
	} else {
		log.Info("valkey ok")
	}
}
