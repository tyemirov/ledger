# ISSUES

Entries record newly discovered requests or changes.

Read @AGENTS.md (Workflow section), @POLICY.md, and relevant stack guides before implementing changes.

Format: `- [ ] [B042] (P1) {I007} Title`

- `[ ]` open, `[-]` taken, `[!]` blocked, `[x]` closed.
- Blocked issues (`[!]`) must include a `Blocked:` line in the body.
- Resolved non-recurring history lives in `.mprlab/ISSUES-ARCHIVE.md`.
- This active tracker keeps open, blocked, planning, and recurring work visible.

## BugFixes

- [!] [B003] (P1) Publish the current mpr-ui browser configuration contract.
  Goal:
  The `mpr-ui@latest` configuration loader accepts the canonical provider map that Ledger uses.
  The published loader still requires the obsolete flat Google fields.
  Requirements:
  - Keep `mpr-ui@latest` as the shared browser integration surface.
  - Publish the provider-map configuration loader from the current mpr-ui source.
  - Do not add the obsolete flat Google configuration to Ledger.
  Validation:
  - Load the Ledger shell with the literal `mpr-ui@latest` URLs.
  - Verify that the loader accepts the explicit Google, Apple, and password provider entries.
  - Verify that `mpr-header` and `mpr-footer` initialize.
  - Final candidate `768f25936497c5aabd426197d21c2100b6e5d9a1` passed local CI with eight browser scenarios.
  - Local CI includes retained F002 working changes; hosted CI qualifies the committed source separately.
  Deliverables:
  - Keep the nested config producer and current footer menu.
  - Verify the shared UI candidate through the real Ledger service.
  - Cover login, session restoration, request recovery, and logout at desktop and mobile widths.
  - Release the loading overlay after recovery of an existing authenticated workspace.
  - Use `docs/mpr-ui-migration.md` for the remaining publication and production gates.
  Blocked: Shared publication, public TLS, the F002 Pages change, and live authentication acceptance remain incomplete.

- [x] [B002] (P1) Make every production theme-switcher quadrant functional.
  Goal:
  Each quadrant of the square footer control selects its distinct Ledger theme mode.
  Requirements:
  - Use the established default-light, sunrise-light, default-dark, and forest-dark mode order.
  - Keep the header, footer, and Ledger workspace on one shared theme configuration.
  - Apply each mode to both the document and body surfaces.
  - Keep every application control readable in each palette.
  Validation:
  - Click each quadrant through the real browser entry point.
  - Verify each quadrant selects its expected theme, palette, active position, and workspace surface.
  - Run the repository frontend checks and `make ci`.
  Resolution:
  - The production header and footer now share the established four-mode configuration.
  - Ledger-owned semantic tokens now render default-light, sunrise-light, default-dark, and forest-dark palettes.
  - Browser coverage verifies all four quadrant hit areas and resulting workspace surfaces.

- [x] [B001] (P1) Authenticate the tenant in a batch request.
  Goal:
  The public `Batch` RPC rejects each request before its handler can read the nested tenant ID.
  Requirements:
  - Read the tenant ID from `BatchRequest.account` at the authentication boundary.
  - Use one tenant identity contract for all `CreditService` RPCs.
  - Accept a batch request only when its tenant credential matches its tenant ID.
  - Reject each missing, unknown, or mismatched tenant credential.
  Validation:
  - Send batch requests through the real `ledgerd` gRPC entrypoint.
  - Verify a valid batch request changes the addressed Ledger account.
  - Verify tenant A cannot use tenant B credentials or data.
  Resolution:
  - The shared authentication boundary reads `BatchRequest.account.tenant_id`.
  - Persistent credentials authenticate the nested tenant before the handler runs.
  - Every handler compares the authenticated tenant with its addressed tenant.

## Improvements

