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
* Audit-friendly — no balance overwrites, all changes are recorded

---

## Architecture

```
[Your App / Web API]
        |
        |  gRPC
        v
 [Ledger Service]  <--->  PostgreSQL
```

* `pkg/ledger` – core domain logic (ledger) reusable as a Go module
* `internal/store/gormstore` – database-backed implementation of `ledger.Store` (SQLite/PostgreSQL via GORM)
* `internal/grpcserver` – gRPC API bindings
* `api/credit/v1` – protobuf definitions

### Authentication

Every gRPC request must include an `authorization` metadata header carrying the per-tenant Bearer token:

```
authorization: Bearer <tenant_secret_key>
```

The server extracts `tenant_id` from the request body, looks up the matching `secret_key` in `config.yml`, and verifies the token. Requests that fail authentication receive a gRPC `Unauthenticated` code; requests for an unknown tenant receive `PermissionDenied`.

Deploy the gRPC port on a private interface or cluster-internal network and front it with an HTTP gateway for end-user session validation. The per-tenant secret key authenticates **service-to-service** traffic between your gateway and the ledger.

### Library vs. service

You can run the hosted service (`cmd/credit`) or embed the domain logic via `pkg/ledger`.
See:

* `docs/integration.md` for end-to-end guidance on both integration styles.
* `docs/api.md` for an RPC-by-RPC reference (including idempotency, refunds, batch semantics, and reservation TTLs).

---

## Requirements

* Go 1.25+
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
  listen_addr: "${GRPC_LISTEN_ADDR:-:50051}"

tenants:
  - id: "default"
    name: "Default Tenant"
    secret_key: "${DEFAULT_TENANT_SECRET:-default-secret}"
  - id: "demo"
    name: "Demo Tenant"
    secret_key: "${DEMO_TENANT_SECRET:-demo-secret}"
```

Each tenant requires a non-empty `id` and `secret_key`. Clients must send the matching secret as a Bearer token in the `authorization` gRPC metadata header (see [Authentication](#authentication)).

Environment variables:

| Variable                | Default                   | Description                                                    |
| ----------------------- | ------------------------- | -------------------------------------------------------------- |
| `DATABASE_URL`          | `sqlite:///tmp/ledger.db` | Database connection string (`postgres://...` or `sqlite:///…`) |
| `GRPC_LISTEN_ADDR`      | `:50051`                  | gRPC server listen address                                     |
| `DEFAULT_TENANT_SECRET` | `default-secret`          | Bearer token for the `default` tenant                          |
| `DEMO_TENANT_SECRET`    | `demo-secret`             | Bearer token for the `demo` tenant                             |

---

## Running the service

```bash
DATABASE_URL=postgres://postgres:postgres@localhost:5432/credit?sslmode=disable \
GRPC_LISTEN_ADDR=:50051 \
DEFAULT_TENANT_SECRET=my-secret \
go run ./cmd/credit
```
To build a standalone binary named `ledgerd`:

```bash
go build -o ledgerd ./cmd/credit
DEFAULT_TENANT_SECRET=my-secret ./ledgerd
```

---

## Usage

Below are example calls using [`grpcurl`](https://github.com/fullstorydev/grpcurl).

Mutation RPCs return `entry_id` + `created_unix_utc` so clients can correlate requests with the persisted ledger entry without an extra `ListEntries` round-trip.

### Check balance

```bash
grpcurl -plaintext \
  -H 'authorization: Bearer default-secret' \
  -d '{"tenant_id":"default","user_id":"user123","ledger_id":"default"}' \
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
  -H 'authorization: Bearer default-secret' \
  -d '{
    "tenant_id":"default",
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
  -H 'authorization: Bearer default-secret' \
  -d '{
    "tenant_id":"default",
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
  -H 'authorization: Bearer default-secret' \
  -d '{
    "tenant_id":"default",
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
  -H 'authorization: Bearer default-secret' \
  -d '{
    "tenant_id":"default",
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
  -H 'authorization: Bearer default-secret' \
  -d '{
    "tenant_id":"default",
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
  -H 'authorization: Bearer default-secret' \
  -d '{
    "tenant_id":"default",
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
  -H 'authorization: Bearer default-secret' \
  -d '{
    "account": { "tenant_id":"default", "user_id":"user123", "ledger_id":"default" },
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
  -H 'authorization: Bearer default-secret' \
  -d '{
    "tenant_id":"default",
    "user_id":"user123",
    "ledger_id":"default",
    "reservation_id":"order-555"
  }' localhost:50051 credit.v1.CreditService/GetReservation
```

### List reservations

```bash
grpcurl -plaintext \
  -H 'authorization: Bearer default-secret' \
  -d '{
    "tenant_id":"default",
    "user_id":"user123",
    "ledger_id":"default",
    "limit": 20,
    "statuses": ["active", "captured", "released"]
  }' localhost:50051 credit.v1.CreditService/ListReservations
```

### List ledger entries

```bash
grpcurl -plaintext \
  -H 'authorization: Bearer default-secret' \
  -d '{
    "tenant_id":"default",
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
make lint  # runs go vet, staticcheck, and ineffassign
make test  # executes go test with 100% coverage enforcement
make ci    # runs fmt + lint + test
```

Docker Compose reads configuration from `.env.ledger`, so the container runtime matches the CLI flag/environment setup.

---

## Database Selection

The CLI defaults to SQLite when `DATABASE_URL` is not set (file path via `DATABASE_URL=sqlite:///...`). The provided Docker Compose stack provisions PostgreSQL by default; `ledgerd` applies its schema automatically via GORM on startup.

To run against Postgres outside Compose, set `DATABASE_URL` to a Postgres DSN (for example `postgres://...`) and ensure the database exists. The server chooses the correct GORM driver based on the URL scheme.

---

## Demo Application

All demo assets (UI, Docker compose, optional backend) live under `demo/`. The ledger service code remains agnostic of the demo; see `demo/README.md` inside that folder for usage.

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
