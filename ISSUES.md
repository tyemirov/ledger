# ISSUES (Append-only Log)

Entries record newly discovered requests or changes, with their outcomes. No instructive content lives here. Read @NOTES.md for the process to follow when fixing issues.

Read @AGENTS.md, @AGENTS.GO.md, @AGENTS.DOCKER.md, @AGENTS.FRONTEND.md, @AGENTS.GIT.md, @POLICY.md, @NOTES.md, @README.md and @ISSUES.md. Start working on open issues. Work autonomously and stack up PRs.

Each issue is formatted as `- [ ] [LG-<number>]`. When resolved it becomes -` [x] [LG-<number>]`

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
    
    Read the docs and follow the docs under docs/TAuth/usage.md and docs/mpr-ui/custom-elements.md, docs/mpr-ui/demo-index-auth.md, @docs/mpr-ui/demo/.

    1. Build a demo page that copies the @docs/mpr-ui/demo
    2. Add ledger service to the docker orchestration
    3. Wire ledger service to operate with transactions as described above
    
    
    - 2025-11-17: Authored `demo/docs/lg-100-demo-plan.md`, outlining the multi-service architecture (TAuth + demo HTTP API + ledgerd + ghttp) plus the UI/backend tasks, endpoints, and testing strategy required for LG-101.

- [x] [LG-101] Build the demo transaction API service described in `demo/docs/lg-100-demo-plan.md`.
    - Added `demo/backend/cmd/walletapi` + `demo/backend/internal/walletapi` with Cobra/Viper config, zap logging, CORS, and TAuth session validation plus an insecure gRPC dialer for local development.
    - Wired the ledger client for `Grant`, `Spend`, `GetBalance`, and `ListEntries` with per-request timeouts plus error mapping for duplicate idempotency and insufficient funds.
    - Exposed `/api/session`, `/api/bootstrap`, `/api/wallet`, `/api/transactions`, and `/api/purchases`, including idempotent bootstrap grants and automatic wallet responses.

- [x] [LG-102] Ship the declarative front-end bundle under `demo/frontend/ui` per `demo/docs/lg-100-demo-plan.md`.
    - Authored `demo/frontend/ui/index.html`, `styles.css`, and `app.js` that load `mpr-ui`, TAuth’s auth-client, Alpine, and GIS in the documented order.
    - Implemented wallet metrics, the 5-coin transaction button, purchase controls, and ledger history with toast/status banners for the three core scenarios.
    - Used the auth-client callbacks to bootstrap the wallet, fetch balances, and call the new API endpoints with credentialed fetch helpers.

- [x] [LG-103] Provide hosting/orchestration and local tooling for the demo stack (`demo/docs/lg-100-demo-plan.md` “Hosting with ghttp” + “Local Orchestration / Compose”).
    - Added `demo/backend/Dockerfile`, `demo/backend/.env.walletapi.example`, `demo/.env.tauth.example`, and `demo/docker-compose.yml` so contributors can run ledgerd, TAuth, walletapi, and ghttp together.
    - Documented the ghttp workflow plus the compose steps (including env copies) in `demo/docs/demo.md` and linked the section from README.

- [x] [LG-104] Add integration tests, CI wiring, and documentation from the “Implementation Breakdown” + “Validation & Monitoring Strategy” sections of `demo/docs/lg-100-demo-plan.md`.
    - Introduced `demo/backend/internal/walletapi/server_test.go`, which spins up an in-memory ledger + HTTP stack via bufconn/httptest and asserts bootstrap, spend success, insufficient funds, and purchase scenarios.
    - Extended docs (`demo/docs/demo.md`) with the scenario checklist and manual validation steps; ensured `make test` exercises the new package while retaining coverage gates.

