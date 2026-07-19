# ISSUES (Append-only Log)

Entries record newly discovered requests or changes, with their outcomes. No instructive content lives here. Read @NOTES.md for the process to follow when fixing issues.

Read @AGENTS.md, @AGENTS.GO.md, @AGENTS.DOCKER.md, @AGENTS.FRONTEND.md, @AGENTS.GIT.md, @POLICY.md, @NOTES.md, @README.md and @ISSUES.md. Start working on open issues. Work autonomously and stack up PRs.

Each issue is formatted as `- [ ] [<section-letter><number>]` (`F`, `I`, `B`, `M`, or `P`). When resolved it becomes `- [x] [<section-letter><number>]`.

## Features

## Improvements

- [x] [I204] (P1) Extract ledger core into a reusable Go library. Resolved: domain types + store interfaces enforced, adapters updated, tests/ci passing.
  - Promote `internal/credit` into a public `pkg/ledger` module with explicit domain types and invariants.
    - Define a storage interface suitable for both in-process and service-hosted deployments.
    - Provide a default SQL-backed implementation (adapting existing gorm stores) while keeping the core domain independent of GORM.
- [x] [I205] (P2) Add integration documentation for service and library usage. Resolved: expanded integration guide with domain types, store wiring, and error contracts.
  - Document how to run ledger as a standalone gRPC microservice (config, migrations, networking) and how to consume it from other languages.
    - Document how to embed the future `pkg/ledger` library in Go services, including storage wiring, transaction patterns, and error contracts.
- [x] [I206] (P2) Support multiple ledgers per user. Resolved: ledger_id threaded through API/service/store/schema; demo/docs updated; migration path omitted per no-backward-compat requirement.
  - Allow a single user_id to own multiple ledger accounts (introduce a ledger/account namespace or composite key).
    - Update storage constraints, API inputs, and reservation/entry lookups to include the ledger identifier.
    - Provide a migration path for existing single-ledger data.
- [x] [I207] (P2) Introduce first-class multi-tenant support (tenant_id). Resolved: tenant_id required across API/service/store/schema; demo/docs/examples updated; no migration path.
  - Require tenant_id in API/service/store boundaries and schema keys.
    - Update demo/docs/examples to send tenant_id alongside ledger_id and user_id.
    - Skip migration path (backward compatibility not required).
- [x] [I208] (P2) Make demo tenant_id and ledger_id defaults configurable via env. Resolved: demo config/flags use env-backed defaults; env templates/docs/tests updated, tooling passing.
  - Add DEMOAPI_DEFAULT_TENANT_ID and DEMOAPI_DEFAULT_LEDGER_ID to demo config and env templates.
    - Update demo handlers and docs to use config values instead of hardcoded defaults.
- [x] [I209] (P2) Make ledger data directory configurable for Docker workflows. Resolved: data dir is only used by DATABASE_URL; no extra env added, compose mounts align to `/srv/data`, tooling passing.
  - Add LEDGER_DATA_DIR to .env.ledger and wire compose volume targets to use it.
    - Update compose wiring so ledger uses the configured data directory.
- [x] [I210] (P1) Add server-managed bootstrap grants for new accounts. Resolved: introduced `BootstrapGrantPolicy` + `BOOTSTRAP_GRANTS_JSON` config and applied a deterministic one-time grant on first account access (new/empty accounts only), with idempotency-safe retries under concurrency; docs + env templates updated; `make ci` passing.
  - Provide optional bootstrap configuration (amount/metadata/idempotency prefix) per tenant+ledger.
  - Apply the bootstrap grant exactly once when an account is created (or first accessed), without requiring the caller to orchestrate a grant.
  - Use deterministic idempotency keys so repeated calls are safe; treat duplicate idempotency as no-op.
  - Update config/env/README and add coverage for concurrent account creation.