- [x] [I027] (P1) Standardize HTTP health at `/healthz`.
  Goal:
  Make `/healthz` the canonical health endpoint for the Ledger web and API
  origin. Use the endpoint for readiness without application requests.

  Requirements:
  - Keep unauthenticated `GET /healthz` on the combined web and API origin.
  - Return `200` only when the origin can serve its current application contract.
  - Return a non-success status when a required runtime dependency prevents service.
  - Send `Cache-Control: no-store` on every health response.
  - Keep the response free from credentials and internal state.
  - Do not mutate application state during a probe.
  - Do not record a probe as application usage or an audit event.
  - Do not emit routine information-level request events for successful probes.
  - Keep failed probe evidence in container and deployment diagnostics.
  - Use `/healthz` for local Compose, runtime capability, and public health checks.
  - Set `start_interval: 1s` and `interval: 30s` for Docker probes.
  - Set a bounded `start_period` for the HTTP startup contract.
  - Preserve protocol-native readiness for the gRPC service.
  - Keep the selected manifest contract unchanged.

  Deliverables:
  - Update the endpoint, request logging, orchestration, manifest, documentation, and black-box tests.

  Validation:
  - Verify unauthenticated `GET /healthz` returns `200` and `Cache-Control: no-store`.
  - Verify a required dependency failure returns a non-success status.
  - Verify the gRPC readiness contract remains protocol-native.
  - Verify Docker probes use the required startup and steady intervals.
  - Verify successful probes create no routine request events.
  - Verify failed probes retain diagnostic evidence.
  - Run `make ci`.

  Resolution:
  - Added bounded database readiness and minimal `503` responses.
  - Suppressed successful probe events and retained failure diagnostics.
  - Updated local probe timing and the OpenAPI contract.
  - `make ci` passed, including browser and local lifecycle tests.

- [ ] [I025] (P1) {F001,F002} Complete the one-time production UserAccount migration.
  Goal:
  I025 authorizes one migration of the retained production data.
  It does not add a migration capability to normal startup or deployment.
  The 2026-09-02 production audit found legacy tenant data in the retained SQLite database.
  The database contains seven `hecate` accounts, four `ps` accounts, and 41 ledger entries.
  The migration preserves all accounting history and gives each approved UserAccount its Ledger tenants.
  Each current client uses its canonical tenant ID and tenant credential after deployment.
  Requirements:
  - Use the retained `ledger-data` SQLite database as the only production migration source.
  - Select one production TAuth tenant for the Ledger workspace.
  - Resolve each owner from the exact TAuth issuer, TAuth tenant ID, and immutable TAuth user ID.
  - Verify each approved owner email through its production TAuth profile.
  - Store only the exact TAuth identity values in the private migration input.
  - Keep each owner email and identity value out of source control.
  - Include `ps`, `hecate`, and `namesignal` in the complete legacy tenant set.
  - Assign each legacy tenant to an operator-approved UserAccount.
  - Generate one canonical tenant UUID and one tenant credential for each legacy tenant.
  - Preserve every Ledger account, ledger entry, reservation, balance, timestamp, metadata value, and idempotency key.
  - Stop Ledger writes before the database snapshot and migration.
  - Create and verify a recoverable database snapshot before mutation.
  - Store the migration mapping in one private mode-0600 operator file.
  - Keep private mapping values out of Git, logs, plans, receipts, and issue records.
  - Use GORM for each migration transaction and schema change.
  - Keep the data migration valid for SQLite and PostgreSQL.
  - Use the existing bounded migration command only for this production change.
  - Run the command as a separate operator action before the canonical runtime starts.
  - Do not call the migration from Ledger startup, `make deploy`, or a recurring deployment step.
  - Do not add migration input to the normal runtime contract.
  - Keep application-specific migration logic out of `mprlab-gateway`.
  - Stage each canonical tenant ID and tenant credential in its client before runtime activation.
  - Use each client repository's canonical private deployment input.
  - Activate Ledger and each client in one controlled production sequence.
  - Reject each old tenant ID and static credential after the migration.
  - Do not add dual reads, dual writes, legacy credentials, or compatibility paths.
  - Remove the migration command, private input, and temporary operator automation after production verification.
  Deliverables:
  - Prepare one temporary Ledger-owned operator procedure for the existing migration command.
  - Prepare one validated private owner mapping for all configured legacy tenants.
  - Prepare canonical tenant identifiers and credentials for PoodleScanner, Hecate, and NameSignal.
  - Update each client through its repository-owned deployment contract.
  - Remove the obsolete static tenant secrets from the Ledger deployment input.
  - Delete all temporary migration code, targets, files, and documentation after production verification.
  - Keep the final runtime and deployment contracts free of migration behavior.
  - Record the production migration and client verification results without secret values.
  Validation:
  - Run `make ci` after the last source change.
  - Run the migration against a private copy of the production database before production mutation.
  - Verify the copy contains the canonical schema and no legacy `accounts` table.
  - Verify the production operator action uses the exact released Ledger artifact.
  - Record current account, entry, reservation, and balance values immediately before production migration.
  - Compare each recorded value with the canonical database after migration.
  - Verify each Ledger account references its approved canonical Ledger tenant.
  - Authenticate through the selected production TAuth tenant and open the same UserAccount.
  - Verify the UserAccount lists each approved migrated Ledger tenant.
  - Request a balance through each updated production client and its canonical tenant credential.
  - Verify each old static credential and legacy tenant ID is rejected.
  - Verify no private mapping value appears in Git, logs, plans, receipts, or browser data.
  - Verify normal startup does not do or schedule a data migration.
  - Verify subsequent deployments require only the canonical schema and normal runtime inputs.
  - Verify the repository contains no migration command, target, mapping file, or startup migration branch.
  - Record source CI, release, publication, deployment, runtime, and client acceptance as separate results.
  - Remove the migration command only after all production checks pass.

