# Changelog

## Unreleased

- No unreleased items yet.

## [v1.0.0] - 2026-03-31

### Features ✨
- Add tenant secret key validation and authentication interceptor
- Implement tenant validation based on config.yml
- Add refund support to Batch RPC and refund RPC for debit-referencing reimbursements
- Introduce smart constructors for signed amounts, tenant IDs, and ledger IDs with validation
- Feature complete demo stack with Google sign-in and configurable default tenant/ledger IDs
- Emit service-level operation logs and log unary gRPC requests
- Enable multi-platform support for Docker images
- Introduce ledger identifiers and entry inputs with documented API semantics

### Improvements ⚙️
- Add batch gRPC credit mutations and add refund support to batch processing
- Use SQLite for demo environment and improve SQLite handling with WAL and busy timeout reduction
- Improve scalability of ledger API and add GORM AutoMigrate support for Postgres
- Enhance docker-compose setup to use Postgres and standardized service orchestration
- Align demo UI with mpr-ui styling and enable 4-mode square theme switching
- Raise ledger test coverage to 95% and add comprehensive UI and backend test coverage
- Refactor module namespace and clean up demo orchestration and integration setup
- Maintain CI workflows enforcing coverage and tooling compliance
- Use published tauth module and standardize Docker image builds on Go 1.25
- Enable logging and diagnostics with operation logs and entry ID filters

### Bug Fixes 🐛
- Fix demo auth cookie domain, HTTPS entrypoint, UI styling, and compose ledger migrations
- Reject refund idempotency collisions and detect SQLite unique constraints
- Resolve demo demo README and links to documentation and site catalog
- Fix demo API Docker build and TAuth config provisioning
- Skip redundant bootstrap grants in demo setup
- Address SQLITE_BUSY issues and ensure capture uses distinct idempotency keys
- Fix various demo compose commands and container orchestration shortcuts

### Testing 🧪
- Add playright coverage for demo UI and demobackend error handling tests
- Add comprehensive operation logging and ledger integration tests
- Assert Google sign-in button visibility and stabilize UI flows
- Add ledger command coverage for file URL SQLite DSNs
- Enforce module-wide test coverage gates for CI

### Docs 📚
- Document ledger integration, error contracts, and API idempotency semantics
- Refresh demo stack instructions and clarify service vs. library locations
- Add LG-301 ledger logging documentation
- Remove broken and symlinked demo and documentation folders
- Update README with demo compose commands and HTTPS entrypoint setup
