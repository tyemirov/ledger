# Local Ledger Workspace

The local runtime contains Ledger, TAuth, and one same-origin `ghttp` entrypoint. Ledger serves the current workspace and control plane.

The proxy sends `/auth` and `/me` to TAuth. It sends all other paths to the Ledger HTTP listener.

Ledger and TAuth use separate SQLite volumes. The local shutdown command preserves both volumes.

## Start

Run this command from the repository root:

```bash
make up
```

The command builds Ledger from the current source. It generates one private local signing key in `.cache/ledger-local`.

The command returns after all local readiness checks pass. Open `http://localhost:8000/` for the browser workspace.

Use `localhost:50051` for a local gRPC client. Create a Ledger tenant and its credential in the browser workspace first.

## Stop

Run this command from the repository root:

```bash
make down
```

The command stops only the `ledger-local` Compose project. It preserves the SQLite data and the local signing key.