- [x] [I211] (P1) Add a backfill/bootstrap command for existing accounts. Resolved: added `ledgerd bootstrap-backfill` CLI to apply configured bootstrap grants to existing accounts missing them (scoped per tenant+ledger), introduced store-level account listing/pagination to support backfill without raw SQL, implemented idempotency-safe no-op handling (duplicates allowed only when existing entry type is `grant`), and added coverage for large datasets + error paths; `make ci` passing.
  - Provide a CLI/admin command to apply the configured bootstrap grant to all existing accounts missing it.
  - Add store-level account listing/pagination to support backfill without direct SQL in callers.
  - Treat duplicate idempotency keys as no-op; emit a summary of accounts updated vs skipped.
  - Document the workflow and add integration tests for large account sets.
- [x] [I212] (P1) Support grant-only history and "last grant" queries in the gRPC API. Resolved: `ListEntriesRequest.types` filter enables grant-only paging and `limit=1` last-grant lookups; `make ci` passing.
  - Callers need to display "last grant" reliably without paging through large volumes of non-grant entries (holds/spends/captures).
  - Options:
    - Add `type` filtering (or a dedicated `ListGrants` RPC) so clients can request only grant entries.
    - Add a `GetLastGrant` RPC that returns the most recent grant entry (entry_id, amount_cents, created_unix_utc, metadata_json).
  - Ensure results are ordered by creation timestamp and include deterministic pagination/cursors for high-activity accounts.

- [x] [I213] (P1) Use PostgreSQL in Docker Compose orchestration (replace SQLite). Resolved: root + demo compose now provision Postgres and run `db/migrations.sql` via a one-shot migrator; `.env.ledger` defaults to Postgres; docs updated; `make ci` passing.

- [x] [I214] (P1) Run Postgres migrations via GORM (remove manual SQL migrator). Resolved: `ledgerd` now `AutoMigrate`s for SQLite+Postgres; compose `migrate` services removed; `db/migrations.sql` deleted; docs updated; `make ci` passing.

- [x] [I215] (P0) Add batch gRPC operations for high-volume credit mutations. Resolved: added unary Batch RPC with atomic/best-effort semantics and per-item results, enforced `maxBatchOperations=5000`, implemented Postgres savepoint-backed nested tx support, and added coverage across service/store/grpc; `make ci` passing.
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

- [x] [I216] (P0) Add first-class refunds referencing debit entries (spend/capture). Resolved: added `Refund` RPC + refund ledger entry type referencing original debit entries, enforced refund<=debit invariants with idempotency-safe retries, updated stores + gRPC server, and expanded coverage; `make ci` passing.
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

- [x] [I217] (P1) Support reservation TTLs and automatic expiry cleanup. Resolved: added `expires_at_unix_utc` to `Reserve` + `BatchReserve` APIs, persisted TTL on reservations and hold entries, excluded expired active reservations from `available_cents` calculations, and rejected capture attempts on expired reservations; coverage added and `make ci` passing.
  Context: leaked holds (reservations that are never released due to caller crashes / canceled contexts) permanently reduce `available_cents`. Consumers need a safety net even when they use Reserve/Capture/Release flows.
  Deliverables:
  - Extend `ReserveRequest` with `expires_at_unix_utc` (mirroring `GrantRequest`), and treat expired holds as no longer impacting `available_cents`.
  - Decide on cleanup semantics:
    - Option A: ignore expired holds when computing balance/available and when enforcing spend/reserve.
    - Option B (preferred for auditability): lazily emit an automatic `release` entry for expired holds using a deterministic idempotency key (for example `auto_release:<reservation_id>:<expires_at_unix_utc>`), so the ledger remains append-only and explainable.
  - Add deterministic behavior under concurrent cleanup (avoid double-releasing).
  - Add tests with an injected clock to validate expiry and ensure available funds recover after TTL.

- [x] [I218] (P1) Add reservation introspection APIs (GetReservation / ListReservations). Resolved: added gRPC APIs returning computed reservation state (held/captured/expired + timestamps) with store support for paging/filtering; tests added and `make ci` passing.
  Context: today, callers cannot reliably introspect reservation state without paging and aggregating `ListEntries`, which is slow and brittle for high-activity accounts.
  Deliverables:
  - Add `GetReservation` to return the computed state for a `reservation_id` (reserved, captured, released, remaining held, created time, expires time, status).
  - Add `ListReservations` to page reservations for an account with optional filters (status, created time cursor).
  - Ensure computations are consistent with `GetBalance` enforcement rules (especially once TTL/expiry is supported).
  - Add integration tests covering partial capture, full capture, release, and expiry states.

