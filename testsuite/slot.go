package testsuite

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	valkey "github.com/gomodule/redigo/redis"
	"github.com/stretchr/testify/require"
)

// Each test binary claims a slot - a small dense number which multiplexes the resources that are bounded rather than
// namespaceable: its valkey database (16 + slot, in the 16-31 band reserved for tests) and its certificates table
// (prefixed Test<slot>) in the shared DynamoDB emulator. The claim is an advisory lock in Postgres held for the
// binary's lifetime, so it evaporates when the run that owns it dies - and the slot's valkey database is flushed on
// claim, clearing anything a dead run left behind. If every slot is taken - concurrently running binaries can
// saturate the pool - claiming waits for one to free up rather than failing.

const (
	slotCount    = 16
	slotVKDBBase = 16 // valkey databases 16-31

	// DSN format for a slot's valkey database
	vkTestDSNFormat = "valkey://valkey:6379/%d"
)

// this binary's slot - claimed on first use
var binSlot = sync.OnceValues(func() (int, error) {
	ctx := context.Background()

	owner, err := dbOwner()
	if err != nil {
		return 0, err
	}

	deadline := time.Now().Add(3 * time.Minute)
	for {
		for s := range slotCount {
			var got bool
			if err := owner.QueryRowContext(ctx, `SELECT pg_try_advisory_lock($1)`, dbKey(fmt.Sprintf("slot_%d", s))).Scan(&got); err != nil {
				return 0, fmt.Errorf("error trying claim on slot: %w", err)
			}
			if got {
				return s, flushVKDB(slotVKDB(s))
			}
		}
		if time.Now().After(deadline) {
			return 0, fmt.Errorf("timed out waiting for an unclaimed slot (0-%d)", slotCount-1)
		}
		time.Sleep(250 * time.Millisecond)
	}
})

// claimSlot returns this binary's slot, claiming one on first use
func claimSlot(t *testing.T) int {
	t.Helper()

	slot, err := binSlot()
	require.NoError(t, err, "error claiming slot")
	return slot
}

// slotVKDB returns the valkey database number for the given slot
func slotVKDB(slot int) int {
	return slotVKDBBase + slot
}

// flushVKDB clears out the given valkey database
func flushVKDB(n int) error {
	vc, err := valkey.DialURL(fmt.Sprintf(vkTestDSNFormat, n))
	if err != nil {
		return fmt.Errorf("error connecting to valkey: %w", err)
	}
	defer vc.Close()

	if _, err := vc.Do("FLUSHDB"); err != nil {
		return fmt.Errorf("error flushing valkey database: %w", err)
	}
	return nil
}
