# System Architecture

This document is the architectural source of truth for AssetLoop (物迹). It defines the system's stable shape independently from the delivery plan.

## 1. Architectural intent

AssetLoop (物迹) is a modular Go monolith that runs locally with no required external service and can later run as a multi-tenant SaaS without replacing its domain or application layers.

The architecture optimizes for:

- traceable economic history;
- replaceable databases, blob stores, and market providers;
- a single source of business rules shared by Web, MCP, CLI, and jobs;
- local-first installation and safe forward upgrades;
- low operational and contributor complexity.

## 2. System context

```text
                    +------------------+
 screenshots/chat ->|    AI Harness    |
                    +--------+---------+
                             | semantic MCP calls
                             v
+---------+       +----------+-----------+       +----------------+
| Browser |------>| AssetLoop / 物迹      |<------| OS Cron / Worker|
+---------+ HTTP  | Web + MCP + CLI      | jobs  +----------------+
                  +----------+-----------+
                             |
             +---------------+----------------+
             |               |                |
             v               v                v
        Database Store   Attachment Store  Market/FX Sources
        SQLite/Postgres  Local/Aliyun OSS  OneBound/others
```

The AI Harness interprets images and conversation and confirms every extracted field with the user before invoking a write MCP tool. A semantic MCP mutation therefore represents confirmed intent, but the application still validates identity, tenant scope, input shape, lifecycle invariants, money evidence, and transaction boundaries before writing.

## 3. Architectural style

The system uses ports and adapters inside one process:

```text
Transport adapters
  Web | MCP | CLI | Scheduler
              |
              v
Application services and ports
              |
              v
Pure domain model

Infrastructure adapters implement application ports:
  SQLite | PostgreSQL | Local files | Aliyun OSS | OneBound
```

Dependencies point inward. Domain code has no knowledge of transports or infrastructure.

Authentication is a transport concern, while authorization is an application concern. A
transport resolves an authenticated principal; application services decide whether that
principal may perform the requested capability for the resolved tenant.

## 4. Runtime modules

### 4.1 Domain

Owns entities, value objects, invariants, and pure calculations:

- tenant identity;
- item category, product model, product variant, and asset;
- asset lifecycle events and correction relationships;
- money, currencies, FX evidence, and base-currency calculations;
- market observations, price points, and valuation inputs.

It performs no I/O.

### 4.2 Application

Owns use cases and ports:

- persist user-confirmed semantic commands received from Web or MCP;
- create or resolve category/model/variant;
- record purchase, repair, sale, void, and replacement;
- attach evidence;
- refresh and normalize market observations;
- calculate valuation and lifecycle statistics;
- authorize tenant-scoped capabilities for owners, editors, and viewers;
- enforce tenant and transaction boundaries.

Application services are the only supported write path.

### 4.3 Transport adapters

- Web translates HTTP requests and renders server-side HTML.
- MCP translates semantic tool calls into application commands and queries.
- CLI wires process-level commands.
- Scheduler invokes the same refresh use cases as CLI and SaaS workers.

Transport adapters contain authentication, parsing, and response formatting, not economic rules.

### 4.4 Infrastructure adapters

- Stores translate application operations to SQLite or PostgreSQL.
- Blob stores translate logical object keys to local files or Aliyun OSS.
- Market providers translate external APIs into normalized observation DTOs.
- FX providers supply dated exchange rates with provenance.

## 5. Domain backbone

```text
Tenant
  |
  +-- TenantSettings (base currency, market preferences)
  |
  +-- ItemCategory
        |
        +-- ProductModel
              |
              +-- ProductVariant  <-- market price identity
                    |
                    +-- Asset     <-- physical owned item
                          |
                          +-- AssetEvent[]
```

`ProductVariant` contains only attributes that affect market identity and price, such as storage capacity. `Asset` contains instance attributes such as serial number and normally color. Category-specific condition schemes map assets and market listings to comparable condition codes.

Transactions group related cash and lifecycle effects, while asset events remain the append-only lifecycle record.
Confirmed events cannot be updated or deleted at either Store or database level. A correction
atomically appends a zero-value void event plus a replacement economic event that references the
original; the original row remains unchanged and queryable.
User-facing lifecycle collections never render the technical void row as a separate event. Their
default effective view excludes voided originals; an explicit history option adds those originals
back with a voided marker while preserving the same server-side filtering, sorting, and pagination.

Each tenant may register additional event types without changing the fixed meanings of purchase,
repair, sale, and void. A custom type records a stable display name and exactly one cash-flow effect:
expense, income, or neutral. The event row keeps that name and the resulting signed base amount, so
later configuration cannot rewrite history. Neutral events persist a zero amount and do not lock the
tenant base currency. Custom types do not implicitly change the built-in acquired, repairing, or sold
status transitions.

## 6. Money architecture

Money is represented as:

```text
signed minor-unit integer + ISO currency code
```

The tenant base currency is the accounting and statistics currency. When an original currency differs, a persisted record contains both representations and the exact rate evidence used for conversion.

```text
Original money + FX rate/date/source -> Base money -> all internal statistics
```

