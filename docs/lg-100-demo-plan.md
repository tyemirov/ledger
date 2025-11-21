# LG-100 Demo Execution Plan

## Objectives and Success Criteria
- Demonstrate an end-to-end "virtual currency" experience that exercises the ledger gRPC service plus the tooling stack under `tools/`.
- Ship a single-page demo that authenticates with TAuth, shows wallet state, and lets the user execute the three mandated flows: (1) spend 5 coins when a 20-coin starting balance exists, (2) reject the spend when balance < 5, (3) spend until balance hits 0 after a refill.
- Keep the front-end declarative by relying on `mpr-ui` custom elements (header, footer, login, theme toggle) per `tools/mpr-ui/README.md` and `docs/custom-elements.md`.
- Host the static UI with `ghttp` so it can run from any directory (mirrors `tools/ghttp/README.md`).
- Authenticate every backend call with TAuth-issued session cookies (`app_session`), validated via `tools/TAuth/pkg/sessionvalidator`.

## Proposed Architecture
```
+-------------+        +----------------------+        +------------------+
|  ghttp      |        | demo transaction API |        | ledger gRPC svc  |
| serves UI   |<------>| (new HTTP service)   |<------>| cmd/credit       |
+-------------+        +----------------------+        +------------------+
        ^                        ^   ^                          ^
        |                        |   |                          |
        |    mpr-ui + auth JS    |   |gRPC client               |
        |    calls /auth/*       |   |                          |
        |                        |   |                          |
        +------------------------+   +--------------------------+
                             TAuth (tools/TAuth)
```
Components:
1. **Ledger gRPC server** (`cmd/credit`, existing) – stores append-only entries in SQLite/Postgres. We map `1 coin = 100 cents` so 5 coins = 500 cents and the initial 20 coins grant = 2,000 cents (integer math only).
2. **TAuth** (`tools/TAuth`) – verifies Google Sign-In, issues JWT-backed cookies, exposes `/auth/*`, `/me`, `/static/auth-client.js`. Configure with `APP_ENABLE_CORS=true` and `APP_CORS_ALLOWED_ORIGINS=http://localhost:8000` so the UI origin can exchange cookies.
3. **Demo transaction API** (new Go binary under `cmd/demoapi` + `internal/demoapi`) – HTTP/JSON façade that validates TAuth cookies, applies the "5 coins per transaction" rules, and talks to the ledger via the generated gRPC client (`api/credit/v1`).
4. **UI bundle** (static assets under `demo/ui/`) – HTML/CSS/JS referencing the CDN-hosted `mpr-ui.css`/`mpr-ui.js`, Alpine bootstrap per docs, the TAuth `auth-client.js`, and GIS script. Served by `ghttp --directory demo/ui 8000`.

## Component Responsibilities & Integration Details

### Ledger gRPC (`cmd/credit`)
- Run `ledgerd` via `DATABASE_URL=sqlite:///tmp/demo-ledger.db GRPC_LISTEN_ADDR=:50051 ledgerd`.
- Operations that the demo backend will call:
  - `Grant(user_id, amount_cents=2000, idempotency_key=bootstrap:<user>)` to seed new accounts.
  - `Spend(user_id, amount_cents=500, idempotency_key=spend:<uuid>)` whenever the user clicks the 5-coin button.
  - `Grant(user_id, amount_cents=requested*100, idempotency_key=topup:<uuid>)` when buying coins.
  - `GetBalance(user_id)` to render totals/availability before allowing a spend; `ErrInsufficientFunds` surfaces when `Spend` fails so we can show the second scenario.
  - `ListEntries(user_id, limit=20)` to display ledger history in the UI (optional but recommended for transparency).

### Authentication via TAuth
- Start `tauth` from `tools/TAuth` (either `go run ./cmd/server` or the compose example) with:
  - `APP_LISTEN_ADDR=:8080`, `APP_GOOGLE_WEB_CLIENT_ID=<demo-client-id>`, `APP_JWT_SIGNING_KEY=...`, `APP_COOKIE_DOMAIN=localhost`, `APP_ENABLE_CORS=true`, `APP_DEV_INSECURE_HTTP=true` to allow HTTP during development.
  - `.env.tauth` already documents these knobs.
