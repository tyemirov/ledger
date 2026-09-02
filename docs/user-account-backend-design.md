# UserAccount Backend Design

## Status

This document defines the canonical backend architecture for P001. The backend implementation follows this contract.

The exact hosted TAuth and route profile and the production migration owner mapping remain operator inputs. Ledger does not infer these values.

## Product Goal

UserAccount gives one authenticated person a Ledger-owned identity. Each UserAccount can own any number of Ledger tenants.

A Ledger tenant is a customer workspace. A Ledger tenant is not a TAuth tenant.

The backend stores tenant ownership. Static configuration does not define customer tenants in the target contract.

## Previous Contract

Ledger exposed a private gRPC data plane. The data plane performed credit operations against PostgreSQL or SQLite.

The previous `accounts` table stored Ledger accounts. Each Ledger account used a tenant ID, user ID, and ledger ID.

The previous runtime loaded tenant IDs and plaintext tenant secrets from configuration. Tenant creation required a configuration change and a service restart.

The previous backend had no UserAccount resource. It also had no tenant ownership, tenant management API, or persistent tenant credential registry.

The previous demo validated TAuth sessions for demo routes. The demo sent all authenticated users to one configured Ledger tenant.

B001 recorded a separate defect in the previous `Batch` authentication path.

## Confirmed Requirements

- Let one authenticated person provision one UserAccount.
- Let one UserAccount create any number of Ledger tenants.
- Do not apply a product tenant count limit.
- Persist each UserAccount and each Ledger tenant.
- Persist the owner relationship for each Ledger tenant.
- Authorize every tenant operation against the authenticated UserAccount.
- Keep TAuth and `mpr-ui` as the browser authentication authority.
- Keep authentication sessions outside the Ledger domain.
- Keep the accounting library application-agnostic.
- Keep the gRPC accounting API as the private data plane.
- Use one forward-only canonical contract.
- Remove the static tenant registry after the bounded data migration.

## Non-Goals

- Do not implement login, logout, session refresh, or password management in Ledger.
- Do not make Ledger an identity provider.
- Do not move credit operations to the control plane.
- Do not add tenant sharing, invitations, or roles without a confirmed requirement.
- Do not add tenant deletion without a confirmed retention contract.
- Do not add a legacy tenant authentication path.
- Do not preserve static tenant configuration after migration.

## Domain Terms

UserAccount is the Ledger identity for one authenticated TAuth user. It owns Ledger tenant resources.

TAuth tenant is the authentication realm for the Ledger application. Its ID comes from the TAuth session and application configuration.

Ledger tenant is a customer workspace inside Ledger. A UserAccount can own many Ledger tenants.

Ledger account is the current credit namespace. One tenant ID, one user ID, and one ledger ID identify it.

Tenant credential authenticates an application client for one Ledger tenant. It does not authenticate a browser user.

## System Boundaries

```text
Browser
  |
  | TAuth session
  v
Ledger control plane ----> UserAccount service ----> UserAccount store
  |                               |
  |                               +-------------> Ledger tenant store
  |
  +---- validates sessions through the published TAuth validator

Application client
  |
  | tenant credential
  v
Ledger gRPC data plane --> accounting service ------> Ledger account store
```

The control plane manages UserAccount and Ledger tenant resources. It validates TAuth sessions before resource authorization.

The data plane performs credit operations. It resolves a tenant credential to one Ledger tenant before an accounting operation.

Both planes use the same database and canonical tenant records. They do not duplicate tenant ownership or credential state.

The reusable `pkg/ledger` package keeps accounting rules only. UserAccount and browser authorization remain in internal application packages.

The recommended runtime uses separate HTTP and gRPC listeners in `ledgerd`. The HTTP listener is public only through the selected gateway route.

The gRPC listener remains private. Both listeners use one dependency graph, one database pool, and one coordinated shutdown sequence.

## Identity Contract

Ledger accepts only a session from the configured TAuth tenant. The published TAuth session validator verifies the signature, issuer, and expiration.

The control plane also validates the session tenant ID and user ID. It rejects an empty or unexpected identity claim.

The canonical external identity contains these values:

- TAuth issuer.
- TAuth tenant ID.
- TAuth user ID.

Ledger creates one UserAccount for each unique external identity. A unique database constraint enforces this rule.

Ledger does not store an email address, display name, avatar URL, session token, refresh token, or TAuth password.

TAuth profile data remains TAuth data. A browser reads profile data through the shared authentication surface.

TAuth roles do not grant Ledger tenant ownership. Ledger authorizes ownership from its own durable records.

## Persistence Model

### `user_accounts`

| Column | Type | Contract |
| --- | --- | --- |
| `user_account_id` | UUID | Primary key generated by Ledger. |
| `auth_issuer` | text | Validated TAuth issuer. |
| `auth_tenant_id` | text | Validated TAuth tenant ID. |
| `auth_user_id` | text | Validated TAuth user ID. |
| `created_at` | UTC timestamp | Immutable creation time. |

