# Ledger Service

A standalone **gRPC-based virtual credits ledger** written in Go.
Provides core operations for granting, reserving, spending, capturing, releasing, and refunding virtual currency, plus high-volume batch mutation APIs.

The service implements an **append-only ledger** with full auditability and idempotency protections.
It is intentionally **application-agnostic** — you decide when and why credits are earned or spent, this service only enforces the accounting.

---

## Features

* Append-only ledger with immutable entries
* Atomic operations with PostgreSQL transactions
* Idempotency keys to make operations safe to retry
* Holds/reservations with later capture/release
* Expiration support for promotional credits
* First-class refunds referencing debit entries (enforces refund <= debit)
* Batch gRPC operations for high-volume mutation (atomic or best-effort)
* Reservation introspection APIs (GetReservation / ListReservations)
* ListEntries filtering (types / reservation_id / idempotency_key_prefix)
* gRPC API for integration from any language
* Authenticated browser workspace for UserAccount, tenant, and credential management
* Owner-scoped Ledger tenants with no application tenant-count limit
* One-time, revocable tenant credentials for application clients
* Audit-friendly — no balance overwrites, all changes are recorded

---

## Architecture

```text
Browser --> GitHub Pages frontend
   |
   | credentialed HTTPS
   v
ledger-api gateway --> mpr-ui and TAuth + Ledger HTTP control plane
                                              |
Application client --------> private gRPC ----+--> PostgreSQL or SQLite
```

* `pkg/ledger` – core domain logic (ledger) reusable as a Go module
* `pkg/gormstore` – database-backed implementation of `ledger.Store` (SQLite/PostgreSQL via GORM)
* `internal/grpcserver` – gRPC API bindings
* `internal/controlplane` – authenticated HTTP resources and the browser workspace
* `internal/useraccount` – UserAccount and Ledger tenant ownership rules
* `api/credit/v1` – protobuf definitions

### Authentication

Every gRPC request must include an `authorization` metadata header carrying the per-tenant Bearer token:

```
authorization: Bearer <tenant_credential>
```

The server extracts `tenant_id` from the request body and verifies a stored, revocable credential digest. The credential must belong to the addressed tenant. The same rule applies to the nested account in `Batch`.

Deploy the gRPC port on a private interface or cluster-internal network. A UserAccount owner creates tenant credentials through the authenticated HTTP control plane. The control plane validates the TAuth session on every protected request.

### Library vs. service

You can run the hosted service (`cmd/credit`) or embed the domain logic via `pkg/ledger`.
See:

* `docs/integration.md` for end-to-end guidance on both integration styles.
* `docs/api.md` for an RPC-by-RPC reference (including idempotency, refunds, batch semantics, and reservation TTLs).

---

## Requirements

* Go 1.26+
* SQLite (default file-based runtime) or PostgreSQL 13+ if you supply a Postgres `DATABASE_URL`
* `protoc` with Go plugins (`protoc-gen-go`, `protoc-gen-go-grpc`)

---

## Installation

Clone the repository:

```bash
git clone https://github.com/MarkoPoloResearchLab/ledger.git
cd ledger
```

Install dependencies:

```bash
go mod tidy
```

When you use PostgreSQL, make sure that the database exists and set `DATABASE_URL`.
The service applies its schema automatically via GORM on startup (same as SQLite).

Generate gRPC code (if you modify `.proto` files):

```bash
protoc \\
  --go_out=. --go-grpc_out=. \\
  --go_opt=paths=source_relative --go-grpc_opt=paths=source_relative \\
  api/credit/v1/credit.proto
```

---

## Configuration

The service reads `config.yml` (override with `--config <path>`). Environment variables in the YAML are expanded at startup.

