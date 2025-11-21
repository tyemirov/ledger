# ISSUES (Append-only Log)

Entries record newly discovered requests or changes, with their outcomes. No instructive content lives here. Read @NOTES.md for the process to follow when fixing issues.

Read @AGENTS.md, @AGENTS.GO.md, @AGENTS.DOCKER.md, @AGENTS.FRONTEND.md, @AGENTS.GIT.md, @POLICY.md, @NOTES.md, @README.md and @ISSUES.md. Start working on open issues. Work autonomously and stack up PRs.

Each issue is formatted as `- [ ] [LG-<number>]`. When resolved it becomes -` [x] [LG-<number>]`

## Features (110-199)

- [ ] [LG-110] Prepare a demo of a web app which uses ledger backend for transactions. 
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

## Improvements (200–299)

## BugFixes (301–399)

## Maintenance (407–499)

## Planning 
do not work on the issues below, not ready
