# ISSUES (Append-only Log)

Entries record newly discovered requests or changes, with their outcomes. No instructive content lives here. Read @NOTES.md for the process to follow when fixing issues.

Read @AGENTS.md, @AGENTS.GO.md, @AGENTS.DOCKER.md, @AGENTS.FRONTEND.md, @AGENTS.GIT.md, @POLICY.md, @NOTES.md, @README.md and @ISSUES.md. Start working on open issues. Work autonomously and stack up PRs.

Each issue is formatted as `- [ ] [<ID>-<number>]`. When resolved it becomes -` [x] [<ID>-<number>]`

## Features (100-199)

- [x] [LG-100] Prepare a demo of a web app which uses ledger backend for transactions. A deliverable is a plan of execution.
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
    - 2025-11-17: Authored `docs/lg-100-demo-plan.md`, outlining the multi-service architecture (TAuth + demo HTTP API + creditd + ghttp) plus the UI/backend tasks, endpoints, and testing strategy required for LG-101.

- [x] [LG-101] Build the demo transaction API service described in `docs/lg-100-demo-plan.md`.
    - Added `demo/backend/cmd/walletapi` + `demo/backend/internal/walletapi` with Cobra/Viper config, zap logging, CORS, and TAuth session validation plus an insecure gRPC dialer for local development.
    - Wired the ledger client for `Grant`, `Spend`, `GetBalance`, and `ListEntries` with per-request timeouts plus error mapping for duplicate idempotency and insufficient funds.
    - Exposed `/api/session`, `/api/bootstrap`, `/api/wallet`, `/api/transactions`, and `/api/purchases`, including idempotent bootstrap grants and automatic wallet responses.

- [x] [LG-102] Ship the declarative front-end bundle under `demo/frontend/ui` per `docs/lg-100-demo-plan.md`.
    - Authored `demo/frontend/ui/index.html`, `styles.css`, and `app.js` that load `mpr-ui`, TAuth’s auth-client, Alpine, and GIS in the documented order.
    - Implemented wallet metrics, the 5-coin transaction button, purchase controls, and ledger history with toast/status banners for the three core scenarios.
    - Used the auth-client callbacks to bootstrap the wallet, fetch balances, and call the new API endpoints with credentialed fetch helpers.

- [x] [LG-103] Provide hosting/orchestration and local tooling for the demo stack (`docs/lg-100-demo-plan.md` “Hosting with ghttp” + “Local Orchestration / Compose”).
    - Added `demo/backend/Dockerfile`, `demo/backend/.env.walletapi.example`, `demo/.env.tauth.example`, and `demo/docker-compose.yml` so contributors can run creditd, TAuth, walletapi, and ghttp together.
    - Documented the ghttp workflow plus the compose steps (including env copies) in `docs/demo.md` and linked the section from README.

- [x] [LG-104] Add integration tests, CI wiring, and documentation from the “Implementation Breakdown” + “Validation & Monitoring Strategy” sections of `docs/lg-100-demo-plan.md`.
    - Introduced `demo/backend/internal/walletapi/server_test.go`, which spins up an in-memory ledger + HTTP stack via bufconn/httptest and asserts bootstrap, spend success, insufficient funds, and purchase scenarios.
    - Extended docs (`docs/demo.md`) with the scenario checklist and manual validation steps; ensured `make test` exercises the new package while retaining coverage gates.