- [x] [I219] (P2) Improve gRPC ergonomics: return entry IDs and add ListEntries filtering. Resolved: `Empty` responses now include `entry_id` + `created_unix_utc`, and `ListEntriesRequest` supports `types`, `reservation_id`, and `idempotency_key_prefix`; store/service/server/tests updated; `make ci` passing.
  Context: mutating RPCs currently return `Empty`, forcing clients to call `ListEntries` (and sometimes page) to correlate actions to ledger entries. This is especially painful for operational tooling and for "last grant" UX.
  Deliverables:
  - Return `entry_id` + `created_unix_utc` from mutating RPCs (`Grant`, `Spend`, `Reserve`, `Capture`, `Release`) and optionally include the updated balance in the response to reduce round-trips.
  - Extend `ListEntriesRequest` with server-side filters (at least `type`, `reservation_id`, and `idempotency_key` prefix) plus deterministic pagination/cursors.
  - Align with I212 ("last grant") so the API can satisfy grant-only and last-grant queries without client-side paging.
  - Add tests asserting filters are applied correctly and pagination is stable.

- [x] [I220] (P1) Add Refund support to Batch gRPC operations. Resolved: added `BatchRefundOp` to proto + Batch execution path, supporting refund-by-entry-id and refund-by-original-idempotency-key with idempotency-safe duplicates and over-refund rejection; coverage added; `make ci` passing.
  Context: after I216, callers can create first-class refunds, but batch flows cannot yet issue many refunds in one request. High-volume consumers should be able to batch reimbursements without falling back to thousands of unary `Refund` calls.
  Deliverables:
  - Extend `BatchOperation` with `RefundOp` supporting `oneof original { original_entry_id | original_idempotency_key }` plus `amount_cents`, `idempotency_key`, `metadata_json`.
  - Implement atomic/best-effort semantics consistent with existing Batch behavior (duplicate idempotency treated as success, surfaced as `duplicate=true` in per-item results).
  - Add coverage for batch refund by entry id and by original idempotency key, including over-refund rejection and duplicate idempotency handling.

- [x] [I224] (P2) Docs: clarify Capture/Release idempotency semantics. Resolved: added `docs/api.md` as the gRPC reference and documented that `Capture`/`Release` safe retries may return `reservation_closed` (state checked before idempotency), preventing clients from treating retries as hard failures; `make ci` passing.
- [x] [I221] (P1) Document ledger API and semantics (Refund/Batch/Reservations/Idempotency). Resolved: expanded README + integration guide and added an API reference doc covering RPCs, request/response fields, idempotency/duplicate semantics, refunds, batch behavior, reservation TTL/expiry, and introspection APIs; `make ci` passing.
  - Update README usage examples to include Refund, Batch, GetReservation/ListReservations, and ListEntries filtering.
  - Document idempotency expectations and duplicate semantics (including batch `duplicate=true` vs error behavior on key collisions).
  - Document refunds referencing spend/capture debits and the enforced invariant that refunds cannot exceed the original debit.
  - Add a single coherent API reference doc (`docs/api.md`) and link it from README/integration docs.

- [x] [I222] (P1) Remove server-managed bootstrap grants/backfill to keep ledger client-agnostic
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

- [x] [I223] (P2) Demo stack: showcase Refund/Batch/Reservations capabilities end-to-end
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


## BugFixes

- [x] [B303] (P1) Allow negative totals from SumTotal so expired grants don't break balance/spend flows. Resolved: signed totals added; balance/spend now handle negatives without store errors.
  - Remove rejection of negative sums and ensure Reserve/Spend returns ErrInsufficientFunds when totals are negative.

