# Demo Stack Guide

This document mirrors `demo/docs/lg-100-demo-plan.md` and describes how to run the end-to-end wallet scenario that combines ledgerd (ledger), TAuth, the new demo API, and the static UI.

## Components

1. **ledgerd** (`cmd/ledger`) – append-only ledger exposed via gRPC on `:50051` (plaintext; host and container).
2. **TAuth** (`tools/TAuth`) – Google Sign-In + JWT session issuer on `:8080`.
3. **demoapi** (`demo/backend/cmd/demoapi`) – HTTP façade that validates TAuth sessions and performs ledger RPCs.
4. **ghttp** (`ghcr.io/temirov/ghttp`) – static server for `demo/ui` on `:8000`.

## Manual Run (Go toolchain)

1. **ledgerd**
   ```bash
   DATABASE_URL=sqlite:///tmp/demo-ledger.db GRPC_LISTEN_ADDR=:50051 go run ./cmd/ledger
   ```
2. **TAuth** (run from `tools/TAuth` and reuse its README instructions)
   ```bash
   cd tools/TAuth
   APP_LISTEN_ADDR=:8080 \
   APP_GOOGLE_WEB_CLIENT_ID="your-client-id.apps.googleusercontent.com" \
   APP_JWT_SIGNING_KEY="secret" \
   APP_COOKIE_DOMAIN=localhost \
   APP_ENABLE_CORS=true \
   APP_CORS_ALLOWED_ORIGINS=http://localhost:8000 \
   APP_DEV_INSECURE_HTTP=true \
   APP_DATABASE_URL=sqlite:///data/tauth.db \
   go run ./cmd/server
   ```
3. **demoapi** (signing key, issuer, cookie name, and timeout must match TAuth)
   ```bash
   cd demo/backend
   DEMOAPI_LISTEN_ADDR=:9090 \
   DEMOAPI_LEDGER_ADDR=localhost:50051 \
   DEMOAPI_LEDGER_INSECURE=true \
   DEMOAPI_LEDGER_TIMEOUT=3s \
   DEMOAPI_ALLOWED_ORIGINS=http://localhost:8000 \
   DEMOAPI_JWT_SIGNING_KEY="secret" \
   DEMOAPI_JWT_ISSUER=mprlab-auth \
   DEMOAPI_JWT_COOKIE_NAME=app_session \
   DEMOAPI_TAUTH_BASE_URL=http://localhost:8080 \
   go run ./cmd/demoapi
   ```
4. **Static UI** (requires `ghttp` binary or Docker image)
   ```bash
   ghttp --directory demo/ui 8000
   ```
   The page defaults to the demo API origin `http://localhost:9090` via the `<body data-api-base-url="...">` attribute. Override that value if your API is published elsewhere.
5. Open `http://localhost:8000` and sign in via the header button. The UI will automatically bootstrap the wallet and call `/api/transactions` and `/api/purchases` as you interact with the buttons.

## Docker Compose Workflow

The repository ships a single compose file under `demo/docker-compose.yml` plus env templates so you can run the entire stack with one command.

1. Copy the env templates:
   ```bash
   cd demo
   cp .env.demoapi.example .env.demoapi
   cp .env.tauth.example .env.tauth
   cp .env.ledgersvc.example .env.ledgersvc
   ```
   Edit the files so `DEMOAPI_JWT_SIGNING_KEY` matches `APP_JWT_SIGNING_KEY` and provide your Google OAuth Web Client ID.
2. Start the wallet stack (ledgerd + demoapi + ghttp + TAuth):
   ```bash
   docker compose up --build
   ```
3. Visit `http://localhost:8000` (ghttp), `http://localhost:9090/api/wallet` (demoapi), and `http://localhost:8080` (TAuth) to confirm connectivity. The UI loads `http://localhost:8080/demo/config.js`, so whatever Google OAuth Web Client ID you set in `.env.tauth` is automatically injected into `<mpr-header>`—no need to edit the HTML file manually.
4. Stop everything with `docker compose down`.

Volumes `ledger_data` and `tauth_data` persist ledger entries plus refresh tokens. Remove them with `docker volume rm ledger_ledger_data ledger_tauth_data` if you need a fresh state.

## Scenario Checklist

Once authenticated:

1. The UI automatically grants 20 coins (POST `/api/bootstrap`).
2. Click **Spend 5 coins** three times – the first four transactions should succeed until the balance reaches 5 coins.
3. Click the button again to observe the **insufficient funds** state (HTTP 409 converted to a banner message).
4. Use **Buy coins** (5 or 10) to top up and watch the ledger list update.
5. Continue spending until the **zero balance** banner appears, confirming the third requirement from LG-100.

Monitor logs for:

- `demoapi`: zap logs containing `status` + `user_id` fields.
- `ledgerd`: gRPC operations landing in the ledger.
- `tauth`: nonce/login/refresh lifecycle.

The UI also surfaces toast banners for auth/sign-out events so flows remain observable without tailing logs.

## Auth-Only Login Demo (LG-301)

For a lightweight login-only check (no ledger or demoapi dependencies), start only TAuth + ghttp manually (or comment out the other services in `demo/docker-compose.yml`) and follow the same UI steps. There isn’t a dedicated compose profile anymore; the primary compose stack always runs the full demo.

## Self-Contained End-to-End Demo from `demo/`

When you want the full ledger + demoapi + UI flow without referencing files outside `demo/`, use the bundled Dockerfiles and compose file in that directory. The Dockerfiles build both binaries from the checked-out repository (with `replace` directives), while ghttp/TAuth images continue to come from GHCR.

1. Populate the environment files as described above.
2. Launch the stack (builds ledgerd/demoapi from the Dockerfiles in `demo/` and pulls TAuth + ghttp from GHCR):
   ```bash
   docker compose up --build
   ```
3. Visit `http://localhost:8000` (UI), `http://localhost:9090/api/wallet` (demoapi), `http://localhost:8080` (TAuth), and `http://localhost:50051` (ledger gRPC). Stop with `docker compose down`.

## Automated UI Check (no manual clicking)

- Install Playwright once (at repo root): `npm install --no-save playwright@1.41.2`
- Run the stubbed auth + wallet flow without touching the mouse:

  ```bash
  node demo/tests/auth-refresh.spec.js
  ```

  The script spins up a static server for `demo/ui`, stubs TAuth + wallet API endpoints, and asserts the header/UI stay authenticated across a reload (refresh-token path). It’s purely local—no Google sign-in required.
