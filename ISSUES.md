# ISSUES (Append-only Log)

Entries record newly discovered requests or changes, with their outcomes. No instructive content lives here. Read @NOTES.md for the process to follow when fixing issues.

Read @AGENTS.md, @AGENTS.GO.md, @AGENTS.DOCKER.md, @AGENTS.FRONTEND.md, @AGENTS.GIT.md, @POLICY.md, @NOTES.md, @README.md and @ISSUES.md. Start working on open issues. Work autonomously and stack up PRs.

Each issue is formatted as `- [ ] [LG-<number>]`. When resolved it becomes -` [x] [LG-<number>]`

## Features (114–199)

## Improvements (206–299)

- [ ] [LG-204] (P1) Extract ledger core into a reusable Go library.
  - Promote `internal/credit` into a public `pkg/ledger` module with explicit domain types and invariants.
    - Define a storage interface suitable for both in-process and service-hosted deployments.
    - Provide a default SQL-backed implementation (adapting existing gorm stores) while keeping the core domain independent of GORM.
- [ ] [LG-205] (P2) Add integration documentation for service and library usage.
  - Document how to run ledger as a standalone gRPC microservice (config, migrations, networking) and how to consume it from other languages.
    - Document how to embed the future `pkg/ledger` library in Go services, including storage wiring, transaction patterns, and error contracts.


## BugFixes (302–399)

## Maintenance (400–499)

## Planning (500–599)
*do not implement yet*

