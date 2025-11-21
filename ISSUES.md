# ISSUES (Append-only Log)

Entries record newly discovered requests or changes, with their outcomes. No instructive content lives here. Read @NOTES.md for the process to follow when fixing issues.

Read @AGENTS.md, @AGENTS.GO.md, @AGENTS.DOCKER.md, @AGENTS.FRONTEND.md, @AGENTS.GIT.md, @POLICY.md, @NOTES.md, @README.md and @ISSUES.md. Start working on open issues. Work autonomously and stack up PRs.

Each issue is formatted as `- [ ] [LG-<number>]`. When resolved it becomes -` [x] [LG-<number>]`

## Features (110-199)

- [x] [LG-110] Prepare a demo of a web app which uses ledger backend for transactions. 
    - Rely on mpr-ui for the backend. Use a header and a footer. Use mpr-ui declarative syntax. Find mpr-ui for the references under @tools/mpr-ui. Only use CDN links in the actuall code and never reference tools folder which is gitignored.
    - Rely on TAuth for authentication. Find TAuth for the references under @tools/TAuth. Only use github reference in the actuall code and never reference tools folder which is gitignored.
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
    
    Read the docs and follow the docs under docs/TAuth/usage.md and docs/mpr-ui/custom-elements.md, docs/mpr-ui/demo-index-auth.md, @docs/mpr-ui/demo/.

    1. Build a demo page that copies the @docs/mpr-ui/demo
    2. Add ledger service to the docker orchestration
    3. Wire ledger service to operate with transactions as described above
    A deliverable is a plan of execution. The plan is a series of open issues in @ISSUES.md
    Non-deliverable: code changes
    - Plan delivered via LG-111..LG-113 below.

- [ ] [LG-111] Build the static demo UI with mpr-ui + TAuth and hook it to the ledger demo API.
    - Create `demo/ui` assets (HTML/CSS/JS) that load CDN `mpr-ui.css`/`mpr-ui.js`, GIS, and TAuth `auth-client.js` in the documented order; avoid referencing `tools/` in shipped markup.
    - Configure `<mpr-header>`/`<mpr-footer>` using the TAuth endpoints (`/auth/nonce`, `/auth/google`, `/auth/logout`, `/me`) and Google Web Client ID injected from config so the UI signs in via cookies only.
    - Implement wallet views that call `demoapi` endpoints (`/api/bootstrap`, `/api/wallet`, `/api/transactions`, `/api/purchases`) to cover the three required flows: spend succeeds at 5 coins, spend rejected when balance <5, and zero-balance after exhausting 20-coin seed + top-ups.
    - Surface status/history in the UI (balance cards, entry list, banners) using declarative markup, constants, and event wiring per `docs/mpr-ui/demo-index-auth.md` and `docs/mpr-ui/custom-elements.md`.
    - Piggyback on the proven `tools/mpr-ui/demo/tauth-demo.html` patterns for script ordering, auth-client loading, and header config; adapt for the ledger UI without importing from `tools/` at runtime.
    - Keep the ledger service agnostic: build a new demo backend and move all demo interactions from the existing `cmd/demoapi`/`internal/demoapi` façade so the new backend so that UI never couples directly to ledger gRPC.

- [ ] [LG-112] Fix and extend the Docker/ops scaffolding for the demo stack.
    - Add the missing `demo` directory with env templates (`demo/.env.tauth.example`, `.env.demoapi`) and config script to pass the Google client ID/base URL to the front end without editing HTML.
    - Update `docker-compose.demo.yml` to mount `demo/ui`, align ports (`ledgerd` host mapping, demoapi 9090, tauth 8080, ghttp 8000), and ensure volumes persist SQLite data for ledger and TAuth.
    - Ensure CORS/cookie settings stay consistent across TAuth and demoapi (issuer, cookie name, signing key) and that the UI pulls `auth-client.js` from the TAuth container.
    - Refresh `README.md`/`docs/demo.md` with end-to-end run instructions (compose + manual Go flow), artifact locations, and required secrets.
    - Mirror the compose + env wiring used in `tools/mpr-ui/docker-compose.tauth.yml` where applicable so contributors recognize the flow and avoid duplicating configuration logic.

- [ ] [LG-113] Add automated coverage for the auth + wallet demo flows.
    - Introduce end-to-end tests (Playwright or equivalent) that exercise login via TAuth, bootstrap grant, 5-coin spend success, insufficient funds rejection, zero-balance state, and purchase top-up using the demo UI.
    - Provide a test harness that seeds TAuth with a deterministic signing key and uses stub GIS credentials (or a fake token path) so tests run offline while still validating the sessionvalidator + cookie flow.
    - Wire the suite into `make test`/`make ci` so coverage stays ~100% and failures block merges; document how to run the tests locally and in CI without external network calls.

## Improvements (200–299)

- [x] [LG-201] Align naming to "ledger" across services and binaries.
    - Ensure the core daemon is referred to as `ledger`/`ledgerd` (not `creditd`) in docs, binaries, and orchestrators.
    - Sweep CLI flags, README, Compose files, and sample commands to use ledger-centric naming while keeping functionality unchanged.
    - Keep gRPC API package paths stable unless a broader migration is planned; focus on user-facing labels and process names.

## BugFixes (301–399)

## Maintenance (407–499)

## Planning 
do not work on the issues below, not ready
