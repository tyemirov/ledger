# Changelog

## Unreleased

### Improvements ⚙️
- [I022] Migrate Ledger's complete production declaration to the sibling gateway's schema-v3 selected-manifest contract with per-service placement, typed private values, and an explicit Docker-context exclusion for the ignored deployment input.
- Release preparation, publication, and deployment now use a repository-owned immutable container artifact and canonical app-owned runtime declaration.

### Bug Fixes 🐛
- Keep production reachability lint scoped to packages with non-test Go sources so black-box release-contract packages remain part of CI without being misclassified as dead production code.
- Make `make release`, `make publish`, and `make deploy` retry-safe: exact releases verify without version bumps or rebuilds, publication never overwrites immutable assets/tags, completed remote state remains verifiable without local staging, missing images fail with an explicit diagnostic, and every release entrypoint uses the dependency-free helper through Python 3 without requiring `uv`.

## [v1.0.4] - 2026-07-28

- Merge pull request #74 from tyemirov/bugfix/B307-idempotent-release-lifecycle
- test: assert canonical python3 runtime and invocations in release lifecycle
- fix(release): invoke release_helper.py with python3 and update shebang
- fix(release): ensure all entrypoints use Python 3 helper without uv dependency
- docs(issues): note removal of undeclared `uv` runtime dependency in release flow
- fix(release): make release, publish, and deploy safe and idempotent

## [v1.0.3] - 2026-07-28

- Merge pull request #73 from tyemirov/bugfix/B306-repository-owned-release
- Merge remote-tracking branch 'origin/master' into bugfix/B306-repository-owned-release
- chore(demo): remove .env.tauth.example from Google Client ID update script
- docs(env): clarify runtime.env.example is non-operational and update examples
- Merge pull request #72 from tyemirov/tyemirov/bugfix/B305-issues-id-format
- Merge pull request #71 from tyemirov/bugfix/B306-repository-owned-release
- Merge pull request #70 from tyemirov/tyemirov/bugfix/B305-issues-id-format
- Fix B306 repository-owned release lifecycle
- docs: rename demo plan file and update texts for clarity
- docs: update issue formatting to use section-letter IDs instead of LG- prefixes

## [v1.0.2] - 2026-04-01

### Features ✨
- Support for default environment variable fallback values in configuration files.

### Improvements ⚙️
- Moved agentic flow files to the `.mprlab` directory for better organization.
- Updated Go dependencies for improved stability and performance.
- Enhanced config loading to expand variables with default values.
- Updated README to clarify environment variables and tenant secret generation.
- Removed outdated environment and coverage files to clean up the repository.

### Bug Fixes 🐛
- _No changes._

### Testing 🧪
- Added tests for configuration variable expansion with defaults and environment overrides.

### Docs 📚
- Updated documentation and demo files to align with latest configuration changes.
- Relocated multiple documentation files under `.mprlab` directory.

## [v1.0.1] - 2026-03-31

### Features ✨
- Added mandatory per-tenant bearer token authentication for gRPC requests.
- Introduced tenant secret key configuration and validation.
- Updated demo backend to support and require ledger secret key for authentication.

### Improvements ⚙️
- Enhanced README with detailed authentication and tenant secret key usage instructions.
- Improved environment variable documentation including tenant tokens.
- Updated all example grpcurl commands to include authorization headers.
- Strengthened config validation for the ledger secret key.

### Bug Fixes 🐛
- _No changes._

### Testing 🧪
- _No changes._

### Docs 📚
- Added sections on authentication, tenant secret keys, and improved API usage examples.
- Documented environment variables for tenant secret keys and usage in demo project.
- Updated integration and API documentation to reflect secure service-to-service communication.
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
- Add ledger logging documentation
- Remove broken and symlinked demo and documentation folders
- Update README with demo compose commands and HTTPS entrypoint setup
