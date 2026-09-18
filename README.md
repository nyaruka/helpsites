![Helpsites](.github/banner.svg)

[![Build Status](https://github.com/nyaruka/helpsites/workflows/CI/badge.svg)](https://github.com/nyaruka/helpsites/actions?query=workflow%3ACI)

Serves the platform's help sites: the public face of a workspace's helpdesk, read on a domain of the
workspace's own (`help.example.com`, pointed here by CNAME). Helpsites terminates TLS for those domains,
obtaining a certificate for each from Let's Encrypt on demand the first time a request for it arrives, and
renders the site's pages from the platform's database.

## Building

```
go build ./cmd/helpsites
```

## Running

Configuration is by environment variables prefixed `HELPSITES_` (or a `helpsites.toml`); `helpsites -help`
lists them. Three listeners:

- **HTTPS** (443) — the sites, one certificate per verified domain, issued on demand
- **HTTP** (80) — the health check at `/healthz`, ACME HTTP-01 challenges, and a redirect to HTTPS
- **internal** (8031) — `/hi/*`, the previews the platform proxies for a workspace looking at its own site

Certificates from Let's Encrypt are kept in a DynamoDB table shared by every instance, or on disk for a local
run; a local run can also skip the CA altogether with self-signed certificates. `-help` describes the settings.

## Testing

The suite is integration-level: it needs a Postgres it can create databases in, a Valkey, a DynamoDB it can
create tables in (the suite's are prefixed `Test`), and `pg_restore` on the path to load
`testsuite/testdata/postgres.dump`.

```
go test ./...
```
