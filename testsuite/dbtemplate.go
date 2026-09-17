package testsuite

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"fmt"
	"hash/fnv"
	"os"
	"os/exec"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/lib/pq"
	"github.com/stretchr/testify/require"
	"github.com/vinovest/sqlx"
)

// Each test gets its own database, cloned from a template built once per test dump. Cloning is a file copy
// inside Postgres and takes ~50ms, whereas restoring the dump takes ~1s - and because no test ever inherits
// another's database, there's no reset step to get wrong.
//
// Test binaries run concurrently unless -p=1 is given, and worktrees share a Postgres, so databases in use by
// one binary have to be invisible to another's sweep. Each binary holds advisory locks on what it's using -
// shared on its template, exclusive on each of its live test databases - and the sweeper only drops what it
// can lock itself. Locks are released by Postgres when a connection dies, so a database is reclaimable
// exactly when the run that owned it is gone.

const (
	// maintenance database we connect to in order to create and drop test databases
	dbAdminDSN = "postgres://helpsites_test:temba@postgres/postgres?sslmode=disable"

	// DSN format for a per-test database
	dbTestDSNFormat = "postgres://helpsites_test:temba@postgres/%s?sslmode=disable&Timezone=UTC"

	dbTestPrefix     = "helpsites_test_t"
	dbTemplatePrefix = "helpsites_test_tpl_"

	// advisory lock key which serializes template creation across test binaries
	dbTemplateLockID = 0x68656c70
)

var dbCounter atomic.Int64

// admin pool is opened once per test binary and left open for its lifetime
var dbAdmin = sync.OnceValues(func() (*sqlx.DB, error) {
	db, err := sqlx.Open("postgres", dbAdminDSN)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(4)
	return db, db.Ping()
})

// dbOwner is a connection pinned for the lifetime of the test binary, holding the advisory locks which mark
// what this binary is using. It's never closed - Postgres releases its locks when the process exits.
var dbOwner = sync.OnceValues(func() (*sql.Conn, error) {
	admin, err := dbAdmin()
	if err != nil {
		return nil, err
	}
	return admin.DB.Conn(context.Background())
})

// only check the template exists once per test binary - after that we know it's there
var dbTemplate = sync.OnceValues(func() (string, error) {
	name, err := templateName()
	if err != nil {
		return "", err
	}
	return name, ensureTemplate(context.Background(), name)
})

// templateName is keyed by the content hash of the dump, so regenerating it automatically builds a new
// template and leaves the old one to be swept.
func templateName() (string, error) {
	d, err := os.ReadFile(testdataPath("postgres.dump"))
	if err != nil {
		return "", fmt.Errorf("error reading test dump: %w", err)
	}

	sum := sha256.Sum256(append(d, []byte(schemaExtras)...))

	return fmt.Sprintf("%s%x", dbTemplatePrefix, sum[:6]), nil
}

// dbKey derives the advisory lock key which guards a given database
func dbKey(dbName string) int64 {
	h := fnv.New64a()
	h.Write([]byte(dbName))
	return int64(h.Sum64())
}

