## v26.3.9 (2026-09-29)
 * List an article once however its chunks clean up, and keep underscores inside words
 * Only strip column styles from a table row made entirely of them
 * Strip markdown and the title prefix from search result snippets
 * Switch CHANGELOG headers to ATX style

## v26.3.8 (2026-09-28)
 * Claim per-test valkey databases with assertvk.ClaimDB, coordinated with other projects' tests

## v26.3.7 (2026-09-28)
 * Rely on mailroom to limit knowledge searches to the site's source
 * Remove text search fallback from help site search
 * Don't cache search results when mailroom errors

## v26.3.6 (2026-09-23)
 * Restrict knowledge searches to the site's source

## v26.3.5 (2026-09-23)
 * Leave creating the certificates table to the platform and fail startup if it's missing

## v26.3.4 (2026-09-22)
 * Resolve storage images and style palette columns when serving articles
 * Require the storage URL to be configured rather than defaulting it

## v26.3.3 (2026-09-22)
 * Rename the internal listener's preview token to an auth token
 * Fix the site settings link

## v26.3.2 (2026-09-22)
 * Move the module to /v26 and replace the built-in Sentry integration with hooks
 * Trim the README

## v26.3.1 (2026-09-22)
 * Drop the mailroom probe from the startup connection check

## v26.3.0 (2026-09-22)
 * Initial version: on-demand TLS and page rendering for help sites on their own domains
 * Store certificates in DynamoDB, named by a table prefix, and create the table on startup
 * Test connections to backing services at startup
 * Add Sentry error reporting