```yaml
service:
  database_url: "${DATABASE_URL:-sqlite:///tmp/ledger.db}"
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

Static tenants and plaintext configuration secrets are not supported. An authenticated person provisions one UserAccount, creates named tenants, and creates or revokes each tenant credential through the HTTP control plane.

Environment variables:

The runtime configuration accepts `DATABASE_URL`, `LEDGER_PUBLIC_ORIGIN`, `TAUTH_GOOGLE_CLIENT_ID`, `TAUTH_JWT_SIGNING_KEY`, `TAUTH_SESSION_COOKIE_NAME`, `TAUTH_TENANT_ID`, and `TAUTH_URL`. The production manifest supplies the fixed public origins as tracked values. It gets the TAuth tenant ID, cookie name, Google client ID, and signing key from its `tauth_tenant` resource. Listener addresses and TAuth paths are explicit configuration fields.

### HTTP control plane

The HTTP listener serves the browser workspace at `/`, its checked assets under `/assets/ledger/`, the public browser authentication config at `/config-ui.yaml`, and `GET /healthz`.

The listener also exposes these TAuth-protected resources:

- `PUT` and `GET /api/user-account`.
- `POST` and `GET /api/tenants`.
- `GET /api/tenants/{tenant_id}`.
- `POST` and `GET /api/tenants/{tenant_id}/credentials`.
- `DELETE /api/tenants/{tenant_id}/credentials/{credential_id}`.

The workspace requests a protected resource only after `mpr-ui` reports an authenticated TAuth session. Unsafe requests require the exact configured `Origin` and `X-Ledger-CSRF: 1`. Tenant and credential creation also require `Idempotency-Key`. A new credential secret appears only in its successful creation response.

---

## Running the service

Production artifacts follow the repository lifecycle:

```bash
make release
make publish
make deploy
```

All three commands use the installed `mprlab-gateway` runtime.
Each command passes the application Git root through `--app-root`.
Make sure that `mprlab-gateway` is on `PATH`.
Use `MPRLAB_GATEWAY_EXECUTABLE` to select an explicit installed command path.
Keep operator inventory and private config under `MPRLAB_GATEWAY_OPERATOR_ROOT`.
The default operator root is `$HOME/.config/mprlab-gateway`.

Use `make test-installed-gateway` for the Make wrapper integration checks.
Use `make test-gateway-plan` to validate the current tracked source through the installed Gateway planner.
This check commits a disposable Git fixture and plans its release without publication or deployment.

Ledger declares its Go container and a new retained `ledger-data` volume.
The manifest retires the legacy `mprlab-nginx-gateway/ledger-api` service.
It also declares the non-secret configuration, `ledger.grpc`, and `ledger.http` capabilities.
`.mprlab/deploy/resources.yml` is the only tracked production declaration. That manifest
uses the permanent versionless contract and contains the SemVer release policy. Its one service
declares singular gateway placement and binds the database plus the required TAuth, browser, and public-origin values through typed resources. The exact private values live only in the ignored mode-0600
`.mprlab/deploy/.env` input. The Docker build excludes this file. Only deployment reads it.
Release and publication do not read it.
The legacy volume remains untouched. The gateway owns release sealing,
immutable publication, deployment convergence, and retry verification. Ledger
has no Node lifecycle dependency.

### Hosted profile

The hosted application uses two origins:

- `https://ledger.mprlab.com` is the immutable GitHub Pages frontend.
- `https://ledger-api.mprlab.com` is the Caddy-routed API and authentication origin.

The API route sends `/auth` and `/me` to `tauth.http`. It sends `/api`, `/config-ui.yaml`, and `/healthz` to `ledger.http`. The gRPC capability stays private. The public health check is `https://ledger-api.mprlab.com/healthz`.

The route provisions the TAuth tenant `ledger`. It uses `ledger_session` and `ledger_refresh` cookies with the `ledger-api.mprlab.com` domain. The browser sends credentialed requests to the API origin. Ledger and TAuth allow only the exact frontend origin.

The browser exchanges the Google Identity Services credential at `/auth/google` on the API origin. It does not use a Google redirect callback. Google web client `611549676198-d8800qv64voofseor1qod1euto5duivu.apps.googleusercontent.com` must authorize `https://ledger.mprlab.com` as a JavaScript origin.

The ignored production input must define `DATABASE_URL`, `TAUTH_GOOGLE_CLIENT_ID`, and `TAUTH_JWT_SIGNING_KEY`. The manifest supplies `LEDGER_PUBLIC_ORIGIN` as `https://ledger.mprlab.com`. It supplies `TAUTH_URL` as `https://ledger-api.mprlab.com`.

```bash
go run ./cmd/credit --config configs/config.ledger.yml
```
To build a standalone binary named `ledgerd`:

