# ISSUES ARCHIVE

Resolved non-recurring history moved out of `.mprlab/ISSUES.md` during backlog hygiene runs.
Recurring runbooks remain active in `.mprlab/ISSUES.md`.

Archive note 2026-08-09:
- moved 27 resolved non-recurring entries out of the active tracker;
- preserved recurring Maintenance history (`M009R`-`M024R`) in `.mprlab/ISSUES.md`;
- corrected stale issue references (`I212`→`I009`, `I216`→`I013`, `I214`→`I011`) while preserving the resolved history.

## BugFixes

- [x] [B001] (P1) Allow negative totals from SumTotal so expired grants don't break balance/spend flows.
  Resolved: signed totals added; balance/spend now handle negatives without store errors.
  - Remove rejection of negative sums and ensure Reserve/Spend returns ErrInsufficientFunds when totals are negative.

- [x] [B002] (P1) Treat file:// SQLite DSNs as sqlite, not unsupported.
  Resolved: `resolveDriver` now maps `file:` DSNs to sqlite and preserves `file::memory:?cache=shared`; coverage restored and `make ci` passing.
  - `cmd/credit` resolveDriver currently returns `file` as an unsupported database scheme when the DSN is `file://...`.
  - Ensure file-based SQLite DSNs like `file:///tmp/ledger.db` and `file::memory:?cache=shared` are treated as sqlite and continue to work.

- [x] [B003] (P1) Align issue external IDs with the ISSUES.ED parser.
  Resolved: migrated legacy `LG-###` entries in `.mprlab/ISSUES.md` to section-coded external IDs and normalized section headings, then removed remaining repo-local legacy `LG-*` doc labels that no longer map to the current tracker scheme.

- [x] [B004] (P1) Own the release, container, and deployment lifecycle.
  Ledger's release lifecycle depended on sibling `agentSkills/gitrelease` tooling and its deployable runtime was not declared at the canonical app-owned path.
  Resolution 2026-07-30: Ledger now owns only the declarative schema-v2 `.mprlab/deploy/resources.yml` input and the exact zero-argument `make release`, `make publish`, and `make deploy` delegators. The exact sibling `../mprlab-gateway` owns the Ansible lifecycle, receipts, immutable publication, and deployment reconciliation. Ledger declares one Go container, a fresh retained `ledger-data` volume, bounded retirement of the legacy `mprlab-nginx-gateway/ledger-api` service without removing its old volume, four operator secret references, and the `ledger.grpc` endpoint with TCP readiness. Repository-owned lifecycle scripts and their obsolete tests were deleted; no Node dependency, compatibility path, release, publication, or production deployment was introduced.

- [x] [B005] (P0) Make the canonical release, publish, and deploy lifecycle safely idempotent.
  Repeating `make release` at the exact `v1.0.3` release commit incorrectly selected `v1.0.4`, erased the local prepared-artifact directory, rebuilt payloads, and failed only when `v1.0.3..HEAD` produced no release-note commits. Repeated publication also overwrites GitHub assets with `--clobber` and recreates already-correct container tags instead of verifying immutable state, while a completed published release cannot be verified after local staging is absent. Make exact-tag release preparation a verified no-op before any mutation, make publication immutable and resumable, support a read-only published-state verification path when staging is absent, keep gateway deployment convergent, and cover all retry paths through real CLI entrypoints.
  Resolved 2026-07-30: retry and immutable-state behavior is now one gateway-owned contract shared by every application. Ledger no longer carries a second release engine, Python helper, local artifact state machine, or deploy script. Its three public commands delegate the exact selected Git root to the sibling gateway, which seals and verifies the application and gateway identities and reuses exact receipts on retry.

## Improvements

- [x] [I001] (P1) Extract ledger core into a reusable Go library.
  Resolved: domain types + store interfaces enforced, adapters updated, tests/ci passing.
  - Promote `internal/credit` into a public `pkg/ledger` module with explicit domain types and invariants.
    - Define a storage interface suitable for both in-process and service-hosted deployments.
    - Provide a default SQL-backed implementation (adapting existing gorm stores) while keeping the core domain independent of GORM.

