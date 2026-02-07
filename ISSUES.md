# ISSUES (Append-only Log)

Entries record newly discovered requests or changes, with their outcomes. No instructive content lives here. Read @NOTES.md for the process to follow when fixing issues.

Read @AGENTS.md, @AGENTS.GO.md, @AGENTS.DOCKER.md, @AGENTS.FRONTEND.md, @AGENTS.GIT.md, @POLICY.md, @NOTES.md, @README.md and @ISSUES.md. Start working on open issues. Work autonomously and stack up PRs.

Each issue is formatted as `- [ ] [LG-<number>]`. When resolved it becomes -` [x] [LG-<number>]`

## Features (114–199)

## Improvements (207–299)

- [x] [LG-204] (P1) Extract ledger core into a reusable Go library. Resolved: domain types + store interfaces enforced, adapters updated, tests/ci passing.
  - Promote `internal/credit` into a public `pkg/ledger` module with explicit domain types and invariants.
    - Define a storage interface suitable for both in-process and service-hosted deployments.
    - Provide a default SQL-backed implementation (adapting existing gorm stores) while keeping the core domain independent of GORM.
- [x] [LG-205] (P2) Add integration documentation for service and library usage. Resolved: expanded integration guide with domain types, store wiring, and error contracts.
  - Document how to run ledger as a standalone gRPC microservice (config, migrations, networking) and how to consume it from other languages.
    - Document how to embed the future `pkg/ledger` library in Go services, including storage wiring, transaction patterns, and error contracts.
- [x] [LG-206] (P2) Support multiple ledgers per user. Resolved: ledger_id threaded through API/service/store/schema; demo/docs updated; migration path omitted per no-backward-compat requirement.
  - Allow a single user_id to own multiple ledger accounts (introduce a ledger/account namespace or composite key).
    - Update storage constraints, API inputs, and reservation/entry lookups to include the ledger identifier.
    - Provide a migration path for existing single-ledger data.
- [x] [LG-207] (P2) Introduce first-class multi-tenant support (tenant_id). Resolved: tenant_id required across API/service/store/schema; demo/docs/examples updated; no migration path.
  - Require tenant_id in API/service/store boundaries and schema keys.
    - Update demo/docs/examples to send tenant_id alongside ledger_id and user_id.
    - Skip migration path (backward compatibility not required).
- [x] [LG-208] (P2) Make demo tenant_id and ledger_id defaults configurable via env. Resolved: demo config/flags use env-backed defaults; env templates/docs/tests updated, tooling passing.
  - Add DEMOAPI_DEFAULT_TENANT_ID and DEMOAPI_DEFAULT_LEDGER_ID to demo config and env templates.
    - Update demo handlers and docs to use config values instead of hardcoded defaults.
- [x] [LG-209] (P2) Make ledger data directory configurable for Docker workflows. Resolved: data dir is only used by DATABASE_URL; no extra env added, compose mounts align to `/srv/data`, tooling passing.
  - Add LEDGER_DATA_DIR to .env.ledger and wire compose volume targets to use it.
    - Update compose wiring so ledger uses the configured data directory.
- [ ] [LG-210] (P1) Add server-managed bootstrap grants for new accounts. Unresolved.
  - Provide optional bootstrap configuration (amount/metadata/idempotency prefix) per tenant+ledger.
  - Apply the bootstrap grant exactly once when an account is created (or first accessed), without requiring the caller to orchestrate a grant.
  - Use deterministic idempotency keys so repeated calls are safe; treat duplicate idempotency as no-op.
  - Update config/env/README and add coverage for concurrent account creation.
- [ ] [LG-211] (P1) Add a backfill/bootstrap command for existing accounts. Unresolved.
  - Provide a CLI/admin command to apply the configured bootstrap grant to all existing accounts missing it.
  - Add store-level account listing/pagination to support backfill without direct SQL in callers.
  - Treat duplicate idempotency keys as no-op; emit a summary of accounts updated vs skipped.
  - Document the workflow and add integration tests for large account sets.
- [ ] [LG-212] (P1) Support grant-only history and "last grant" queries in the gRPC API. Unresolved.
  - Callers need to display "last grant" reliably without paging through large volumes of non-grant entries (holds/spends/captures).
  - Options:
    - Add `type` filtering (or a dedicated `ListGrants` RPC) so clients can request only grant entries.
    - Add a `GetLastGrant` RPC that returns the most recent grant entry (entry_id, amount_cents, created_unix_utc, metadata_json).
  - Ensure results are ordered by creation timestamp and include deterministic pagination/cursors for high-activity accounts.

- [x] [LG-213] (P1) Use PostgreSQL in Docker Compose orchestration (replace SQLite). Resolved: root + demo compose now provision Postgres and run `db/migrations.sql` via a one-shot migrator; `.env.ledger` defaults to Postgres; docs updated; `make ci` passing.

- [x] [LG-214] (P1) Run Postgres migrations via GORM (remove manual SQL migrator). Resolved: `ledgerd` now `AutoMigrate`s for SQLite+Postgres; compose `migrate` services removed; `db/migrations.sql` deleted; docs updated; `make ci` passing.


## BugFixes (302–399)

- [x] [LG-303] (P1) Allow negative totals from SumTotal so expired grants don't break balance/spend flows. Resolved: signed totals added; balance/spend now handle negatives without store errors.
  - Remove rejection of negative sums and ensure Reserve/Spend returns ErrInsufficientFunds when totals are negative.

