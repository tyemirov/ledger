# Ledger Service

A standalone **gRPC-based virtual credits ledger** written in Go.
Provides core operations for granting, reserving, spending, capturing, releasing, and refunding virtual currency, plus high-volume batch mutation APIs.

The service implements an **append-only ledger** with full auditability and idempotency protections.
It is intentionally **application-agnostic** — you decide when and why credits are earned or spent, this service only enforces the accounting.

---

## Features

* Append-only ledger with immutable entries
* Atomic operations using PostgreSQL transactions
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
Browser --> mpr-ui and TAuth --> Ledger HTTP control plane
                                      |
Application client --> private gRPC --+--> PostgreSQL or SQLite
```

* `pkg/ledger` – core domain logic (ledger) reusable as a Go module
* `internal/store/gormstore` – database-backed implementation of `ledger.Store` (SQLite/PostgreSQL via GORM)
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

When targeting PostgreSQL, ensure the database exists and set `DATABASE_URL` accordingly.
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
  jwt_issuer: "${TAUTH_JWT_ISSUER}"
  tauth_tenant_id: "${TAUTH_TENANT_ID}"
  session_cookie_name: "${TAUTH_SESSION_COOKIE_NAME}"
  public_origin: "${LEDGER_PUBLIC_ORIGIN}"

ui:
  description: "Ledger"
  tauth_url: "${TAUTH_URL}"
  google_client_id: "${TAUTH_GOOGLE_CLIENT_ID}"
  login_path: "${TAUTH_LOGIN_PATH}"
  logout_path: "${TAUTH_LOGOUT_PATH}"
  nonce_path: "${TAUTH_NONCE_PATH}"
  session_path: "${TAUTH_SESSION_PATH}"
```

Static tenants and plaintext configuration secrets are not supported. An authenticated person provisions one UserAccount, creates named tenants, and creates or revokes each tenant credential through the HTTP control plane.

Environment variables:

The committed configuration requires `DATABASE_URL`, `LEDGER_PUBLIC_ORIGIN`, `TAUTH_GOOGLE_CLIENT_ID`, `TAUTH_JWT_ISSUER`, `TAUTH_JWT_SIGNING_KEY`, `TAUTH_LOGIN_PATH`, `TAUTH_LOGOUT_PATH`, `TAUTH_NONCE_PATH`, `TAUTH_SESSION_COOKIE_NAME`, `TAUTH_SESSION_PATH`, `TAUTH_TENANT_ID`, and `TAUTH_URL`. Listener addresses are explicit configuration fields.

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

All three commands delegate to the exact sibling `../mprlab-gateway`. Ledger
declares its Go container and a new retained `ledger-data` volume.
The manifest retires the legacy `mprlab-nginx-gateway/ledger-api` service.
It also declares the non-secret configuration and the `ledger.grpc` endpoint.
`.mprlab/deploy/resources.yml` is the only tracked production declaration. That manifest
uses the permanent versionless contract and contains the SemVer release policy. Its one service
declares singular gateway placement and binds the database plus the required TAuth, browser, and public-origin values through one typed `private_values` resource. The exact values live only in the ignored mode-0600
`.mprlab/deploy/.env` input, which is excluded from the Ledger Docker build
context and read only by deployment. Release and publication do not read it.
The legacy volume remains untouched. The gateway owns release sealing,
immutable publication, deployment convergence, and retry verification. Ledger
has no Node lifecycle dependency.

```bash
go run ./cmd/credit --config configs/config.ledger.yml
```
To build a standalone binary named `ledgerd`:

```bash
go build -o ledgerd ./cmd/credit
./ledgerd --config configs/config.ledger.yml
```

### Forward-only data migration

An existing database with the legacy `accounts` table does not start. Run the bounded migration once with an explicit mode-0600 YAML mapping:

```bash
ledgerd --config configs/config.ledger.yml migrate-user-accounts --mapping /private/ledger-user-account-mapping.yml
```

The mapping declares the complete legacy tenant set, each canonical tenant UUID and name, its exact TAuth owner identity, and one canonical `ledger_<credential_uuid>_<secret>` credential. The command validates the complete mapping before mutation, retains accounting history, renames `accounts` to `ledger_accounts`, and rejects a rerun.

---

## Usage

Below are example calls using [`grpcurl`](https://github.com/fullstorydev/grpcurl).

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

Refunds are first-class entries linked to an original debit entry; the ledger enforces that refunds cannot exceed the original debit amount.

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
```

Docker Compose reads configuration from `.env.ledger`, so the container runtime matches the CLI flag/environment setup.

---

## Database Selection

The CLI defaults to SQLite when `DATABASE_URL` is not set (file path via `DATABASE_URL=sqlite:///...`). The provided Docker Compose stack runs SQLite by default using the `DATABASE_URL` in `.env.ledger`.

To run against Postgres outside Compose, set `DATABASE_URL` to a Postgres DSN (for example `postgres://...`) and ensure the database exists. The server chooses the correct GORM driver based on the URL scheme.

---

## Local Workspace

The `demo/` composition runs Ledger, TAuth, and one same-origin proxy. Ledger serves the same embedded workspace that the production binary serves. See `demo/README.md` for the required private local config and profile commands.

---

## Notes

* **Amounts** are stored as integer cents to avoid floating point errors.
  - `spend` entries store debits as negative `amount_cents`; refunds/grants are positive.
* **Idempotency keys** must be unique per account for each logical operation.
  Use UUIDs or other request-unique identifiers.
  - If your client treats `duplicate_idempotency_key` as a no-op success, strongly namespace keys by operation to avoid collisions across entry types.
* The service never overwrites balances — everything is computed from ledger entries.
* For **permanent credits**, set `expires_at_unix_utc` to `0`. Use expiry only for explicitly time-limited promotions.

---

## License

MIT — See `LICENSE` file.