- [x] [I002] (P2) Add integration documentation for service and library usage.
  Resolved: expanded integration guide with domain types, store wiring, and error contracts.
  - Document how to run ledger as a standalone gRPC microservice (config, migrations, networking) and how to consume it from other languages.
    - Document how to embed the future `pkg/ledger` library in Go services, including storage wiring, transaction patterns, and error contracts.

- [x] [I003] (P2) Support multiple ledgers per user.
  Resolved: ledger_id threaded through API/service/store/schema; demo/docs updated; migration path omitted per no-backward-compat requirement.
  - Allow a single user_id to own multiple ledger accounts (introduce a ledger/account namespace or composite key).
    - Update storage constraints, API inputs, and reservation/entry lookups to include the ledger identifier.
    - Provide a migration path for existing single-ledger data.

- [x] [I004] (P2) Introduce first-class multi-tenant support (tenant_id).
  Resolved: tenant_id required across API/service/store/schema; demo/docs/examples updated; no migration path.
  - Require tenant_id in API/service/store boundaries and schema keys.
    - Update demo/docs/examples to send tenant_id alongside ledger_id and user_id.
    - Skip migration path (backward compatibility not required).

- [x] [I005] (P2) Make demo tenant_id and ledger_id defaults configurable via env.
  Resolved: demo config/flags use env-backed defaults; env templates/docs/tests updated, tooling passing.
  - Add DEMOAPI_DEFAULT_TENANT_ID and DEMOAPI_DEFAULT_LEDGER_ID to demo config and env templates.
    - Update demo handlers and docs to use config values instead of hardcoded defaults.

- [x] [I006] (P2) Make ledger data directory configurable for Docker workflows.
  Resolved: data dir is only used by DATABASE_URL; no extra env added, compose mounts align to `/srv/data`, tooling passing.
  - Add LEDGER_DATA_DIR to .env.ledger and wire compose volume targets to use it.
    - Update compose wiring so ledger uses the configured data directory.

- [x] [I007] (P1) Add server-managed bootstrap grants for new accounts.
  Resolved: introduced `BootstrapGrantPolicy` + `BOOTSTRAP_GRANTS_JSON` config and applied a deterministic one-time grant on first account access (new/empty accounts only), with idempotency-safe retries under concurrency; docs + env templates updated; `make ci` passing.
  - Provide optional bootstrap configuration (amount/metadata/idempotency prefix) per tenant+ledger.
  - Apply the bootstrap grant exactly once when an account is created (or first accessed), without requiring the caller to orchestrate a grant.
  - Use deterministic idempotency keys so repeated calls are safe; treat duplicate idempotency as no-op.
  - Update config/env/README and add coverage for concurrent account creation.

- [x] [I008] (P1) Add a backfill/bootstrap command for existing accounts.
  Resolved: added `ledgerd bootstrap-backfill` CLI to apply configured bootstrap grants to existing accounts missing them (scoped per tenant+ledger), introduced store-level account listing/pagination to support backfill without raw SQL, implemented idempotency-safe no-op handling (duplicates allowed only when existing entry type is `grant`), and added coverage for large datasets + error paths; `make ci` passing.
  - Provide a CLI/admin command to apply the configured bootstrap grant to all existing accounts missing it.
  - Add store-level account listing/pagination to support backfill without direct SQL in callers.
  - Treat duplicate idempotency keys as no-op; emit a summary of accounts updated vs skipped.
  - Document the workflow and add integration tests for large account sets.