- [x] [I024] (P0) Use the permanent versionless selected application manifest.
  Goal:
  Use one selected application manifest contract without a schema number.
  Requirements:
  - Remove `schema_version` from `.mprlab/deploy/resources.yml`.
  - Require only `owner`, `release`, and `resources` at the manifest root.
  - Reject each numbered selected application manifest form.
  - Preserve independent schema contracts.
  Validation:
  - Run `make ci` after the last repository change.
  - Plan release through gateway commit `753c727` without production contact.
  Resolution:
  - The manifest preserves the SemVer release scheme without a schema number.
  - The compiled lifecycle contract rejects a `schema_version` field.

- [x] [I023] (P0) Move the release policy into the resource manifest.
  Goal:
  Use one tracked application file for release and deployment configuration.
  Requirements:
  - Set the manifest schema version to 4.
  - Add `release.scheme: semver` to the manifest.
  - Delete `.mprlab/release.yml`.
  - Keep the resource graph and lifecycle commands unchanged.
  Validation:
  - Pass the lifecycle contract test.
  - Pass the sibling gateway manifest plan.
  - Pass `make ci`.
  Resolution 2026-08-12:
  - Moved the SemVer policy into the schema-4 resource manifest.
  - Deleted the obsolete `.mprlab/release.yml` file.
  - Kept the resource graph and lifecycle commands unchanged.
  - Updated the lifecycle contract and current deployment documentation.
  - The lifecycle contract failed against schema 3 and passed against schema 4.
  - The final `make ci` run passed with 100 percent coverage.
  - The sibling plan remains pending until the gateway checkout is clean.
  - Changed files: `.mprlab/deploy/resources.yml`, `.mprlab/ISSUES.md`,
    `CHANGELOG.md`, `README.md`, and
    `tests/lifecyclecontract/lifecycle_contract_test.go`.

## Maintenance

- [ ] [M001R] (P2) Backlog hygiene and archive.
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
  Last run: 2026-08-09.
- [ ] [M002R] (P2) Polish open issues.
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
- [ ] [M003R] (P2) Architecture and policy review.
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
- [ ] [M004R] (P1) Dependency and security audit.
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
- [ ] [M005R] (P1) CI, release, and artifact health.
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
- [ ] [M006R] (P1) Code contract and static hygiene.
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
- [ ] [M007R] (P1) Production drift and health.
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
- [ ] [M008R] (P2) Documentation and runbook hygiene.
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
- [x] [M009R] (P0) Increase test coverage to 95%.
  Resolved: ledger tests expanded to 96.7% coverage; coverage gate raised to 95%, tooling passing.
  Increase test coverage to 95%
- [x] [M010R] (P0) Enforce coverage gate across the entire Go module.
  Resolved: `make test-unit` now computes module-wide coverage (excluding generated `api/credit/v1`); service/store integration tests added; `make ci` passing with total coverage 95.2%.
  - Current `make test` only enforces coverage for `pkg/ledger`, leaving `cmd/credit` + `internal/*` effectively untested.
  - Update coverage gate to measure module-wide coverage (excluding generated protobuf package) and add integration tests that exercise the service end-to-end.
