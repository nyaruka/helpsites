v26.3.2 (2026-09-22)
-------------------------
 * Move the module to /v26 and replace the built-in Sentry integration with hooks
 * Trim the README

v26.3.1 (2026-09-22)
-------------------------
 * Drop the mailroom probe from the startup connection check

v26.3.0 (2026-09-22)
-------------------------
 * Initial version: on-demand TLS and page rendering for help sites on their own domains
 * Store certificates in DynamoDB, named by a table prefix, and create the table on startup
 * Test connections to backing services at startup
 * Add Sentry error reporting