- [x] [I009] (P1) Support grant-only history and "last grant" queries in the gRPC API.
  Resolved: `ListEntriesRequest.types` filter enables grant-only paging and `limit=1` last-grant lookups; `make ci` passing.
  - Callers need to display "last grant" reliably without paging through large volumes of non-grant entries (holds/spends/captures).
  - Options:
    - Add `type` filtering (or a dedicated `ListGrants` RPC) so clients can request only grant entries.
    - Add a `GetLastGrant` RPC that returns the most recent grant entry (entry_id, amount_cents, created_unix_utc, metadata_json).
  - Ensure results are ordered by creation timestamp and include deterministic pagination/cursors for high-activity accounts.

- [x] [I010] (P1) Use PostgreSQL in Docker Compose orchestration (replace SQLite).
  Resolved: root + demo compose now provision Postgres and run `db/migrations.sql` via a one-shot migrator; `.env.ledger` defaults to Postgres; docs updated; `make ci` passing.

- [x] [I011] (P1) Run Postgres migrations via GORM (remove manual SQL migrator).
  Resolved: `ledgerd` now `AutoMigrate`s for SQLite+Postgres; compose `migrate` services removed; `db/migrations.sql` deleted; docs updated; `make ci` passing.

- [x] [I012] (P0) Add batch gRPC operations for high-volume credit mutations.
  Resolved: added unary Batch RPC with atomic/best-effort semantics and per-item results, enforced `maxBatchOperations=5000`, implemented Postgres savepoint-backed nested tx support, and added coverage across service/store/grpc; `make ci` passing.
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

- [x] [I013] (P0) Add first-class refunds referencing debit entries (spend/capture).
  Resolved: added `Refund` RPC + refund ledger entry type referencing original debit entries, enforced refund<=debit invariants with idempotency-safe retries, updated stores + gRPC server, and expanded coverage; `make ci` passing.
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

- [x] [I014] (P1) Support reservation TTLs and automatic expiry cleanup.
  Resolved: added `expires_at_unix_utc` to `Reserve` + `BatchReserve` APIs, persisted TTL on reservations and hold entries, excluded expired active reservations from `available_cents` calculations, and rejected capture attempts on expired reservations; coverage added and `make ci` passing.
  Context: leaked holds (reservations that are never released due to caller crashes / canceled contexts) permanently reduce `available_cents`. Consumers need a safety net even when they use Reserve/Capture/Release flows.
  Deliverables:
  - Extend `ReserveRequest` with `expires_at_unix_utc` (mirroring `GrantRequest`), and treat expired holds as no longer impacting `available_cents`.
  - Decide on cleanup semantics:
    - Option A: ignore expired holds when computing balance/available and when enforcing spend/reserve.
    - Option B (preferred for auditability): lazily emit an automatic `release` entry for expired holds using a deterministic idempotency key (for example `auto_release:<reservation_id>:<expires_at_unix_utc>`), so the ledger remains append-only and explainable.
  - Add deterministic behavior under concurrent cleanup (avoid double-releasing).
  - Add tests with an injected clock to validate expiry and ensure available funds recover after TTL.

- [x] [I015] (P1) Add reservation introspection APIs (GetReservation / ListReservations).
  Resolved: added gRPC APIs returning computed reservation state (held/captured/expired + timestamps) with store support for paging/filtering; tests added and `make ci` passing.
  Context: today, callers cannot reliably introspect reservation state without paging and aggregating `ListEntries`, which is slow and brittle for high-activity accounts.
  Deliverables:
  - Add `GetReservation` to return the computed state for a `reservation_id` (reserved, captured, released, remaining held, created time, expires time, status).
  - Add `ListReservations` to page reservations for an account with optional filters (status, created time cursor).
  - Ensure computations are consistent with `GetBalance` enforcement rules (especially once TTL/expiry is supported).
  - Add integration tests covering partial capture, full capture, release, and expiry states.

