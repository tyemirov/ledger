# Ledger gRPC API Reference

This document is the canonical reference for the Ledger gRPC surface (`credit.v1.CreditService`) and its behavioral contracts (idempotency, refunds, batch semantics, reservation TTLs, and introspection APIs).

For "how to run the service" or "how to embed the Go library", see the [integration guide](integration.md).

## Account Model

Every mutation and query is scoped to an **account** identified by:

- `tenant_id`
- `ledger_id`
- `user_id`

Idempotency keys are enforced **per account**.

## Authentication

Every gRPC request must include the `authorization` metadata header:

```
authorization: Bearer <tenant_credential>
```

The server extracts `tenant_id` from the request body and verifies the Bearer credential against the stored digest for that tenant. `Batch` uses `account.tenant_id`. Every handler also compares the authenticated tenant with the addressed tenant.

| Failure reason          | gRPC code          | Message                            |
| ----------------------- | ------------------ | ---------------------------------- |
| Missing `tenant_id`     | `Unauthenticated`  | `missing tenant_id`               |
| Tenant mismatch         | `PermissionDenied` | `tenant is not authorized`        |
| Missing metadata        | `Unauthenticated`  | `missing metadata`                |
| Missing `authorization` | `Unauthenticated`  | `missing authorization header`    |
| Wrong format            | `Unauthenticated`  | `invalid authorization header format` |
| Wrong secret            | `Unauthenticated`  | `invalid secret key`              |

An authenticated UserAccount owner creates and revokes credentials through the Ledger HTTP control plane. A credential secret is returned once. Ledger stores only its SHA-256 digest.

The authenticated browser workspace creates and revokes these credentials. The HTTP listener also serves `/config-ui.yaml` as the only browser authentication config. The HTTP control plane contract is defined in `api/control/v1/openapi.yaml`.

## Data Model

### Entries (append-only)

The ledger is append-only. Operations append immutable entries and balances are derived from the entry stream.

Entry types (`Entry.type`):

- `grant`: A credit with optional expiry through `expires_at_unix_utc`.
- `hold` (credit hold created by `Reserve`)
- `reverse_hold`: Releases a hold. `Release` or `Capture` emits this entry.
- `spend`: A debit stored as a **negative** `amount_cents`.
- `refund`: A credit linked to a prior debit. `Entry.refund_of_entry_id` identifies the original debit entry.

Notes:

- `Spend` and `Capture` both produce `spend` debit entries (negative `amount_cents`).
- `Reserve` produces a `hold` entry and a reservation record.
- `Release` produces a `reverse_hold` entry and finalizes the reservation as released.

### Reservations

Reservations model held funds that are later captured or released.

Reservation statuses:

- `active`
- `captured`
- `released`

Reservations may carry an optional TTL (`expires_at_unix_utc`). When a reservation expires while still `active`, its held funds no longer reduce `available_cents`, and captures are rejected.

## Idempotency

All mutating operations accept an `idempotency_key`. The ledger enforces uniqueness per account:

- Keys identify intended operations within an account. Callers must reuse a key only for the same intended operation.
- Duplicate handling depends on the operation and existing entry type. A reused key does not verify that the request payload matches.

gRPC behavior:

- `Grant` returns `AlreadyExists` with `duplicate_idempotency_key` when the key already exists.
- `Spend` validates available funds before the entry write. A retry can return `insufficient_funds` before detection of a duplicate key.
- `Reserve` validates available funds and creates the reservation before the entry write.
  A retry can return `insufficient_funds` or `reservation_exists` before detection of a duplicate key.
  For `Spend` and `Reserve`, a detected entry-key collision returns `AlreadyExists` with `duplicate_idempotency_key`.
- `Refund` returns the existing entry when the key identifies a refund. It does not compare the new amount, original debit, or metadata.
  A key that identifies another entry type returns `AlreadyExists`. When the caller supplies an original key, Ledger resolves that debit first.
- `Capture` rejects finalized or expired reservations with `FailedPrecondition` / `reservation_closed` before detection of a duplicate key.
- `Release` rejects finalized reservations with `FailedPrecondition` / `reservation_closed`. It can release an expired reservation whose status remains `active`.
  After successful capture or release, a retry returns `reservation_closed`.
