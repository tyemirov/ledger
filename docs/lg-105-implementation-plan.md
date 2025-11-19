# LG-105 Implementation Plan

## Context
LG-105 tracks the "ledger demo" experience that lives entirely under `ledger_demo/`. We already migrated the prior proof-of-concept into that directory and have baseline Playwright coverage for authentication, balances, transactions, purchases, and persistence. This plan enumerates the remaining technical work to modernize the codebase per the latest requirements (no defaults, explicit configuration, wallet-specific helpers, and replayable tests).

## Goals
1. End-to-end browser flow (TAuth → walletapi → creditd) works without fallbacks, survives reloads, and surfaces ledger history.
2. Front-end gains a structured API client + Alpine stores so UI logic stays declarative.
3. Backend exposes explicit `/api/session` semantics and enforces strict config validation at startup.
4. Playwright suite (under `ledger_demo/tests`) becomes the canonical integration test battery referenced by `make test`.

## Work Breakdown

### 1. Refine Backend (`ledger_demo/backend`)
- [ ] Update `cmd/walletapi` flags/env docs to include `WALLETAPI_SESSION_HEADER` constants (if needed) and ensure `Config.Validate` checks every field (no implicit defaults).
- [ ] Replace the current gin handler wiring with a constructor-driven `Server` struct so tests (and future services) can reuse the HTTP router without `Run`.
- [x] Introduce typed request/response structs (e.g., `WalletEnvelope`, `TransactionEnvelope`, `SessionEnvelope`) with smart constructors to avoid `map[string]any` writes.
- [ ] Ensure `/api/session` pulls data out of TAuth claims (already stubbed). Expand tests to assert 401 → login, 200 → data with `expires` field for the new Playwright spec.

### 2. Front-End Restructure (`ledger_demo/frontend/ui`)
- [x] Extract `wallet-api.js` that exports `createWalletClient({ baseUrl })` with methods `getSession`, `bootstrap`, `getWallet`, `spend`, `purchase`. Each method returns normalized objects (coins, cents, entries) and throws with codes when HTTP fails.
- [x] Create `auth-flow` helper that orchestrates `initAuthClient`, tracks the logged-in profile, and exposes `restoreSession()` (invokes the new API client) so the UI boot file simply mounts stores and renders.
- [ ] Update `app.js` to import/use the new helpers; remove inline `apiFetch`/`state` objects; turn transaction/purchase form logic into Alpine components (e.g., `<section x-data="WalletPanel()">`).
- [x] Move strings for banners/statuses into a `constants.js` file to avoid scattering literal text.

### 3. Testing (`ledger_demo/tests`)
- [x] Expand `auth.spec.js` to include a regression case where the stub clears the session and ensures the UI returns to the signed-out state.
- [x] Add a helper to assert ledger history entries (presence/count/order) after each transaction/purchase.
- [x] Confirm `playwright.config.js` records screenshots/video on failure (flip `use.screenshot = 'only-on-failure'`, `trace = 'retain-on-failure'`).
- [x] Update the stub server to simulate `/api/session`, login, logout, and ledger entry mutations for realistic flows.

### 4. Documentation & Tooling
- [ ] Update `docs/demo.md` and `README.md` once the new helper modules and commands ship (include `npm run test:ui` instructions referencing `ledger_demo/tests`).
- [ ] Document environment variables inside `ledger_demo/backend/.env.walletapi.example` with explanations.
- [ ] Capture manual validation steps (login → reload → spend/purchase) in `docs/lg-105-implementation-plan.md` once the code lands.

## Deliverables
- Code changes covering backend, frontend, tests, and docs.
- Playwright suite green via `make test`.
- ISSUE [LG-105] updated/checked-off once everything merges.

## Manual Validation Checklist
1. Copy `.env.walletapi.example` and `.env.tauth.example`, fill in matching secrets, and start `docker compose -f ledger_demo/docker-compose.yml up --build`.
2. Visit `http://localhost:8000`, click **Sign in with Google**, and confirm the 20-coin bootstrap banner plus ledger entry appear.
3. Refresh the page; the user should remain authenticated and wallet panels stay visible.
4. Click **Spend 5 coins** four times—three successes followed by the insufficient-funds banner; ledger history should grow after each click.
5. Use **Buy more coins** (10) and watch the balance rise; continue spending until the zero-balance banner displays.