- [x] [LG-105] Build a fresh `demo/` package that contains both the Go HTTP façade (importing the ledger gRPC client) and the standalone front-end bundle required by `docs/lg-100-demo-plan.md`. The existing materials under `tools/` are informative only—no runtime dependencies on that tree.
    - Create `demo/backend` with a new Go module (or sub-package) that compiles to `demo/backend/cmd/walletapi`. Reuse Cobra/Viper for config, wire the gRPC client to `credit.v1.CreditService`, and expose `/api/session`, `/api/bootstrap`, `/api/wallet`, `/api/transactions`, `/api/purchases` exactly as described in LG-101. All configuration (TAuth base URL, JWT key, ledger addr, timeout, allowed origins) must come from flags/env with no defaults; validation happens in `PreRunE` and the process fails fast if anything is missing.
    - Author a `demo/frontend` directory housing `index.html`, `styles.css`, and `app.js`. Reference `mpr-ui` CSS/JS and GIS via CDN URLs only; load `http://localhost:8080/static/auth-client.js` at runtime just like the production stack. Use Alpine (module import) for interactivity and replicate the wallet layout described in LG-100, but ensure the page makes zero references to `tools/mpr-ui` assets.
    - Implement a front-end config bootstrapper: fetch `/demo/config.js` from TAuth before initializing `mpr-ui`, set `<mpr-header site-id>` dynamically, and refuse to load when the response is absent. All other endpoints (login/logout/nonce) must be bound via attributes instead of hardcoded constants.
    - Build a shared `demo/frontend/walletApiClient.js` helper that wraps `fetch` to the backend service with `credentials: 'include'` and typed error handling. Methods: `fetchSession`, `bootstrapWallet`, `getWallet`, `spendCoins`, `purchaseCoins`. Each returns parsed JSON typed per LG-100 invariants (coins as integers, ledger entries with timestamps).
    - Port the LG-100 flows into `app.js`: maintain Alpine stores for auth, wallet, transactions, and status banners; disable buttons while network calls are pending; display ledger history and zero-balance warnings exactly as the plan requires. No default values—if a response is missing required fields, throw and surface an unrecoverable banner.
    - Compose a dedicated Dockerfile + docker-compose overlay under `demo/` so contributors can run `creditd`, the new wallet API, TAuth, and a static file server (ghttp) from one command. Ensure `.env` samples (e.g., `.env.walletapi`, `.env.tauth`) live inside `demo/` and are the only source of runtime configuration.
    - Add Playwright coverage under `demo/tests/` that drives the new front end end-to-end. Write a stub server (similar to the existing root-level harness) to intercept `/demo/config.js`, `/static/auth-client.js`, GIS, and the wallet API endpoints so the four mandatory scenarios (sign-in prompt, successful spend, insufficient funds, purchase replenishment) and the zero-balance banner are asserted. Wire these tests into `make test`.
    - Update repository docs (`README.md`, `docs/demo.md`, and `docs/lg-100-demo-plan.md` if necessary) to point to the new `demo/` workflow, including the commands for building/running the Docker stack and executing the Playwright suite.

## Improvements (200–299)

## BugFixes (300–399)

- [x] [LG-300] The demo stack was renamed; all references should use the `demo/` paths.
    - Updated documentation and scripts to drop the old name; Playwright script now points to `demo/tests`.
    - Fixed `demo/docker-compose.yml` to reference `demo/backend/Dockerfile` and adjusted that Dockerfile to build from `./demo/backend/cmd/walletapi`.
    - `docker compose -f demo/docker-compose.yml config` now resolves without path errors; `make test` (Go + Playwright) passes.

- [ ] [LG-301] I am unable to log into demo. Fix the login first with complete disregard to ledger server (remove all ledger-related functionality and just ensure we have a stable login).
Read the docs and follow the docs under docs/TAuth/usage.md and docs/mpr-ui/custom-elements.md, docs/mpr-ui/demo-index-auth.md, @docs/mpr-ui/demo/.

Deliver the page that allows a user to log-in and stay logged in after the page refresh. Copy the demo from docs/mpr-ui/demo/ and use it as a start.

