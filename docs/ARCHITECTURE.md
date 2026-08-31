# System Architecture

This document is the architectural source of truth for Item LCC Track. It defines the system's stable shape independently from the delivery plan.

## 1. Architectural intent

Item LCC Track is a modular Go monolith that runs locally with no required external service and can later run as a multi-tenant SaaS without replacing its domain or application layers.

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
| Browser |------>| Item LCC Track       |<------| OS Cron / Worker|
+---------+ HTTP  | Web + MCP + CLI      | jobs  +----------------+
                  +----------+-----------+
                             |
             +---------------+----------------+
             |               |                |
             v               v                v
        Database Store   Attachment Store  Market/FX Sources
        SQLite/Postgres  Local/Aliyun OSS  OneBound/others
```

The AI Harness interprets images and conversation. The application does not trust extracted data until application validation and required user confirmation have completed.

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

- confirm an AI-generated import draft;
- create or resolve category/model/variant;
- record purchase, repair, sale, void, and replacement;
- attach evidence;
- refresh and normalize market observations;
- calculate valuation and lifecycle statistics;
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

## 6. Money architecture

Money is represented as:

```text
signed minor-unit integer + ISO currency code
```

The tenant base currency is the accounting and statistics currency. When an original currency differs, a persisted record contains both representations and the exact rate evidence used for conversion.

```text
Original money + FX rate/date/source -> Base money -> all internal statistics
```

The first monetary record locks the tenant base currency. Changing it later requires a dedicated, audited migration operation, not a settings edit.

## 7. Persistence architecture

### 7.1 Store port

`Store` exposes business-oriented operations, not a generic repository per table. Each operation owns its database transaction when atomicity is required.

SQLite and PostgreSQL have independent SQL and sqlc output, with a shared conformance suite. Vendor-specific SQL stays inside its adapter.

### 7.2 Migration rules

- Each logical migration version exists for both dialects.
- Application upgrades apply forward-only migrations.
- SQLite upgrades take a verified backup before schema changes.
- PostgreSQL production upgrades run once as a release job under an advisory lock.
- Destructive changes use expand-contract across releases.

### 7.3 Tenant boundary

Every business row is tenant-owned. Local mode creates one default tenant; SaaS resolves tenant identity through authentication. No query or uniqueness rule may rely on an implicit single tenant.

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

Deployment configuration includes database DSN, HTTP address, local blob root, OSS endpoint/bucket, and secret credentials.

Auditable tenant business settings stay in the database: base currency, market region, provider priority, refresh cadence, condition schemes, and valuation policy.

Secrets never become ordinary tenant-setting rows. SaaS deployments may replace local `.env` secret values with secret-manager references without changing application services.

## 11. Deployment shapes

### Local

```text
single binary
  + SQLite file
  + local attachment directory (default)
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

## 13. Architecture change protocol

A change requires updating this document before implementation if it does any of the following:

- adds a runtime service or persistent infrastructure dependency;
- reverses an inward dependency;
- introduces a database, blob store, or provider outside an existing port;
- changes money representation, event history, tenant ownership, or migration guarantees;
- changes the single-binary/server-rendered architecture;
- weakens auditability, secret handling, or data-upgrade safety.

Implementation convenience alone is not sufficient justification. The change must state the new invariant, migration path, and compatibility tests.