Rates are stored as signed-safe fixed-point integers scaled by `100,000,000` and mean “base major
units per original major unit.” Conversion applies the ISO minor-unit exponent for both currencies
and rounds once to the nearest base minor unit. Floating point is never used for persisted or
calculated money. When the original currency equals the base currency, original amount/currency
and FX evidence remain null; otherwise all evidence fields are mandatory. Web confirms them in its
event form; the AI Harness confirms them in conversation before making a semantic MCP write.

The first monetary record locks the tenant base currency. Changing it later requires a dedicated, audited migration operation, not a settings edit.

## 7. Persistence architecture

### 7.1 Store port

`Store` exposes business-oriented operations, not a generic repository per table. Each operation owns its database transaction when atomicity is required.

SQLite and PostgreSQL have independent SQL and sqlc output, with a shared conformance suite. Vendor-specific SQL stays inside its adapter.

User-facing collection queries use resource-specific application options and results. The
application layer trims filters, caps page sizes, and validates sort keys and directions against a
fixed allowlist. SQLite and PostgreSQL then perform filtering, sorting, and pagination in SQL.
Related rows required by one page are loaded in bulk joins or CTEs (for example, a page of product
models and all of those models' variants); aggregate screens use aggregate Store operations rather
than calling a detail query once per row. Query count therefore remains bounded as result counts
grow. Web handlers never assemble SQL and do not implement in-memory pagination as a fallback.

The current collection contract covers assets with lifecycle summaries, models with variants,
tenant members, asset lifecycle records, and tenant-scoped custom event-type options. Both Store
adapters run the same conformance tests for search, filters, sorting, paging, related-row hydration,
aggregate totals, and custom event-type isolation.

The lifecycle Store atomically creates the grouping transaction and event, locks the base currency,
and, for corrections, writes the void and replacement rows in the same database transaction.
Harness-confirmed MCP commands append formal lifecycle events directly through this same application
use case. Historical installations may retain a dormant `import_drafts` compatibility table from an
older migration; active code neither reads nor writes it, and a later contract migration may remove it
only after an explicit data-retention decision.

### 7.2 Migration rules

- Each logical migration version exists for both dialects.
- Application upgrades apply forward-only migrations.
- SQLite upgrades take a verified backup before schema changes.
- PostgreSQL production upgrades run once as a release job under an advisory lock.
- Destructive changes use expand-contract across releases.

### 7.3 Tenant boundary

Every business row is tenant-owned. Local mode creates one default tenant; SaaS resolves tenant identity through authentication. No query or uniqueness rule may rely on an implicit single tenant.

### 7.4 Identity and authorization boundary

Users are global identities. `tenant_memberships` binds a user to a tenant with exactly one
of three roles:

- `owner`: manages members and tenant settings and has all tenant business capabilities;
- `editor`: maintains catalog, assets, attachments, and lifecycle records;
- `viewer`: reads tenant data without mutating it.

Platform operations are not a tenant role. The first version has no cross-tenant super-admin
screen and no platform identity implicitly gains access to tenant business data.

The Web transport uses random opaque sessions whose token hashes are stored in the database;
it does not use bearer JWTs. First-run setup creates the initial tenant and owner. Authentication
may be disabled only when the HTTP listener is loopback-only, in which case the application
uses the default local owner identity.

```text
HTTP request
  -> authenticate session and resolve user + tenant
  -> application capability check
  -> tenant-scoped Store operation
```

Every Store query remains tenant-scoped even after authorization. Hiding a Web control is never
treated as authorization. State-changing Web requests require CSRF validation, and membership or
authentication changes produce security audit events.

User interface preferences belong to the global user identity. `users.locale` selects a registered
code-defined language pack and `users.theme` selects `system`, `light`, or `dark`; both values are
resolved again with every authenticated request, including local disabled-auth mode. Anonymous
setup and login pages resolve locale from a same-site cookie, then `Accept-Language`, and finally
fall back to `zh-CN`. Locale changes are application validation, not database enums, so adding a
language does not require a schema migration.

The SSR response owns first paint: `<html lang>` and `<html data-theme>` are emitted by Go before
the page loads. CSS uses semantic variables, with `system` delegated to
`prefers-color-scheme`; no client-side theme bootstrap or runtime translation service is required.
Predictable application validation crosses the transport boundary as language-neutral
`InputError` codes. Unexpected infrastructure errors are logged server-side and rendered only as
a localized generic message.

## 8. Attachment architecture

```text
Attachment use case
       |
       +-> metadata Store: store_id + object_key + checksum
       |
       +-> BlobStore registry[store_id]
                    |
             Local | Aliyun OSS
```

`ObjectKeyMapper` creates the same logical key for every backend:

```text
tenants/{tenant_id}/attachments/{yyyy}/{mm}/{attachment_id}/{variant}.{ext}
```

The metadata row selects the store for reads. A configuration value selects the default store for new writes. Therefore a configuration switch never relocates or hides existing bytes.

A store migration copies the object under the same key, verifies size and SHA-256, changes `store_id`, then optionally removes the old object.

## 9. Market data architecture

```text
MarketDataProvider
        |
        v
raw normalized listings
        |
match variant -> reject accessories/services -> deduplicate
        |
condition mapping -> outlier filter -> aggregate
        |
dated FX conversion -> persisted price point
```

Providers own transport mechanics only. The shared pipeline owns market meaning. Price series are keyed by variant, condition, region, and provider so sources cannot be mixed invisibly.

The initial provider is OneBound. Manual import is the second implementation used for testing and fallback. A versioned remote HTTP provider protocol is deferred until an external provider must run without recompiling the application.

## 10. Configuration architecture

Deployment configuration comes from defaults, optional `.env`, and real environment variables, with real environment variables taking precedence.

Deployment configuration includes database DSN, HTTP address, authentication mode, local blob root, OSS endpoint/bucket, and secret credentials.

Auditable tenant business settings stay in the database: base currency, market region, provider priority, refresh cadence, condition schemes, and valuation policy.

Secrets never become ordinary tenant-setting rows. SaaS deployments may replace local `.env` secret values with secret-manager references without changing application services.

## 11. Deployment shapes

### Local

```text
single binary
  + SQLite file
  + local attachment directory (default)
  + local account/session authentication
  + optional Aliyun OSS
  + OS scheduler
```

### SaaS

```text
application replicas
  + PostgreSQL
  + Aliyun OSS
  + external secret manager
  + scheduled worker
  + authentication/tenant resolution
```

Both shapes invoke the same application services and domain model.

## 12. Failure and consistency model

- Database transactions protect related metadata and lifecycle changes.
- A blob write completes and is verified before attachment metadata becomes `ready`.
- Failed cross-resource operations leave a recoverable pending record or an orphan eligible for cleanup; they must not fabricate a ready attachment.
- External provider failures are classified as authentication, rate limit, temporary, unsupported, or invalid response.
- Market refresh failure never corrupts the last valid price point.
- Economic records remain reconstructable from append-only events and FX evidence.

### 12.1 Regression and full-element verification

Each accepted behavior leaves an automated regression test at the narrowest useful layer. A
single `TestFullElementScenario` is the executable product spine: it starts with authentication
and tenant isolation, then grows to cover catalog, concrete assets, lifecycle events, original
currency evidence, corrections, and Web access as those slices are delivered.

The scenario runs through application services and the real SQLite and PostgreSQL Store adapters.
UAT runs it before packaging, and packaged-binary smoke tests remain a separate gate so an
in-process test cannot substitute for verifying the shipped artifact.

## 13. Architecture change protocol

A change requires updating this document before implementation if it does any of the following:

- adds a runtime service or persistent infrastructure dependency;
- reverses an inward dependency;
- introduces a database, blob store, or provider outside an existing port;
- changes money representation, event history, tenant ownership, or migration guarantees;
- changes the single-binary/server-rendered architecture;
- weakens auditability, secret handling, or data-upgrade safety.

Implementation convenience alone is not sufficient justification. The change must state the new invariant, migration path, and compatibility tests.

## 14. Delivery architecture

```text
dev baseline
    |
    +-- dev-<scope> | feature/<scope> | fix/<scope>
                    |
                    +-- rapid edits + narrow tests + live local instance
                    |
                    +-- checkpoint commits + work-branch secret scan
                    |
                    +-- user marks UAT checkpoint
                    |
                    +-- full validation + squash PR --> uat (protected)
                                         |
                                  package + smoke test
                                         |
                                    PR --> prod (protected)
                                         |
                                  package + GitHub release
```

`dev`, `uat`, and `prod` are permanent. Work branches are disposable; permanent branches are not. GitHub uses `prod` as the repository default branch so public visitors see published production history, while `dev` remains the baseline for new development work. During dense product iteration, the local development instance follows the active work branch: each accepted edit is smoke-tested at the narrowest useful layer and reflected locally without invoking the full release suite. Work-branch pushes retain secret scanning, but UAT promotion begins only when the user identifies the accumulated batch as a checkpoint. UAT and production builds share one workflow and differ only by GitHub environment, retention, and the final production-release step. This prevents a separately maintained production build path from drifting away from the artifact tested in UAT.

Delivery stops after every UAT build and artifact verification. Production promotion is a separate user-authorized action tied to one identified UAT commit and its evidence; workflow success alone never grants that authorization. Cross-platform packaging uses `.zip` for Windows and `.tar.gz` for Linux and macOS.

The GitHub repository is public, so repository visibility is a security boundary: all tracked content and delivery metadata are assumed public. GitHub Push Protection blocks supported credential patterns before acceptance, while the required `secret-scan` CI job scans the complete fetched Git history with a checksum-pinned Gitleaks release. Runtime secrets remain outside Git in local `.env` files, GitHub Environment secrets, or a production secret manager.

Rollback is Git-based: revert the smallest offending commit or revert the promotion pull request, then run the same pipeline again. Database rollback remains forward-only and uses a corrective migration; branch rollback never runs destructive down migrations against persisted data.