Use one unique constraint on `auth_issuer`, `auth_tenant_id`, and `auth_user_id`.

### `tenants`

| Column | Type | Contract |
| --- | --- | --- |
| `tenant_id` | UUID | Primary key generated by Ledger. |
| `owner_user_account_id` | UUID | Required foreign key to `user_accounts`. |
| `name` | text | Required human-readable tenant name of at most 80 Unicode characters. |
| `created_at` | UTC timestamp | Immutable creation time. |

Use a restrictive owner foreign key. Do not remove a UserAccount while it owns a Ledger tenant.

### `idempotency_records`

| Column | Type | Contract |
| --- | --- | --- |
| `user_account_id` | UUID | UserAccount that made the request. |
| `operation` | text | Canonical creation operation. |
| `key_digest` | bytes | Digest of the idempotency key. |
| `request_digest` | bytes | Digest of the canonical request. |
| `resource_id` | UUID | Resource that the first request created. |
| `created_at` | UTC timestamp | Immutable creation time. |

Use one unique constraint on `user_account_id`, `operation`, and `key_digest`.

The same key and request return the original resource. The same key and a different request return a conflict.

### `tenant_credentials`

This table replaces the static tenant credential registry.

| Column | Type | Contract |
| --- | --- | --- |
| `credential_id` | UUID | Public credential identifier. |
| `tenant_id` | UUID | Required foreign key to `tenants`. |
| `secret_digest` | bytes | Digest of the generated secret. |
| `created_at` | UTC timestamp | Immutable creation time. |
| `revoked_at` | UTC timestamp | Null until revocation. |

Ledger returns a generated credential secret only once. Ledger never stores or logs the plaintext secret.

The credential wire format is `ledger_<credential_uuid>_<base64url_secret>`. The secret contains 256 random bits. Ledger stores the SHA-256 digest of the secret component.

### `ledger_accounts`

Rename the current `accounts` table to `ledger_accounts`. This name prevents confusion with UserAccount.

Keep the unique tenant ID, user ID, and ledger ID constraint. Add a foreign key from tenant ID to `tenants`.

Keep ledger entries and reservations subordinate to one Ledger account. Add explicit foreign keys for both tables.

### `control_events`

Control events provide an append-only record for security-sensitive control plane changes.

| Column | Type | Contract |
| --- | --- | --- |
| `event_id` | UUID | Primary key generated by Ledger. |
| `actor_user_account_id` | UUID | UserAccount that caused the event. |
| `event_type` | text | Canonical event type. |
| `resource_type` | text | Canonical resource type. |
| `resource_id` | UUID | Addressed resource. |
| `request_id` | text | Request correlation identifier. |
| `created_at` | UTC timestamp | Immutable event time. |

Do not put TAuth claims, session values, tenant secrets, or request bodies in a control event.

## Control Plane API

The recommended control plane is a resource-oriented HTTP API. It does not mirror the accounting gRPC API.

All routes require a valid TAuth session. Each state-changing request also requires the selected cross-site request protection.

Use one OpenAPI document as the canonical control plane contract. Validate the server and each repository client against that document.

| Method | Resource | Result |
| --- | --- | --- |
| `PUT` | `/api/user-account` | Provision the current UserAccount idempotently. |
| `GET` | `/api/user-account` | Read the current UserAccount. |
| `POST` | `/api/tenants` | Create one owned Ledger tenant. |
| `GET` | `/api/tenants` | List owned Ledger tenants with cursor pagination. |
| `GET` | `/api/tenants/{tenant_id}` | Read one owned Ledger tenant. |
| `POST` | `/api/tenants/{tenant_id}/credentials` | Create one revocable tenant credential. |
| `GET` | `/api/tenants/{tenant_id}/credentials` | List credential metadata without secrets. |
| `DELETE` | `/api/tenants/{tenant_id}/credentials/{credential_id}` | Revoke one credential. |

No UserAccount or tenant update or delete route is part of the confirmed scope.

### UserAccount Provisioning

`PUT /api/user-account` uses only the validated TAuth identity. The request does not accept a user ID or UserAccount ID.

The first request creates the UserAccount and returns `201 Created`. A repeated request returns `200 OK` with the same representation.

The operation remains safe across concurrent requests. The unique external identity constraint selects one canonical UserAccount.

`GET /api/user-account` returns `404 Not Found` when the UserAccount does not exist. A safe read never provisions state.

### Tenant Creation

`POST /api/tenants` requires an `Idempotency-Key` header. The server takes the owner from the authenticated UserAccount.

The request contains one `name` field and never accepts an owner ID. The transaction creates the tenant, idempotency record, and control event together.