- [x] [I016] (P2) Improve gRPC ergonomics: return entry IDs and add ListEntries filtering.
  Resolved: `Empty` responses now include `entry_id` + `created_unix_utc`, and `ListEntriesRequest` supports `types`, `reservation_id`, and `idempotency_key_prefix`; store/service/server/tests updated; `make ci` passing.
  Context: mutating RPCs currently return `Empty`, forcing clients to call `ListEntries` (and sometimes page) to correlate actions to ledger entries. This is especially painful for operational tooling and for "last grant" UX.
  Deliverables:
  - Return `entry_id` + `created_unix_utc` from mutating RPCs (`Grant`, `Spend`, `Reserve`, `Capture`, `Release`) and optionally include the updated balance in the response to reduce round-trips.
  - Extend `ListEntriesRequest` with server-side filters (at least `type`, `reservation_id`, and `idempotency_key` prefix) plus deterministic pagination/cursors.
  - Align with I009 ("last grant") so the API can satisfy grant-only and last-grant queries without client-side paging.
  - Add tests asserting filters are applied correctly and pagination is stable.

- [x] [I017] (P1) Add Refund support to Batch gRPC operations.
  Resolved: added `BatchRefundOp` to proto + Batch execution path, supporting refund-by-entry-id and refund-by-original-idempotency-key with idempotency-safe duplicates and over-refund rejection; coverage added; `make ci` passing.
  Context: after I013, callers can create first-class refunds, but batch flows cannot yet issue many refunds in one request. High-volume consumers should be able to batch reimbursements without falling back to thousands of unary `Refund` calls.
  Deliverables:
  - Extend `BatchOperation` with `RefundOp` supporting `oneof original { original_entry_id | original_idempotency_key }` plus `amount_cents`, `idempotency_key`, `metadata_json`.
  - Implement atomic/best-effort semantics consistent with existing Batch behavior (duplicate idempotency treated as success, surfaced as `duplicate=true` in per-item results).
  - Add coverage for batch refund by entry id and by original idempotency key, including over-refund rejection and duplicate idempotency handling.

- [x] [I018] (P2) Docs: clarify Capture/Release idempotency semantics.
  Resolved: added `docs/api.md` as the gRPC reference and documented that `Capture`/`Release` safe retries may return `reservation_closed` (state checked before idempotency), preventing clients from treating retries as hard failures; `make ci` passing.

- [x] [I019] (P1) Document ledger API and semantics (Refund/Batch/Reservations/Idempotency).
  Resolved: expanded README + integration guide and added an API reference doc covering RPCs, request/response fields, idempotency/duplicate semantics, refunds, batch behavior, reservation TTL/expiry, and introspection APIs; `make ci` passing.
  - Update README usage examples to include Refund, Batch, GetReservation/ListReservations, and ListEntries filtering.
  - Document idempotency expectations and duplicate semantics (including batch `duplicate=true` vs error behavior on key collisions).
  - Document refunds referencing spend/capture debits and the enforced invariant that refunds cannot exceed the original debit.
  - Add a single coherent API reference doc (`docs/api.md`) and link it from README/integration docs.

- [x] [I020] (P1) Remove server-managed bootstrap grants/backfill to keep ledger client-agnostic.
  Summary: ledger must remain a generic, client-agnostic transactional service. Server-managed bootstrap grants (and the bootstrap-backfill admin command) embed client policy into the ledger ("new account gets X cents"), making the ledger non-neutral and causing hidden, state-mutating side effects on read-like flows (e.g. `GetBalance` can write).
  Desired end state:
  - Ledger provides only explicit, transactional primitives (`Grant`, `Spend`, `Reserve/Capture/Release`, `Refund`, `Batch`, introspection).
  - Client apps implement "bootstrap credits" by issuing an explicit `Grant` with a deterministic idempotency key and metadata.
  Deliverables:
  - Remove `BOOTSTRAP_GRANTS_JSON` configuration and `--bootstrap-grants-json` flag.
  - Remove `ledgerd bootstrap-backfill` command (and any bootstrap-only store plumbing if it becomes unused).
  - Remove `BootstrapGrantPolicy` and `WithBootstrapGrantPolicy`, plus all "apply bootstrap on access" logic from `pkg/ledger.Service`.
  - Update docs (`README.md`, `docs/integration.md`, `docs/api.md`, `.env.ledger`) to remove bootstrap references and document client-side bootstrap via `Grant` + idempotency.
  - Tooling: `timeout -k 350s -s SIGKILL 350s make ci` passes (coverage gate included).
  Resolved 2026-02-10: removed `BOOTSTRAP_GRANTS_JSON` + bootstrap grant policy/backfill tooling so ledger mutations are always explicit RPCs; updated docs/env templates and deleted bootstrap-only store plumbing; `make ci` passing.