- [x] [B304] (P1) Treat file:// SQLite DSNs as sqlite, not unsupported. Resolved: `resolveDriver` now maps `file:` DSNs to sqlite and preserves `file::memory:?cache=shared`; coverage restored and `make ci` passing.
  - `cmd/credit` resolveDriver currently returns `file` as an unsupported database scheme when the DSN is `file://...`.
  - Ensure file-based SQLite DSNs like `file:///tmp/ledger.db` and `file::memory:?cache=shared` are treated as sqlite and continue to work.

- [x] [B305] (P1) Align issue external IDs with the ISSUES.ED parser. Resolved: migrated legacy `LG-###` entries in `.mprlab/ISSUES.md` to section-coded external IDs and normalized section headings, then removed remaining repo-local legacy `LG-*` doc labels that no longer map to the current tracker scheme.

- [x] [B306] (P1) Own the release, container, and deployment lifecycle.
  Ledger's release lifecycle depended on sibling `agentSkills/gitrelease` tooling and its deployable runtime was not declared at the canonical app-owned path.
  Resolution 2026-07-18: vendored the release/container helper bundle, routed `make release`, `make publish`, and `make deploy` through repo-owned entrypoints, declared the app-owned deployment resource and runtime assets, and added black-box release-contract coverage. Production reachability lint now excludes `_test.go`-only contract packages without weakening dead production-package detection. Focused `make check-unused-packages` and the complete `make ci` passed with 100% statement coverage for every production package; no release, publish, or deployment action ran.

## Maintenance

### Recurring

- [ ] [M400R] (P2) Backlog hygiene and archive
  Goal:
  Keep the issue tracker reliable, readable, and focused on active work while preserving resolved history in the appropriate archive.

  Requirements:
  - Cadence: run weekly during active development and before each release cut.
  - Validate section names, identifier prefixes, recurrence suffixes, priority markers, dependencies, and duplicate IDs against the current `issues-md-format.md`.
  - Reconcile stale statuses, duplicate issues, broken references, obsolete instructions, and entries filed under the wrong section.
  - Move completed non-recurring history to the repository issue archive or durable documentation when the active tracker becomes noisy.
  - Keep active, blocked, planning, and recurring entries visible in `ISSUES.md`.

  Deliverables:
  - Normalized `ISSUES.md` structure and statuses.
  - Updated issue archive or docs when completed entries are removed from the active tracker.
  - A short `Last run:` note summarizing the cleanup and any follow-up issues filed.

  Validation:
  - Re-read `ISSUES.md` after edits and confirm every issue is under the right section with a unique section-aware ID.
  - Confirm recurring entries remain open and keep the `R` suffix.
  - Confirm no active, blocked, recurring, or planning work was archived.

- [ ] [M401R] (P2) Polish open issues
  Goal:
  Keep unresolved work executable by making each open issue concrete, ordered, and testable.

  Requirements:
  - Cadence: run weekly during active development and before handing a repo to automated execution.
  - Review every unresolved non-recurring issue for missing context, dependencies, repro steps, acceptance criteria, and validation expectations.
  - Make priorities concrete and ensure each open issue has actionable deliverables.
  - Merge duplicate open issues or add explicit dependency links when separate entries must remain.
  - Do not close or implement issues as part of this polish pass unless that work is separately requested.

  Deliverables:
  - Open issues with enough detail for a person or agent to execute without rediscovery.
  - New or updated dependency markers where ordering matters.
  - A short `Last run:` note listing the number of issues polished and any blockers found.

  Validation:
  - Sample the open entries after the pass and confirm each has clear next actions and validation expectations.
  - Confirm no recurring runbook was marked complete.
  - Confirm duplicates were merged or explicitly cross-referenced.

