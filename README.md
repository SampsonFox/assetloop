# Item LCC Track

AI-assisted personal asset lifecycle, resale valuation, and holding-cost tracker.

The repository is currently in architecture and planning stage.

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

