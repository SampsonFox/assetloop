# AssetLoop / 物迹

AI-assisted personal asset lifecycle, resale valuation, and holding-cost tracker.

The repository contains the v0.1 foundation plus authentication/RBAC, tenant-isolated asset catalog management, and append-only purchase/repair/sale lifecycle tracking with exact base-currency accounting and matching SQLite/PostgreSQL adapters.

## Start here

- [Project guardrails](AGENTS.md)
- [Code map](CODEMAP.md)
- [System architecture](docs/ARCHITECTURE.md)
- [Project plan](docs/PROJECT_PLAN.md)
- [Development and release workflow](docs/DEVELOPMENT_WORKFLOW.md)

## Confirmed baseline

- Go modular monolith; server-rendered Web plus semantic MCP tools.
- SQLite by default, PostgreSQL for SaaS.
- Local attachments by default, Aliyun OSS supported.
- OneBound as the first replaceable market-data provider.
- Tenant base-currency accounting with original-currency audit evidence.

## Run locally

Go 1.26 or newer is required for development. Runtime defaults need no external service.

```sh
go run ./cmd/assetloop
```

The default SQLite database is `./data/assetloop.db`; health is available at `http://127.0.0.1:8080/healthz`. Open `http://127.0.0.1:8080/setup` on first start to create the tenant Owner. Copy `.env.example` to `.env` only when overriding defaults.

No arguments defaults to `serve`: SQLite is checked and safely migrated before the Web server
starts. On Windows, extract the release and double-click `assetloop.exe`; configuration and data
are resolved beside the executable, and startup errors remain visible until Enter is pressed.
Then open `http://127.0.0.1:8080/` in a browser. Command-line launches keep their working directory.
Explicit `serve` and `migrate` remain supported. PostgreSQL deployments run `assetloop migrate`
as a separate release step; server startup only checks schema compatibility.

After setup, `/` opens the concrete asset list. It supports list/card views, a deliberate empty state, and one shared drawer for creating or editing an asset. Owner and Editor users maintain categories, models, price-distinguishing variants such as 256GB/512GB, and category icons at `/admin/catalog`; Viewer users receive no management controls. `/overview` contains the base-currency summary.

Each asset detail page accepts purchase, repair, sale, and append-only corrections. Non-base-currency entries require explicit rate/date/source confirmation and preserve the original evidence while all totals use the tenant base currency. The AI Harness confirms recognized fields in conversation before invoking a semantic MCP write; that call records the lifecycle event directly through the same application service, without a second pending-import queue.

`AUTH_MODE=local` is the secure default. `AUTH_MODE=disabled` is accepted only when `HTTP_ADDR` is loopback-only and creates an implicit local Owner.

```sh
go test ./...
```

Set `TEST_POSTGRES_DSN` to run the PostgreSQL Store conformance test. CI always runs it against a real PostgreSQL service.