- [ ] [M402R] (P2) Architecture and policy review
  Goal:
  Catch architecture, policy, and workflow drift before it becomes hidden maintenance debt.

  Requirements:
  - Cadence: run monthly, before large refactors, and after major framework or runtime changes.
  - Review the codebase, docs, and workflow against `AGENTS.md`, `POLICY.md`, stack guides, and the current architecture notes.
  - Look for drift from forward-only contracts, edge-validation boundaries, smart-constructor usage, testing policy, and module ownership.
  - Record findings as new Maintenance issues with concrete scope, priority, and validation.
  - Close the pass with a no-action note only when the review finds no actionable drift.

  Deliverables:
  - New Maintenance issues for each actionable architecture or policy drift finding.
  - Updated notes on areas reviewed and areas intentionally left unchanged.
  - A short `Last run:` note with the review scope and outcome.

  Validation:
  - Confirm every finding is represented as an issue with owner-readable context and validation criteria.
  - Confirm no implementation changes were mixed into the review runbook unless separately requested.
  - Confirm all recurring runbooks remain open.

- [ ] [M403R] (P1) Dependency and security audit
  Goal:
  Keep third-party dependencies, runtime versions, and security-sensitive configuration within the current supported contract.

  Requirements:
  - Cadence: run weekly for active apps and before each release cut.
  - Inspect package managers, lockfiles, language toolchains, container bases, and generated clients for known vulnerabilities or stale direct dependencies.
  - Review auth, secret, CORS, CSP, SQL, network, and permission-sensitive configuration for drift from the current contract.
  - Prefer current supported dependencies; do not add compatibility shims for obsolete dependency behavior.
  - File separate Maintenance or BugFix issues for each actionable vulnerability, unsupported runtime, or security-contract gap.

  Deliverables:
  - Documented audit commands or data sources used for the pass.
  - Updated issues for each actionable dependency or security finding.
  - A short `Last run:` note with clean result or follow-up issue IDs.

  Validation:
  - Rerun the repository-native audit, lint, or dependency checks used for the pass.
  - Confirm every finding is either filed, fixed under a separate issue, or explicitly marked not applicable with evidence.
  - Confirm no secrets or private payloads were written into the tracker.

- [ ] [M404R] (P1) CI, release, and artifact health
  Goal:
  Keep the repository's validation, release, publication, and generated artifact surfaces trustworthy.

  Requirements:
  - Cadence: run before every release, publish, or deploy, and weekly for critical services.
  - Verify repository-native CI, lint, format, coverage, release, publish, Docker image, Pages, and artifact workflows still match the documented contract.
  - Check generated artifacts, release tags, published images, and Pages outputs for source-to-public drift.
  - File concrete follow-up issues for failing gates, stale artifacts, missing release prerequisites, or undocumented workflow changes.
  - Do not perform production deployment from this runbook unless the operator explicitly requests that deployment.

  Deliverables:
  - Recorded gate status and artifact surfaces inspected.
  - Follow-up issues for each reproducible CI, release, publish, or artifact drift problem.
  - A short `Last run:` note with commands run and any skipped surfaces.

  Validation:
  - Use repository-native `make` targets or documented release helpers for checks.
  - Confirm release and deployment ownership boundaries remain separate.
  - Confirm public or published artifacts match the intended source revision when that surface is inspected.

- [ ] [M405R] (P1) Code contract and static hygiene
  Goal:
  Keep source contracts explicit, current, and statically guarded against policy drift.

  Requirements:
  - Cadence: run monthly and before large refactors.
  - Scan for dead code, unused exports, duplicated literals, silent fallbacks, legacy aliases, compatibility reads, and zero-but-invalid domain states.
  - Check static analysis, coverage, schema, and contract guards that are supposed to prevent drift.
  - File focused Maintenance issues for each concrete violation instead of broad cleanup placeholders.
  - Keep the current canonical contract only; do not preserve obsolete behavior unless a product requirement explicitly says so.

  Deliverables:
  - Issue entries for each actionable static hygiene or contract violation.
  - Notes on static tools, searches, and contract guards used during the pass.
  - A short `Last run:` note with clean result or follow-up issue IDs.

  Validation:
  - Rerun the relevant static checks, contract tests, or repository searches used to identify drift.
  - Confirm every finding has a narrow follow-up issue and does not duplicate existing backlog work.
  - Confirm no implementation changes were mixed into the audit unless separately requested.