- [x] [LG-105] Build a fresh `demo/` package that contains both the Go HTTP façade (importing the ledger gRPC client) and the standalone front-end bundle required by `demo/docs/lg-100-demo-plan.md`. The existing materials under `tools/` are informative only—no runtime dependencies on that tree.
    - Create `demo/backend` with a new Go module (or sub-package) that compiles to `demo/backend/cmd/walletapi`. Reuse Cobra/Viper for config, wire the gRPC client to `credit.v1.CreditService`, and expose `/api/session`, `/api/bootstrap`, `/api/wallet`, `/api/transactions`, `/api/purchases` exactly as described in LG-101. All configuration (TAuth base URL, JWT key, ledger addr, timeout, allowed origins) must come from flags/env with no defaults; validation happens in `PreRunE` and the process fails fast if anything is missing.
    - Author a `demo/frontend` directory housing `index.html`, `styles.css`, and `app.js`. Reference `mpr-ui` CSS/JS and GIS via CDN URLs only; load `http://localhost:8080/static/auth-client.js` at runtime just like the production stack. Use Alpine (module import) for interactivity and replicate the wallet layout described in LG-100, but ensure the page makes zero references to `tools/mpr-ui` assets.
    - Implement a front-end config bootstrapper: fetch `/demo/config.js` from TAuth before initializing `mpr-ui`, set `<mpr-header site-id>` dynamically, and refuse to load when the response is absent. All other endpoints (login/logout/nonce) must be bound via attributes instead of hardcoded constants.
    - Build a shared `demo/frontend/walletApiClient.js` helper that wraps `fetch` to the backend service with `credentials: 'include'` and typed error handling. Methods: `fetchSession`, `bootstrapWallet`, `getWallet`, `spendCoins`, `purchaseCoins`. Each returns parsed JSON typed per LG-100 invariants (coins as integers, ledger entries with timestamps).
    - Port the LG-100 flows into `app.js`: maintain Alpine stores for auth, wallet, transactions, and status banners; disable buttons while network calls are pending; display ledger history and zero-balance warnings exactly as the plan requires. No default values—if a response is missing required fields, throw and surface an unrecoverable banner.
    - Compose a dedicated Dockerfile + docker-compose overlay under `demo/` so contributors can run `ledgerd`, the new wallet API, TAuth, and a static file server (ghttp) from one command. Ensure `.env` samples (e.g., `.env.walletapi`, `.env.tauth`) live inside `demo/` and are the only source of runtime configuration.
    - Add Playwright coverage under `demo/tests/` that drives the new front end end-to-end. Write a stub server (similar to the existing root-level harness) to intercept `/demo/config.js`, `/static/auth-client.js`, GIS, and the wallet API endpoints so the four mandatory scenarios (sign-in prompt, successful spend, insufficient funds, purchase replenishment) and the zero-balance banner are asserted. Wire these tests into `make test`.
    - Update repository docs (`README.md`, `demo/docs/demo.md`, and `demo/docs/lg-100-demo-plan.md` if necessary) to point to the new `demo/` workflow, including the commands for building/running the Docker stack and executing the Playwright suite.

- [x] [LG-106] Prepare a demo of a web app which uses ledger backend for transactions. 
    - Rely on mpr-ui for the front-end. Use a header and a footer. Use mpr-ui declarative syntax
    - Rely on TAuth for authentication. Usge TAuth. Mimic the demo that mpr-ui provides under @docs/mpr-ui/demo
    - Have a simple case of 
    transaction button that takes 5 units of virtual currency
    1. enough funds -- transaction succeed
    2. not enough funds -- transaction fails
    3. enough funds after which there is 0 units of virtual currency left

    A single button which says transact with a virtual currency be 5 coins per transaction. A user gets 20 coins when an account is created. A user can buy coins at any time. once the coins are depleded, a user can no longwer transact untill a user obtains the coins

    The architecture shall be -- a backend that supports TAuth authentication (uses a client that TAuth exposes) by accepting the JWTs and verifying them against google service
    a backend service that integrates with Ledger and verifies that the use has sufficient balance for the transactions
    a web service that serves the stand alone front end (if we need to combine multiple services we may need to use nginx)

    Find dependencies documentation and code to understand the integration under docs/. Read the docs and follow the docs under docs/TAuth/usage.md and docs/mpr-ui/custom-elements.md, docs/mpr-ui/demo-index-auth.md, @docs/mpr-ui/demo/.

    1. Build a demo page that copies the @docs/mpr-ui/demo
    2. Add ledger service to the docker orchestration
    3. Wire ledger service to operate with transactions as described above

    The deliverables are:
    a stand-alone demo folder that contains
    1. new demo app front-end and backend
    2. docker-compose orchestration of all the dependent services

    Restrictions: no file-level references can be made outside of the demo/ folder
    - 2025-11-20: Implemented the wallet UI under `demo/ui/` (Alpine stores + `wallet-api-client.js`) with Spend/Purchase controls, ledger history, zero-balance/insufficient banners, and updated docs (`demo/README.md`, `demo/docs/demo.md`, `demo/docs/lg-106-demo-plan.md`). The UI now reads the demo API origin from `data-api-base-url` and the Compose workflows exercise all three LG-106 scenarios.
    
