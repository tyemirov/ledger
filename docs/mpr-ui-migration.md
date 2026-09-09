# Ledger Shared UI Migration

Tracking issue: B003. Shared dependency: mpr-ui I009.

## Source Contract

Ledger uses literal `mpr-ui@latest` URLs for its three shared assets.
The Go HTTP producer emits the Google provider under `auth.providers.google`.
Apple and password remain disabled.
The existing provider identifiers and endpoint paths stay under the Ledger configuration owner.

The footer uses the shared `menu` contract.
It retains both documentation links and all four theme selections.
The browser client uses `MPRUI.authenticatedFetch` for protected API requests.
Ledger validates authentication before domain work, so mutation recovery can repeat an unauthorized request.
The workspace releases its loading overlay after authentication recovery.

## Candidate Qualification

The test candidate is `bbc21cc264d96b51195e0c1a264ad43f14a205ad`.
`web/shared-ui-candidate.js` records and verifies all asset SHA-256 values.
The test cache stays under `web/node_modules`.
The production asset URLs remain unchanged.

Run `make test-shared-ui` for the focused browser checks.
Run `make ci` after the last stack change.

The browser checks compile and start the real Ledger CLI with SQLite.
They load the real page, config producer, shared UI, browser client, and protected API.
Controlled Google and TAuth responses establish a signed test session through the login response.
The tests cover login, restoration, read recovery, mutation recovery, logout, and the footer at 390 and 1280 pixels.
The mutation check proves one persisted tenant after one rejected request and its successful repeat.
These controlled checks do not establish live Google or TAuth acceptance.

The first candidate run failed because the documentation menu was absent.
A subsequent run found that the loading overlay blocked logout after authentication recovery.
The source changes correct both failures.

Local `make ci` passed on September 9, 2026.
It passed static analysis, the Go coverage gate, eight browser scenarios, local lifecycle checks, and the Pages artifact check.
The validation log is `/tmp/ledger-b003-ci-final.log`.

## Related Working Changes

F002 has separate, uncommitted Pages, profile, CORS, deployment, and API changes in the primary checkout.
B003 reuses the existing nested producer change and YAML script correction.
B003 does not include the remaining F002 changes.
Local checks use the retained primary checkout.
Hosted checks must qualify the exact B003 commit separately.
F002 must qualify its complete Pages and API release unit before deployment.

## Public Acceptance

`mpr-ui-public-assets-2026-09-09.json` records the public HTTPS inspection.
The Ledger website returned a certificate hostname mismatch.
The API health endpoint returned a TLS handshake error.
No public runtime or cache result is established for Ledger.

1. Complete the F002 Pages and API source changes.
2. Qualify the same final library candidate used by all I009 consumers.
3. Prepare the Ledger maintenance procedure and measure browser and CDN cache behavior.
4. Obtain the user-selected publication window through I009.
5. Have the user publish the shared library and deploy the approved Ledger release unit.
6. Verify valid TLS for `ledger.mprlab.com` and `ledger-api.mprlab.com`.
7. Verify the public Pages release marker, config, and literal shared asset URLs.
8. Complete real Google login, restoration, protected workspace actions, and logout.
9. Record the deployed release identity and public browser results.
10. Close B003 only after its publication and public acceptance gates pass.