- [ ] [M406R] (P1) Production drift and health
  Goal:
  Detect when production, public, or scheduled runtime state has drifted from the intended repository contract.

  Requirements:
  - Cadence: run weekly for deployed services and after each publish or deploy.
  - Compare current source, runtime configuration, published images, public routes, scheduled jobs, and health checks for drift.
  - Inspect real operator-facing surfaces rather than assuming merged source is deployed.
  - File follow-up issues for stale images, stale Pages output, missing routes, failed monitors, invalid production config, or undocumented runtime differences.
  - Stop before production deploy or destructive operator actions unless the operator explicitly requests them.

  Deliverables:
  - Recorded source revision, public artifact, route, image, or health surfaces inspected.
  - Follow-up issues for each source-to-runtime drift finding.
  - A short `Last run:` note with evidence links or commands used.

  Validation:
  - Verify inspected production or public surfaces directly where access is available.
  - Confirm any deploy-required finding is filed with the exact publish/deploy boundary and owner.
  - Confirm no production state was changed by the audit unless explicitly requested.

- [ ] [M407R] (P2) Documentation and runbook hygiene
  Goal:
  Keep durable documentation and runbooks aligned with the current behavior users and operators actually rely on.

  Requirements:
  - Cadence: run before release cuts and after merge bursts that change user-facing or operator-facing behavior.
  - Review README, ARCHITECTURE, PRD, CHANGELOG, docs, runbooks, setup guides, and local workflow notes for stale behavior or missing new contracts.
  - Update docs when closed issues changed durable behavior, public APIs, operator workflows, release semantics, or deployment expectations.
  - Remove or rewrite stale instructions instead of preserving obsolete alternatives.
  - File separate issues for documentation gaps that require product or implementation decisions.

  Deliverables:
  - Updated documentation or filed follow-up issues for each gap.
  - A short `Last run:` note listing docs inspected and changes made.
  - Cross-references from archived issue history to durable docs when useful.

  Validation:
  - Check links, command names, paths, and public contract descriptions touched by the pass.
  - Confirm docs describe the current canonical path only.
  - Confirm issue archive and active tracker references remain consistent.

- [x] [M400] (P0) Increase test coverage to 95%. Resolved: ledger tests expanded to 96.7% coverage; coverage gate raised to 95%, tooling passing.
  Increase test coverage to 95%

- [x] [M401] (P0) Enforce coverage gate across the entire Go module. Resolved: `make test-unit` now computes module-wide coverage (excluding generated `api/credit/v1`); service/store integration tests added; `make ci` passing with total coverage 95.2%.
  - Current `make test` only enforces coverage for `pkg/ledger`, leaving `cmd/credit` + `internal/*` effectively untested.
  - Update coverage gate to measure module-wide coverage (excluding generated protobuf package) and add integration tests that exercise the service end-to-end.

- [x] [M402] (P1) Fix demo backend Docker build failing due to outdated ledger proto dependency. Resolved: bumped `demo/backend` dependency on `github.com/MarkoPoloResearchLab/ledger` so generated proto includes `tenant_id`/`ledger_id`; `go test ./...` and demo `docker build` passing.
  - `demo/backend` imports `github.com/MarkoPoloResearchLab/ledger/api/credit/v1` but pins an older module version missing `tenant_id`/`ledger_id` fields, breaking `demo/Dockerfile` builds.
  - Update `demo/backend/go.mod` to a ledger module version that matches the current API and ensure `demo/docker-compose.yml` builds succeed.

- [x] [M403] (P1) Fix demo Compose TAuth container failing to start without config.yaml. Resolved: added `demo/tauth.config.yaml` + compose mount and `TAUTH_CONFIG_FILE`; updated demo UI to load `tauth.js` from TAuth and aligned demo issuer to `tauth`.
  - Current `demo/docker-compose.yml` uses `ghcr.io/tyemirov/tauth:latest`, which now requires a YAML config file (defaults to `config.yaml`) and exits if it is missing.
  - Provide a minimal demo `config.yaml` and wire it into compose via volume mount + `TAUTH_CONFIG_FILE`.

