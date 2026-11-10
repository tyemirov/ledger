# Ledger

Ledger manages virtual credits for applications. It records grants, holds, charges, releases, and refunds in an append-only entry history.
Run it as a gRPC service, or embed its Go library in your application.

Use Ledger for prepaid API credits, promotional balances, or jobs that reserve credits before work starts.
Your application controls prices, credit grants, and settlement decisions. Ledger applies the accounting rules.

## Choose an integration

| Approach | Use it when | Dependencies and responsibilities |
| --- | --- | --- |
| **gRPC service** | Multiple services or languages share a credit ledger. | SQLite or PostgreSQL stores the data. TAuth authenticates the management workspace. Application clients use tenant credentials. |
| **Embedded Go library** | A Go application needs Ledger changes inside its database transaction. | `pkg/ledger` and `pkg/gormstore` share the application's GORM transaction. Your application owns authentication, authorization, and schema management. |

The embedded adapter lets application records and Ledger entries commit or roll back together.
See the [integration guide](docs/integration.md) for both approaches.

**License:** [PolyForm Noncommercial 1.0.0](LICENSE). For commercial use, [request separate licensing terms](COMMERCIAL_LICENSE.md).

## Account model

```text
UserAccount (one authenticated TAuth identity)
  └─ Ledger tenant (owned workspace and client credentials)
       └─ Ledger account (user_id + ledger_id within the tenant)
            ├─ Accounting entries
            └─ Reservations
```

Each Ledger account uses the full `(tenant_id, user_id, ledger_id)` identity.
The application selects `user_id` and `ledger_id`. These values identify credit balances independently of the workspace owner's identity.

A UserAccount can own multiple tenants without an application tenant-count limit.
A tenant credential authorizes operations within its tenant.
The browser workspace manages tenants and credentials. Credit operations use the private gRPC API.

## Try it locally

Install Git, Make, Docker with Compose, OpenSSL, and curl. Start the Docker daemon.
Run these commands:

```bash
git clone https://github.com/tyemirov/ledger.git
cd ledger
make up
```

Open [the local workspace](http://localhost:8000/). Sign in with Google, create a tenant, and create a tenant credential.
Save the credential when the workspace shows it. The secret appears only in the creation response.

The local runtime builds Ledger and starts TAuth, a same-origin proxy, and separate SQLite volumes.
`make up` checks the page, health endpoint, browser config, TAuth session route, and protected control plane.
Run `make down` to stop the runtime and preserve its data.
See [local runtime details](demo/README.md) and [setup](docs/setup.md) for prerequisites and configuration.

## Complete a credit flow

Install [grpcurl](https://github.com/fullstorydev/grpcurl). Run the example from the repository root after local setup.
Set `LEDGER_TENANT_ID` to the tenant UUID. Supply `LEDGER_TENANT_CREDENTIAL` through your private shell environment.
Export both variables.

Define this shell function. It supplies the protobuf schema and expands the credential inside `grpcurl`.

```bash
ledger_rpc() {
  grpcurl -plaintext -expand-headers -proto api/credit/v1/credit.proto \
    -H 'authorization: Bearer ${LEDGER_TENANT_CREDENTIAL}' \
    -d @ localhost:50051 "credit.v1.CreditService/$1"
}
```

Grant 1,000 units, reserve 500, then capture the reservation:

```bash
ledger_rpc Grant <<JSON
{
  "tenant_id": "${LEDGER_TENANT_ID}",
  "user_id": "readme-example",
  "ledger_id": "default",
  "amount_cents": 1000,
  "idempotency_key": "readme-grant-1",
  "expires_at_unix_utc": 0,
  "metadata_json": "{}"
}
JSON
ledger_rpc Reserve <<JSON
{
  "tenant_id": "${LEDGER_TENANT_ID}",
  "user_id": "readme-example",
  "ledger_id": "default",
  "amount_cents": 500,
  "reservation_id": "readme-order-1",
  "idempotency_key": "readme-reserve-1",
  "metadata_json": "{}"
}
JSON
ledger_rpc Capture <<JSON
{
  "tenant_id": "${LEDGER_TENANT_ID}",
  "user_id": "readme-example",
  "ledger_id": "default",
  "amount_cents": 500,
  "reservation_id": "readme-order-1",
  "idempotency_key": "readme-capture-1",
  "metadata_json": "{}"
}
JSON
ledger_rpc GetBalance <<JSON
{
  "tenant_id": "${LEDGER_TENANT_ID}",
  "user_id": "readme-example",
  "ledger_id": "default"
}
JSON
```

For a fresh account, the balances change as follows:

| Step | Total | Available |
| --- | ---: | ---: |
| Grant | 1,000 | 1,000 |
| Reserve | 1,000 | 500 |
| Capture | 500 | 500 |

The final response contains `totalCents: "500"` and `availableCents: "500"`.
Mutation responses include an entry ID and creation time.
For another example run, select a new `user_id`. For retries, keep the original operation keys and follow the [retry contract](docs/api.md#idempotency).
See the [RPC examples](docs/api.md#grpcurl-examples) for release, direct spend, refund, batch, and history queries.

## Accounting guarantees and boundaries

- **Entry history:** Credit changes append entries. Balances derive from entries and active reservations. Reservation status changes when a reservation closes.
- **Integer amounts:** The API uses `amount_cents`. Your application defines the credit unit and any currency policy.
- **Transactions:** Mutations use database transactions. An atomic batch applies all operations together. Each batch addresses one account and accepts at most 5,000 operations.
- **Retry control:** Idempotency keys are unique per account. Duplicate responses depend on the operation. Reuse a key only for the same intended operation.
- **Reservations:** Capture requires the full reserved amount. Use atomic release-and-spend for a smaller final charge.
- **Expiration:** Grants and reservations can expire. Zero means no expiry. An expired active reservation stops reducing available funds and rejects capture.
- **Refunds:** Refund entries reference a debit. Their cumulative amount cannot exceed that debit.
- **Credentials:** A tenant credential supports repeated requests until revocation. Ledger stores its digest and discloses the secret once.

For the service, keep gRPC on a private network and supply a tenant Bearer credential on every request.
The HTTP control plane validates the TAuth session and tenant ownership for protected resources.
For embedded use, the application supplies these authorization boundaries.

The source includes the service, browser workspace, and public GORM adapter.
Official Go and Python clients and `ledgerctl` remain planned under F003.
Package publication and hosted deployment require separate verification. The deployment guide describes the tracked configuration.

## Documentation

| Task | Guide |
| --- | --- |
| Configure or build the service | [Setup](docs/setup.md) |
| Integrate gRPC or embed Go | [Integration guide](docs/integration.md) |
| Understand RPCs, errors, retries, and examples | [gRPC API reference](docs/api.md) |
| Integrate tenant and credential management | [HTTP OpenAPI contract](api/control/v1/openapi.yaml) |
| Operate the MPR hosted profile | [Deployment guide](docs/deployment.md) |
| Inspect readiness behavior | [Health endpoint](docs/health.md) |

## Development

Use the toolchain in [go.mod](go.mod) and Node.js with npm for the frontend checks.
The [setup guide](docs/setup.md#development-tools) lists additional tooling.

```bash
make fmt   # checks Go formatting
make lint  # checks Go and browser modules
make test  # runs Go coverage and integration suites
make ci    # runs format, lint, and tests
```

The Go test target enforces 100% statement coverage. Integration checks include browser, local runtime, and Pages artifact tests.
Contributions follow the [contributor license terms](CONTRIBUTOR_LICENSE.md).