## Improvements (200–299)

## BugFixes (300–399)

- [x] [LG-308] The refresh does log out after log in.
Here is a working solution, end to end
```
roots[4]:
  - path: /Users/tyemirov/Development/MarcoPoloResearchLab/mpr-ui/demo/tauth-demo.html
    name: tauth-demo.html
    type: file
    size: 6.9kb
    lastModified: "2025-11-20 00:54"
    mimeType: "text/html; charset=utf-8"
    content: "<!DOCTYPE html>\n<html lang=\"en\">\n  <head>\n    <meta charset=\"utf-8\" />\n    <meta name=\"viewport\" content=\"width=device-width, initial-scale=1.0\" />\n    <title>TAuth + mpr-ui (Docker Compose)</title>\n    <link\n      rel=\"stylesheet\"\n      href=\"https://cdn.jsdelivr.net/gh/MarcoPoloResearchLab/mpr-ui@latest/mpr-ui.css\"\n    />\n    <script\n      defer\n      src=\"https://cdn.jsdelivr.net/npm/alpinejs@3.13.5/dist/module.esm.js\"\n      type=\"module\"\n    ></script>\n    <script\n      defer\n      src=\"http://localhost:8080/static/auth-client.js\"\n      crossorigin=\"anonymous\"\n    ></script>\n    <script\n      id=\"mpr-ui-bundle\"\n      defer\n      src=\"https://cdn.jsdelivr.net/gh/MarcoPoloResearchLab/mpr-ui@latest/mpr-ui.js\"\n    ></script>\n    <script src=\"https://accounts.google.com/gsi/client\" async defer></script>\n    <style>\n      :root {\n        --card-background: rgba(255, 255, 255, 0.9);\n        --card-border: rgba(0, 0, 0, 0.1);\n      }\n      body.theme-dark {\n        --card-background: rgba(18, 21, 23, 0.8);\n        --card-border: rgba(255, 255, 255, 0.2);\n      }\n      main {\n        max-width: 960px;\n        margin: 0 auto;\n        padding: 2rem 1.5rem 3rem;\n      }\n      .demo-section {\n        margin-top: 2rem;\n      }\n      .session-card {\n        border: 1px solid var(--card-border);\n        border-radius: 0.75rem;\n        padding: 1.25rem;\n        background: var(--card-background);\n        box-shadow: 0 8px 24px rgba(0, 0, 0, 0.08);\n      }\n      .session-card__profile {\n        display: flex;\n        gap: 1rem;\n        align-items: center;\n      }\n      .session-card__profile ul {\n        list-style: none;\n        padding: 0;\n        margin: 0;\n      }\n      .session-card__profile li + li {\n        margin-top: 0.25rem;\n      }\n      .session-card__avatar {\n        width: 56px;\n        height: 56px;\n        border-radius: 999px;\n        object-fit: cover;\n      }\n      .session-card__expires {\n        margin-top: 1rem;\n        font-size: 0.95rem;\n        color: var(--mpr-color-text-muted, #555);\n      }\n      .logout-button {\n        margin-top: 1rem;\n        border: 0;\n        border-radius: 999px;\n        padding: 0.75rem 1.5rem;\n        background: #0c67ff;\n        color: #fff;\n        font-weight: 600;\n        cursor: pointer;\n      }\n      .logout-button:hover {\n        background: #0842a8;\n      }\n      ol {\n        padding-left: 1.25rem;\n      }\n      ol li + li {\n        margin-top: 0.5rem;\n      }\n    </style>\n  </head>\n  <body class=\"theme-light\" data-demo-palette=\"default\">\n    <mpr-header\n      id=\"demo-header\"\n      brand-label=\"Marco Polo Research Lab\"\n      brand-href=\"https://mprlab.com/\"\n      nav-links='[\n        { \"label\": \"Docs\", \"href\": \"https://github.com/MarcoPoloResearchLab/mpr-ui/blob/master/README.md\" },\n        { \"label\": \"Architecture\", \"href\": \"https://github.com/MarcoPoloResearchLab/mpr-ui/blob/master/ARCHITECTURE.md\" }\n      ]'\n      site-id=\"991677581607-r0dj8q6irjagipali0jpca7nfp8sfj9r.apps.googleusercontent.com\"\n      base-url=\"http://localhost:8080\"\n      login-path=\"/auth/google\"\n      logout-path=\"/auth/logout\"\n      nonce-path=\"/auth/nonce\"\n      settings=\"true\"\n      settings-label=\"Settings\"\n    ></mpr-header>\n    <main>\n      <h1>Docker Compose demo</h1>\n      <p>\n        This page connects the <code>mpr-header</code> web component to a local\n        TAuth service running under Docker Compose. Sign in with Google, observe\n        the authenticated profile state, and use the logout button to clear your\n        session.\n      </p>\n\n      <section aria-labelledby=\"profile-status-title\" class=\"demo-section\">\n        <h2 id=\"profile-status-title\">Current session</h2>\n        <p>\n          Google sign-in requests are exchanged with\n          <code>http://localhost:8080</code>. The helper script provided by TAuth\n          monitors <code>/me</code> and <code>/auth/refresh</code> to keep a\n          valid browser session without storing tokens in localStorage.\n        </p>\n        <div\n          class=\"session-card\"\n          data-demo-auth-status\n          aria-live=\"polite\"\n          aria-atomic=\"true\"\n        >\n          <p>Awaiting connection to the TAuth service…</p>\n        </div>\n        <button type=\"button\" class=\"logout-button\" data-demo-logout>\n          Sign out\n        </button>\n      </section>\n\n      <section aria-labelledby=\"instructions-title\" class=\"demo-section\">\n        <h2 id=\"instructions-title\">How it works</h2>\n        <ol>\n          <li>The frontend is served by the gHTTP container on port 8000.</li>\n          <li>\n            TAuth listens on port 8080, issuing HttpOnly session cookies scoped\n            to <code>localhost</code>.\n          </li>\n          <li>\n            The header component requests <code>/auth/nonce</code> before\n            rendering the Google button and exchanges the returned credential via\n            <code>/auth/google</code>.\n          </li>\n          <li>\n            The embedded <code>auth-client.js</code> helper refreshes sessions by\n            calling <code>/auth/refresh</code> whenever a request returns 401.\n          </li>\n          <li>\n            Clicking <strong>Sign out</strong> invokes <code>/auth/logout</code>\n            and resets the profile snapshot below.\n          </li>\n        </ol>\n      </section>\n    </main>\n\n    <mpr-footer\n      id=\"page-footer\"\n      prefix-text=\"Built by Marco Polo Research Lab\"\n      privacy-link-label=\"Privacy &amp; Terms\"\n      privacy-link-href=\"#privacy\"\n      privacy-modal-content=\"\n        <h1>Privacy Policy — TAuth demo</h1>\n        <p><strong>Effective Date:</strong> 2025-10-11</p>\n        <p>The local demo exchanges your Google profile with a self-hosted TAuth instance.</p>\n        <p>No data leaves your machine, and session cookies are cleared when you stop the Compose stack.</p>\n      \"\n      links-collection='{\n        \"style\": \"drop-up\",\n        \"text\": \"Explore more demos\",\n        \"links\": [\n          { \"label\": \"Marco Polo Research Lab\", \"url\": \"https://mprlab.com\" },\n          { \"label\": \"LoopAware\", \"url\": \"https://loopaware.mprlab.com\" },\n          { \"label\": \"Gravity Notes\", \"url\": \"https://gravity.mprlab.com\" },\n          { \"label\": \"Allergy Wheel\", \"url\": \"https://allergy.mprlab.com\" },\n          { \"label\": \"Countdown Calendar\", \"url\": \"https://countdown.mprlab.com\" }\n        ]\n      }'\n      theme-switcher=\"square\"\n      theme-config='{\"attribute\":\"data-demo-theme\",\"targets\":[\"body\"],\"initialMode\":\"default-light\",\"modes\":[{\"value\":\"default-light\",\"attributeValue\":\"light\",\"classList\":[\"theme-light\"],\"dataset\":{\"data-demo-palette\":\"default\"}},{\"value\":\"sunrise-light\",\"attributeValue\":\"light\",\"classList\":[\"theme-light\"],\"dataset\":{\"data-demo-palette\":\"sunrise\"}},{\"value\":\"default-dark\",\"attributeValue\":\"dark\",\"classList\":[\"theme-dark\"],\"dataset\":{\"data-demo-palette\":\"default\"}},{\"value\":\"forest-dark\",\"attributeValue\":\"dark\",\"classList\":[\"theme-dark\"],\"dataset\":{\"data-demo-palette\":\"forest\"}}]}'\n    ></mpr-footer>\n    <script defer src=\"./status-panel.js\"></script>\n  </body>\n</html>\n"
  - path: /Users/tyemirov/Development/MarcoPoloResearchLab/mpr-ui/demo/status-panel.js
    name: status-panel.js
    type: file
    size: 3.9kb
    lastModified: "2025-11-14 23:19"
    mimeType: "text/plain; charset=utf-8"
    content: "// @ts-check\n'use strict';\n\nconst STATUS_HOST_SELECTOR = '[data-demo-auth-status]';\nconst LOGOUT_BUTTON_SELECTOR = '[data-demo-logout]';\n\n/**\n * @typedef {object} AuthProfile\n * @property {string} [display]\n * @property {string} [user_email]\n * @property {string} [avatar_url]\n * @property {string} [expires]\n * @property {string[]} [roles]\n */\n\n/**\n * Renders the session snapshot with the provided profile.\n * @param {AuthProfile | null | undefined} profile\n * @returns {void}\n */\nfunction renderSession(profile) {\n  const host = document.querySelector(STATUS_HOST_SELECTOR);\n  if (!host) {\n    return;\n  }\n  const roles = Array.isArray(profile?.roles) ? profile.roles : [];\n  const roleLabel = roles.length ? roles.join(', ') : 'user';\n  host.replaceChildren();\n  if (!profile) {\n    const title = document.createElement('h3');\n    title.textContent = 'Signed out';\n    const details = document.createElement('p');\n    details.textContent = 'Use the Google Sign-In button in the header to begin.';\n    host.append(title, details);\n    return;\n  }\n  const profileContainer = document.createElement('div');\n  profileContainer.classList.add('session-card__profile');\n  if (profile.avatar_url) {\n    const avatar = document.createElement('img');\n    avatar.classList.add('session-card__avatar');\n    avatar.src = profile.avatar_url;\n    avatar.alt = profile.display || 'Avatar';\n    profileContainer.append(avatar);\n  }\n  const list = document.createElement('ul');\n  const nameItem = document.createElement('li');\n  const nameLabel = document.createElement('strong');\n  nameLabel.textContent = 'Name:';\n  nameItem.append(nameLabel, document.createTextNode(` ${profile.display || 'Unknown'}`));\n  const emailItem = document.createElement('li');\n  const emailLabel = document.createElement('strong');\n  emailLabel.textContent = 'Email:';\n  emailItem.append(emailLabel, document.createTextNode(` ${profile.user_email || 'Hidden'}`));\n  const roleItem = document.createElement('li');\n  const roleLabelElement = document.createElement('strong');\n  roleLabelElement.textContent = 'Roles:';\n  roleItem.append(roleLabelElement, document.createTextNode(` ${roleLabel}`));\n  list.append(nameItem, emailItem, roleItem);\n  profileContainer.append(list);\n  const expiryParagraph = document.createElement('p');\n  expiryParagraph.classList.add('session-card__expires');\n  if (profile.expires) {\n    const readableExpires = new Date(profile.expires).toLocaleString();\n    const timeElement = document.createElement('time');\n    timeElement.dateTime = profile.expires;\n    timeElement.textContent = readableExpires;\n    expiryParagraph.append(\n      document.createTextNode('Current session cookie expires at '),\n      timeElement,\n      document.createTextNode('.')\n    );\n  } else {\n    expiryParagraph.textContent =\n      'Session cookie expiry unavailable (auto-refresh will keep you signed in until you sign out).';\n  }\n  const refreshParagraph = document.createElement('p');\n  refreshParagraph.classList.add('session-card__expires');\n  refreshParagraph.textContent =\n    'The refresh token keeps renewing this session in the background until you click Sign out or stop the stack.';\n  host.append(profileContainer, expiryParagraph, refreshParagraph);\n}\n\nfunction wireLogoutButton() {\n  const button = document.querySelector(LOGOUT_BUTTON_SELECTOR);\n  if (!button) {\n    return;\n  }\n  button.addEventListener('click', () => {\n    if (typeof window.logout === 'function') {\n      window.logout();\n    }\n  });\n}\n\nfunction initSessionPanel() {\n  renderSession(typeof window.getCurrentUser === 'function' ? window.getCurrentUser() : null);\n  document.addEventListener('mpr-ui:auth:authenticated', (event) => {\n    renderSession(event?.detail?.profile ?? null);\n  });\n  document.addEventListener('mpr-ui:auth:unauthenticated', () => {\n    renderSession(null);\n  });\n  wireLogoutButton();\n}\n\nif (document.readyState === 'loading') {\n  document.addEventListener('DOMContentLoaded', initSessionPanel);\n} else {\n  initSessionPanel();\n}\n"
  - path: /Users/tyemirov/Development/MarcoPoloResearchLab/mpr-ui/.env.tauth
    name: .env.tauth
    type: file
    size: 468b
    lastModified: "2025-11-20 00:16"
    mimeType: "text/plain; charset=utf-8"
    content: "# Copy this file to .env.tauth and replace placeholder values before running docker compose.\nAPP_LISTEN_ADDR=:8080\nAPP_GOOGLE_WEB_CLIENT_ID=991677581607-r0dj8q6irjagipali0jpca7nfp8sfj9r.apps.googleusercontent.com\nAPP_JWT_SIGNING_KEY=bG9jYWwtc2lnbmluZy1rZXktc2FtcGxlLXRlc3QtMTIzNDU2Nzg5MA==\nAPP_DATABASE_URL=sqlite:///data/tauth.db\nAPP_ENABLE_CORS=true\nAPP_CORS_ALLOWED_ORIGINS=http://localhost:8000,http://127.0.0.1:8000\nAPP_DEV_INSECURE_HTTP=true\nAPP_COOKIE_DOMAIN=\n\n"
  - path: /Users/tyemirov/Development/MarcoPoloResearchLab/mpr-ui/docker-compose.tauth.yml
    name: docker-compose.tauth.yml
    type: file
    size: 585b
    lastModified: "2025-11-20 00:54"
    mimeType: "text/plain; charset=utf-8"
    content: "services:\n  frontend:\n    image: ghcr.io/tyemirov/ghttp:latest\n    pull_policy: always\n    depends_on:\n      tauth:\n        condition: service_started\n    command: [\"--directory\", \"/app/demo\", \"8000\"]\n    ports:\n      - \"8000:8000\"\n    volumes:\n      - ./demo/:/app/demo\n      - ./demo/tauth-demo.html:/app/demo/index.html:ro\n    restart: unless-stopped\n\n  tauth:\n    image: ghcr.io/tyemirov/tauth:latest\n    pull_policy: always\n    env_file:\n      - ./.env.tauth\n    ports:\n      - \"8080:8080\"\n    volumes:\n      - tauth_data:/data\n    restart: unless-stopped\n\nvolumes:\n  tauth_data:\n"
summary:
  totalFiles: 4
  totalSize: 12kb
```