- Front-end loading order per `tools/mpr-ui/README.md`: CSS → Alpine bootstrap → `http://localhost:8080/static/auth-client.js` → `mpr-ui.js` → GIS script.
- `<mpr-header>` attributes: `base-url="http://localhost:8080"`, `login-path="/auth/google"`, `logout-path="/auth/logout"`, `nonce-path="/auth/nonce"`, `site-id=<client-id>`.
- Demo API trusts TAuth by importing `github.com/tyemirov/tauth/pkg/sessionvalidator`:
  - Configure with the same `APP_JWT_SIGNING_KEY` and `Issuer` used by TAuth (default `tauth`).
  - Use the provided Gin middleware or wrap it in `net/http` middleware to extract claims (user id/email/avatar/roles) before hitting business logic.

### Demo Transaction API (new)
- Implement under `cmd/demo/main.go` using Cobra+Viper (mirrors `cmd/credit`). Key config:
  - `DEMOAPI_LISTEN_ADDR` (HTTP port, default `:9090`).
  - `DEMOAPI_TAUTH_BASE_URL` (default `http://localhost:8080`) to reuse in documentation/responses.
  - `DEMOAPI_JWT_SIGNING_KEY` + `DEMOAPI_JWT_ISSUER` to set up the session validator.
  - `DEMOAPI_LEDGER_ADDR` for the gRPC endpoint (default `localhost:50051`).
- Wire a shared `grpc.ClientConn` to `creditv1.NewCreditServiceClient` with unary interceptors for logging/timeouts.
- Expose the following HTTP endpoints (all JSON, served after the auth middleware):

| Endpoint | Method | Purpose | Ledger interaction |
| --- | --- | --- | --- |
| `/api/session` | GET | Returns the authenticated TAuth profile (id, email, avatar) so UI can display it. | None. |
| `/api/bootstrap` | POST | Idempotently seed the user's wallet with 20 coins. Called automatically after login. | `Grant(2000 cents)` w/ deterministic idempotency key `bootstrap:<userId>`. If already granted, swallow the `ErrDuplicateIdempotencyKey`. |
| `/api/wallet` | GET | Current balance + available funds + ledger history (latest 10 entries). | `GetBalance`, `ListEntries`. |
| `/api/transactions` | POST | Execute the 5-coin spend scenario. Body contains optional metadata (`{ "reason": "demo" }`). Responds with `{ status: "success" }` or `{ status: "insufficient_funds" }`. | `Spend(500 cents)` using a new UUID-based idempotency key per click. Map `ErrInsufficientFunds` to HTTP 409. |
| `/api/purchases` | POST | Buy coins (body `{ "coins": 5 }`). Validates `coins` >= 5 & multiple of 5. Grants `<coins>*100` cents. | `Grant` with metadata describing the top-up source. |

- Optional: future reservation scenarios can reuse `Reserve/Capture`, but LG-100 only needs direct spends.
- Responses include updated balance so UI updates without extra fetches.
- Log structured events with zap and wrap ledger/TAuth errors per `POLICY.md`.
- Use `context.WithTimeout` (~3s) for each gRPC call to avoid hanging the UI.

### Front-End (mpr-ui + custom JS)
- Directory structure: `demo/ui/index.html`, `demo/ui/app.js`, `demo/ui/styles.css`.
- `index.html` loads assets in the documented order and includes:
  - `<mpr-header>` for auth/navigation.
  - `<main>` with:
    - Wallet summary card showing total + available coins (converted from cents).
    - Transaction button: `<button id="transact" class="mpr-btn">Spend 5 coins</button>` accompanied by status text. Button disabled when balance < 5 or when API call is in-flight.
    - Purchase form: radio buttons (`5`, `10`, `20` coins) + "Buy Coins" CTA calling `/api/purchases`.
    - Timeline/list for the three scenarios (success, insufficient funds, zero balance) updating via DOM.
  - `<mpr-footer>` anchored at the bottom.
- `app.js` responsibilities:
  - Call `initAuthClient({ baseUrl: "http://localhost:8080", onAuthenticated, onUnauthenticated })` so we know when the user is ready.
  - After authentication: POST `/api/bootstrap`, then fetch `/api/wallet` to hydrate the UI.
  - Wrap `fetch` helpers that include `credentials: "include"` and hit the demo API origin (`http://localhost:9090`).
  - Listen for `mpr-ui:auth:unauthenticated` to clear UI state.
  - Display scenario status messages using CSS states (e.g., success banner, error banner, zero-balance notice once total == 0).