- `Batch` can report a duplicate as successful with `duplicate=true` and `ok=true`.
  Funds and reservation validation can fail first for spends and reservations.
  A refund key owned by a non-refund entry returns `ok=false`, `duplicate=false`, and `error_code=duplicate_idempotency_key`.

Client guidance:

- Treat duplicates as success only for a retry of the same intended operation.
- Use operation namespaces for idempotency keys, such as `grant:<...>`, `spend:<...>`, and `refund:<...>`, to prevent accidental collisions.
- For `Capture` / `Release`, treat `reservation_closed` as a terminal state and use `GetReservation` to disambiguate a safe retry from a competing finalization.

## RPCs

### GetBalance

Returns derived balances:

- `total_cents`: sum of all credits/debits (after applying expiry rules)
- `available_cents`: spendable balance after subtracting active (non-expired) holds

### Grant

Appends a `grant` credit entry.

Key fields:

- `amount_cents` must be positive
- `expires_at_unix_utc`:
  - `0` means **no expiry** (permanent credits)
  - otherwise a unix timestamp (UTC seconds)

Response:

- `Empty { entry_id, created_unix_utc }`

### Spend

Appends a `spend` debit entry (stored as a negative `amount_cents`).

Response:

- `Empty { entry_id, created_unix_utc }`

### Reserve

Creates an `active` reservation and appends a `hold` entry.

Key fields:

- `reservation_id` is the reservation handle used for later capture/release.
- `expires_at_unix_utc` optionally sets a TTL for the reservation hold.

Response:

- `Empty { entry_id, created_unix_utc }` where `entry_id` is the `hold` entry.

### Capture

