# Demo Stack Guide

This document mirrors `docs/lg-100-demo-plan.md` and describes how to run the end-to-end wallet scenario that combines ledgerd (ledger), TAuth, the new demo API, and the static UI.

## Components

1. **ledgerd** (`cmd/ledger`) – append-only ledger exposed via gRPC on `:50051` (plaintext; host and container).
2. **TAuth** (`tools/TAuth`) – Google Sign-In + JWT session issuer on `:8080`.
3. **demoapi** (`cmd/demoapi`) – HTTP façade that validates TAuth sessions and performs ledger RPCs.
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
2. Start the wallet stack (the compose file uses profiles; `demo` boots ledgerd + demoapi + ghttp + TAuth):
   ```bash
   docker compose up --build --profile demo
   ```
3. Visit `http://localhost:8000` (ghttp), `http://localhost:9090/api/wallet` (demoapi), and `http://localhost:8080` (TAuth) to confirm connectivity. The UI loads `http://localhost:8080/demo/config.js`, so whatever Google OAuth Web Client ID you set in `.env.tauth` is automatically injected into `<mpr-header>`—no need to edit the HTML file manually.
4. Stop everything with `docker compose down --profile demo`.

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

For a lightweight login-only check (no ledger or demoapi dependencies), reuse the same compose file with the `auth` profile so the stack stays self-contained:

1. Populate `.env.tauth` inside `demo/` if you haven't already.
2. Run only the auth profile:
   ```bash
   cd demo
   docker compose up --build --profile auth
   ```
3. Open `http://localhost:8000` (served from `demo/ui/index.html`). The page pulls `http://localhost:8080/demo/config.js`, uses `<mpr-header>` pointed at TAuth (`/auth/nonce`, `/auth/google`, `/auth/logout`), loads `auth-client.js`, and keeps you signed in after refresh.
4. Stop the login stack with `docker compose down --profile auth`.

## Self-Contained End-to-End Demo from `demo/`

When you want the full ledger + demoapi + UI flow without referencing files outside `demo/`, use the bundled Dockerfiles and compose file in that directory. All images are either built from public sources (ledger/demoapi via Go module download) or pulled from GHCR/CDN.

1. Populate the environment files as described above.
2. Launch the demo profile (builds ledgerd/demoapi from the Dockerfiles in `demo/` and pulls TAuth + ghttp from GHCR):
   ```bash
   docker compose up --build --profile demo
   ```
3. Visit `http://localhost:8000` (UI), `http://localhost:9090/api/wallet` (demoapi), `http://localhost:8080` (TAuth), and `http://localhost:7000` (ledger gRPC). Stop with `docker compose down --profile demo`.
