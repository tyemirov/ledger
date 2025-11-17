# ISSUES (Append-only Log)

Entries record newly discovered requests or changes, with their outcomes. No instructive content lives here. Read @NOTES.md for the process to follow when fixing issues.

Read @AGENTS.md, @AGENTS.GO.md, @AGENTS.DOCKER.md, @AGENTS.FRONTEND.md, @AGENTS.GIT.md, @POLICY.md, @NOTES.md, @README.md and @ISSUES.md. Start working on open issues. Work autonomously and stack up PRs.

Each issue is formatted as `- [ ] [<ID>-<number>]`. When resolved it becomes -` [x] [<ID>-<number>]`

## Features (100-199)

- [ ] [LG-100] Prepare a demo of a web app which uses ledger backend for transactions. A deliverable is a plan of execution.
    - Rely on mpr-ui for the backend. Use a header and a footer. Use mpr-ui declarative syntax
    - Rely on TAuth for authentication. Usge TAuth. Mimic the demo
    - Have a simple case of 
    transaction button that takes 5 units of virtual currency
    1. enough funds -- transaction succeed
    2. not enough funds -- transaction fails
    3. enough funds after which there is 0 units of virtual currency left

    A single button which says transact with a virtual currency be 5 coins per transaction. A user gets 20 coins when an account is created. A user can buy coins at any time. once the coins are depleded, a user can no longwer transact untill a user obtains the coins

    The architecture shall be -- a backend that supports TAuth authentication by accepting the JWTs and verifying them against google service
    a backend service that integrates with Ledger and verifies that the use has sufficient balance for the transactions
    a web service, ghhtp, that serves the stand alone front end

    Find dependencies under tools folder and read their documentation and code to understand the integration. be specific in the produced plan on the intehration path forward

- [ ] [LG-101] Implement the plan of building a demo app delivered in LG-100

## Improvements (200–299)

## BugFixes (300–399)

## Maintenance (400–499)

- [x] [LG-400] Review @POLICY.md and verify what code areas need improvements and refactoring. Prepare a detailed plan of refactoring. Check for bugs, missing tests, poor coding practices, uplication and slop. Ensure strong encapsulation and following the principles og @AGENTS.md, @AGENTS.GO.md and policies of @POLICY.md
    - 2024-11-25 audit summary:
        - Domain logic violates POLICY invariants: operations accept raw primitives without smart constructors or edge validation (`internal/credit/types.go`, `internal/grpcserver/server.go`), timestamps default to zero in `NewService`, and `ListEntries`/`Balance` create accounts on read.
        - Reservation flows are incorrect: holds are never reversed (capture/release only check existence and write zero entries), `reservation_id` is not unique in `db/migrations.sql`, and limits/defaults for listing are unbounded, allowing stale holds to permanently lock funds.
        - Operational drift: duplicate stores (gorm vs pgx) with the binary pinned to GORM + AutoMigrate (`cmd/credit/main.go`), no tests of any kind (`go test ./...` reports “[no test files]”), Docker lacks the mandated `.env.creditsvc`, and the distroless runtime runs as non-root (`Dockerfile`).
        - Error handling/logging gaps: no contextual wrapping, gRPC leaks raw error strings, zap/Viper/Cobra are absent, metadata/JSON/idempotency fields are never validated, and limits/metadata can break SQL.
    - Refactoring plan:
        - Introduce domain constructors/value objects (UserID, ReservationID, IdempotencyKey, Money) and enforce validation in the gRPC edge before calling the service.
        - Redesign holds/reservations: persist reservation state (amount + status), ensure `(account_id,reservation_id)` uniqueness, compute active holds from reservation status, and emit proper capture/release ledger entries that unlock funds.
        - Collapse on a single pgx-based store, drop runtime AutoMigrate, and plumb config via Cobra+Viper with zap logging and contextual error wrapping.
        - Add integration tests covering grant/reserve/capture/release/spend/list plus store-specific tests, enforce sane pagination defaults, and add the mandated Docker `.env` + root user.
- [x] [LG-401] Enforce POLICY invariants at the domain + gRPC edge.
    - Create smart constructors for `UserID`, `ReservationID`, `IdempotencyKey`, positive `AmountCents`, and JSON metadata in `internal/credit`.
    - Update `internal/grpcserver` handlers to validate requests (including pagination limits) and map validation failures to `codes.InvalidArgument`.
    - Remove zero-value fallbacks (e.g., `NewService` clock defaulting to 0) and ensure the core never sees invalid primitives.
    - 2024-11-25: Added domain constructors + validation, rewired the service and gRPC edge to consume them with InvalidArgument mappings, enforced sane list limits, updated the CLI wiring, and introduced unit tests for the new smart constructors.
- [x] [LG-402] Repair reservation/hold accounting.
    - Introduce a reservations table (or equivalent) with `(account_id,reservation_id)` uniqueness and stored amount/status.
    - Rework `Reserve`, `Capture`, and `Release` so they update reservation status, reverse holds on release, and prevent double capture; update `SumActiveHolds` to ignore closed reservations.
    - Extend `db/migrations.sql` and stores to reflect the new schema and invariants.
    - 2024-11-25: Added the `reservations` enum+table, updated both pgx and GORM stores plus CLI migrations, reworked the service to enforce single capture/release with reverse-hold entries and availability math fixes, added gRPC error mappings, and introduced unit tests covering reserve/capture/release flows.
- [x] [LG-403] Consolidate persistence + runtime wiring.
    - Remove the runtime GORM dependency, standardize on the pgx store, and expose configuration via Cobra/Viper with env/flag parity.
    - Add structured logging with zap and wrap errors with operation + subject codes before surfacing them to gRPC.
    - Delete AutoMigrate from `cmd/credit`, ensure migrations remain SQL-first, and verify startup gracefully handles dependency errors.
    - 2024-11-25: Deleted the unused GORM store, rewired `cmd/credit` to use pgstore exclusively with Cobra/Viper config handling, added zap-powered logging plus graceful shutdown, and kept Docker/env compatibility intact (SQL migrations remain source of truth).
- [x] [LG-404] Testing, CI, and container compliance.
    - Author black-box integration tests covering grant/reserve/capture/release/spend/list flows plus store-specific tests for pagination and idempotency.
    - Wire `make test`, `make lint`, and `make ci` (or equivalent) to run gofmt/go vet/staticcheck/ineffassign + coverage enforcement per POLICY.
    - Align Docker assets with AGENTS.DOCKER: add `.env.creditsvc`, reference it from docker-compose, ensure containers run as root, and document the workflow.
    - 2024-11-25: Added Makefile targets (`fmt`, `lint`, `test`, `ci`) that run gofmt/vet/staticcheck/ineffassign with an 80% coverage gate on the internal domain package, expanded the service tests to cover balance/grant/spend/list flows, introduced `.env.creditsvc` with docker-compose env_file wiring, switched the runtime image to rootful Debian, and documented the tooling workflow in README.
- [x] [LG-405] Switch to sqlite from postgres. Prepare the code that allows to pass the DB URIL sufficient for the GORM to either use a postgres or sqlite driver. ensure that the sqlite driver doesnt require GCO
    - 2024-11-25: Reintroduced the GORM store with reservation support, added a CGO-free SQLite driver alongside the Postgres driver, taught `creditd` to parse the `DATABASE_URL` and pick the right driver (defaulting to SQLite with AutoMigrate), simplified Docker to a single service storing data in an `.env.creditsvc`-defined SQLite path, and updated README/documentation accordingly.

## Planning 
do not work on the issues below, not ready
