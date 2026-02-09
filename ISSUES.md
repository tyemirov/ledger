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
- [x] [LG-210] (P1) Add server-managed bootstrap grants for new accounts. Resolved: introduced `BootstrapGrantPolicy` + `BOOTSTRAP_GRANTS_JSON` config and applied a deterministic one-time grant on first account access (new/empty accounts only), with idempotency-safe retries under concurrency; docs + env templates updated; `make ci` passing.
  - Provide optional bootstrap configuration (amount/metadata/idempotency prefix) per tenant+ledger.
  - Apply the bootstrap grant exactly once when an account is created (or first accessed), without requiring the caller to orchestrate a grant.
  - Use deterministic idempotency keys so repeated calls are safe; treat duplicate idempotency as no-op.
  - Update config/env/README and add coverage for concurrent account creation.
- [x] [LG-211] (P1) Add a backfill/bootstrap command for existing accounts. Resolved: added `ledgerd bootstrap-backfill` CLI to apply configured bootstrap grants to existing accounts missing them (scoped per tenant+ledger), introduced store-level account listing/pagination to support backfill without raw SQL, implemented idempotency-safe no-op handling (duplicates allowed only when existing entry type is `grant`), and added coverage for large datasets + error paths; `make ci` passing.
  - Provide a CLI/admin command to apply the configured bootstrap grant to all existing accounts missing it.
  - Add store-level account listing/pagination to support backfill without direct SQL in callers.
  - Treat duplicate idempotency keys as no-op; emit a summary of accounts updated vs skipped.
  - Document the workflow and add integration tests for large account sets.
- [x] [LG-212] (P1) Support grant-only history and "last grant" queries in the gRPC API. Resolved: `ListEntriesRequest.types` filter enables grant-only paging and `limit=1` last-grant lookups; `make ci` passing.
  - Callers need to display "last grant" reliably without paging through large volumes of non-grant entries (holds/spends/captures).
  - Options:
    - Add `type` filtering (or a dedicated `ListGrants` RPC) so clients can request only grant entries.
    - Add a `GetLastGrant` RPC that returns the most recent grant entry (entry_id, amount_cents, created_unix_utc, metadata_json).
  - Ensure results are ordered by creation timestamp and include deterministic pagination/cursors for high-activity accounts.

- [x] [LG-213] (P1) Use PostgreSQL in Docker Compose orchestration (replace SQLite). Resolved: root + demo compose now provision Postgres and run `db/migrations.sql` via a one-shot migrator; `.env.ledger` defaults to Postgres; docs updated; `make ci` passing.

- [x] [LG-214] (P1) Run Postgres migrations via GORM (remove manual SQL migrator). Resolved: `ledgerd` now `AutoMigrate`s for SQLite+Postgres; compose `migrate` services removed; `db/migrations.sql` deleted; docs updated; `make ci` passing.

- [x] [LG-215] (P0) Add batch gRPC operations for high-volume credit mutations. Resolved: added unary Batch RPC with atomic/best-effort semantics and per-item results, enforced `maxBatchOperations=5000`, implemented Postgres savepoint-backed nested tx support, and added coverage across service/store/grpc; `make ci` passing.
  Context: real consumers (for example ProductScanner) may need to issue thousands of refunds/grants for a single job. Doing this via one unary gRPC request per product is slow and easy to break when callers run inside canceled request contexts (leading to partial execution and operational noise).
  Deliverables:
  - Add a unary `Batch` RPC that executes many operations against the same account (`tenant_id`, `ledger_id`, `user_id`) in a single DB transaction, returning per-item results.
  - Support operations: `Grant`, `Spend`, `Reserve`, `Capture`, `Release`.
  - Provide `atomic` (all-or-nothing) vs `best_effort` semantics, with per-item errors/codes for `best_effort` so callers can retry only failed items.
  - Preserve idempotency: each operation carries its own `idempotency_key`; duplicates should be treated as success (and ideally surfaced as `duplicate=true` in the per-item result).
  - Enforce a sane max batch size / request bytes limit and return a stable error when exceeded.
  - Add end-to-end tests that issue a large batch (>= 5k ops) and assert: (1) performance is acceptable, (2) idempotency works, (3) atomic mode rolls back on a failure, (4) best-effort mode returns per-item failure reasons.
  Proposed proto sketch:
  ```proto
  message AccountContext { string user_id = 1; string ledger_id = 2; string tenant_id = 3; }
  message BatchRequest { AccountContext account = 1; repeated BatchOperation operations = 2; bool atomic = 3; }
  message BatchOperation { string operation_id = 1; oneof operation { GrantOp grant = 2; SpendOp spend = 3; ReserveOp reserve = 4; CaptureOp capture = 5; ReleaseOp release = 6; } }
  message BatchOperationResult { string operation_id = 1; bool ok = 2; string error_code = 3; string error_message = 4; string entry_id = 5; bool duplicate = 6; }
  message BatchResponse { repeated BatchOperationResult results = 1; }
  ```

- [x] [LG-216] (P0) Add first-class refunds referencing debit entries (spend/capture). Resolved: added `Refund` RPC + refund ledger entry type referencing original debit entries, enforced refund<=debit invariants with idempotency-safe retries, updated stores + gRPC server, and expanded coverage; `make ci` passing.
  Context: consumers currently use `Grant` to reimburse users, but this loses audit semantics (refund vs grant) and cannot enforce "refund <= original debit".
  Deliverables:
  - Add a `Refund` RPC that creates a `refund` ledger entry referencing an original debit entry (a `spend` or `capture`).
  - Validate that refunds cannot exceed the original debit amount minus prior refunds for the same debit.
  - Preserve idempotency using the provided `idempotency_key` (duplicate idempotency = no-op success).
  - Include enough fields in the response to make it auditable (`entry_id`, `created_unix_utc`, and optionally updated balance).
  - Add coverage for: refund of spend, refund of capture, over-refund rejection, duplicate idempotency handling, and concurrent refunds against the same debit.
  Proposed proto sketch:
  ```proto
  message RefundRequest {
    string user_id = 1;
    string ledger_id = 2;
    string tenant_id = 3;
    oneof original { string original_entry_id = 4; string original_idempotency_key = 5; }
    int64 amount_cents = 6;
    string idempotency_key = 7;
    string metadata_json = 8;
  }
  message RefundResponse { string entry_id = 1; int64 created_unix_utc = 2; BalanceResponse balance = 3; }
  ```