- [x] [LG-307] Demo UI goes blank after login; ledger flows inaccessible.
    - Observed blank page post-auth, spend button not usable; needs UI bootstrap and ledger flow verification.

- [x] [LG-300] The demo stack was renamed; all references should use the `demo/` paths.
    - Updated documentation and scripts to drop the old name; Playwright script now points to `demo/tests`.
    - Fixed `demo/docker-compose.yml` to reference `demo/backend/Dockerfile` and adjusted that Dockerfile to build from `./demo/backend/cmd/walletapi`.
    - `docker compose -f demo/docker-compose.yml config` now resolves without path errors; `make test` (Go + Playwright) passes.

- [x] [LG-301] I am unable to log into demo. Fix the login first with complete disregard to ledger server (remove all ledger-related functionality and just ensure we have a stable login).
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

    - 2025-11-19: Added `demo/ui/` auth-only page that loads TAuth config from `/demo/config.js`, points `<mpr-header>` at `http://localhost:8080`, and initializes `auth-client.js` dynamically so `/auth/nonce` requests hit TAuth instead of the static host. Included `.env.tauth.example` plus compose profiles in `demo/docker-compose.yml` so contributors can run either the auth-only flow (`--profile auth`) or the full demo (`--profile demo`) using a single file with only internet-sourced dependencies.
    - 2025-11-20: Hardened the demo auth flow by requiring `/demo/config.js` to load before rendering, loading `auth-client.js` from the configured TAuth origin, adding GIS + single Alpine module bootstrap, extending the refresh Playwright stub to assert nonce/cookie handling and reload persistence, and softening wallet error handling to flag demo API network issues instead of clearing auth. `make lint` and `make test` pass with the updated front-end bundle.
