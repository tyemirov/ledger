# Credit Service

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
 [Credit Service]  <--->  PostgreSQL
```

* `internal/credit` – core domain logic (ledger)
* `internal/store/pgstore` – PostgreSQL implementation of `credit.Store`
* `internal/grpcserver` – gRPC API bindings
* `api/credit/v1` – protobuf definitions

---

## Requirements

* Go 1.21+
* PostgreSQL 13+
* `protoc` with Go plugins (`protoc-gen-go`, `protoc-gen-go-grpc`)

---

## Installation

Clone the repository:

```bash
git clone https://github.com/yourorg/creditsvc.git
cd creditsvc
```

Install dependencies:

```bash
go mod tidy
```

Run migrations:

```bash
psql -h localhost -U postgres -d credit -f db/migrations.sql
```

Generate gRPC code (if you modify `.proto` files):

```bash
protoc --go_out=. --go-grpc_out=. api/credit/v1/credit.proto
```

---

## Configuration

Environment variables:

| Variable           | Default                                                              | Description                  |
| ------------------ | -------------------------------------------------------------------- | ---------------------------- |
| `DATABASE_URL`     | `postgres://postgres:postgres@localhost:5432/credit?sslmode=disable` | PostgreSQL connection string |
| `GRPC_LISTEN_ADDR` | `:7000`                                                              | gRPC server listen address   |

---

## Running the service

```bash
DATABASE_URL=postgres://postgres:postgres@localhost:5432/credit?sslmode=disable \
GRPC_LISTEN_ADDR=:7000 \
go run ./cmd/creditd
```

---

## Usage

Below are example calls using [`grpcurl`](https://github.com/fullstorydev/grpcurl).

### Check balance

```bash
grpcurl -plaintext -d '{"user_id":"user123"}' localhost:7000 credit.v1.CreditService/GetBalance
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
  "user_id":"user123",
  "amount_cents": 1000,
  "idempotency_key":"grant-1",
  "expires_at_unix_utc":0,
  "metadata_json":"{\"reason\":\"signup_bonus\"}"
}' localhost:7000 credit.v1.CreditService/Grant
```

### Reserve credit

```bash
grpcurl -plaintext -d '{
  "user_id":"user123",
  "amount_cents": 500,
  "reservation_id":"order-555",
  "idempotency_key":"reserve-1",
  "metadata_json":"{\"order_id\":555}"
}' localhost:7000 credit.v1.CreditService/Reserve
```

### Capture reservation

```bash
grpcurl -plaintext -d '{
  "user_id":"user123",
  "reservation_id":"order-555",
  "idempotency_key":"capture-1",
  "amount_cents":500,
  "metadata_json":"{\"order_id\":555}"
}' localhost:7000 credit.v1.CreditService/Capture
```

### Release reservation

```bash
grpcurl -plaintext -d '{
  "user_id":"user123",
  "reservation_id":"order-555",
  "idempotency_key":"release-1",
  "metadata_json":"{\"order_id\":555}"
}' localhost:7000 credit.v1.CreditService/Release
```

### Spend without reservation

```bash
grpcurl -plaintext -d '{
  "user_id":"user123",
  "amount_cents": 200,
  "idempotency_key":"spend-1",
  "metadata_json":"{\"action\":\"purchase\"}"
}' localhost:7000 credit.v1.CreditService/Spend
```

### List ledger entries

```bash
grpcurl -plaintext -d '{
  "user_id":"user123",
  "before_unix_utc": 1893456000,
  "limit": 20
}' localhost:7000 credit.v1.CreditService/ListEntries
```

---

## Development

Use the provided `Makefile` targets for local tooling:

```bash
make fmt   # verifies gofmt formatting
make lint  # runs go vet, staticcheck, and ineffassign
make test  # executes go test with >=80% coverage enforcement for internal packages
make ci    # runs fmt + lint + test
```

Docker Compose reads configuration from `.env.creditsvc`, so the container runtime matches the CLI flag/environment setup.

---

## Notes

* **Amounts** are stored as integer cents to avoid floating point errors.
* **Idempotency keys** must be unique per account for each logical operation.
  Use UUIDs or other request-unique identifiers.
* The service never overwrites balances — everything is computed from ledger entries.

---

## License

MIT — See `LICENSE` file.