- [x] [M404] (P1) Demo UI: apply missing styles and fix TAuth script load order so wallet/actions work. Resolved: added `demo/ui/styles.css`; updated `<mpr-header>` to `tauth-*`/`google-site-id`/`tauth-tenant-id`; ensured `tauth.js` is present before `mpr-ui.js` boots; UI now renders balances and disables actions until authenticated; `make ci` + `cd demo && make ci` passing.
  - Demo page currently renders largely unstyled because it uses custom classnames without a CSS file.
  - `mpr-ui` auth bootstrap expects `window.initAuthClient` to exist when `mpr-ui.js` runs; the current dynamic loader can race and prevent auth events (wallet never loads).

- [x] [M405] (P1) Demo stack: serve the UI over HTTPS on `:4443` via ghttp using the computercat TLS cert/key and proxy auth/API routes through the same origin. Resolved: ghttp now terminates TLS on host `:4443` using `demo/certs`, proxies `/api` + TAuth routes, and demo docs/config derive base URLs from the current origin; `make ci` + `cd demo && make ci` passing.
  - Replace the HTTP-only `:8000` demo UI entrypoint with `https://localhost:4443`.
  - Wire ghttp TLS with the `computercat-cert.pem` / `computercat-key.pem` pair and proxy `/api`, `/auth`, `/me`, `/tauth.js` to the backing services.

- [x] [M406] (P1) Demo stack: make TAuth cookies host-only so auth works on `computercat.tyemirov.net` and LAN origins. Resolved: demo TAuth `cookie_domain`/`APP_COOKIE_DOMAIN` now empty (host-only), so cookies are issued for the active origin; `make ci` + `cd demo && make ci` passing.

- [x] [M407] (P1) Demo stack: ensure Postgres schema is migrated by GORM when running Compose. Resolved: `demo/docker-compose.yml` now builds `ledgerd` from the repo `Dockerfile` (includes I214 Postgres `AutoMigrate`) so fresh Postgres volumes get tables automatically; `make ci` + `cd demo && make ci` passing.

- [x] [M408] (P1) Demo UI: keep mpr-ui and page styles on the same light/dark theme (avoid mixed palettes). Resolved: `data-mpr-theme` is now the single source of truth (set on `<html>`/`<body>` and toggled via the footer theme switcher); custom demo CSS now keys off `data-mpr-theme`; `cd demo && make ci` passing.

- [x] [M409] (P2) Demo UI: remove the header Account/settings button. Resolved: removed `<mpr-header>` settings attributes so the Account button no longer renders; `make ci` + `cd demo && make ci` passing.

- [x] [M410] (P2) Demo UI: square theme switcher should expose four modes (not two). Resolved: footer theme-config now defines 4 modes (default-light, sunrise-light, default-dark, forest-dark) and demo CSS keys palette overrides off `data-demo-palette`; `make ci` + `cd demo && make ci` passing.

- [x] [M411] (P2) Demo UI: show account details (photo/name/email) from the header user menu; remove the hero session card. Resolved: added a user-menu action that opens an account-details modal; removed the session card; Playwright coverage added; `make ci` + `cd demo && make ci` passing.

- [x] [M412] (P1) Demo UI: footer theme switcher does not switch themes. Resolved: header now uses the same 4-mode theme-config as footer so themeManager config isn't overridden; Playwright coverage added; `cd demo && make ci` passing.

- [x] [M413] (P2) Demo UI: "Docs" header link should open the rendered integration guide (not the GitHub repo root). Resolved: header now points to `docs/integration.md` on GitHub; Playwright coverage added; `cd demo && make ci` passing.

- [x] [M414] (P2) Demo UI: footer links menu should match the mpr-ui demo site catalog ("Built by Marco Polo Research Lab"). Resolved: footer dropdown updated to the mpr-ui site catalog; Playwright assertions added; `cd demo && make ci` passing.

- [x] [M415] (P1) Enforce production-only unused-code gates in CI. Resolved: `make ci` now runs `staticcheck`/`errcheck` without tests, checks production reachability via `deadcode` + package-deps validation, and verifies `CGO_ENABLED=0` builds; removed unreachable `internal/store/pgstore` and updated docs.

## Planning
*do not implement yet*