```

## Maintenance (400–499)

- [x] [LG-400] Review @POLICY.md and verify what code areas need improvements and refactoring. Prepare a detailed plan of refactoring. Check for bugs, missing tests, poor coding practices, uplication and slop. Ensure strong encapsulation and following the principles og @AGENTS.md, @AGENTS.GO.md and policies of @POLICY.md
    - 2024-11-25 audit summary:
        - Domain logic violates POLICY invariants: operations accept raw primitives without smart constructors or edge validation (`internal/credit/types.go`, `internal/grpcserver/server.go`), timestamps default to zero in `NewService`, and `ListEntries`/`Balance` create accounts on read.
        - Reservation flows are incorrect: holds are never reversed (capture/release only check existence and write zero entries), `reservation_id` is not unique in `db/migrations.sql`, and limits/defaults for listing are unbounded, allowing stale holds to permanently lock funds.
        - Operational drift: duplicate stores (gorm vs pgx) with the binary pinned to GORM + AutoMigrate (`cmd/ledger/main.go`), no tests of any kind (`go test ./...` reports “[no test files]”), Docker lacks the mandated `.env.ledgersvc`, and the distroless runtime runs as non-root (`Dockerfile`).
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
    - Delete AutoMigrate from `cmd/ledger`, ensure migrations remain SQL-first, and verify startup gracefully handles dependency errors.
    - 2024-11-25: Deleted the unused GORM store, rewired `cmd/ledger` to use pgstore exclusively with Cobra/Viper config handling, added zap-powered logging plus graceful shutdown, and kept Docker/env compatibility intact (SQL migrations remain source of truth).
- [x] [LG-404] Testing, CI, and container compliance.
    - Author black-box integration tests covering grant/reserve/capture/release/spend/list flows plus store-specific tests for pagination and idempotency.
    - Wire `make test`, `make lint`, and `make ci` (or equivalent) to run gofmt/go vet/staticcheck/ineffassign + coverage enforcement per POLICY.
    - Align Docker assets with AGENTS.DOCKER: add `.env.ledgersvc`, reference it from docker-compose, ensure containers run as root, and document the workflow.
    - 2024-11-25: Added Makefile targets (`fmt`, `lint`, `test`, `ci`) that run gofmt/vet/staticcheck/ineffassign with an 80% coverage gate on the internal domain package, expanded the service tests to cover balance/grant/spend/list flows, introduced `.env.ledgersvc` with docker-compose env_file wiring, switched the runtime image to rootful Debian, and documented the tooling workflow in README.
- [x] [LG-405] Switch to sqlite from postgres. Prepare the code that allows to pass the DB URIL sufficient for the GORM to either use a postgres or sqlite driver. ensure that the sqlite driver doesnt require GCO
    - 2024-11-25: Reintroduced the GORM store with reservation support, added a CGO-free SQLite driver alongside the Postgres driver, taught `ledgerd` to parse the `DATABASE_URL` and pick the right driver (defaulting to SQLite with AutoMigrate), simplified Docker to a single service storing data in an `.env.ledgersvc`-defined SQLite path, and updated README/documentation accordingly.

- [x] [LG-406] Establish github workflows for testing and docker image release. Use an example under @docs/workflow for inspiration

## Planning 
do not work on the issues below, not ready
