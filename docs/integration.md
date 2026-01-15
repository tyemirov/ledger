## Ledger Integration Guide

This document explains how consumers can adopt the ledger either as a standalone gRPC service (`ledgerd`) or by embedding the Go library exposed from `pkg/ledger`.

### 1. Running the gRPC microservice

1. Build or download the `ledgerd` binary (`go build ./cmd/credit`).
2. Provide a database via `DATABASE_URL` (`sqlite:///...` or `postgres://...`) and an optional `GRPC_LISTEN_ADDR` (defaults to `:50051`):

```bash
DATABASE_URL=sqlite:///tmp/ledger.db GRPC_LISTEN_ADDR=:50051 ./ledgerd
```

SQLite databases are created automatically. For Postgres, apply the schema first:

```bash
psql -h localhost -U postgres -d credit -f db/migrations.sql
```

The server prepares the schema, listens for gRPC requests, and logs every RPC (method, duration, code, user_id when present). Deploy the gRPC port on a private interface or internal network, then front it with your gateway for authentication and rate limiting. Integration steps for any language:

* Generate gRPC stubs from `api/credit/v1/credit.proto`.
* Call the relevant RPCs (`Grant`, `Spend`, `Reserve`, `ListEntries`, etc.) using both the `user_id` and `ledger_id` that identify the account in the ledger.
* Enforce authentication/authorization in your gateway; the ledger service trusts whatever `user_id` you provide.

See `README.md` for Docker Compose examples that pair `ledgerd` with demo applications.

### 2. Embedding the Go library

`pkg/ledger` exposes the domain model so Go services can host the ledger in-process without gRPC overhead.

Key components:

```go
import (
    "github.com/MarkoPoloResearchLab/ledger/pkg/ledger"
    "github.com/MarkoPoloResearchLab/ledger/internal/store/gormstore"
)

func newLedgerService(db *gorm.DB, clock func() int64) (*ledger.Service, error) {
    store := gormstore.New(db)
    return ledger.NewService(store, clock)
}
```

* `ledger.Service` defines operations (`Grant`, `Spend`, `Reserve`, `Capture`, `Release`, `Balance`, `ListEntries`).
* `ledger.Store` is the storage interface. Use `internal/store/gormstore` for GORM-backed projects or `internal/store/pgstore` for pgx pools. Custom stores can satisfy the interface to target other databases.
* Validation happens at the edge: construct `ledger.UserID`, `ledger.LedgerID`, `ledger.PositiveAmountCents`, `ledger.ReservationID`, `ledger.IdempotencyKey`, and `ledger.MetadataJSON` before invoking the service.
* Store implementations consume `ledger.EntryInput` values and return `ledger.Entry` records; use the smart constructors (`NewEntryInput`, `NewEntry`, `NewReservation`) to enforce invariants.

When embedding, reuse your existing application database and transaction management. Because the ledger code does not spawn goroutines or hold globals, you can scope it per request or as a singleton.

Example edge construction:

```go
userID, err := ledger.NewUserID(request.UserId)
ledgerID, err := ledger.NewLedgerID(request.LedgerId)
amount, err := ledger.NewPositiveAmountCents(request.AmountCents)
reservationID, err := ledger.NewReservationID(request.ReservationId)
idempotencyKey, err := ledger.NewIdempotencyKey(request.IdempotencyKey)
metadata, err := ledger.NewMetadataJSON(request.MetadataJson)
```

The service methods expect these domain types and will return domain errors if business rules are violated.

### 3. Error contracts

The ledger returns sentinel errors you can match with `errors.Is`:

* `ErrInvalidUserID`, `ErrInvalidLedgerID`, `ErrInvalidReservationID`, `ErrInvalidIdempotencyKey`, `ErrInvalidAmountCents`, `ErrInvalidMetadataJSON` for input validation failures.
* `ErrInsufficientFunds` when a spend or reserve would overdraw the account.
* `ErrDuplicateIdempotencyKey` when a request reuses an idempotency key.
* `ErrReservationExists`, `ErrUnknownReservation`, `ErrReservationClosed` for reservation state issues.

Store adapters wrap errors with `ledger.OperationError` so downstream callers can log stable codes (`operation.subject.code`).
At the gRPC boundary, these map to standard gRPC codes (InvalidArgument, AlreadyExists, FailedPrecondition, NotFound).

### 4. Choosing an approach

| Scenario                              | Recommended path                        |
|--------------------------------------|-----------------------------------------|
| Multiple services / polyglot clients | Deploy `ledgerd` and call it via gRPC.  |
| Single Go service needs credits      | Embed `pkg/ledger` + `gormstore`/`pgstore`. |
| Need both                            | Host `ledgerd` for general use; embed the library in Go components that need tighter coupling. |

Both approaches share the same domain code, ensuring identical business rules regardless of deployment style.