- [x] [I021] (P2) Demo stack: showcase Refund/Batch/Reservations capabilities end-to-end.
  Summary: the demo stack currently exercises only `Grant` + `Spend` against `ledgerd`. Expand the demo backend + UI to showcase the newer ledger capabilities (refunds, batch RPC, and reservation flows) so adopters can validate semantics quickly without reading proto docs.
  Deliverables:
  - Demo backend (`demo/backend`): add HTTP endpoints that drive ledger RPCs for:
    - Reservation flow: `Reserve`, `Capture`, `Release`, and introspection via `GetReservation` / `ListReservations`.
    - Refund flow: unary `Refund` (idempotent full refund for a selected spend/capture debit).
    - Batch flow: demonstrate `Batch` with `Spend` and `Refund` operations (atomic vs best-effort).
  - Demo UI (`demo/ui`):
    - Add controls to create/capture/release holds and display reservation state.
    - Add refund actions for debit entries (refund-by-entry-id) and a batch spend/refund action.
    - Keep bootstrap credits client-managed (explicit `Grant` with deterministic idempotency; duplicates treated as success).
  - Docs: update `demo/README.md` scenario checklist to include the new demo actions and the ledger RPCs they exercise.
  - Validation: `timeout -k 350s -s SIGKILL 350s make ci` and `timeout -k 350s -s SIGKILL 350s (cd demo && make ci)` pass.
  Resolved 2026-02-10: demo backend now exposes reservation/refund/batch endpoints; demo UI includes hold capture/release, per-entry refunds, and batch spend/refund controls; `demo/README.md` updated; tooling passing.

- [x] [I022] (P0) Adopt the sibling gateway schema-v3 lifecycle.
  Goal:
  Make the committed Ledger production declaration consumable by the current exact sibling gateway without changing local demo orchestration or running a production lifecycle stage.
  Requirements:
  - Preserve the one Ledger image, singular gateway placement, fresh retained `ledger-data` volume, bounded retirement of `mprlab-nginx-gateway/ledger-api`, committed runtime config, and `ledger.grpc` capability.
  - Replace schema-v2 top-level dependencies, project placement, profiles, and direct secret references with schema v3, per-service placement, and one typed private-values resource.
  - Keep one ignored mode-0600 `.mprlab/deploy/.env` input containing the four exact production values and exclude it from every Docker context that contains it.
  - Keep local demo Compose and the zero-argument `make release`, `make publish`, and `make deploy` wrappers unchanged.
  Deliverables:
  - Current manifest, private-input boundary, deployment documentation, changelog, and black-box lifecycle contract coverage.
  Validation:
  - `make fmt`, `make lint`, `make test`, and `make ci` pass.
  - The clean sibling gateway passes release, publish, and deploy plans plus selected-manifest isolation for the committed Ledger revision.
  - No release, publication, deployment, production access, or unrelated application inspection occurs.
  Resolution 2026-08-03: migrated the complete Ledger production graph to schema v3 with singular service placement, four typed private outputs, an exact Docker exclusion, and preserved fresh/legacy volume boundaries; Ledger formatting, lint, tests, and full CI passed with 100% production coverage, and the clean sibling gateway passed release, publish, deploy, and selected-manifest-isolation plans without production mutation.