- [x] [LG-304] (P1) Treat file:// SQLite DSNs as sqlite, not unsupported. Resolved: `resolveDriver` now maps `file:` DSNs to sqlite and preserves `file::memory:?cache=shared`; coverage restored and `make ci` passing.
  - `cmd/credit` resolveDriver currently returns `file` as an unsupported database scheme when the DSN is `file://...`.
  - Ensure file-based SQLite DSNs like `file:///tmp/ledger.db` and `file::memory:?cache=shared` are treated as sqlite and continue to work.

## Maintenance (401–499)

- [x] [LG-400] (P0) Increase test coverage to 95%. Resolved: ledger tests expanded to 96.7% coverage; coverage gate raised to 95%, tooling passing.
  Increase test coverage to 95%

- [x] [LG-401] (P0) Enforce coverage gate across the entire Go module. Resolved: `make test-unit` now computes module-wide coverage (excluding generated `api/credit/v1`); service/store integration tests added; `make ci` passing with total coverage 95.2%.
  - Current `make test` only enforces coverage for `pkg/ledger`, leaving `cmd/credit` + `internal/*` effectively untested.
  - Update coverage gate to measure module-wide coverage (excluding generated protobuf package) and add integration tests that exercise the service end-to-end.

- [x] [LG-402] (P1) Fix demo backend Docker build failing due to outdated ledger proto dependency. Resolved: bumped `demo/backend` dependency on `github.com/MarkoPoloResearchLab/ledger` so generated proto includes `tenant_id`/`ledger_id`; `go test ./...` and demo `docker build` passing.
  - `demo/backend` imports `github.com/MarkoPoloResearchLab/ledger/api/credit/v1` but pins an older module version missing `tenant_id`/`ledger_id` fields, breaking `demo/Dockerfile` builds.
  - Update `demo/backend/go.mod` to a ledger module version that matches the current API and ensure `demo/docker-compose.yml` builds succeed.

- [x] [LG-403] (P1) Fix demo Compose TAuth container failing to start without config.yaml. Resolved: added `demo/tauth.config.yaml` + compose mount and `TAUTH_CONFIG_FILE`; updated demo UI to load `tauth.js` from TAuth and aligned demo issuer to `tauth`.
  - Current `demo/docker-compose.yml` uses `ghcr.io/tyemirov/tauth:latest`, which now requires a YAML config file (defaults to `config.yaml`) and exits if it is missing.
  - Provide a minimal demo `config.yaml` and wire it into compose via volume mount + `TAUTH_CONFIG_FILE`.

- [x] [LG-404] (P1) Demo UI: apply missing styles and fix TAuth script load order so wallet/actions work. Resolved: added `demo/ui/styles.css`; updated `<mpr-header>` to `tauth-*`/`google-site-id`/`tauth-tenant-id`; ensured `tauth.js` is present before `mpr-ui.js` boots; UI now renders balances and disables actions until authenticated; `make ci` + `cd demo && make ci` passing.
  - Demo page currently renders largely unstyled because it uses custom classnames without a CSS file.
  - `mpr-ui` auth bootstrap expects `window.initAuthClient` to exist when `mpr-ui.js` runs; the current dynamic loader can race and prevent auth events (wallet never loads).

- [x] [LG-405] (P1) Demo stack: serve the UI over HTTPS on `:4443` via ghttp using the computercat TLS cert/key and proxy auth/API routes through the same origin. Resolved: ghttp now terminates TLS on host `:4443` using `demo/certs`, proxies `/api` + TAuth routes, and demo docs/config derive base URLs from the current origin; `make ci` + `cd demo && make ci` passing.
  - Replace the HTTP-only `:8000` demo UI entrypoint with `https://localhost:4443`.
  - Wire ghttp TLS with the `computercat-cert.pem` / `computercat-key.pem` pair and proxy `/api`, `/auth`, `/me`, `/tauth.js` to the backing services.

- [x] [LG-406] (P1) Demo stack: make TAuth cookies host-only so auth works on `computercat.tyemirov.net` and LAN origins. Resolved: demo TAuth `cookie_domain`/`APP_COOKIE_DOMAIN` now empty (host-only), so cookies are issued for the active origin; `make ci` + `cd demo && make ci` passing.

- [x] [LG-407] (P1) Demo stack: ensure Postgres schema is migrated by GORM when running Compose. Resolved: `demo/docker-compose.yml` now builds `ledgerd` from the repo `Dockerfile` (includes LG-214 Postgres `AutoMigrate`) so fresh Postgres volumes get tables automatically; `make ci` + `cd demo && make ci` passing.

- [x] [LG-408] (P1) Demo UI: keep mpr-ui and page styles on the same light/dark theme (avoid mixed palettes). Resolved: `data-mpr-theme` is now the single source of truth (set on `<html>`/`<body>` and toggled via the footer theme switcher); custom demo CSS now keys off `data-mpr-theme`; `cd demo && make ci` passing.

- [x] [LG-409] (P2) Demo UI: remove the header Account/settings button. Resolved: removed `<mpr-header>` settings attributes so the Account button no longer renders; `make ci` + `cd demo && make ci` passing.

- [x] [LG-410] (P2) Demo UI: square theme switcher should expose four modes (not two). Resolved: footer theme-config now defines 4 modes (default-light, sunrise-light, default-dark, forest-dark) and demo CSS keys palette overrides off `data-demo-palette`; `make ci` + `cd demo && make ci` passing.


## Planning (500–599)
*do not implement yet*
