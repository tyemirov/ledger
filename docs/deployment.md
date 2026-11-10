# Ledger deployment

This guide describes the repository's MPR hosted profile and operator lifecycle.
Use [setup](setup.md) for a local runtime or a standalone source build.
The configuration below describes tracked intent. Verify publication and runtime state separately for each operation.

## Architecture

```text
Browser --> GitHub Pages frontend
   |
   | credentialed HTTPS
   v
Caddy API route --> TAuth authentication and Ledger HTTP control plane
                                               |
Application client --> private gRPC ------------+--> SQLite or PostgreSQL
```

One Ledger process serves separate HTTP and gRPC listeners.
The browser workspace manages UserAccount, tenant, and credential resources.
Application clients do accounting operations through tenant-authenticated gRPC.

## Lifecycle commands

Install the released `mprlab-gateway` runtime and add its command directory to `PATH`.
Use `MPRLAB_GATEWAY_EXECUTABLE` to select an explicit installed command path.
Keep operator inventory and private configuration under `MPRLAB_GATEWAY_OPERATOR_ROOT`.
The default operator root is `$HOME/.config/mprlab-gateway`.

Run each command when its lifecycle phase is authorized:

```bash
make release
make publish
make deploy
```

Each Make wrapper passes the application Git root through `--app-root`.
Release prepares artifacts. Publication exposes those artifacts. Deployment activates the published artifacts.
Gateway owns release sealing, immutable publication, deployment convergence, and retry verification.
The manifest enables `make ci`, which requires the Go and Node.js development tools described in [setup](setup.md#development-tools).

The repository contract requires retry-safe lifecycle operations.
An exact release retry verifies the release without a version change or rebuild.
Publication creates missing state, verifies matching immutable state, and rejects conflicts.
Deployment verifies the published image and repeats the convergent Gateway transaction.

Use `make test-installed-gateway` to verify the Make wrappers.
Use `make test-gateway-plan` to plan the tracked source through the installed Gateway.
The planner check commits a disposable Git fixture. It does not publish or deploy the application.

## Selected manifest

[.mprlab/deploy/resources.yml](../.mprlab/deploy/resources.yml) is the only tracked production declaration.
It uses the versionless contract with `owner`, `release`, `lifecycle`, `artifacts`, `publications`, and `targets`.
Its release policy selects SemVer.

The manifest declares a multi-platform Go container and a static frontend bundle.
It publishes the image to `ghcr.io/tyemirov/ledger`.
It publishes the frontend to the `gh-pages` branch in `tyemirov/ledger`.
The service uses one placement in the `gateway` group and the retained `ledger-data` volume.
The manifest retires `mprlab-nginx-gateway/ledger-api`. The declaration leaves the old volume untouched.

### Environment inputs

The selected manifest declares `environment.source: .mprlab/deploy/.env`.
The path is relative to the application repository. The file is ignored by Git and excluded from the Docker build context.

The `environment.source` field is optional in the Gateway contract.
With no selected source file, Gateway uses process values only.
When both sources supply an input, the process value takes precedence.
An unreadable selected file rejects the manifest, even when process values exist.

The current manifest declares these private inputs:

- `DATABASE_URL`
- `TAUTH_GOOGLE_CLIENT_ID`
- `TAUTH_JWT_SIGNING_KEY`
- `DEPLOY_SUDO_PASSWORD_TUTOSH`
- `MPRLAB_TAUTH_MANAGEMENT_URL`
- `MPRLAB_TAUTH_PROVISIONING_CREDENTIALS`

The last two inputs remain declared. The current manifest has no TAuth tenant resource that provisions a tenant.
Configure the selected TAuth tenant through its own management contract.
The Ledger service receives the database URL, Google client ID, and signing key from the corresponding inputs.
It receives the tenant ID, cookie name, and public origins as tracked fixed values.

Gateway reads manifest inputs once at the manifest boundary.
It validates configuration before CI, builds, host contact, or provider effects.
It supplies the validated configuration to subsequent operations.
A missing required input stops the operation.
Keep private input values outside tracked artifacts and diagnostics.

## Hosted profile

| Component | Tracked value |
| --- | --- |
| Frontend | `https://ledger.mprlab.com` |
| API and authentication | `https://ledger-api.mprlab.com` |
| Public health endpoint | `https://ledger-api.mprlab.com/healthz` |
| TAuth issuer | `tauth` |
| TAuth tenant | `ledger` |
| Session and refresh cookies | `ledger_session`, `ledger_refresh` |
| Cookie domain | `ledger-api.mprlab.com` |

Caddy routes `/auth` and `/me` to `tauth.http`.
It routes `/api`, `/config-ui.yaml`, and `/healthz` to `ledger.http`.
The browser sends credentialed requests to the API origin.
Ledger and TAuth allow the exact frontend origin.
The manifest fixes `LEDGER_PUBLIC_ORIGIN` to the frontend origin and `TAUTH_URL` to the API origin.

The browser exchanges the Google Identity Services credential at `/auth/google` on the API origin.
This flow uses a credential exchange, rather than a Google redirect callback.
The selected Google web client must authorize `https://ledger.mprlab.com` as a JavaScript origin.
The recorded client ID is `611549676198-d8800qv64voofseor1qod1euto5duivu.apps.googleusercontent.com`.
Verify the actual selected client against the `TAUTH_GOOGLE_CLIENT_ID` input for the operation.

### Private endpoints

The manifest declares same-host `ledger.grpc` and `ledger.http` capabilities.
It also declares `ledger.grpc.private-lan` and `ledger.http.private-lan`.
The private LAN host ports are 50051 for gRPC and 18082 for HTTP.
The public browser route exposes the HTTP control plane. The gRPC endpoint remains private.

The service readiness check uses HTTP `/healthz`. The gRPC endpoint declaration uses a TCP health check.

## Database history and acceptance

The service accepts the canonical UserAccount schema and rejects an obsolete `accounts` table before mutation.
The one-time production migration completed on September 15, 2026, according to I029.
The migration preserved 15 accounts, 53 entries, and balances across three tenants.
The temporary migration command was removed after verification.

The issue records separate source implementation, package publication, deployment, and public acceptance.
F004 records the public embedded adapter and its SQLite integration evidence. Package publication remains separate.
I026 records pending hosted deployment and client acceptance.
The earlier I025 and F002 status markers remain inconsistent with later migration evidence in I029.
Use current operator evidence to record actual release, publication, deployment, browser authentication, and application balance results.

After a hosted deployment, verify the frontend and `/.mprlab-release.json` marker.
Verify API `/healthz`, browser authentication, tenant ownership, credential creation and revocation, and a client balance request.
Keep the observed artifact identity and results with the operation record.
