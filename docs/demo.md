# Demo Stack Guide

This document mirrors `docs/lg-100-demo-plan.md` and describes how to run the end-to-end wallet scenario that combines creditd (ledger), TAuth, the wallet API under `ledger_demo/backend`, and the static UI under `ledger_demo/frontend`.

## Components

1. **creditd** (`cmd/credit`) – append-only ledger exposed via gRPC on `:7000` (Compose publishes it on host port `7700` so macOS Control Center can keep `7000` free).
2. **TAuth** (`tools/TAuth`) – Google Sign-In + JWT session issuer on `:8080`.
3. **walletapi** (`ledger_demo/backend/cmd/walletapi`) – HTTP façade that validates TAuth sessions and performs ledger RPCs.
4. **ghttp** (`ghcr.io/temirov/ghttp`) – static server for `ledger_demo/frontend/ui` on `:8000`.

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
3. **walletapi** (signing key, issuer, cookie name, and timeout must match TAuth)
  ```bash
  WALLETAPI_LISTEN_ADDR=:9090 \
  WALLETAPI_LEDGER_ADDR=localhost:7000 \
  WALLETAPI_LEDGER_INSECURE=true \
  WALLETAPI_LEDGER_TIMEOUT=3s \
  WALLETAPI_ALLOWED_ORIGINS=http://localhost:8000 \
  WALLETAPI_JWT_SIGNING_KEY="secret" \
  WALLETAPI_JWT_ISSUER=tauth \
  WALLETAPI_JWT_COOKIE_NAME=app_session \
  WALLETAPI_TAUTH_BASE_URL=http://localhost:8080 \
  go run ./ledger_demo/backend/cmd/walletapi
  ```
4. **Static UI** (requires `ghttp` binary or Docker image)
  ```bash
  ghttp --directory ledger_demo/frontend/ui 8000
  ```
5. Open `http://localhost:8000` and sign in via the header button. The UI will automatically bootstrap the wallet and call `/api/transactions` and `/api/purchases` as you interact with the buttons.

## Docker Compose Workflow

The repository ships `ledger_demo/docker-compose.yml` plus env templates so you can run the entire stack with one command.

1. Copy the env templates:
   ```bash
   cd ledger_demo
   cp backend/.env.walletapi.example backend/.env.walletapi
   cp .env.tauth.example .env.tauth
   ```
   Edit both files so `WALLETAPI_JWT_SIGNING_KEY` matches `APP_JWT_SIGNING_KEY` and provide your Google OAuth Web Client ID.
2. Start the stack (creditd publishes on host port `7700`; edit `ledger_demo/docker-compose.yml` if you need a different port):
   ```bash
   docker compose -f ledger_demo/docker-compose.yml up --build
   ```
3. Visit `http://localhost:8000` (ghttp), `http://localhost:9090/api/wallet` (walletapi), and `http://localhost:8080` (TAuth) to confirm connectivity. The UI must load `http://localhost:8080/demo/config.js` so `<mpr-header>` receives the Google OAuth Web Client ID defined in `ledger_demo/.env.tauth`; if that script fails or the env variable is empty, sign-in will not work.
4. Stop everything with `docker compose -f ledger_demo/docker-compose.yml down`.

Volumes `ledger_data` and `tauth_data` persist ledger entries plus refresh tokens. Remove them with `docker volume rm ledger_ledger_data ledger_tauth_data` if you need a fresh state.

## Scenario Checklist

Once authenticated:

1. The UI automatically grants 20 coins (POST `/api/bootstrap`).
2. Click **Spend 5 coins** three times – the first four transactions should succeed until the balance reaches 5 coins.
3. Click the button again to observe the **insufficient funds** state (HTTP 409 converted to a banner message).
4. Use **Buy coins** (5 or 10) to top up and watch the ledger list update.
5. Continue spending until the **zero balance** banner appears, confirming the third requirement from LG-100.

Monitor logs for:

- `walletapi`: zap logs containing `status` + `user_id` fields.
- `creditd`: gRPC operations landing in the ledger.
- `tauth`: nonce/login/refresh lifecycle.

The UI also surfaces toast banners for auth/sign-out events so flows remain observable without tailing logs. For automated coverage, run `npm run test:ui`, which executes the Playwright suite under `ledger_demo/tests` (the same command is wired into `make test`).
