# ISSUES (Append-only Log)

Entries record newly discovered requests or changes, with their outcomes. No instructive content lives here. Read @NOTES.md for the process to follow when fixing issues.

Each issue is formatted as `- [ ] [<ID>-<number>]`. When resolved it becomes -` [x] [<ID>-<number>]`

## Features (100-199)

- [ ] [LG-100] Prepare a demo of a web app which uses ledger backend for transactions. A deliverable is a plan of execution.
    - Rely on mpr-ui for the backend. Use a header and a footer
    - Rely on TAuth for authentication. Usge TAuth. Mimic the demo
    - Have a simple case of 
    transaction button that takes 2 units of virtual currency
    1. enough funds -- transaction succeed
    2. not enough funds -- transaction fails
    3. enough funds after which there is 0 units of virtual currency left

    A single button which says transact with a virtual currency be 5 coins per transaction. A user gets 20 coins when an account is created. A user can buy coins at any time. once the coins are depleded, a user can no longwer transact untill a user obtains the coins

    The architecture shall be -- a backend that supports TAuth authentication by accepting the JWTs and verifying them against google service
    a backend service that integrates with Ledger and verifies that the use has sufficient balance for the transactions
    a web service, ghhtp, that serves the stand alone front end

    Find dependencies under tools folder and read their documentation and code to understand the integration. be specific in the produced plan on the intehration path forward

## Improvements (200–299)

## BugFixes (300–399)

## Maintenance (400–499)

## Planning 
do not work on the issues below, not ready
