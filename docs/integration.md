## Ledger Integration Guide

This document explains how consumers can adopt the ledger either as a standalone gRPC service (`ledgerd`) or by embedding the Go library exposed from `pkg/ledger`.

### 1. Running the gRPC microservice

1. Build or download the `ledgerd` binary (`go build ./cmd/credit`).
2. Provide a database via `DATABASE_URL` (`sqlite:///...` or `postgres://...`) and an optional `GRPC_LISTEN_ADDR` (defaults to `:50051`):

```bash
DATABASE_URL=sqlite:///tmp/ledger.db GRPC_LISTEN_ADDR=:50051 ./ledgerd
```

The server automatically prepares the schema, listens for gRPC requests, and logs every RPC (method, duration, code, user_id when present). Integration steps for any language:

* Generate gRPC stubs from `api/credit/v1/credit.proto`.
* Call the relevant RPCs (`Grant`, `Spend`, `Reserve`, `ListEntries`, etc.) using the user identifier that represents your account in the ledger.
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
* Validation happens at the edge: construct `ledger.UserID`, `ledger.IdempotencyKey`, `ledger.AmountCents`, etc., before invoking the service.

When embedding, reuse your existing application database and transaction management. Because the ledger code does not spawn goroutines or hold globals, you can scope it per request or as a singleton.

### 3. Choosing an approach

| Scenario                              | Recommended path                        |
|--------------------------------------|-----------------------------------------|
| Multiple services / polyglot clients | Deploy `ledgerd` and call it via gRPC.  |
| Single Go service needs credits      | Embed `pkg/ledger` + `gormstore`/`pgstore`. |
| Need both                            | Host `ledgerd` for general use; embed the library in Go components that need tighter coupling. |

Both approaches share the same domain code, ensuring identical business rules regardless of deployment style.