- [x] [M011R] (P1) Fix demo backend Docker build failing due to outdated ledger proto dependency.
  Resolved: bumped `demo/backend` dependency on `github.com/MarkoPoloResearchLab/ledger` so generated proto includes `tenant_id`/`ledger_id`; `go test ./...` and demo `docker build` passing.
  - `demo/backend` imports `github.com/MarkoPoloResearchLab/ledger/api/credit/v1` but pins an older module version missing `tenant_id`/`ledger_id` fields, breaking `demo/Dockerfile` builds.
  - Update `demo/backend/go.mod` to a ledger module version that matches the current API and ensure `demo/docker-compose.yml` builds succeed.
- [x] [M012R] (P1) Fix demo Compose TAuth container failing to start without config.yaml.
  Resolved: added `demo/tauth.config.yaml` + compose mount and `TAUTH_CONFIG_FILE`; updated demo UI to load `tauth.js` from TAuth and aligned demo issuer to `tauth`.
  - Current `demo/docker-compose.yml` uses `ghcr.io/tyemirov/tauth:latest`, which now requires a YAML config file (defaults to `config.yaml`) and exits if it is missing.
  - Provide a minimal demo `config.yaml` and wire it into compose via volume mount + `TAUTH_CONFIG_FILE`.
- [x] [M013R] (P1) Demo UI: apply missing styles and fix TAuth script load order so wallet/actions work.
  Resolved: added `demo/ui/styles.css`; updated `<mpr-header>` to `tauth-*`/`google-site-id`/`tauth-tenant-id`; ensured `tauth.js` is present before `mpr-ui.js` boots; UI now renders balances and disables actions until authenticated; `make ci` + `cd demo && make ci` passing.
  - Demo page currently renders largely unstyled because it uses custom classnames without a CSS file.
  - `mpr-ui` auth bootstrap expects `window.initAuthClient` to exist when `mpr-ui.js` runs; the current dynamic loader can race and prevent auth events (wallet never loads).
- [x] [M014R] (P1) Demo stack: serve the UI over HTTPS on `:4443` via ghttp using the computercat TLS cert/key and proxy auth/API routes through the same origin.
  Resolved: ghttp now terminates TLS on host `:4443` using `demo/certs`, proxies `/api` + TAuth routes, and demo docs/config derive base URLs from the current origin; `make ci` + `cd demo && make ci` passing.
  - Replace the HTTP-only `:8000` demo UI entrypoint with `https://localhost:4443`.
  - Wire ghttp TLS with the `computercat-cert.pem` / `computercat-key.pem` pair and proxy `/api`, `/auth`, `/me`, `/tauth.js` to the backing services.
- [x] [M015R] (P1) Demo stack: make TAuth cookies host-only so auth works on `computercat.tyemirov.net` and LAN origins.
  Resolved: demo TAuth `cookie_domain`/`APP_COOKIE_DOMAIN` now empty (host-only), so cookies are issued for the active origin; `make ci` + `cd demo && make ci` passing.
- [x] [M016R] (P1) Demo stack: ensure Postgres schema is migrated by GORM when running Compose.
  Resolved: `demo/docker-compose.yml` now builds `ledgerd` from the repo `Dockerfile` (includes I011 Postgres `AutoMigrate`) so fresh Postgres volumes get tables automatically; `make ci` + `cd demo && make ci` passing.
- [x] [M017R] (P1) Demo UI: keep mpr-ui and page styles on the same light/dark theme (avoid mixed palettes).
  Resolved: `data-mpr-theme` is now the single source of truth (set on `<html>`/`<body>` and toggled via the footer theme switcher); custom demo CSS now keys off `data-mpr-theme`; `cd demo && make ci` passing.
- [x] [M018R] (P2) Demo UI: remove the header Account/settings button.
  Resolved: removed `<mpr-header>` settings attributes so the Account button no longer renders; `make ci` + `cd demo && make ci` passing.
- [x] [M019R] (P2) Demo UI: square theme switcher should expose four modes (not two).
  Resolved: footer theme-config now defines 4 modes (default-light, sunrise-light, default-dark, forest-dark) and demo CSS keys palette overrides off `data-demo-palette`; `make ci` + `cd demo && make ci` passing.
