# Demo Stack Guide

This document mirrors `docs/lg-100-demo-plan.md` and describes how to run the end-to-end wallet scenario that combines creditd (ledger), TAuth, the wallet API under `demo/backend`, and the static UI under `demo/frontend`.

> LG-301 note: the UI is currently **login-only** to debug TAuth + Google sign-in. Ledger and wallet calls remain available behind the proxy, but the front-end panels are disabled until authentication is stable.

## Components

1. **creditd** (`cmd/credit`) – append-only ledger exposed via gRPC on `:7000` (Compose publishes it on host port `7700` so macOS Control Center can keep `7000` free).
2. **TAuth** (`tools/TAuth`) – Google Sign-In + JWT session issuer on `:8080`.
3. **walletapi** (`demo/backend/cmd/walletapi`) – HTTP façade that validates TAuth sessions and performs ledger RPCs.
4. **web** (`nginx:alpine`) – serves `demo/frontend/ui` on `:8000` and reverse-proxies `/auth/*` and `/api/*` to the backend services so cookies stay on the same origin.

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
  go run ./demo/backend/cmd/walletapi
  ```
4. **Static UI + proxy** (requires Nginx or the docker-compose stack)
  ```bash
  docker run --rm -p 8000:8000 -v $(pwd)/demo/frontend/ui:/usr/share/nginx/html -v $(pwd)/demo/nginx.conf:/etc/nginx/conf.d/default.conf nginx:1.25-alpine
  ```
5. Open `http://localhost:8000` and sign in via the header button. The UI surfaces only the authentication card while we debug login stability; ledger and wallet endpoints stay available behind the proxy but are not invoked by the page.

## Docker Compose Workflow

The repository ships `demo/docker-compose.yml` plus env templates so you can run the entire stack. **Treat `demo/` as a fully standalone application**: you can copy this directory anywhere (or check it out independently) because its Go backend lives in its own module (`demo/backend/go.mod`) and consumes the published `github.com/MarkoPoloResearchLab/ledger` module for gRPC types. Compose commands should be executed from inside the `demo/` directory (paths are relative). Build the ledger image once from the repo root before bringing up the demo stack:

```bash
docker build -t ledger-creditd .
```

1. Copy the env templates:
   ```bash
   cd demo
   cp backend/.env.walletapi.example backend/.env.walletapi
   cp .env.tauth.example .env.tauth
   ```
   Edit both files so `WALLETAPI_JWT_SIGNING_KEY` matches `APP_JWT_SIGNING_KEY` and provide your Google OAuth Web Client ID. In Google Cloud Console, open that Web client and add **both** `http://localhost:8000` (the Nginx proxy that serves the UI and forwards `/auth/*` + `/static/*`) and `http://localhost:8080` (direct access to TAuth) to the **Authorized JavaScript origins** list—Google will omit the nonce claim if those origins are missing, causing TAuth to reject the credential exchange per `docs/TAuth/usage.md`.
2. Start the stack from the `demo/` directory (creditd publishes on host port `7700`; edit `demo/docker-compose.yml` if you need a different port):
   ```bash
   docker compose up --build
   ```
3. Visit `http://localhost:8000` (Nginx proxy), `http://localhost:9090/api/wallet` (walletapi), and `http://localhost:8080` (TAuth) to confirm connectivity. The UI loads `/demo/config.js` through the proxy so `<mpr-header>` receives the Google OAuth Web Client ID defined in `demo/.env.tauth`; if that script fails or the env variable is empty, sign-in will not work. While LG-301 is active the page renders only the auth card; wallet endpoints stay reachable for manual testing but are not called from the UI.
4. Stop everything with `docker compose down`.

Volumes `ledger_data` and `tauth_data` persist ledger entries plus refresh tokens. Remove them with `docker volume rm ledger_ledger_data ledger_tauth_data` if you need a fresh state.

## Scenario Checklist

Focus on authentication while ledger flows are disabled in the UI:

1. Load `http://localhost:8000` and confirm the page shows a signed-out prompt.
2. Click the Google sign-in button; TAuth should accept the nonce + credential exchange and the session card should show your profile.
3. Reload the page and verify the session persists (the card is already populated without clicking anything).
4. Click **Sign out** and confirm the cookies clear plus the UI returns to the signed-out state.

Monitor logs for:

- `tauth`: nonce/login/refresh lifecycle.
- `walletapi`/`creditd`: optional while LG-301 is active; the UI no longer calls these services but they remain available for manual exercises.

The UI surfaces auth banners in place of wallet toasts. For automated coverage, run `npm run test:ui`, which executes the Playwright suite under `demo/tests` (the same command is wired into `make test`).
