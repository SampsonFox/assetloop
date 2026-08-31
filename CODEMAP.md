# Code Map

Purpose: give agents and contributors the smallest useful reading set before they search the repository. Keep this file concise and update it whenever paths or ownership change.

Status: architecture-only repository; paths marked **planned** do not exist yet.

## Authority map

| Need | Read |
|---|---|
| Non-negotiable rules | `AGENTS.md` |
| System boundaries and data flow | `docs/ARCHITECTURE.md` |
| Product scope and delivery phases | `docs/PROJECT_PLAN.md` |
| Find the relevant package | this file |

## Planned entry points

| Path | Responsibility |
|---|---|
| `cmd/assetloop/` | Single binary and subcommand wiring |
| `internal/web/` | HTTP transport, templates, static assets |
| `internal/mcp/` | Semantic MCP tool transport |
| `internal/scheduler/` | Refresh-job entry adapters |
| `internal/application/` | Use cases, validation orchestration, transaction boundaries |
| `internal/domain/` | Pure domain types, invariants, calculations |

## Planned infrastructure adapters

| Path | Responsibility | Allowed dependency |
|---|---|---|
| `internal/store/sqlite/` | SQLite Store and sqlc queries | application ports, domain |
| `internal/store/postgres/` | PostgreSQL Store and sqlc queries | application ports, domain |
| `internal/blob/local/` | Local filesystem BlobStore | blob port |
| `internal/blob/aliyun/` | Aliyun OSS BlobStore | blob port, Aliyun SDK |
| `internal/market/onebound/` | OneBound request/response adapter | market port |
| `internal/market/manual/` | Manual/imported market observations | market port |
| `migrations/sqlite/` | SQLite forward migrations | none |
| `migrations/postgres/` | PostgreSQL forward migrations | none |

## Planned core ports

| Symbol | Expected location | Implementations |
|---|---|---|
| `Store` | `internal/application/ports.go` | SQLite, PostgreSQL |
| `BlobStore` | `internal/application/ports.go` | Local, Aliyun OSS |
| `ObjectKeyMapper` | `internal/blob/key_mapper.go` | one shared mapper |
| `MarketDataProvider` | `internal/application/ports.go` | OneBound, Manual |
| `FXProvider` | `internal/application/ports.go` | selected FX source |

## Read paths by task

| Task | Start here | Then read only |
|---|---|---|
| Change money or FX behavior | `internal/domain/` | relevant application use case and Store mapping |
| Add lifecycle event | domain asset-event files | application use case, both Store adapters, both migrations if schema changes |
| Add database field | both migration directories | both sqlc query directories, Store conformance tests |
| Add attachment behavior | blob port and key mapper | local and Aliyun adapters, attachment application service |
| Add market provider | market port | provider adapter plus shared normalization pipeline |
| Change MCP tool | `internal/mcp/` | called application service; never inspect Store unless service contract changes |
| Change Web screen | `internal/web/` | called application service and template only |
| Change deployment config | `internal/config/` | `.env.example`, README deployment section |
| Change architecture | `AGENTS.md` | `docs/ARCHITECTURE.md`, then affected code |

## Core execution flows

```text
Screenshot import:
AI Harness -> MCP -> Import use case -> confirmation -> Store + BlobStore

Manual Web edit:
Browser -> Web handler -> application use case -> Store

Market refresh:
Scheduler/CLI -> refresh use case -> MarketDataProvider
              -> normalization -> FX conversion -> Store

Attachment read:
Web/MCP -> attachment use case -> attachment metadata Store
        -> store registry by store_id -> BlobStore.Open(object_key)
```

## Maintenance rule

Do not list every file. List stable entry points, ports, and ownership boundaries. Once code exists, use `rg` inside the selected path rather than loading the whole repository.
