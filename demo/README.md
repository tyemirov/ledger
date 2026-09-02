# Local Ledger Workspace

The local stack runs Ledger, TAuth, and one same-origin `ghttp` entrypoint. Ledger serves the production workspace and control plane.

The proxy sends `/auth` and `/me` to TAuth. It sends all other paths to the Ledger HTTP listener.

## Configuration

Keep the local private values in `demo/configs/.env.ledger` and `demo/configs/.env.tauth`. The Ledger file must define these names:

- `DATABASE_URL`
- `LEDGER_PUBLIC_ORIGIN`
- `TAUTH_GOOGLE_CLIENT_ID`
- `TAUTH_JWT_ISSUER`
- `TAUTH_JWT_SIGNING_KEY`
- `TAUTH_SESSION_COOKIE_NAME`
- `TAUTH_TENANT_ID`
- `TAUTH_URL`

Use the same TAuth tenant, signing key, cookie name, and Google client ID in both local services.

For the `localhost` profile, use `http://localhost:8000` for `LEDGER_PUBLIC_ORIGIN` and `TAUTH_URL`.

## Start

Run one profile from `demo/`:

```bash
./up.sh localhost
```

```bash
./up.sh computercat
```

The `computercat` profile keeps the existing host TLS file contract. Run `./down.sh` to stop the stack.

After sign-in, Ledger provisions one UserAccount. You can create tenants and separate application credentials from the workspace.
