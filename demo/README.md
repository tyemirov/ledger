# Demo Stack Guide

This demo is self-contained under `demo/`. Runtime config lives in `demo/configs/`, the UI is served through `ghttp`, and the stack can be started in one of two frontend modes:

- `localhost`: plain HTTP on `http://localhost:8000`
- `computercat`: HTTPS on `https://localhost:4443` or the computercat host name, using host TLS files

## Components

1. `ledgerd` on `:50051`
2. `tauth` on `:8081`
3. `demoapi` on `:9090`
4. `ghttp` as the UI entrypoint, with `/api`, `/auth`, `/me`, and `/tauth.js` proxied through the same origin

## Config Layout

- `demo/configs/config.yml`: ledger service config
- `demo/configs/.env.ledger`: ledger runtime env
- `demo/configs/tauth.config.yaml`: TAuth config
- `demo/configs/.env.tauth`: TAuth runtime env
- `demo/configs/.env.demoapi`: demo backend env

The shipped demo uses the `demo` tenant and the `demo` ledger ID end to end.

## Google OAuth Client ID

To replace the Google OAuth Web client ID:

```bash
cd demo
make configure-google-client-id GOOGLE_CLIENT_ID="your-client-id.apps.googleusercontent.com"
```

That updates `demo/config.js`, the UI fallbacks, `demo/configs/tauth.config.yaml`, and the TAuth env files in `demo/configs/`.

## Start The Demo

From `demo/`:

```bash
./up.sh localhost
```

or:

```bash
./up.sh computercat
```

`localhost` is the default if you omit the argument.

Direct Compose equivalents:

```bash
docker compose --profile localhost up --build
docker compose --profile computercat up --build
```

## Frontend Modes

`localhost`:
- No TLS certificates required
- UI served at `http://localhost:8000`

`computercat`:
- Uses TLS on `:4443`
- Expects host certificate files mounted through:
  - `DEMO_TLS_CERT_FILE`
  - `DEMO_TLS_KEY_FILE`
- If those variables are unset, Compose defaults to `/media/share/Drive/exchange/certs/computercat/computercat-cert.pem` and `/media/share/Drive/exchange/certs/computercat/computercat-key.pem`

## Stop The Demo

From `demo/`:

```bash
./down.sh
```

## Smoke Check

After startup:

1. Open the frontend for the selected profile.
2. Sign in through the header.
3. Confirm the wallet bootstraps and the balance appears.
4. Spend, purchase, reserve, capture, release, refund, and batch actions should all flow through the proxied `/api` routes.

Persistent volumes:

- `ledger_data`
- `tauth_data`