Finalizes an `active` reservation as `captured`. The capture amount must equal the reserved amount.
For a smaller final charge, use an atomic release-and-spend batch. See [embedded settlement](integration.md#2-embedding-the-go-library).

Effects (single transaction):

- Appends a `reverse_hold` entry.
- Appends a `spend` debit entry (negative amount) and returns it.

Expired or already-finalized reservations are rejected (`FailedPrecondition` / `reservation_closed`).

Idempotency note: retries after a successful capture may return `reservation_closed` (rather than `duplicate_idempotency_key`) because reservation state is validated before idempotency conflicts are evaluated. Use `GetReservation` to confirm the final state.

Response:

- `Empty { entry_id, created_unix_utc }` where `entry_id` is the `spend` debit entry.

### Release

Finalizes an `active` reservation as `released` and appends a `reverse_hold` entry.
An expired reservation can be released if its status is still `active`.

Idempotency note: retries after a successful release may return `reservation_closed` (rather than `duplicate_idempotency_key`) because reservation state is validated before idempotency conflicts are evaluated. Use `GetReservation` to confirm the final state.

Response:

- `Empty { entry_id, created_unix_utc }` where `entry_id` is the `reverse_hold` entry.

### Refund

Appends a `refund` entry that references a prior debit entry.

Original reference:

- `original_entry_id` (exact debit entry id), or
- `original_idempotency_key` (the debit's idempotency key)

Constraints:

- The original entry must be a debit (`spend`) entry (negative amount).
- The ledger enforces: `sum(refunds for original) <= abs(original debit)`.

Response:

- `RefundResponse { entry_id, created_unix_utc }`

### Batch

Executes multiple mutations against the same account in one request.

Semantics:

- `atomic=true`: All operations succeed together. If an operation fails, the batch rolls back all changes. Undone operations return `error_code=rolled_back`.
- `atomic=false` (best-effort): Each operation runs independently inside one transaction with savepoints. Results report failures per item.

Limits:

- Maximum operations per batch: 5000 (server rejects larger requests with `InvalidArgument` / `batch_too_large`).

Result fields:

- `ok=true`: The operation applied. `entry_id` and `created_unix_utc` are present.
- `duplicate=true`: The duplicate succeeds without another change. `ok=true`, and `entry_id` may be empty.
- `ok=false`: The operation failed. `error_code` and `error_message` are present.

Refund operations are supported via `BatchRefundOp` and follow the same "refund cannot exceed debit" invariant as the unary `Refund` RPC.

### ListEntries

Pages the append-only entry stream in reverse-chronological order (newest first).

Fields:

- `before_unix_utc`: upper bound cursor (defaults to now when `0`)
- `limit`: page size
- `types`: optional server-side type filter (strings matching `Entry.type`)
- `reservation_id`: optional filter
- `idempotency_key_prefix`: optional prefix filter (useful for deterministic correlation)

### GetReservation

Returns the computed state for one reservation (`Reservation` message), including:

- `status` (`active`/`captured`/`released`)
- `expires_at_unix_utc`
- `expired`: true only when the reservation expired while still `active`
- `held_cents`: held amount (0 if not active or expired)
- `captured_cents`: captured amount (0 unless captured)

### ListReservations

Pages reservations for an account in reverse-chronological order (newest first).

Fields:

- `before_created_unix_utc`: upper bound cursor
- `limit`: page size
- `statuses`: optional filter (`active`, `captured`, `released`)

## Stable Error Codes (gRPC status messages)

Unary and batch per-item errors use stable string codes that map to gRPC status codes:

- `invalid_user_id` (`InvalidArgument`)
- `invalid_ledger_id` (`InvalidArgument`)
- `invalid_tenant_id` (`InvalidArgument`)
- `invalid_reservation_id` (`InvalidArgument`)
- `invalid_entry_id` (`InvalidArgument`)
- `invalid_idempotency_key` (`InvalidArgument`)
- `invalid_amount_cents` (`InvalidArgument`)
- `invalid_metadata_json` (`InvalidArgument`)
- `invalid_entry_type` (`InvalidArgument`)
- `insufficient_funds` (`FailedPrecondition`)
- `unknown_reservation` (`NotFound`)
- `unknown_entry` (`NotFound`)
- `duplicate_idempotency_key` (`AlreadyExists`)
- `reservation_exists` (`AlreadyExists`)
- `reservation_closed` (`FailedPrecondition`)
- `invalid_refund_original` (`FailedPrecondition`)
- `refund_exceeds_debit` (`FailedPrecondition`)

For batch operations, `rolled_back` indicates an operation was undone due to `atomic=true` behavior.

## grpcurl examples

Start the [local runtime](../demo/README.md). Sign in, create a tenant, and create its credential.
Install `grpcurl` and run these commands from the repository root.
Export `LEDGER_TENANT_ID` and `LEDGER_TENANT_CREDENTIAL` through your private environment.
`grpcurl` uses the supplied protobuf schema for these examples.

Define this shell function:

```bash
ledger_rpc() {
  grpcurl -plaintext -expand-headers -proto api/credit/v1/credit.proto \
    -H 'authorization: Bearer ${LEDGER_TENANT_CREDENTIAL}' \
    -d @ localhost:50051 "credit.v1.CreditService/$1"
}
```

Use a fresh `user123` account for the examples. Run the grant before the dependent mutations.
Use distinct keys for new operations. Follow the retry contract for an existing operation.
Mutation responses include `entry_id` and `created_unix_utc`. `grpcurl` displays protobuf JSON field names and string-encoded 64-bit integers.

### Grant credit

```bash
ledger_rpc Grant <<JSON
{
    "tenant_id":"${LEDGER_TENANT_ID}",
    "user_id":"user123",
    "ledger_id":"default",
    "amount_cents": 1000,
    "idempotency_key":"grant-1",
    "expires_at_unix_utc":0,
    "metadata_json":"{\\"reason\\":\\"signup_bonus\\"}"
  }
JSON
```

### Get balance

```bash
ledger_rpc GetBalance <<JSON
{"tenant_id":"${LEDGER_TENANT_ID}","user_id":"user123","ledger_id":"default"}
JSON
```

After the grant above, the balance response contains:

```json
{
  "totalCents": "1000",
  "availableCents": "1000"
}
```


### Reserve credit

```bash
ledger_rpc Reserve <<JSON
{
    "tenant_id":"${LEDGER_TENANT_ID}",
    "user_id":"user123",
    "ledger_id":"default",
    "amount_cents": 500,
    "reservation_id":"order-555",
    "idempotency_key":"reserve-1",
    "metadata_json":"{\\"order_id\\":555}"
  }
JSON
```

### Capture reservation

```bash
ledger_rpc Capture <<JSON
{
    "tenant_id":"${LEDGER_TENANT_ID}",
    "user_id":"user123",
    "ledger_id":"default",
    "reservation_id":"order-555",
    "idempotency_key":"capture-1",
    "amount_cents":500,
    "metadata_json":"{\\"order_id\\":555}"
  }
JSON
```

### Release reservation

For this alternative outcome, reserve a separate 100 units before release:

```bash
ledger_rpc Reserve <<JSON
{"tenant_id":"${LEDGER_TENANT_ID}","user_id":"user123","ledger_id":"default","amount_cents":100,"reservation_id":"order-release-555","idempotency_key":"reserve-release-1","metadata_json":"{}"}
JSON
```


```bash
ledger_rpc Release <<JSON
{
    "tenant_id":"${LEDGER_TENANT_ID}",
    "user_id":"user123",
    "ledger_id":"default",
    "reservation_id":"order-release-555",
    "idempotency_key":"release-1",
    "metadata_json":"{\\"order_id\\":555}"
  }
JSON
```

### Spend without reservation

```bash
ledger_rpc Spend <<JSON
{
    "tenant_id":"${LEDGER_TENANT_ID}",
    "user_id":"user123",
    "ledger_id":"default",
    "amount_cents": 200,
    "idempotency_key":"spend-1",
    "metadata_json":"{\\"action\\":\\"purchase\\"}"
  }
JSON
```

### Refund a debit (spend/capture)

Refunds are first-class entries that link to an original debit entry. The ledger prevents refunds that exceed the original debit amount.

```bash
ledger_rpc Refund <<JSON
{
    "tenant_id":"${LEDGER_TENANT_ID}",
    "user_id":"user123",
    "ledger_id":"default",
    "original_idempotency_key":"spend-1",
    "amount_cents": 50,
    "idempotency_key":"refund-1",
    "metadata_json":"{\\"reason\\":\\"reimbursement\\"}"
  }
JSON
```

### Batch operations

Use `Batch` to execute many mutations for a single account in one request. Results report each operation separately. Recognized duplicates can return `duplicate=true`. See [idempotency](#idempotency) for state and key conflicts.

```bash
ledger_rpc Batch <<JSON
{
    "account": { "tenant_id":"${LEDGER_TENANT_ID}", "user_id":"user123", "ledger_id":"default" },
    "atomic": false,
    "operations": [
      {
        "operation_id": "refund-1",
        "refund": {
          "original_idempotency_key": "spend-1",
          "amount_cents": 50,
          "idempotency_key": "refund-1",
          "metadata_json": "{\\"reason\\":\\"reimbursement\\"}"
        }
      },
      {
        "operation_id": "refund-2",
        "refund": {
          "original_idempotency_key": "spend-1",
          "amount_cents": 25,
          "idempotency_key": "refund-2",
          "metadata_json": "{\\"reason\\":\\"reimbursement\\"}"
        }
      }
    ]
  }
JSON
```

### Get reservation state

```bash
ledger_rpc GetReservation <<JSON
{
    "tenant_id":"${LEDGER_TENANT_ID}",
    "user_id":"user123",
    "ledger_id":"default",
    "reservation_id":"order-555"
  }
JSON
```

### List reservations

```bash
ledger_rpc ListReservations <<JSON
{
    "tenant_id":"${LEDGER_TENANT_ID}",
    "user_id":"user123",
    "ledger_id":"default",
    "limit": 20,
    "statuses": ["active", "captured", "released"]
  }
JSON
```

### List ledger entries

```bash
ledger_rpc ListEntries <<JSON
{
    "tenant_id":"${LEDGER_TENANT_ID}",
    "user_id":"user123",
    "ledger_id":"default",
    "types": ["refund", "spend", "grant"],
    "idempotency_key_prefix":"refund",
    "before_unix_utc": 1893456000,
    "limit": 20
  }
JSON
```


## Amounts and expiry

Amounts use integer `amount_cents`. Spend entries store negative amounts. Grant and refund entries store positive amounts.
Set `expires_at_unix_utc` to `0` for permanent credits or reservations.
Applications decide which grants have a time limit.
