# Ledger setup

## Local workspace

Use `make up` from the repository root for the complete local runtime.
Install Git, Make, Docker with Compose, OpenSSL, and curl. Start the Docker daemon before the command.
The build uses container tooling. Host Go and protobuf tools are necessary only for source work.

The local runtime contains Ledger, TAuth, and one same-origin proxy.
Open `http://localhost:8000/` for the workspace. Use `localhost:50051` for gRPC.
Sign in with Google, create a named tenant, and create a credential for that tenant.
Export the tenant UUID and credential for your application client.

The local script generates a signing key under `.cache/ledger-local` and supplies the local environment files.
Ledger and TAuth use separate SQLite volumes. `make down` preserves the volumes and signing key.
The local Google client comes from [the startup script](../demo/up.sh).
See [the local contract](../demo/README.md) for start, stop, routing, and readiness behavior.

## Build from source

Use Go 1.26.5 or later, as declared in [go.mod](../go.mod).
Run these commands from the repository root:

```bash
go mod download
go build -o ledgerd ./cmd/credit
```

Provide the configuration values below. Then start the service:

```bash
./ledgerd --config configs/config.ledger.yml
```

For a source run, use this command with the same configuration:

```bash
go run ./cmd/credit --config configs/config.ledger.yml
```

The default configuration path is `config.yml`. Use `--config <path>` to select another file.
The configuration file is mandatory. The loader rejects unknown fields and missing required values.

## Runtime configuration

[configs/config.ledger.yml](../configs/config.ledger.yml) is the tracked service configuration.
The loader expands environment variables in YAML at startup.
It also accepts `${VARIABLE:-default}` expressions when the selected file declares a default.
The tracked file requires `DATABASE_URL` explicitly.

```yaml
service:
  database_url: "${DATABASE_URL}"
  grpc_listen_addr: ":50051"
  http_listen_addr: ":8080"

auth:
  jwt_signing_key: "${TAUTH_JWT_SIGNING_KEY}"
  jwt_issuer: "tauth"
  tauth_tenant_id: "${TAUTH_TENANT_ID}"
  session_cookie_name: "${TAUTH_SESSION_COOKIE_NAME}"
  public_origin: "${LEDGER_PUBLIC_ORIGIN}"

ui:
  description: "Ledger"
  tauth_url: "${TAUTH_URL}"
  google_client_id: "${TAUTH_GOOGLE_CLIENT_ID}"
  login_path: "/auth/google"
  logout_path: "/auth/logout"
  nonce_path: "/auth/nonce"
  session_path: "/auth/session"
```

| Variable | Purpose |
| --- | --- |
| `DATABASE_URL` | SQLite path or PostgreSQL DSN |
| `LEDGER_PUBLIC_ORIGIN` | Exact browser origin allowed for control-plane requests |
| `TAUTH_JWT_SIGNING_KEY` | Key shared with the selected TAuth tenant |
| `TAUTH_TENANT_ID` | TAuth realm accepted by the control plane |
| `TAUTH_SESSION_COOKIE_NAME` | Cookie that contains the TAuth session |
| `TAUTH_URL` | Browser authentication origin |
| `TAUTH_GOOGLE_CLIENT_ID` | Google web client used by the workspace |

Listener addresses, issuer, and TAuth paths are explicit fields in the selected YAML file.
The standalone service requires its control-plane configuration as well as its gRPC configuration.
The loader rejects static tenant declarations. The control plane creates tenants and stores credential digests.

Configure the TAuth tenant with the same signing key, issuer, cookie name, and browser origin.
See [the local TAuth configuration](../demo/configs/tauth.config.yaml) for the local profile.
See [deployment](deployment.md) for the tracked hosted profile.

### Database selection

Use a SQLite URL such as `sqlite:///tmp/ledger.db`, or a PostgreSQL DSN such as `postgres://user:password@host/database`.
SQLite creates its database file automatically. For PostgreSQL, create the database before startup.
The configured PostgreSQL user must be able to create the required tables and indexes.
The service selects the GORM driver from the URL scheme and applies the canonical schema at startup.

The repository has no PostgreSQL server-version acceptance matrix.

The service rejects an obsolete `accounts` table before schema changes.
Existing embedded databases use the application's controlled schema procedure. See [embedded integration](integration.md#2-embedding-the-go-library).

## HTTP and gRPC boundaries

The HTTP listener serves the workspace at `/`, assets under `/assets/ledger/`, public config at `/config-ui.yaml`, and `GET /healthz`.
The HTTP control plane exposes these TAuth-protected resources:

- `PUT` and `GET /api/user-account`.
- `POST` and `GET /api/tenants`.
- `GET /api/tenants/{tenant_id}`.
- `POST` and `GET /api/tenants/{tenant_id}/credentials`.
- `DELETE /api/tenants/{tenant_id}/credentials/{credential_id}`.

The browser requests protected resources after `mpr-ui` reports an authenticated session.
Unsafe requests require the exact configured `Origin` and `X-Ledger-CSRF: 1`.
Tenant and credential creation also require `Idempotency-Key`.
A credential secret appears only in its successful creation response.
The [OpenAPI contract](../api/control/v1/openapi.yaml) defines the representations and errors.

Every gRPC request requires `authorization: Bearer <tenant_credential>` metadata.
The credential must belong to the request's `tenant_id`. `Batch` uses the nested `account.tenant_id`.
Ledger checks the stored credential digest and revocation state.
Use a private interface or private network for gRPC. See the [API reference](api.md#authentication).

## Source layout

| Path | Responsibility |
| --- | --- |
| `pkg/ledger` | Reusable accounting domain and service |
| `pkg/gormstore` | SQLite and PostgreSQL GORM adapter |
| `internal/grpcserver` | gRPC handlers |
| `internal/controlplane` | HTTP resources and browser workspace |
| `internal/useraccount` | UserAccount identity and provisioning |
| `internal/tenant` | Tenant ownership and credentials |
| `api/credit/v1` | Protobuf schema and generated Go bindings |

## Development tools

Use Node.js and npm for the browser checks. The frontend targets install their declared npm dependencies and Chromium test runtime.
The Go lint target installs its declared analyzers for the active toolchain.
Use Docker for browser-server, local runtime, and Pages artifact integration checks.
Use Bash, curl, and OpenSSL for the local runtime scripts.
Use the installed `mprlab-gateway` command for lifecycle planning and operations.

```bash
make fmt
make lint
make test
make ci
make test-embedded-store
make test-installed-gateway
make test-gateway-plan
```

`make fmt` checks formatting. `make format` applies Go formatting.
`make test` includes the Go coverage gate and integration suites.
`make test-embedded-store` checks the public adapter with real SQLite storage.
The deployment guide explains the Gateway checks.

### Protobuf regeneration

For protobuf changes, install `protoc`, `protoc-gen-go`, and `protoc-gen-go-grpc`.
Run this command from the repository root:

```bash
protoc \
  --go_out=. --go-grpc_out=. \
  --go_opt=paths=source_relative --go-grpc_opt=paths=source_relative \
  api/credit/v1/credit.proto
```
