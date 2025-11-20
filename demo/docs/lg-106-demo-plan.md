# LG-106 Ledger Demo Plan

## Objectives
- Ship a clear execution plan for a public demo that exercises the ledger gRPC backend through a web UI backed by TAuth authentication.
- Cover the three required scenarios with a single “Spend 5 coins” control: (1) success when the starting 20 coins exist, (2) failure when balance < 5 coins, (3) success after buying coins followed by a zero-balance state.
- Keep the UI declarative with `mpr-ui` header/footer components and Alpine-driven state; host the static bundle via `ghttp`.
- Authenticate every HTTP call with TAuth session cookies; the backend validates them before touching the ledger.

## Current State and Gaps
- Ledger service (`cmd/ledger`) is present; Docker (`demo/docker-compose.yml`) runs it on port `50051` (plaintext gRPC via SQLite).
- Demo API exists (`demo/backend/cmd/demoapi` + `demo/backend/internal/demoapi`), already exposing `/api/session`, `/api/bootstrap`, `/api/wallet`, `/api/transactions`, `/api/purchases` with constants matching the required flows (5-coin spend, 20-coin bootstrap, 5-coin purchase increments). Integration tests cover the spend/insufficient/purchase paths.
- TAuth is assumed via the published image (`ghcr.io/tyemirov/tauth:latest`); `.env.tauth.example` in `demo/` keeps the local configuration aligned with production defaults.
- `ghttp` (under `tools/ghttp`) serves the `demo/ui` bundle, which now houses the ledger demo UI described in this plan.
- Docs in `docs/TAuth/usage.md` and `docs/mpr-ui/*` describe the auth flow, script ordering, and declarative component usage; the plan must align with those contracts.

## Proposed Architecture
```
[Browser: mpr-ui + Alpine + auth-client.js]
    |  (GET /auth/nonce, /auth/google, /auth/refresh, /me via TAuth)
    |  (fetch '/api/*' with credentials to demoapi)
    v
[ghttp static host on :8000]  -- serves demo UI bundle
    |
    v
[demoapi on :9090] -- Gin + sessionvalidator -> gRPC client
    |
    v
[ledgerd on :50051] -- append-only ledger (SQLite)

[TAuth on :8080] -- Google nonce + session cookies for the UI and demoapi
```

## Identity and `mpr-ui` Integration
- Script order per `docs/mpr-ui/demo-index-auth.md`: `mpr-ui.css` → Alpine ES module → `http://localhost:8080/static/auth-client.js` → `mpr-ui.js` → `https://accounts.google.com/gsi/client`.
- `<mpr-header>` attributes: `site-id` from configuration, `nonce-path="/auth/nonce"`, `login-path="/auth/google"`, `logout-path="/auth/logout"`, `base-url="http://localhost:8080"`. Footer stays declarative (`<mpr-footer>`).
- Rely on TAuth endpoints (`/auth/nonce`, `/auth/google`, `/auth/logout`, `/auth/refresh`, `/me`) exactly as documented in `docs/TAuth/usage.md`; cookies must include `SameSite=None; Secure` when cross-origin.
- Bootstrap client configuration by fetching `/demo/config.js` from TAuth (or ghttp fallback) to populate the GIS client ID and API origins before rendering the header.
- Use `auth-client.js` helpers: `initAuthClient` hydrates session state, `apiFetch` retries on `401`, `getCurrentUser` seeds the UI, `logout` drives sign-out.

## Backend and Ledger Flows
- Demo API → ledger mappings (all cents derived from `internal/demoapi/config.go`):
  - `/api/bootstrap` → `Grant(user, 2000 cents)` with idempotency key `bootstrap:<user>`.
  - `/api/transactions` → `Spend(user, 500 cents)`; map `FailedPrecondition/insufficient_funds` to HTTP 409 + status `insufficient_funds`.
  - `/api/purchases` → `Grant(user, coins*100 cents)`; validate `coins >= 5` and multiples of `5`.
  - `/api/wallet` → `GetBalance` + `ListEntries(limit=10, before=now+1s)`.
  - `/api/session` → echo TAuth claims (id, email, display, avatar, roles, expires).