- [x] [LG-217] (P1) Support reservation TTLs and automatic expiry cleanup. Resolved: added `expires_at_unix_utc` to `Reserve` + `BatchReserve` APIs, persisted TTL on reservations and hold entries, excluded expired active reservations from `available_cents` calculations, and rejected capture attempts on expired reservations; coverage added and `make ci` passing.
  Context: leaked holds (reservations that are never released due to caller crashes / canceled contexts) permanently reduce `available_cents`. Consumers need a safety net even when they use Reserve/Capture/Release flows.
  Deliverables:
  - Extend `ReserveRequest` with `expires_at_unix_utc` (mirroring `GrantRequest`), and treat expired holds as no longer impacting `available_cents`.
  - Decide on cleanup semantics:
    - Option A: ignore expired holds when computing balance/available and when enforcing spend/reserve.
    - Option B (preferred for auditability): lazily emit an automatic `release` entry for expired holds using a deterministic idempotency key (for example `auto_release:<reservation_id>:<expires_at_unix_utc>`), so the ledger remains append-only and explainable.
  - Add deterministic behavior under concurrent cleanup (avoid double-releasing).
  - Add tests with an injected clock to validate expiry and ensure available funds recover after TTL.

- [ ] [LG-218] (P1) Add reservation introspection APIs (GetReservation / ListReservations). Unresolved.
  Context: today, callers cannot reliably introspect reservation state without paging and aggregating `ListEntries`, which is slow and brittle for high-activity accounts.
  Deliverables:
  - Add `GetReservation` to return the computed state for a `reservation_id` (reserved, captured, released, remaining held, created time, expires time, status).
  - Add `ListReservations` to page reservations for an account with optional filters (status, created time cursor).
  - Ensure computations are consistent with `GetBalance` enforcement rules (especially once TTL/expiry is supported).
  - Add integration tests covering partial capture, full capture, release, and expiry states.

- [x] [LG-219] (P2) Improve gRPC ergonomics: return entry IDs and add ListEntries filtering. Resolved: `Empty` responses now include `entry_id` + `created_unix_utc`, and `ListEntriesRequest` supports `types`, `reservation_id`, and `idempotency_key_prefix`; store/service/server/tests updated; `make ci` passing.
  Context: mutating RPCs currently return `Empty`, forcing clients to call `ListEntries` (and sometimes page) to correlate actions to ledger entries. This is especially painful for operational tooling and for "last grant" UX.
  Deliverables:
  - Return `entry_id` + `created_unix_utc` from mutating RPCs (`Grant`, `Spend`, `Reserve`, `Capture`, `Release`) and optionally include the updated balance in the response to reduce round-trips.
  - Extend `ListEntriesRequest` with server-side filters (at least `type`, `reservation_id`, and `idempotency_key` prefix) plus deterministic pagination/cursors.
  - Align with LG-212 ("last grant") so the API can satisfy grant-only and last-grant queries without client-side paging.
  - Add tests asserting filters are applied correctly and pagination is stable.

- [x] [LG-220] (P1) Add Refund support to Batch gRPC operations. Resolved: added `BatchRefundOp` to proto + Batch execution path, supporting refund-by-entry-id and refund-by-original-idempotency-key with idempotency-safe duplicates and over-refund rejection; coverage added; `make ci` passing.
  Context: after LG-216, callers can create first-class refunds, but batch flows cannot yet issue many refunds in one request. High-volume consumers should be able to batch reimbursements without falling back to thousands of unary `Refund` calls.
  Deliverables:
  - Extend `BatchOperation` with `RefundOp` supporting `oneof original { original_entry_id | original_idempotency_key }` plus `amount_cents`, `idempotency_key`, `metadata_json`.
  - Implement atomic/best-effort semantics consistent with existing Batch behavior (duplicate idempotency treated as success, surfaced as `duplicate=true` in per-item results).
  - Add coverage for batch refund by entry id and by original idempotency key, including over-refund rejection and duplicate idempotency handling.


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

- [x] [LG-411] (P2) Demo UI: show account details (photo/name/email) from the header user menu; remove the hero session card. Resolved: added a user-menu action that opens an account-details modal; removed the session card; Playwright coverage added; `make ci` + `cd demo && make ci` passing.

- [x] [LG-412] (P1) Demo UI: footer theme switcher does not switch themes. Resolved: header now uses the same 4-mode theme-config as footer so themeManager config isn't overridden; Playwright coverage added; `cd demo && make ci` passing.

- [x] [LG-413] (P2) Demo UI: "Docs" header link should open the rendered integration guide (not the GitHub repo root). Resolved: header now points to `docs/integration.md` on GitHub; Playwright coverage added; `cd demo && make ci` passing.

- [x] [LG-414] (P2) Demo UI: footer links menu should match the mpr-ui demo site catalog ("Built by Marco Polo Research Lab"). Resolved: footer dropdown updated to the mpr-ui site catalog; Playwright assertions added; `cd demo && make ci` passing.


## Planning (500–599)
*do not implement yet*
