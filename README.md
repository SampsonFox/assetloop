# AssetLoop / 物迹

AI-assisted personal asset lifecycle, resale valuation, and holding-cost tracker.

The repository now contains the v0.1 foundation: one Go binary, embedded dual-dialect migrations, and matching SQLite/PostgreSQL Store adapters.

## Start here

- [Project guardrails](AGENTS.md)
- [Code map](CODEMAP.md)
- [System architecture](docs/ARCHITECTURE.md)
- [Project plan](docs/PROJECT_PLAN.md)

## Confirmed baseline

- Go modular monolith; server-rendered Web plus semantic MCP tools.
- SQLite by default, PostgreSQL for SaaS.
- Local attachments by default, Aliyun OSS supported.
- OneBound as the first replaceable market-data provider.
- Tenant base-currency accounting with original-currency audit evidence.

## Run locally

Go 1.25.7 or newer is required for development. Runtime defaults need no external service.

```sh
go run ./cmd/assetloop migrate
go run ./cmd/assetloop serve
```

The default SQLite database is `./data/assetloop.db`; health is available at `http://127.0.0.1:8080/healthz`. Copy `.env.example` to `.env` only when overriding defaults.

```sh
go test ./...
```

Set `TEST_POSTGRES_DSN` to run the PostgreSQL Store conformance test. CI always runs it against a real PostgreSQL service.