- gRPC client uses per-request timeouts (`ledger_timeout`) and insecure credentials for local demos; keep `connectivity.Ready` checks as implemented.
- ledgerd runs against SQLite by default (`DATABASE_URL=sqlite:///data/demo-ledger.db`); idempotency keys and metadata come from the demo API.

## Front-End Build Plan
- Author `demo/ui/index.html`, `styles.css`, `app.js`, plus a small `walletApiClient.js` following `docs/mpr-ui/demo/` markup and component semantics (header/footer/sticky theme toggle).
- Alpine stores:
  - `auth`: reflects `auth-client.js` state; triggers bootstrap call on first authentication.
  - `wallet`: holds balances (coins), ledger entries, zero-balance flag.
  - `transactions`: controls pending states for spend/purchase buttons, surfaces status banners for the three scenarios.
- Network helpers in `walletApiClient.js`:
  - `fetchSession`, `bootstrapWallet`, `getWallet`, `spendCoins`, `purchaseCoins`; all call `apiFetch` with `credentials: "include"` and parse JSON strictly (throw on missing fields).
- UI interactions:
  - On auth → POST `/api/bootstrap`, GET `/api/wallet`, render balances + ledger.
  - “Spend 5 coins” button → POST `/api/transactions`; handle `status` to switch between success/insufficient/zero banners.
  - “Buy coins” controls (5/10/20 presets) → POST `/api/purchases`, then refresh wallet.
  - Persist authentication across reloads via `auth-client.js`; listen to `mpr-ui:auth:*` events to reset state on logout.
- Styling stays minimal, relying on `mpr-ui` tokens; keep markup semantic (cards, lists) per `docs/mpr-ui/custom-elements.md`.

## Orchestration and Configuration
- Keep `.env.tauth.example` under `demo/` aligned with `docs/TAuth/usage.md` (`APP_GOOGLE_WEB_CLIENT_ID`, `APP_JWT_SIGNING_KEY`, `APP_COOKIE_DOMAIN`, `APP_ENABLE_CORS=true`, `APP_CORS_ALLOWED_ORIGINS=http://localhost:8000`, `APP_DEV_INSECURE_HTTP=true`, `APP_DATABASE_URL=sqlite:///data/tauth.db`).
- Reuse `demo/.env.demoapi.example` for the demo API; ensure `DEMOAPI_JWT_SIGNING_KEY` matches TAuth and `DEMOAPI_ALLOWED_ORIGINS=http://localhost:8000`.
- Ensure ledgerd picks up `.env.ledgersvc` (SQLite path and listen addr) or Compose defaults.
- The UI reads the demo API origin from `<body data-api-base-url>` (defaults to `http://localhost:9090`); template or override that attribute if the API is proxied elsewhere.
- Update `demo/docker-compose.yml` to mount the new `demo/ui` assets, point ghttp at that directory, and keep port mappings (`8000` UI, `8080` TAuth, `9090` demoapi, `50051` ledger).
- Document manual run commands (Go toolchain) mirroring Compose, including `ghttp --directory demo/ui 8000`.

## Testing and Validation Strategy
- Keep Go tooling gates per `Makefile` (`make fmt`, `make lint`, `make test`): integration tests already cover API spend/purchase/insufficient flows; extend as needed if contracts change.
- Add Playwright smoke tests under `demo/tests/` to drive the UI (sign-in prompt, successful spend, insufficient funds banner, purchase/top-up, zero-balance notice). Stub TAuth/ledger/demoapi responses where possible for deterministic runs.
- Manual checklist: login via header, observe `/auth/nonce` + `/auth/google` calls, confirm bootstrap grant, execute four spends to zero, observe 409 -> banner, buy coins, and re-spend to zero.

## Delivery Steps
1. Scaffold `demo/ui` with declarative HTML/CSS/JS per `docs/mpr-ui/demo/`, wired to TAuth and backend endpoints, plus strict JSON parsing and error states.
2. Add client helpers (`walletApiClient.js`) and Alpine stores in `app.js` to sequence auth → bootstrap → wallet fetch → spend/purchase flows.
3. Create `.env.tauth.example` and ensure `demo/.env.demoapi.example`/`.env.ledgersvc` stay aligned; update `demo/docker-compose.yml` and README/demo docs with run instructions.
4. Extend or add automated tests (Playwright and any API deltas), then run `make fmt && make lint && make test` before shipping.
