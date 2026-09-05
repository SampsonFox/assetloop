# Code Map

Purpose: give agents and contributors the smallest useful reading set before they search the repository. Keep this file concise and update it whenever paths or ownership change.

Status: v0.1 foundation plus authentication/RBAC, asset catalog, and append-only lifecycle vertical slices are implemented. Attachment, market, MCP, and scheduler paths continue as later slices.

## Authority map

| Need | Read |
|---|---|
| Non-negotiable rules | `AGENTS.md` |
| System boundaries and data flow | `docs/ARCHITECTURE.md` |
| Product scope and delivery phases | `docs/PROJECT_PLAN.md` |
| Branching, promotion, packaging, rollback | `docs/DEVELOPMENT_WORKFLOW.md` |
| Find the relevant package | this file |

## Entry points

| Path | Responsibility |
|---|---|
| `cmd/assetloop/` | Single binary; defaults to `serve` (SQLite check/upgrade then Web), explicit `migrate`, and Windows double-click launch handling |
| `internal/web/` | HTTP transport; asset-list-first SSR UI, server-filtered/sorted/paged tables, detail-style asset create/edit pages, code-defined zh-CN/en language packs, account menu, semantic light/dark themes, shared catalog drawers, inline custom lifecycle event types, and progressively disclosed FX evidence forms |
| `internal/mcp/` | Semantic MCP tool transport |
| `internal/scheduler/` | Refresh-job entry adapters |
| `internal/application/` | Authentication, catalog, lifecycle use cases, validation, and inward ports shared by Web and semantic MCP writes |
| `internal/domain/` | Pure catalog/lifecycle types plus the versioned ISO 4217 catalog, exact minor-unit money, and fixed-point FX logic |
| `internal/config/` | Defaults, optional `.env`, and environment override loading |
| `.github/workflows/ci.yml` | Work-branch secret scanning plus full pull-request/UAT/Prod validation |
| `.github/workflows/package.yml` | Shared UAT/Prod packaging, artifact smoke test, Prod release |

## Infrastructure adapters

| Path | Responsibility | Allowed dependency |
|---|---|---|
| `internal/store/` | Driver opening, embedded Goose migration runner, verified SQLite backup | config, embedded migrations |
| `internal/store/sqlite/` | SQLite Store and committed sqlc output | application ports, domain |
| `internal/store/postgres/` | PostgreSQL Store and committed sqlc output | application ports, domain |
| `internal/blob/local/` | Local filesystem BlobStore | blob port |
| `internal/blob/aliyun/` | Aliyun OSS BlobStore | blob port, Aliyun SDK |
| `internal/market/onebound/` | OneBound request/response adapter | market port |
| `internal/market/manual/` | Manual/imported market observations | market port |
| `migrations/sqlite/` | SQLite forward migrations | none |
| `migrations/postgres/` | PostgreSQL forward migrations | none |

## Core ports

| Symbol | Expected location | Implementations |
|---|---|---|
| `Store` | `internal/application/ports.go` | SQLite, PostgreSQL (implemented) |
| `BlobStore` | `internal/application/ports.go` | Local, Aliyun OSS |
| `ObjectKeyMapper` | `internal/blob/key_mapper.go` | one shared mapper |
| `MarketDataProvider` | `internal/application/ports.go` | OneBound, Manual |
| `FXProvider` | `internal/application/ports.go` | selected FX source |
| `AuthStore` | `internal/application/ports.go` | SQLite, PostgreSQL (implemented) |
| `CatalogStore` | `internal/application/ports.go` | SQLite, PostgreSQL (implemented) |
| `LifecycleStore` | `internal/application/ports.go` | SQLite, PostgreSQL (implemented) |

## Regression spine

| Path | Coverage |
|---|---|
| `internal/application/*_test.go` | validation, exact money/FX conversion, auth, catalog, lifecycle, and role policy |
| `internal/store/storetest/` | shared dual-database auth/catalog/lifecycle behavior, custom event types, bulk relation loading, server-side list query contracts, aggregate summaries, FX evidence, append-only correction, locks, and tenant isolation |
| `internal/store/migration_upgrade_test.go` | previous-schema upgrades, including lifecycle-table expansion, without data loss |
| `internal/store/schema.go`, `migration_lock*.go` | read-only schema compatibility checks and cross-process SQLite/PostgreSQL migration locks |
| `internal/store/migration_safety_test.go`, `migration_lock_test.go` | concurrent upgrades, unchanged-schema backups, version rejection, recovery backups, and crash-released locks |
| `internal/application/lifecycle_write.go`, both Store `lifecycle_write.go` adapters | transaction-bound lifecycle policy and durable tenant/user-scoped idempotency receipts |
| `internal/store/storetest/lifecycle_safety.go`, `internal/web/lifecycle_retry_test.go` | concurrent mutations, independent-connection retries, receipt rollback, and Web form replay |
| `internal/web/i18n.go` | registered locales, stable message keys, browser/cookie locale matching, and zh-CN fallback |
| `internal/web/*_test.go` | auth, CSRF, locale/theme preferences, role-scoped account menu, asset-list states, shared drawers, catalog, progressive FX evidence, correction, import confirmation, totals, and role denial |
| `internal/integration/full_element_test.go` | cumulative auth → persisted preferences → catalog → foreign purchase → repair correction → sale scenario on both databases |

## Read paths by task

| Task | Start here | Then read only |
|---|---|---|
| Change money or FX behavior | `internal/domain/` | relevant application use case and Store mapping |
| Add lifecycle event | `internal/domain/lifecycle.go` | `internal/application/lifecycle.go`, both lifecycle adapters and migration pair |
| Change asset catalog | `internal/application/catalog.go` | domain asset types, both catalog adapters, Web catalog templates |
| Add database field | both migration directories | both sqlc query directories, Store conformance tests |
| Add attachment behavior | blob port and key mapper | local and Aliyun adapters, attachment application service |
| Add market provider | market port | provider adapter plus shared normalization pipeline |
| Change MCP tool | `internal/mcp/` | called application service; never inspect Store unless service contract changes |
| Change Web screen | `internal/web/server.go` | affected template under `templates/`, then `static/app.css` or local `static/app.js`; called application service only when behavior changes |
| Change locale or theme | `internal/web/i18n.go` | affected templates, semantic variables in `static/app.css`, then Web locale/theme tests |
| Change deployment config | `internal/config/` | `.env.example`, README deployment section |
| Change architecture | `AGENTS.md` | `docs/ARCHITECTURE.md`, then affected code |
| Change Git/release workflow | `docs/DEVELOPMENT_WORKFLOW.md` | `AGENTS.md`, delivery architecture, GitHub workflows |

## Core execution flows

```text
Screenshot import:
User confirmation in AI Harness -> semantic MCP write -> lifecycle use case -> Store + BlobStore

Manual Web edit:
Browser -> Web handler -> application use case -> Store

Market refresh:
Scheduler/CLI -> refresh use case -> MarketDataProvider
              -> normalization -> FX conversion -> Store

Attachment read:
Web/MCP -> attachment use case -> attachment metadata Store
        -> store registry by store_id -> BlobStore.Open(object_key)

Correction after a write:
Web/MCP -> append-only correction use case -> void event + replacement event -> Store
```

## Maintenance rule

Do not list every file. List stable entry points, ports, and ownership boundaries. Once code exists, use `rg` inside the selected path rather than loading the whole repository.