The response returns `201 Created` and a `Location` header. A valid retry returns the original tenant representation.

### Tenant Reads

Tenant reads always include the authenticated owner ID in the database query. A route never loads a tenant before owner authorization.

An absent tenant and a tenant owned by another UserAccount both return `404 Not Found`. This behavior prevents tenant enumeration.

The tenant collection uses cursor pagination. The canonical order is creation time followed by tenant ID.

The API bounds each page size. The API does not bound the total tenant count for one UserAccount.

Set `Cache-Control: private, no-store` on each UserAccount and Ledger tenant representation.

### Representations

The UserAccount representation contains these fields:

```json
{
  "user_account": {
    "id": "0196f0ec-1daf-7fb4-86de-0f1df4c2f711",
    "created_at": "2026-09-01T20:00:00Z"
  }
}
```

The Ledger tenant representation contains these fields:

```json
{
  "tenant": {
    "id": "0196f0ec-3e80-7a54-bd2b-56cfe90bf801",
    "name": "Primary production",
    "created_at": "2026-09-01T20:01:00Z"
  }
}
```

All identifiers are opaque. All timestamps use UTC and RFC 3339.

### Error Contract

Use one typed error representation for the control plane:

```json
{
  "error": {
    "code": "tenant_not_found",
    "message": "The tenant resource was not found.",
    "request_id": "01J6Q4N5R0A1B2C3D4E5F6G7H8"
  }
}
```

| Condition | HTTP status | Stable code |
| --- | --- | --- |
| Missing, invalid, or unexpected TAuth session | `401` | `unauthenticated` |
| Missing UserAccount | `404` | `user_account_not_found` |
| Missing or inaccessible Ledger tenant | `404` | `tenant_not_found` |
| Reused key with different request | `409` | `idempotency_conflict` |
| Invalid tenant name | `400` | `tenant_name_invalid` |
| Invalid cursor or limit | `400` | `cursor_invalid` or `limit_invalid` |
| Invalid JSON | `400` | `json_invalid` |
| Missing or invalid idempotency key | `400` | `idempotency_key_invalid` |
| Unexpected origin | `403` | `origin_rejected` |
| Missing cross-site request header | `403` | `csrf_rejected` |
| Unsupported media type | `415` | `content_type_invalid` |

Do not expose stack traces, database errors, TAuth claim values, or ownership details in an error.

## Authorization Rules

Validate authentication once at the HTTP boundary. Construct one validated identity domain type from the TAuth session.

Resolve the UserAccount once for each protected request. Pass a validated UserAccount ID into the application service.

Authorize a Ledger tenant with an owner-scoped store operation. Do not authorize after an unrestricted tenant read.

Do not trust a browser-supplied user ID, owner ID, role, email address, or TAuth tenant ID.

The gRPC data plane must resolve each tenant credential to one Ledger tenant. It must put the authenticated tenant identity in context.

Each gRPC handler must compare the authenticated tenant identity with the addressed tenant ID. A mismatch returns `PermissionDenied`.

The browser TAuth session does not authorize a direct gRPC accounting request. Application clients use tenant credentials for the data plane.

## Go Ownership

Keep UserAccount code outside `pkg/ledger`. The public package remains a reusable accounting library.

Use narrow internal packages with these responsibilities:

- `internal/controlplane`: HTTP routes, representations, and typed errors.
- `internal/tauth`: TAuth session validation adapter.
- `internal/useraccount`: UserAccount domain types and application service.
- `internal/tenant`: Ledger tenant domain types and application service.
- `internal/store/gormstore`: Current database adapters for the new store interfaces.
- `cmd/credit`: Runtime composition for the control plane and data plane.

Use smart constructors for each identifier, identity, idempotency key, and tenant creation command.

Inject time, UUID generation, secret generation, and digest operations. Do not read these effects from domain code.

Use separate store interfaces for UserAccount, tenant ownership, credentials, and accounting. Do not expose `*gorm.DB` outside the adapter.

## Transactions And Concurrency

Provision a UserAccount with one transaction and one unique identity constraint. Treat the constraint as the concurrency authority.

Create a Ledger tenant with one transaction. Include the tenant, idempotency record, and control event in that transaction.

Return the first tenant for a valid retry. Return `409` when the same key identifies different canonical input.

Propagate request cancellation through authentication, authorization, and storage. Do not start durable work after request cancellation.

Use database constraints for ownership and identity invariants. Do not use process-local locks as the correctness authority.

## Security Contract

Use the published TAuth session validator. Do not parse, mint, refresh, or reinterpret a TAuth session in Ledger.

Require the configured TAuth issuer, TAuth tenant ID, and session cookie name. Fail startup when this configuration is incomplete.

Reject a state-changing request without the selected cross-site request proof. Reject an unexpected browser origin.

Accept only the documented JSON media type. Set a bounded request body size before JSON parsing.