- [x] [M020R] (P2) Demo UI: show account details (photo/name/email) from the header user menu; remove the hero session card.
  Resolved: added a user-menu action that opens an account-details modal; removed the session card; Playwright coverage added; `make ci` + `cd demo && make ci` passing.
- [x] [M021R] (P1) Demo UI: footer theme switcher does not switch themes.
  Resolved: header now uses the same 4-mode theme-config as footer so themeManager config isn't overridden; Playwright coverage added; `cd demo && make ci` passing.
- [x] [M022R] (P2) Demo UI: "Docs" header link should open the rendered integration guide (not the GitHub repo root).
  Resolved: header now points to `docs/integration.md` on GitHub; Playwright coverage added; `cd demo && make ci` passing.
- [x] [M023R] (P2) Demo UI: footer links menu should match the mpr-ui demo site catalog ("Built by Marco Polo Research Lab").
  Resolved: footer dropdown updated to the mpr-ui site catalog; Playwright assertions added; `cd demo && make ci` passing.
- [x] [M024R] (P1) Enforce production-only unused-code gates in CI.
  Resolved: `make ci` now runs `staticcheck`/`errcheck` without tests, checks production reachability via `deadcode` + package-deps validation, and verifies `CGO_ENABLED=0` builds; removed unreachable `internal/store/pgstore` and updated docs.


## Features

