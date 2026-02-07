# Demo Stack Guide

This document describes how to run the end-to-end wallet scenario that combines `ledgerd`, TAuth, the demo API, and the static UI. A more detailed plan lives under `demo/docs/lg-100-demo-plan.md` if you need the background notes.

## Components

1. **ledgerd** (`cmd/credit`) – append-only ledger exposed via gRPC on `:50051`.
2. **TAuth** (`tools/TAuth`) – Google Sign-In + JWT session issuer (proxied through ghttp in the Compose workflow).
3. **demo backend** (`backend/cmd/demo`) – HTTP façade that validates TAuth sessions and performs ledger RPCs.
4. **ghttp** (`ghcr.io/tyemirov/ghttp`) – HTTPS entrypoint for `demo/ui` on `:8080` with proxy routes for `/api` and TAuth endpoints.

## Google OAuth Client ID

Before running the stack with your own Google OAuth Web client, sync the ID across the UI and TAuth by running:

```bash
cd demo
make configure-google-client-id GOOGLE_CLIENT_ID="your-client-id.apps.googleusercontent.com"
```

The helper updates `demo/config.js`, all UI fallbacks, and both `.env.tauth` files so the `<mpr-header>` and the TAuth server agree on the same ID. Restart TAuth and reload the UI after changing the value.

## Manual Run (Go toolchain)

1. **ledgerd**
   ```bash
   DATABASE_URL=sqlite:///tmp/demo-ledger.db GRPC_LISTEN_ADDR=:50051 go run ./cmd/credit
   ```
2. **TAuth** (run from `tools/TAuth` and reuse its README instructions)
   ```bash
   cd tools/TAuth
   APP_LISTEN_ADDR=:8081 \
   APP_GOOGLE_WEB_CLIENT_ID="your-client-id.apps.googleusercontent.com" \
   APP_JWT_SIGNING_KEY="secret" \
   APP_COOKIE_DOMAIN=localhost \
   APP_ENABLE_CORS=true \
   APP_CORS_ALLOWED_ORIGINS=https://localhost:8080 \
   APP_DEV_INSECURE_HTTP=true \
   APP_DATABASE_URL=sqlite:///data/tauth.db \
   go run ./cmd/server
   ```
3. **demo backend** (signing key, issuer, cookie name, and timeout must match TAuth)
   ```bash
   cd backend
   DEMOAPI_LISTEN_ADDR=:9090 \
   DEMOAPI_LEDGER_ADDR=localhost:50051 \
   DEMOAPI_LEDGER_INSECURE=true \
   DEMOAPI_LEDGER_TIMEOUT=3s \
   DEMOAPI_DEFAULT_TENANT_ID=default \
   DEMOAPI_DEFAULT_LEDGER_ID=default \
   DEMOAPI_ALLOWED_ORIGINS=https://localhost:8080 \
   DEMOAPI_JWT_SIGNING_KEY="secret" \
   DEMOAPI_JWT_ISSUER=tauth \
   DEMOAPI_JWT_COOKIE_NAME=app_session \
   DEMOAPI_TAUTH_BASE_URL=http://localhost:8081 \
   go run ./cmd/demo
   ```
4. **Static UI** (requires `ghttp` binary or Docker image)
   ```bash
   ghttp 8080 \
     --directory demo/ui \
     --tls-cert demo/certs/computercat-cert.pem \
     --tls-key demo/certs/computercat-key.pem \
     --proxy /api=http://localhost:9090 \
     --proxy /auth=http://localhost:8081 \
     --proxy /me=http://localhost:8081 \
     --proxy /tauth.js=http://localhost:8081
   ```
5. Open `https://localhost:8080` and sign in via the header button. The UI will automatically bootstrap the wallet and call `/api/transactions` and `/api/purchases` as you interact with the buttons.

## Docker Compose Workflow

The repository ships `demo/docker-compose.yml` plus env templates so you can run the entire stack with one command. The compose stack provisions Postgres for `ledgerd`, and `ledgerd` applies its schema automatically via GORM on startup.

1. From the `demo/` directory, ensure the env files exist (copy templates if needed):
   ```bash
   cd demo
   cp -n .env.demoapi.example .env.demoapi
   cp -n .env.tauth.example .env.tauth
   cd -
   ```
   Keep `DEMOAPI_JWT_SIGNING_KEY` aligned with the `jwt_signing_key` in `demo/tauth.config.yaml`. If you need to change the Google OAuth Web Client ID, run:
   ```bash
   cd demo
   make configure-google-client-id GOOGLE_CLIENT_ID="your-client-id.apps.googleusercontent.com"
   ```
2. Start the stack (`ledgerd` binds to host port `50051` to follow the standard gRPC port; adjust `demo/docker-compose.yml` if your machine needs a different port). The Dockerfile builds the backend from the local `demo/backend` sources:
   ```bash
   docker compose up --build
   ```
3. Visit `https://localhost:8080` (ghttp), `http://localhost:9090/api/wallet` (demo backend), and `http://localhost:8081` (TAuth) to confirm connectivity. The UI reads configuration from `/config.js` (served by ghttp), so edits to `demo/config.js` are picked up automatically on reload.
4. Stop everything with `docker compose down`.

Volumes `ledger_postgres_data` and `tauth_data` persist ledger entries plus refresh tokens. Remove them with `docker volume rm ledger_ledger_postgres_data ledger_tauth_data` if you need a fresh state.

## Scenario Checklist

Once authenticated:

1. The UI automatically grants 20 coins (POST `/api/bootstrap`).
2. Click **Spend 5 coins** three times – the first four transactions should succeed until the balance reaches 5 coins.
3. Click the button again to observe the **insufficient funds** state (HTTP 409 converted to a banner message).
4. Use **Buy coins** (5 or 10) to top up and watch the ledger list update.
5. Continue spending until the **zero balance** banner appears, confirming the third requirement from LG-100.

Monitor logs for:

- Demo backend: zap logs containing `status` + `user_id` fields.
- `ledgerd`: gRPC operations landing in the ledger.
- `tauth`: nonce/login/refresh lifecycle.

The UI also surfaces toast banners for auth/sign-out events so flows remain observable without tailing logs.