Do not store unnecessary personal data. UserAccount stores only the stable external identity and Ledger-owned identifiers.

Generate tenant credentials with a cryptographically secure source. Store only the selected digest and non-secret credential metadata.

Never put a session, cookie, credential, idempotency key, email address, or request body in logs.

Rate limits can bound request frequency. A rate limit must not become a persistent tenant count limit.

## Observability And Audit

Give each control plane request one request ID. Return the request ID in each error representation.

Write structured logs for request completion. Include operation, status, latency, UserAccount ID, and addressed resource ID.

Write these initial control event types:

- `user_account.provisioned`.
- `tenant.created`.
- `tenant_credential.created`.
- `tenant_credential.revoked`.

Emit a control event only in the same transaction as its durable state change. Do not emit an event for a failed operation.

## Forward-Only Migration

The migration is a bounded operation. The runtime must contain only the new contract after the migration.

Use this migration sequence:

1. Create the UserAccount, Ledger tenant, credential, idempotency, and control event tables.
2. Require an explicit owner mapping for every current configured tenant.
3. Create one UserAccount and one Ledger tenant for each approved mapping.
4. Rename `accounts` to `ledger_accounts` and add the required foreign keys.
5. Associate every current Ledger account with its canonical Ledger tenant.
6. Create canonical tenant credentials and update each application client.
7. Remove the static tenant registry and each old tenant secret.
8. Remove the migration command and migration-only input after successful verification.

Do not add dual configuration reads, dual credential validation, or a legacy secret format.

The migration must fail before mutation when an owner mapping is missing. It must also reject duplicate or unknown tenant IDs.

The schema and migration contract must work with PostgreSQL and SQLite. Public acceptance must use the repository-supported database targets.

## Black-Box Acceptance

Exercise the control plane through a real HTTP listener. Exercise the data plane through the real gRPC entrypoint.

Cover these UserAccount scenarios:

- Reject a missing, expired, malformed, or unexpected TAuth session.
- Provision one UserAccount from one valid TAuth identity.
- Return the same UserAccount for concurrent provisioning requests.
- Keep two TAuth identities in separate UserAccounts.
- Return `404` from a safe read before provisioning.
- Store no email, display name, avatar URL, cookie, or session token.

Cover these Ledger tenant scenarios:

- Create multiple Ledger tenants for one UserAccount.
- Create more tenants than one collection page can contain.
- Return only the authenticated owner's Ledger tenants.
- Deny access to another UserAccount's Ledger tenant.
- Return the same tenant for the same idempotency key and request.
- Return `409` for the same idempotency key and different request.
- Keep tenant creation atomic after an injected storage failure.
- Propagate request cancellation without a partial tenant.

Cover these data plane scenarios with the persistent credential registry:

- Authenticate a valid tenant credential through every gRPC RPC.
- Reject a revoked tenant credential.
- Reject a tenant credential for a different tenant.
- Prove tenant A cannot read or change tenant B Ledger accounts.
- Prove the public `Batch` RPC obeys the same tenant boundary.

Cover the migration with a real pre-migration database fixture. Verify the exact canonical schema and retained accounting history.

## Implementation Slices

The backend uses these independent implementation slices:

1. Add UserAccount and Ledger tenant domain types, tables, and store contracts.
2. Add the TAuth validation adapter and UserAccount control plane routes.
3. Add tenant creation, owner authorization, pagination, and idempotency.
4. Add the persistent tenant credential registry and data plane authentication.
5. Add the bounded migration and remove the static tenant contract.
6. Add the public runtime route after the exact profile is available.
7. Add the browser frontend in F001.

B001 is resolved in the credential-backed data plane boundary. `Batch` reads the nested account tenant before authentication.

## Selected Decisions

- Use the HTTP control plane under `/api`.
- Provision a UserAccount explicitly with `PUT /api/user-account`.
- Require only a human-readable tenant name during tenant creation.
- Use owner-only tenants. Do not add members or roles.
- Do not add tenant rename, transfer, closure, or deletion.
- Create credentials through a separate tenant credential resource.
- Permit multiple active credentials and explicit revocation.
- Use `ledger_<credential_uuid>_<base64url_secret>` credentials with 256-bit random secrets and SHA-256 digests.
- Require the exact public origin and `X-Ledger-CSRF: 1` for browser mutations.
- Do not add a persistent tenant count limit.
- Retain a disabled identity's UserAccount and tenant ownership until a separate retention policy changes this rule.
- Run one `ledgerd` process with separate HTTP and gRPC listeners.

## Required Operator Inputs

- Provide the exact Ledger TAuth tenant, public origin, cookie name, issuer, and hosted route profile.
- Provide one complete owner mapping for the bounded migration. The mapping must include every legacy configured tenant, including a tenant that has no Ledger account row.