// ensureTemplate creates the template database if it doesn't already exist, then takes a shared lock on it so
// that no other binary's sweep can drop it while we're using it. Safe to call concurrently from multiple test
// binaries as `go test ./...` runs each package as its own process.
func ensureTemplate(ctx context.Context, name string) error {
	admin, err := dbAdmin()
	if err != nil {
		return err
	}
	owner, err := dbOwner()
	if err != nil {
		return err
	}

	// advisory locks are session scoped so we need to pin ourselves to a single connection
	conn, err := admin.DB.Conn(ctx)
	if err != nil {
		return fmt.Errorf("error taking admin connection: %w", err)
	}
	defer func() {
		conn.ExecContext(ctx, `SELECT pg_advisory_unlock_all()`) // don't leak locks back into the pool
		conn.Close()
	}()

	if _, err := conn.ExecContext(ctx, `SELECT pg_advisory_lock($1)`, dbTemplateLockID); err != nil {
		return fmt.Errorf("error taking template lock: %w", err)
	}

	var exists bool
	if err := conn.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM pg_database WHERE datname = $1)`, name).Scan(&exists); err != nil {
		return fmt.Errorf("error checking for template %s: %w", name, err)
	}

	if !exists {
		// build under a scratch name and rename, so that a run which dies midway doesn't leave behind a
		// half-restored database that later runs would take for a complete template
		scratch := name + "_wip"

		// the scratch name matches the template prefix, so claim it too or another binary's sweep will see an
		// unclaimed template and drop it out from under the restore
		if _, err := conn.ExecContext(ctx, `SELECT pg_advisory_lock($1)`, dbKey(scratch)); err != nil {
			return fmt.Errorf("error claiming scratch database: %w", err)
		}

		if err := execDDL(ctx, conn, `DROP DATABASE IF EXISTS %s`, scratch); err != nil {
			return err
		}
		if err := execDDL(ctx, conn, `CREATE DATABASE %s`, scratch); err != nil {
			return err
		}
		if err := restoreDump(ctx, scratch); err != nil {
			return err
		}
		if err := applySchemaExtras(ctx, scratch); err != nil {
			return err
		}
		if err := execDDL(ctx, conn, `ALTER DATABASE %s RENAME TO `+pq.QuoteIdentifier(name), scratch); err != nil {
			return err
		}

		if _, err := conn.ExecContext(ctx, `SELECT pg_advisory_unlock($1)`, dbKey(scratch)); err != nil {
			return fmt.Errorf("error releasing scratch database: %w", err)
		}
	}

	// claim the template before releasing the creation lock, so there's no window in which a binary using an
	// older dump could sweep it out from under us
	if _, err := owner.ExecContext(ctx, `SELECT pg_advisory_lock_shared($1)`, dbKey(name)); err != nil {
		return fmt.Errorf("error claiming template %s: %w", name, err)
	}

	if _, err := conn.ExecContext(ctx, `SELECT pg_advisory_unlock($1)`, dbTemplateLockID); err != nil {
		return fmt.Errorf("error releasing template lock: %w", err)
	}

	return sweepStale(ctx, conn)
}

// schemaExtras is what the platform's schema will have that the dump doesn't yet: the HTML the platform renders an
// article's body to when it's published, and the headings its page lists. TODO: drop once the dump has them.
const schemaExtras = `
ALTER TABLE knowledge_article
    ADD COLUMN IF NOT EXISTS body_html TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS headings JSONB NOT NULL DEFAULT '[]';`

// applySchemaExtras applies the schema extras to the given database
func applySchemaExtras(ctx context.Context, dbName string) error {
	db, err := sqlx.Open("postgres", fmt.Sprintf(dbTestDSNFormat, dbName))
	if err != nil {
		return err
	}
	defer db.Close()

	if _, err := db.ExecContext(ctx, schemaExtras); err != nil {
		return fmt.Errorf("error applying schema extras to %s: %w", dbName, err)
	}
	return nil
}

// restoreDump loads the test dump into the given database
func restoreDump(ctx context.Context, dbName string) error {
	dump, err := os.Open(testdataPath("postgres.dump"))
	if err != nil {
		return fmt.Errorf("error opening test dump: %w", err)
	}
	defer dump.Close()

	// ownership and grants are left out: the dump's objects belong to a role that only exists where it was made
	cmd := exec.CommandContext(ctx, "pg_restore", "-h", "postgres", "-U", "helpsites_test", "-d", dbName, "--no-password", "--no-owner", "--no-privileges")
	cmd.Stdin = dump
	cmd.Env = append(os.Environ(), "PGPASSWORD=temba")

	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("error restoring dump into %s: %w: %s", dbName, err, string(output))
	}

	return nil
}

// sweepStale drops templates built from older dumps, and per-test databases orphaned by runs which died before
// they could clean up after themselves. A database still in use is protected by its owner's advisory lock, so
// anything we can lock ourselves belongs to a run which is no longer around.
func sweepStale(ctx context.Context, conn *sql.Conn) error {
	rows, err := conn.QueryContext(ctx,
		`SELECT datname FROM pg_database WHERE datname LIKE $1 || '%' OR datname LIKE $2 || '%'`,
		dbTemplatePrefix, dbTestPrefix,
	)
	if err != nil {
		return fmt.Errorf("error querying for stale databases: %w", err)
	}

	var candidates []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			rows.Close()
			return err
		}
		candidates = append(candidates, name)
	}
	rows.Close()

	for _, name := range candidates {
		var got bool
		if err := conn.QueryRowContext(ctx, `SELECT pg_try_advisory_lock($1)`, dbKey(name)).Scan(&got); err != nil {
			return fmt.Errorf("error trying lock for %s: %w", name, err)
		}
		if !got {
			continue // in use by a live run, including our own template
		}

		terminateConns(ctx, conn, name)
		execDDL(ctx, conn, `DROP DATABASE IF EXISTS %s`, name)

		conn.ExecContext(ctx, `SELECT pg_advisory_unlock($1)`, dbKey(name))
	}

	return nil
}

// createTestDB clones the template into a uniquely named database for a single test, and returns its name.
func createTestDB(t *testing.T) string {
	t.Helper()

	ctx := context.Background()

	tpl, err := dbTemplate()
	require.NoError(t, err, "error ensuring test database template")

	admin, err := dbAdmin()
	require.NoError(t, err)

	owner, err := dbOwner()
	require.NoError(t, err)

	name := fmt.Sprintf("%s%s_%d", dbTestPrefix, binProcID(), dbCounter.Add(1))

	// claim the database before it exists, so that it's never visible to another binary's sweep unclaimed
	_, err = owner.ExecContext(ctx, `SELECT pg_advisory_lock($1)`, dbKey(name))
	require.NoError(t, err, "error claiming test database")

	conn, err := admin.DB.Conn(ctx)
	require.NoError(t, err)
	defer conn.Close()

	require.NoError(t, execDDL(ctx, conn, `DROP DATABASE IF EXISTS %s`, name))
	require.NoError(t, execDDL(ctx, conn, `CREATE DATABASE %s TEMPLATE `+pq.QuoteIdentifier(tpl), name))

	return name
}

// dropTestDB drops a database created by createTestDB and releases our claim on it
func dropTestDB(t *testing.T, name string) {
	t.Helper()

	ctx := context.Background()

	admin, err := dbAdmin()
	require.NoError(t, err)

	owner, err := dbOwner()
	require.NoError(t, err)

	conn, err := admin.DB.Conn(ctx)
	require.NoError(t, err)
	defer conn.Close()

	terminateConns(ctx, conn, name)

	require.NoError(t, execDDL(ctx, conn, `DROP DATABASE IF EXISTS %s`, name))

	_, err = owner.ExecContext(ctx, `SELECT pg_advisory_unlock($1)`, dbKey(name))
	require.NoError(t, err)
}

// terminateConns kicks off any lingering connections so that the given database can be dropped or cloned
func terminateConns(ctx context.Context, conn *sql.Conn, dbName string) {
	conn.ExecContext(ctx,
		`SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE datname = $1 AND pid != pg_backend_pid()`, dbName,
	)
}

// execDDL runs a DDL statement, quoting the given database name into it as an identifier - which can't be
// done with a bind parameter.
func execDDL(ctx context.Context, conn *sql.Conn, format, dbName string) error {
	stmt := fmt.Sprintf(format, pq.QuoteIdentifier(dbName))

	if _, err := conn.ExecContext(ctx, stmt); err != nil {
		return fmt.Errorf("error executing %q: %w", stmt, err)
	}
	return nil
}