- CSS: reuse tokens from `mpr-ui.css` for spacing/typography; only add layout wrappers.

### Hosting with ghttp
- Vendor the compiled `ghttp` binary or run `go install github.com/temirov/ghttp/cmd/ghttp@latest` (per `tools/ghttp/README.md`).
- Serve the UI directory via `ghttp --directory demo/ui 8000` (adds markdown rendering and zap logs for free). This keeps front-end static and origin-isolated from the APIs.

### Local Orchestration / Compose
- Add `demo/docker-compose.demo.yml` with services:
  - `ledger`: runs `ledgerd` with SQLite volume `./tmp/data:/app/data`.
  - `tauth`: builds from `tools/TAuth` or pulls published image; loads `.env.tauth`.
  - `demo`: builds from the repo, depends on ledger + tauth, shares `APP_JWT_SIGNING_KEY`.
  - `ghttp`: uses `ghcr.io/temirov/ghttp:latest`, mounts `demo/ui`, serves on `8000`.
- Provide `scripts/demo-up.sh` wrapper (optional) that exports the needed env variables and launches the binaries directly for contributors who prefer the Go toolchain over Compose.

## Implementation Breakdown for LG-101
1. **Backend scaffolding** – create `internal/demo` with:
   - Configuration loader (Viper) + `cmd/demo/main.go` entrypoint.
   - Session middleware using `sessionvalidator`.
   - gRPC client wiring (`grpc.WithTransportCredentials(insecure.NewCredentials())` for local dev, allow TLS later).
   - HTTP handlers per table above with smart constructors for request payloads and domain-level validation (positive coins, multiples of 5, metadata JSON via `credit.MetadataJSON`).
2. **UI bundle** – craft static assets referencing `mpr-ui` components, handshake with TAuth events, and calling backend endpoints. Provide sample copy explaining the scenarios.
3. **Orchestration** – add Compose file + docs showing how to run TAuth, ledger, demo API, and ghttp together. Document port map + env variables in README or a dedicated `docs/demo/README.md`.
4. **Testing** –
   - Go integration tests under `internal/demo` using `httptest.Server` + an in-process ledger service (instantiate `credit.Service` with the SQLite store backed by `t.TempDir()` and `grpc/test/bufconn` to avoid real sockets).
   - UI smoke tests via Playwright (stretch) once LG-101 adds the front-end; scenario scripts click the button three times to observe success/failure/zero-state.
5. **CI hooks** – extend `make test` to run new backend tests; include the demo assets in `make lint` or `npm run lint` only if a JS toolchain becomes necessary (today we stay framework-free).

## Validation & Monitoring Strategy
- Every backend endpoint logs structured fields: `user_id`, `operation`, `idempotency_key`, `coins`, latency.
- Map ledger errors to HTTP codes (`ErrInsufficientFunds` → 409, gRPC unavailability → 503). Always relay a JSON error payload consumed by the UI.
- UI displays toast/banners for each scenario; include telemetry hooks (optional) listening to `mpr-ui:auth:*` events to confirm login/out states.
- Manual demo script (to be written in LG-101 docs) walks through: login → auto-grant 20 coins → click `Transact` 4 times (success, success, success, failure) → `Buy Coins (10)` → `Transact` twice to hit zero.

## Deliverables for Implementing LG-101
- `cmd/demo` binary + supporting `internal/demo/...` packages.
- Static UI assets under `demo/ui/` with `mpr-ui` components + JS glue.
- `demo/docker-compose.demo.yml` (or instructions for running binaries manually) plus `.env.demoapi.example` capturing required env vars.
- Documentation snippet (README section or `docs/demo.md`) that references this plan, lists ports, and explains how to run the demo.
- Integration tests verifying ledger balances for the three required scenarios.

Following this plan ties together every dependency under `tools/`, keeps validation at the HTTP edge (per `POLICY.md`), and gives LG-101 a concrete backlog of backend/UI tasks to implement the demo end-to-end.
