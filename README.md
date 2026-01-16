# Ledger Service

A standalone **gRPC-based virtual credits ledger** written in Go.
Provides core operations for granting, reserving, spending, capturing, and releasing virtual currency (e.g., promotional credits, in-app balances).

The service implements an **append-only ledger** with full auditability and idempotency protections.
It is intentionally **application-agnostic** — you decide when and why credits are earned or spent, this service only enforces the accounting.

---

## Features

* Append-only ledger with immutable entries
* Atomic operations using PostgreSQL transactions
* Idempotency keys to make operations safe to retry
* Holds/reservations with later capture/release
* Expiration support for promotional credits
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
* `internal/store/pgstore` – PostgreSQL implementation of `credit.Store`
* `internal/grpcserver` – gRPC API bindings
* `api/credit/v1` – protobuf definitions

### Network exposure and auth

The ledger gRPC server does not implement end-user authentication. Deploy it on a private interface (loopback/cluster-internal) and front it with an HTTP gateway that performs session validation and enforces request rules. In Compose/Kubernetes, point the gateway at `ledger:50051`/`localhost:50051` on the internal network and expose only the gateway externally. Add mTLS or a JWT-validating interceptor at the gRPC layer only if future topologies require crossing trust boundaries.

### Library vs. service

You can run the hosted service (`cmd/credit`) or embed the domain logic via `pkg/ledger`. See `docs/integration.md` for end-to-end guidance on both integration styles.

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

When targeting PostgreSQL, run the SQL migrations:

```bash
psql -h localhost -U postgres -d credit -f db/migrations.sql
```
(SQLite creates its schema automatically on startup.)

Generate gRPC code (if you modify `.proto` files):

```bash
protoc --go_out=. --go-grpc_out=. api/credit/v1/credit.proto
```

---

## Configuration

Environment variables:

| Variable           | Default                                                              | Description                  |
| ------------------ | -------------------------------------------------------------------- | ---------------------------- |
| `DATABASE_URL`     | `sqlite:///tmp/ledger.db`                                             | Database connection string (supports `postgres://...` or `sqlite:///path.db`) |
| `GRPC_LISTEN_ADDR` | `:50051`                                                             | gRPC server listen address   |

---

## Running the service

```bash
DATABASE_URL=postgres://postgres:postgres@localhost:5432/credit?sslmode=disable \
GRPC_LISTEN_ADDR=:50051 \
go run ./cmd/credit
```
To build a standalone binary named `ledgerd`:

```bash
go build -o ledgerd ./cmd/credit
./ledgerd
```

---

## Usage

Below are example calls using [`grpcurl`](https://github.com/fullstorydev/grpcurl).

### Check balance

```bash
grpcurl -plaintext -d '{"tenant_id":"default","user_id":"user123","ledger_id":"default"}' localhost:50051 credit.v1.CreditService/GetBalance
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
grpcurl -plaintext -d '{
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
grpcurl -plaintext -d '{
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
grpcurl -plaintext -d '{
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
grpcurl -plaintext -d '{
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
grpcurl -plaintext -d '{
  "tenant_id":"default",
  "user_id":"user123",
  "ledger_id":"default",
  "amount_cents": 200,
  "idempotency_key":"spend-1",
  "metadata_json":"{\"action\":\"purchase\"}"
}' localhost:50051 credit.v1.CreditService/Spend
```

### List ledger entries

```bash
grpcurl -plaintext -d '{
  "tenant_id":"default",
  "user_id":"user123",
  "ledger_id":"default",
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
make test  # executes go test with >=95% coverage enforcement for internal packages
make ci    # runs fmt + lint + test
```

Docker Compose reads configuration from `.env.ledger`, so the container runtime matches the CLI flag/environment setup.

---

## Database Selection

The service now runs on SQLite by default (file path via `DATABASE_URL=sqlite:///...`). To use PostgreSQL instead, set `DATABASE_URL` to a Postgres DSN (for example `postgres://...`) and run the SQL migrations in `db/migrations.sql`. The CLI automatically chooses the correct GORM driver based on the URL scheme.

---

## Demo Application

All demo assets (UI, Docker compose, optional backend) live under `demo/`. The ledger service code remains agnostic of the demo; see `demo/README.md` inside that folder for usage.

---

## Notes

* **Amounts** are stored as integer cents to avoid floating point errors.
* **Idempotency keys** must be unique per account for each logical operation.
  Use UUIDs or other request-unique identifiers.
* The service never overwrites balances — everything is computed from ledger entries.

---

## License

MIT — See `LICENSE` file.
