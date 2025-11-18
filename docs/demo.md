# Demo Stack Guide

This document mirrors `docs/lg-100-demo-plan.md` and describes how to run the end-to-end wallet scenario that combines creditd (ledger), TAuth, the new demo API, and the static UI.

## Components

1. **creditd** (`cmd/credit`) – append-only ledger exposed via gRPC on `:7000`.
2. **TAuth** (`tools/TAuth`) – Google Sign-In + JWT session issuer on `:8080`.
3. **demoapi** (`cmd/demoapi`) – HTTP façade that validates TAuth sessions and performs ledger RPCs.
4. **ghttp** (`ghcr.io/temirov/ghttp`) – static server for `demo/ui` on `:8000`.

## Manual Run (Go toolchain)

1. **creditd**
   ```bash
   DATABASE_URL=sqlite:///tmp/demo-ledger.db GRPC_LISTEN_ADDR=:7000 go run ./cmd/credit
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
   DEMOAPI_LEDGER_ADDR=localhost:7000 \
   DEMOAPI_LEDGER_INSECURE=true \
   DEMOAPI_LEDGER_TIMEOUT=3s \
   DEMOAPI_ALLOWED_ORIGINS=http://localhost:8000 \
   DEMOAPI_JWT_SIGNING_KEY="secret" \
   DEMOAPI_JWT_ISSUER=tauth \
   DEMOAPI_JWT_COOKIE_NAME=app_session \
   DEMOAPI_TAUTH_BASE_URL=http://localhost:8080 \
   go run ./cmd/demoapi
   ```
4. **Static UI** (requires `ghttp` binary or Docker image)
   ```bash
   ghttp --directory demo/ui 8000
   ```
5. Open `http://localhost:8000` and sign in via the header button. The UI will automatically bootstrap the wallet and call `/api/transactions` and `/api/purchases` as you interact with the buttons.

## Docker Compose Workflow

The repository ships `docker-compose.demo.yml` plus env templates so you can run the entire stack with one command.

1. Copy the env templates:
   ```bash
   cp .env.demoapi.example .env.demoapi
   cp demo/.env.tauth.example demo/.env.tauth
   ```
   Edit both files so `DEMOAPI_JWT_SIGNING_KEY` matches `APP_JWT_SIGNING_KEY` and provide your Google OAuth Web Client ID.
2. Start the stack (creditd is published on host port `7700` by default to avoid macOS Control Center occupying `7000`; adjust `docker-compose.demo.yml` if another port works better):
   ```bash
   docker compose -f docker-compose.demo.yml up --build
   ```
3. Visit `http://localhost:8000` (ghttp), `http://localhost:9090/api/wallet` (demoapi), and `http://localhost:8080` (TAuth) to confirm connectivity.
4. Stop everything with `docker compose -f docker-compose.demo.yml down`.

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
- `creditd`: gRPC operations landing in the ledger.
- `tauth`: nonce/login/refresh lifecycle.

The UI also surfaces toast banners for auth/sign-out events so flows remain observable without tailing logs.
