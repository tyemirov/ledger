# Ledger gRPC API Reference

This document is the canonical reference for the Ledger gRPC surface (`credit.v1.CreditService`) and its behavioral contracts (idempotency, refunds, batch semantics, reservation TTLs, and introspection APIs).

For "how to run the service" or "how to embed the Go library", see `docs/integration.md`.

## Account Model

Every mutation and query is scoped to an **account** identified by:

- `tenant_id`
- `ledger_id`
- `user_id`

Idempotency keys are enforced **per account**.

## Authentication

Every gRPC request must include the `authorization` metadata header:

```
authorization: Bearer <tenant_secret_key>
```

The server extracts `tenant_id` from the request body, looks up the corresponding `secret_key` in the tenant configuration (`config.yml`), and validates the Bearer token.

| Failure reason          | gRPC code          | Message                            |
| ----------------------- | ------------------ | ---------------------------------- |
| Missing `tenant_id`     | `Unauthenticated`  | `missing tenant_id`               |
| Unknown tenant          | `PermissionDenied` | `tenant "<id>" is not authorized` |
| Missing metadata        | `Unauthenticated`  | `missing metadata`                |
| Missing `authorization` | `Unauthenticated`  | `missing authorization header`    |
| Wrong format            | `Unauthenticated`  | `invalid authorization header format` |
| Wrong secret            | `Unauthenticated`  | `invalid secret key`              |

Tenant secrets are configured per tenant in `config.yml` and support environment variable expansion (e.g., `${MY_SECRET:-fallback}`).

## Data Model

### Entries (append-only)

The ledger is append-only. Operations append immutable entries and balances are derived from the entry stream.

Entry types (`Entry.type`):

- `grant` (credit; may be expiring via `expires_at_unix_utc`)
- `hold` (credit hold created by `Reserve`)
- `reverse_hold` (releases a hold; emitted by `Release` or as part of `Capture`)
- `spend` (debit; stored as a **negative** `amount_cents`)
- `refund` (credit linked to a prior debit; `Entry.refund_of_entry_id` points at the original debit entry)

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

- Retrying the *same* logical operation with the same key is safe.
- Reusing a key for a *different* operation is rejected.

gRPC behavior:

- Unary mutations (`Grant`, `Spend`, `Reserve`, `Refund`) return a gRPC error with code `AlreadyExists` and message `duplicate_idempotency_key` when the key already exists.
- Reservation finalization (`Capture`, `Release`) validates reservation state first and returns `FailedPrecondition` / `reservation_closed` once the reservation is no longer `active` (captured, released, or expired). This can also happen on safe retries after a successful call.
- Batch mutations (`Batch`) surface duplicates per-item via `BatchOperationResult.duplicate=true` (and `ok=true`).

Client guidance:

- Treat duplicates as success only when you are certain you are retrying the same logical operation.
- Strongly namespace idempotency keys by operation (for example `grant:<...>`, `spend:<...>`, `refund:<...>`) to avoid accidental collisions.
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

Finalizes an `active` reservation as `captured`.

Effects (single transaction):

- Appends a `reverse_hold` entry.
- Appends a `spend` debit entry (negative amount) and returns it.

Expired or already-finalized reservations are rejected (`FailedPrecondition` / `reservation_closed`).

Idempotency note: retries after a successful capture may return `reservation_closed` (rather than `duplicate_idempotency_key`) because reservation state is validated before idempotency conflicts are evaluated. Use `GetReservation` to confirm the final state.

Response:

- `Empty { entry_id, created_unix_utc }` where `entry_id` is the `spend` debit entry.

### Release

Finalizes an `active` reservation as `released` and appends a `reverse_hold` entry.

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

- `atomic=true`: all-or-nothing. If any operation fails, all are rolled back; operations that were undone return `error_code=rolled_back`.
- `atomic=false` (best-effort): each operation runs independently inside one transaction using savepoints; failures are reported per-item.

Limits:

- Maximum operations per batch: 5000 (server rejects larger requests with `InvalidArgument` / `batch_too_large`).

Result fields:

- `ok=true`: operation applied; `entry_id` + `created_unix_utc` present.
- `duplicate=true`: idempotent no-op success; `ok=true`, and `entry_id` may be empty.
- `ok=false`: failed; `error_code` + `error_message` present.

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