```
ledger-web        | 192.168.65.1 - - [19/Nov/2025:23:21:21 +0000] "GET / HTTP/1.1" 200 6272 "-" "Mozilla/5.0 (Macintosh; Intel Mac OS X 10.15; rv:145.0) Gecko/20100101 Firefox/145.0" "-"
ledger-web        | 192.168.65.1 - - [19/Nov/2025:23:21:21 +0000] "GET /styles.css HTTP/1.1" 200 3513 "http://localhost:8000/" "Mozilla/5.0 (Macintosh; Intel Mac OS X 10.15; rv:145.0) Gecko/20100101 Firefox/145.0" "-"
ledger-tauth      | {"level":"info","ts":1763594481.1039588,"caller":"server/main.go:297","msg":"http","method":"GET","path":"/demo/config.js","status":200,"ip":"192.168.65.1","elapsed":0.000215878}
ledger-tauth      | {"level":"info","ts":1763594481.1043713,"caller":"server/main.go:297","msg":"http","method":"GET","path":"/static/auth-client.js","status":200,"ip":"192.168.65.1","elapsed":0.000259635}
ledger-web        | 192.168.65.1 - - [19/Nov/2025:23:21:21 +0000] "GET /demo/config.js HTTP/1.1" 200 546 "http://localhost:8000/" "Mozilla/5.0 (Macintosh; Intel Mac OS X 10.15; rv:145.0) Gecko/20100101 Firefox/145.0" "-"
ledger-web        | 192.168.65.1 - - [19/Nov/2025:23:21:21 +0000] "GET /static/auth-client.js HTTP/1.1" 200 5462 "http://localhost:8000/" "Mozilla/5.0 (Macintosh; Intel Mac OS X 10.15; rv:145.0) Gecko/20100101 Firefox/145.0" "-"
ledger-web        | 192.168.65.1 - - [19/Nov/2025:23:21:21 +0000] "GET /app.js HTTP/1.1" 200 6363 "http://localhost:8000/" "Mozilla/5.0 (Macintosh; Intel Mac OS X 10.15; rv:145.0) Gecko/20100101 Firefox/145.0" "-"
ledger-web        | 192.168.65.1 - - [19/Nov/2025:23:21:21 +0000] "GET /wallet-api.js HTTP/1.1" 200 4040 "http://localhost:8000/app.js" "Mozilla/5.0 (Macintosh; Intel Mac OS X 10.15; rv:145.0) Gecko/20100101 Firefox/145.0" "-"
ledger-web        | 192.168.65.1 - - [19/Nov/2025:23:21:21 +0000] "GET /constants.js HTTP/1.1" 200 1404 "http://localhost:8000/app.js" "Mozilla/5.0 (Macintosh; Intel Mac OS X 10.15; rv:145.0) Gecko/20100101 Firefox/145.0" "-"
ledger-web        | 192.168.65.1 - - [19/Nov/2025:23:21:21 +0000] "GET /auth-flow.js HTTP/1.1" 200 1637 "http://localhost:8000/app.js" "Mozilla/5.0 (Macintosh; Intel Mac OS X 10.15; rv:145.0) Gecko/20100101 Firefox/145.0" "-"


ledger-tauth      | {"level":"info","ts":1763594481.2422597,"caller":"server/main.go:297","msg":"http","method":"POST","path":"/nonce","status":404,"ip":"192.168.65.1","elapsed":0.000097553}
ledger-web        | 192.168.65.1 - - [19/Nov/2025:23:21:21 +0000] "GET /me HTTP/1.1" 200 6272 "http://localhost:8000/" "Mozilla/5.0 (Macintosh; Intel Mac OS X 10.15; rv:145.0) Gecko/20100101 Firefox/145.0" "-"


ledger-web        | 192.168.65.1 - - [19/Nov/2025:23:21:21 +0000] "POST /auth/nonce HTTP/1.1" 404 18 "http://localhost:8000/" "Mozilla/5.0 (Macintosh; Intel Mac OS X 10.15; rv:145.0) Gecko/20100101 Firefox/145.0" "-"
ledger-web        | 192.168.65.1 - - [19/Nov/2025:23:21:21 +0000] "GET /favicon.ico HTTP/1.1" 200 6272 "http://localhost:8000/" "Mozilla/5.0 (Macintosh; Intel Mac OS X 10.15; rv:145.0) Gecko/20100101 Firefox/145.0" "-"
ledger-tauth      | {"level":"info","ts":1763594490.473501,"caller":"server/main.go:297","msg":"http","method":"POST","path":"/nonce","status":404,"ip":"192.168.65.1","elapsed":0.000028597}
ledger-web        | 192.168.65.1 - - [19/Nov/2025:23:21:30 +0000] "POST /auth/nonce HTTP/1.1" 404 18 "http://localhost:8000/" "Mozilla/5.0 (Macintosh; Intel Mac OS X 10.15; rv:145.0) Gecko/20100101 Firefox/145.0" "-"
ledger-tauth      | {"level":"info","ts":1763594490.4974575,"caller":"server/main.go:297","msg":"http","method":"POST","path":"/nonce","status":404,"ip":"192.168.65.1","elapsed":0.000044506}
ledger-web        | 192.168.65.1 - - [19/Nov/2025:23:21:30 +0000] "POST /auth/nonce HTTP/1.1" 404 18 "http://localhost:8000/" "Mozilla/5.0 (Macintosh; Intel Mac OS X 10.15; rv:145.0) Gecko/20100101 Firefox/145.0" "-"
```

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

- [x] [LG-406] Establish github workflows for testing and docker image release. Use an example under @docs/workflow for inspiration

## Planning 
do not work on the issues below, not ready