- [-] [F001] (P1) {F002} Add the authenticated Ledger workspace.
  Goal:
  An authenticated user can provision one UserAccount and manage any number of owned Ledger tenants in one secure browser workspace.
  Requirements:
  - Start implementation after F002 provides the UserAccount, tenant, credential, TAuth, and public route contracts.
  - Replace the current wallet demo browser surface with the production Ledger workspace.
  - Delete the direct `tauth.js` loader, manual TAuth attributes, and application session restoration code.
  - Delete the obsolete wallet action controls from the browser surface.
  - Keep all accounting mutations on the private gRPC data plane.
  - Use checked ES modules and Alpine for the workspace state.
  - Keep one validated workspace state owner for UserAccount, tenant collection, selected tenant, and credentials.
  - Keep route paths, event names, status values, and user messages in canonical constants or backend representations.
  - Load each MPR Lab browser library through its literal `@latest` tag.
  - Use `/config-ui.yaml` as the only browser authentication input.
  - Mount `mpr-header`, `mpr-user`, and `mpr-footer` through the documented `mpr-ui` contract.
  - Use the selected profile for the exact TAuth origin, tenant, cookie, provider, and route values.
  - Keep local browser authentication on the same origin through the repository proxy.
  - Do not call TAuth endpoints or inspect cookies, tokens, claims, browser storage, or `mpr-ui` internals.
  - Use `mpr-user` as the only displayed TAuth profile and session control.
  - Do not duplicate TAuth profile fields in a Ledger account dialog.
  - Request protected workspace data only after `mpr-ui:auth:authenticated`.
  - Cancel pending workspace requests and clear protected state after `mpr-ui:auth:unauthenticated`.
  - Treat protected API failures as workspace failures after `mpr-ui` reports authentication.
  - Validate the TAuth session at the Ledger HTTP boundary for each protected resource request.
  - Provision the current UserAccount through the canonical P001 operation after authentication.
  - Do not send a browser user ID, owner ID, email address, or TAuth tenant ID in provisioning requests.
  - Release the shared authentication transition only after the first authenticated workspace render.
  - Wait for `MPRUI.whenAutoOrchestrationReady()` before an immediate transition completion event.
  - Show restrained loading, empty, error, and retry states for each workspace resource.
  - Use one selected Ledger tenant for all tenant-specific data and actions.
  - Preserve the selected tenant in the browser URL with its opaque tenant ID.
  - Do not add separate tenant selectors for different panels.
  - Do not provide an `All tenants` scope for credential or mutation controls.
  - Organize the selected tenant into `Overview`, `Credentials`, and `Integration` sections.
  - Require a human-readable tenant name in the approved P001 tenant representation.
  - Show the tenant name as the primary label and the opaque tenant ID as metadata.
  - Show a first-tenant creation action when the owned tenant collection is empty.
  - Keep a compact tenant creation action available after the first tenant exists.
  - Submit only the approved tenant creation fields and a generated idempotency key.
  - Never submit the owner identity from a browser field.
  - Show tenant creation progress without blocking unrelated tenant reads.
  - Select a newly created tenant only after the server returns its canonical representation.
  - Render the tenant collection as dense rows in canonical server order.
  - Use the server cursor for explicit collection pagination.
  - Do not load the complete tenant collection into one unbounded browser request.
  - Reject a stale tenant response after authentication, selection, or collection state changes.
  - Show the selected tenant name, tenant ID, creation time, and credential summary.
  - Do not show tenant rename, transfer, closure, or deletion actions in this issue.
  - Create a tenant credential only through a separate explicit action.
  - Do not return a tenant credential from the tenant creation operation.
  - Show a new credential secret only once after successful creation.
  - Keep the new credential secret only in memory while its disclosure panel is open.
  - Clear the secret after panel closure, navigation, tenant selection, authentication loss, or page exit.
  - Never put a raw or masked secret in a URL, browser storage, log, metric, attribute, or accessible name.
  - Provide explicit copy and confirmation controls before the one-time secret panel closes.
  - List credential identifiers, creation times, and current revocation states without secret values.
  - Permit multiple active credentials so a user can rotate a client without service interruption.
  - Require an exact tenant and an explicit confirmation before credential revocation.
  - Do not revoke an old credential automatically when the user creates a new credential.
  - Reject a stale credential response after authentication or tenant selection changes.
  - Show a copyable gRPC integration example with environment variable placeholders.
  - Keep the raw credential out of the integration example.
  - Link the integration example to the current Ledger API and Go library documentation.
  - Use a centered 1180-pixel shell with a 210-pixel tenant rail on wide screens.
  - Collapse the tenant rail into one compact tenant control below tablet widths.
  - Use solid MPR charcoal surfaces, thin borders, compact controls, and semantic accent colors.
  - Use six-pixel panel radii and restrained spacing for tenant rows and credential panels.
  - Use accent colors only for selection, status, primary actions, warnings, and errors.
  - Do not use a hero section, gradient surface, glass effect, oversized heading, or generic dashboard card grid.
  - Keep the same dense information order on desktop and narrow screens.
  - Collapse secondary metadata before the primary tenant identity or action controls.
  - Prevent horizontal page overflow at each supported viewport.
  - Use semantic navigation, sections, forms, lists, dialogs, buttons, and status regions.
  - Do not target internal `mpr-ui` classes from Ledger styles.
  - Preserve visible focus, logical focus order, keyboard operation, and accessible control names.
  - Move focus into each opened dialog and return focus to its opening control.
  - Make an inactive dialog surface unavailable to keyboard and accessibility APIs.
  - Use short status transitions and remove nonessential motion for `prefers-reduced-motion`.
  - Serve the workspace, `/config-ui.yaml`, and control plane from Ledger-owned deployment resources.
  - Keep the private gRPC capability separate from the public browser route.
  - Declare the public route, health check, configuration, and required private values in the Ledger manifest.
  - Keep each Ledger-specific route and setting out of the generic gateway contract.
  - Use the exact selected deployment profile without an inferred production hostname.
  Deliverables:
  - Add the production Ledger browser entry point, checked modules, semantic markup, and MPR style tokens.
  - Add the canonical `mpr-ui` bootstrap and the app-owned `/config-ui.yaml` representation.
  - Add a validated browser client for the approved UserAccount, tenant, and credential resources.
  - Add the authenticated UserAccount startup and workspace readiness flow.
  - Add tenant creation, selection, pagination, detail, and stale-response isolation.
  - Add one-time credential disclosure, copy, listing, creation, and revocation flows.
  - Add the tenant integration panel with current gRPC and Go library guidance.
  - Remove the obsolete wallet demo UI and its direct TAuth integration path.
  - Update the local composition and selected deployment manifest for the browser route.
  - Update the OpenAPI document, browser types, user documentation, and deployment documentation together.
  - Add Playwright coverage through the real browser entry point and real HTTP server.
  Validation:
  - Prove the page uses `/config-ui.yaml`, `mpr-ui-config.js`, and literal `mpr-ui@latest` assets.
  - Prove the page contains no direct `tauth.js` loader or manual TAuth authentication attributes.
  - Prove an unauthenticated browser sends no protected workspace request.
  - Prove each protected HTTP resource rejects a missing or invalid TAuth session with `401`.
  - Complete a real `mpr-ui` and TAuth browser login without injected cookies.
  - Prove the first authenticated load provisions one UserAccount and releases the transition after render.
  - Prove a restored TAuth session opens the same UserAccount without a second application authentication path.
  - Create multiple named tenants and verify their canonical order, selection, URL state, and pagination.
  - Prove a second UserAccount cannot list, read, select, or change the first UserAccount tenants.
  - Prove tenant creation retries return one tenant and conflicting idempotency use returns the canonical error.
  - Prove stale tenant and credential responses cannot change the selected tenant workspace.
  - Prove one-time credential disclosure clears at each specified state boundary.
  - Prove secrets stay out of URLs, storage, logs, metrics, attributes, accessible names, and integration examples.
  - Prove credential creation does not revoke an existing credential.
  - Prove credential revocation requires the exact tenant and explicit confirmation.
  - Prove keyboard navigation, dialog focus, live status, visible focus, and reduced motion.
  - Inspect the real workspace at 1280, 900, and 390 pixels.
  - Prove the inspected viewports have no horizontal page overflow.
  - Run the repository frontend checks and `make ci` after the last application change.
  - Record source CI, release, publication, deployment, runtime, and public browser acceptance as separate results.
  - Verify the public frontend, browser authentication, backend authorization, and authenticated workspace after deployment.