```bash
go build -o ledgerd ./cmd/credit
./ledgerd --config configs/config.ledger.yml
```

### Current database contract

Ledger accepts only the canonical UserAccount schema. It rejects an obsolete `accounts` table without mutation.
The one-time production migration completed on September 15, 2026. The migration command is removed.

---

## Usage

Below are example calls with [`grpcurl`](https://github.com/fullstorydev/grpcurl).

Mutation RPCs return `entry_id` + `created_unix_utc` so clients can correlate requests with the persisted ledger entry without an extra `ListEntries` round-trip.

### Check balance

```bash
grpcurl -plaintext \
  -H 'authorization: Bearer <tenant_credential>' \
  -d '{"tenant_id":"0196f0ec-3e80-7a54-bd2b-56cfe90bf801","user_id":"user123","ledger_id":"default"}' \
  localhost:50051 credit.v1.CreditService/GetBalance
```

Response:

```json
{
  "total_cents": 1000,
  "available_cents": 1000
}
```

### Grant credit

```bash
grpcurl -plaintext \
  -H 'authorization: Bearer <tenant_credential>' \
  -d '{
    "tenant_id":"0196f0ec-3e80-7a54-bd2b-56cfe90bf801",
    "user_id":"user123",
    "ledger_id":"default",
    "amount_cents": 1000,
    "idempotency_key":"grant-1",
    "expires_at_unix_utc":0,
    "metadata_json":"{\"reason\":\"signup_bonus\"}"
  }' localhost:50051 credit.v1.CreditService/Grant
```

### Reserve credit

```bash
grpcurl -plaintext \
  -H 'authorization: Bearer <tenant_credential>' \
  -d '{
    "tenant_id":"0196f0ec-3e80-7a54-bd2b-56cfe90bf801",
    "user_id":"user123",
    "ledger_id":"default",
    "amount_cents": 500,
    "reservation_id":"order-555",
    "idempotency_key":"reserve-1",
    "metadata_json":"{\"order_id\":555}"
  }' localhost:50051 credit.v1.CreditService/Reserve
```

### Capture reservation

```bash
grpcurl -plaintext \
  -H 'authorization: Bearer <tenant_credential>' \
  -d '{
    "tenant_id":"0196f0ec-3e80-7a54-bd2b-56cfe90bf801",
    "user_id":"user123",
    "ledger_id":"default",
    "reservation_id":"order-555",
    "idempotency_key":"capture-1",
    "amount_cents":500,
    "metadata_json":"{\"order_id\":555}"
  }' localhost:50051 credit.v1.CreditService/Capture
```

### Release reservation

```bash
grpcurl -plaintext \
  -H 'authorization: Bearer <tenant_credential>' \
  -d '{
    "tenant_id":"0196f0ec-3e80-7a54-bd2b-56cfe90bf801",
    "user_id":"user123",
    "ledger_id":"default",
    "reservation_id":"order-555",
    "idempotency_key":"release-1",
    "metadata_json":"{\"order_id\":555}"
  }' localhost:50051 credit.v1.CreditService/Release
```

### Spend without reservation

```bash
grpcurl -plaintext \
  -H 'authorization: Bearer <tenant_credential>' \
  -d '{
    "tenant_id":"0196f0ec-3e80-7a54-bd2b-56cfe90bf801",
    "user_id":"user123",
    "ledger_id":"default",
    "amount_cents": 200,
    "idempotency_key":"spend-1",
    "metadata_json":"{\"action\":\"purchase\"}"
  }' localhost:50051 credit.v1.CreditService/Spend
```

### Refund a debit (spend/capture)

Refunds are first-class entries that link to an original debit entry. The ledger prevents refunds that exceed the original debit amount.

```bash
grpcurl -plaintext \
  -H 'authorization: Bearer <tenant_credential>' \
  -d '{
    "tenant_id":"0196f0ec-3e80-7a54-bd2b-56cfe90bf801",
    "user_id":"user123",
    "ledger_id":"default",
    "original_idempotency_key":"spend-1",
    "amount_cents": 50,
    "idempotency_key":"refund-1",
    "metadata_json":"{\"reason\":\"reimbursement\"}"
  }' localhost:50051 credit.v1.CreditService/Refund
```

### Batch operations (high volume)

Use `Batch` to execute many mutations for a single account in one request. Duplicates are surfaced per-item via `duplicate=true`.

```bash
grpcurl -plaintext \
  -H 'authorization: Bearer <tenant_credential>' \
  -d '{
    "account": { "tenant_id":"0196f0ec-3e80-7a54-bd2b-56cfe90bf801", "user_id":"user123", "ledger_id":"default" },
    "atomic": false,
    "operations": [
      {
        "operation_id": "refund-1",
        "refund": {
          "original_idempotency_key": "spend-1",
          "amount_cents": 50,
          "idempotency_key": "refund-1",
          "metadata_json": "{\"reason\":\"reimbursement\"}"
        }
      },
      {
        "operation_id": "refund-2",
        "refund": {
          "original_idempotency_key": "spend-1",
          "amount_cents": 25,
          "idempotency_key": "refund-2",
          "metadata_json": "{\"reason\":\"reimbursement\"}"
        }
      }
    ]
  }' localhost:50051 credit.v1.CreditService/Batch
```

### Get reservation state

```bash
grpcurl -plaintext \
  -H 'authorization: Bearer <tenant_credential>' \
  -d '{
    "tenant_id":"0196f0ec-3e80-7a54-bd2b-56cfe90bf801",
    "user_id":"user123",
    "ledger_id":"default",
    "reservation_id":"order-555"
  }' localhost:50051 credit.v1.CreditService/GetReservation
```

### List reservations

```bash
grpcurl -plaintext \
  -H 'authorization: Bearer <tenant_credential>' \
  -d '{
    "tenant_id":"0196f0ec-3e80-7a54-bd2b-56cfe90bf801",
    "user_id":"user123",
    "ledger_id":"default",
    "limit": 20,
    "statuses": ["active", "captured", "released"]
  }' localhost:50051 credit.v1.CreditService/ListReservations
```

### List ledger entries

```bash
grpcurl -plaintext \
  -H 'authorization: Bearer <tenant_credential>' \
  -d '{
    "tenant_id":"0196f0ec-3e80-7a54-bd2b-56cfe90bf801",
    "user_id":"user123",
    "ledger_id":"default",
    "types": ["refund", "spend", "grant"],
    "idempotency_key_prefix":"refund",
    "before_unix_utc": 1893456000,
    "limit": 20
  }' localhost:50051 credit.v1.CreditService/ListEntries
```

---

## Development

Use the provided `Makefile` targets for local tooling:

```bash
make fmt   # verifies gofmt formatting
make lint  # runs Go static checks and the checked browser-module compile
make test  # executes go test with 100% coverage enforcement
make ci    # runs format, lint, Go coverage, and browser acceptance checks
make up    # builds and starts the verified localhost runtime
make down  # stops the localhost runtime and preserves its data
```

---

## Database Selection

The CLI defaults to SQLite when `DATABASE_URL` is not set. Use `DATABASE_URL=sqlite:///...` to select a file path.

The local runtime uses SQLite volumes for Ledger and TAuth. The `make down` command preserves both volumes.

To use Postgres outside Compose, set `DATABASE_URL` to a Postgres DSN, for example `postgres://...`. Make sure that the database exists.

The server selects the correct GORM driver from the URL scheme.

---

## Local Workspace

Run `make up` from the repository root. The command builds Ledger from the current source and starts TAuth and the same-origin proxy.

Open `http://localhost:8000/` for the Ledger workspace. Use `localhost:50051` for a local gRPC client.

The command returns after it verifies the page, health route, browser config, TAuth session route, and protected control plane.

Run `make down` to stop the local runtime. See `demo/README.md` for the complete local contract.

---

## Notes

* **Amounts** are stored as integer cents to prevent floating point errors.
  - `spend` entries store debits as negative `amount_cents`. Refund and grant entries are positive.
* **Idempotency keys** must be unique per account for each logical operation.
  Use UUIDs or other request-unique identifiers.
  - If your client treats `duplicate_idempotency_key` as a no-op success, use operation namespaces to prevent key conflicts.
* The service never overwrites balances — everything is computed from ledger entries.
* For **permanent credits**, set `expires_at_unix_utc` to `0`. Use expiry only for explicitly time-limited promotions.

---

## License

MIT — See `LICENSE` file.
