package cmd

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/nyaruka/gocommon/aws/dynamo"
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

	// test DynamoDB, where certificates are kept. Certificates are obtained on demand, so without this a table we
	// can't reach isn't noticed until the first handshake for a new domain. Skipped when we're self-signing, as
	// then nothing ever touches it.
	if rt.Config.TLSMode != runtime.TLSModeSelfSigned {
		if err := dynamo.Test(ctx, rt.Dynamo, rt.Config.CertsTable()); err != nil {
			log.Error("dynamodb not reachable", "error", err)
		} else {
			log.Info("dynamodb ok")
		}
	}

	// test mailroom, if we're configured to use it
	if rt.Config.MailroomURL != "" {
		if err := testMailroom(ctx, rt); err != nil {
			log.Error("mailroom not reachable", "error", err)
		} else {
			log.Info("mailroom ok")
		}
	}
}

// tests mailroom by asking for the health response its listener serves at its root, which exercises the same name
// resolution, TLS and routing as the search calls without needing a token
func testMailroom(ctx context.Context, rt *runtime.Runtime) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rt.Config.MailroomURL+"/", nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "Helpsites")

	resp, err := rt.HTTP.Mailroom.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("mailroom returned %d", resp.StatusCode)
	}
	return nil
}