- [x] [F002] (P1) {P001,B001} Implement the UserAccount backend architecture.
  Goal:
  An authenticated person can provision one UserAccount and manage any number of owned Ledger tenants through a secure control plane.
  Requirements:
  - Use the approved P001 identity, ownership, storage, API, authorization, concurrency, credential, and migration contracts.
  - Keep browser identity in TAuth and accounting operations on private gRPC.
  - Remove static tenant configuration and plaintext tenant secrets.
  - Reject a legacy database until the bounded migration succeeds.
  Validation:
  - Exercise the HTTP control plane with a real server, real database, and signed TAuth cookies.
  - Exercise persistent tenant credentials through the real gRPC entrypoint.
  - Verify owner isolation, idempotency, one-time secret disclosure, credential revocation, and `Batch` authentication.
  - Migrate a real legacy fixture and retain its accounting history.
  - Run `make ci` after the last backend change.
  Resolution:
  - Ledger now stores UserAccounts, owner-scoped tenants, credential digests, idempotency decisions, and append-only control events.
  - One process serves the TAuth-protected HTTP control plane and credential-protected gRPC data plane on separate listeners.
  - The mode-0600 migration validates the complete owner mapping before mutation and rejects a rerun.
  - Static tenant configuration and plaintext tenant authentication are removed.

## Planning
*do not implement yet*

- [x] [P001] (P1) Define the UserAccount backend architecture.
  Goal:
  Define how an authenticated person owns and manages any number of Ledger tenants.
  Requirements:
  - Separate UserAccount, Ledger tenant, TAuth tenant, and Ledger account concepts.
  - Define identity, ownership, storage, API, authorization, concurrency, and migration boundaries.
  - Keep TAuth and `mpr-ui` as the browser authentication authority.
  - Keep the existing gRPC accounting surface as the private data plane.
  - Record each unconfirmed product choice as an open decision.
  Deliverables:
  - Add `docs/user-account-backend-design.md` as the canonical backend design.
  - Identify independent implementation slices without authorizing implementation.
  - List the decisions that require product or operator input.
  Validation:
  - Run the language checker on each changed technical document.
  - Run the Governor repository check.
  - Run `git diff --check`.
  Resolution:
  - `docs/user-account-backend-design.md` defines the canonical UserAccount, tenant, credential, API, authorization, audit, and migration boundaries.
  - The design selects explicit provisioning, owner-only named tenants, separate revocable credentials, exact-origin mutation protection, and one process with two listeners.
  - The exact hosted TAuth profile and production owner mapping remain required operator inputs.
