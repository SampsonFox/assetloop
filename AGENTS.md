# Project Guardrails

These instructions apply to the entire repository.

## Required reading order

Before changing code:

1. Read this file.
2. Read `CODEMAP.md` for the smallest relevant path.
3. Read `docs/ARCHITECTURE.md` only for the affected boundary.
4. Read `docs/PROJECT_PLAN.md` only when changing scope, milestones, or product behavior.

When documents disagree, authority is:

```text
AGENTS.md > docs/ARCHITECTURE.md > docs/PROJECT_PLAN.md > CODEMAP.md > code comments
```

`CODEMAP.md` describes the current tree; it does not override architecture.

## Hard boundaries

### 1. Keep one modular monolith

- MUST ship one Go application with Web, MCP, CLI, and scheduled-job entry points.
- MUST keep business modules in-process behind Go interfaces.
- MUST NOT introduce microservices, a message broker, or distributed transactions without an approved architecture change.

### 2. Protect dependency direction

- `domain` MUST import no database, HTTP, OSS, MCP, OneBound, or framework package.
- `application` MAY depend on domain types and ports, never concrete infrastructure adapters.
- Web, MCP, CLI, and scheduler MUST call application services; they MUST NOT write SQL or call storage/provider SDKs directly.
- Infrastructure adapters MAY depend inward; inward packages MUST NOT depend outward.

### 3. Keep business writes behind use cases

- Every mutation MUST go through an application service that validates tenant, invariants, and transaction boundaries.
- MCP MUST expose semantic tools, never arbitrary SQL or filesystem access.
- HTTP handlers MUST remain transport adapters, not business-logic containers.

### 4. Support exactly two databases

- SQLite is the default local database; PostgreSQL is the SaaS database.
- MUST use `database/sql`, sqlc-generated queries, and explicit Store adapters.
- MUST NOT add a runtime ORM or claim compatibility with an untested database.
- Every schema change MUST include matching SQLite and PostgreSQL forward migrations with the same version.
- Existing persisted data MUST NOT be destructively rewritten or dropped in a normal application upgrade.
- Every business table MUST carry `tenant_id`, including local-only deployments.

### 5. Never use floating point for money

- Monetary amounts MUST use signed integer minor units plus ISO currency code.
- All internal statistics MUST use the tenant base currency.
- A non-base-currency record MUST preserve original amount, currency, rate, rate date, and rate source.
- Base currency MUST be locked after the first monetary record.

### 6. Preserve lifecycle history

- Confirmed asset events are append-only.
- Corrections MUST void and replace; they MUST NOT overwrite the original economic event.
- Transactions may group events, but asset events remain the lifecycle source of truth.

### 7. Keep attachments storage-neutral

- All attachment I/O MUST go through `BlobStore` and the shared `ObjectKeyMapper`.
- Local filesystem and Aliyun OSS MUST use the same logical object key.
- Database rows MUST store `store_id` and `object_key`, never absolute paths, permanent public URLs, or signed URLs.
- Switching the default store MUST NOT make existing attachments unreadable.

### 8. Keep market providers replaceable

- OneBound MUST remain an adapter behind `MarketDataProvider`.
- Provider adapters may authenticate, request, paginate, rate-limit, and map fields.
- Provider adapters MUST NOT own matching, deduplication, outlier filtering, aggregation, FX conversion, or valuation policy.
- Every saved market point MUST retain provider, observation time, original currency, sample count, and calculation provenance.

### 9. Keep secrets out of persisted project data

- The GitHub repository is public. Treat every tracked file, commit, branch, pull request, Action log, artifact name, and review comment as public information.
- Secrets MUST NOT enter Git, logs, fixtures, migration files, or ordinary database configuration rows.
- Local secrets may come from `.env`; production secrets MUST be injectable as environment variables or secret references.
- `.env` MUST remain ignored; `.env.example` contains names only.
- Real secret values MUST NOT be pasted into prompts, issues, pull requests, test output, screenshots, or CI command lines.
- Every pushed checkpoint and promotion pull request MUST pass the repository `secret-scan` check. GitHub Secret Protection and Push Protection MUST remain enabled.
- A suspected leak requires stopping publication, revoking or rotating the credential first, then removing it from current files and Git history before work resumes.

### 10. Keep the AI outside the application core

- The application MUST NOT require a model API key for its core operation.
- The AI Harness owns screenshot understanding and MUST confirm extracted fields with the user before invoking a write MCP tool.
- A semantic MCP mutation is a confirmed user command; the application MUST validate authorization, inputs, invariants, and transaction boundaries, then persist it without a second pending-review step.
- Later corrections through Web or MCP MUST use the same append-only lifecycle correction use case.

### 11. Test every compatibility boundary

- Store behavior MUST run against both SQLite and PostgreSQL.
- Blob behavior MUST run against local storage and an OSS-compatible test double or isolated test bucket.
- Provider normalization MUST be tested with recorded, secret-free fixtures.
- A migration change is incomplete without an old-version upgrade test.

### 12. Reject scope drift

- MUST NOT add speculative abstractions with no second real implementation or immediate boundary requirement.
- MUST NOT replace server-rendered HTML with a SPA without an approved architecture change.
- MUST NOT introduce a second service merely to organize code.
- When a request conflicts with a hard boundary, stop and propose an explicit architecture-document change first.

### 13. Preserve the delivery chain

- `dev`, `uat`, and `prod` are permanent branches; `dev` is the default development baseline.
- The GitHub repository default branch MUST be `prod`, so public visitors land on published production history; this does not make `prod` a development baseline.
- Substantial work MUST use a short-lived `dev-<scope>` branch. Focused follow-ups MAY use `feature/<scope>` or `fix/<scope>`. Production promotion MUST use a short-lived `release/<scope>` branch based on the current `prod`.
- Commit and push each independently verified checkpoint. Do not accumulate unrelated work in one commit.
- During a user-declared rapid local iteration phase, related changes MUST stay on the active work branch and appear in the local development instance immediately after the narrowest relevant test or smoke check passes.
- A passing development checkpoint MUST NOT trigger UAT promotion. The agent MUST NOT open or merge a UAT pull request, run the UAT packaging chain, or delete the active work branch until the user explicitly identifies the current batch as a UAT checkpoint.
- Work-branch pushes run secret scanning but MAY defer the full suite, dual-database full-element run, sqlc verification, and packaging until UAT promotion. Required regression tests still MUST be written with the behavior and run at their narrowest relevant layer during development.
- Promote tested work to `uat` through a squash-merged pull request, then delete the short-lived branch and fast-forward permanent `dev` to the accepted `uat` baseline.
- A successful UAT build is only an acceptance candidate. The agent MUST stop after UAT verification and MUST NOT create a production release branch or pull request until the user explicitly authorizes that specific UAT result for production.
- Promote only accepted UAT content to `prod`: reconcile the tested UAT tree into a `release/<scope>` branch based on current `prod`, verify that its application tree matches the accepted UAT commit except explicit release metadata, and squash-merge its pull request to `prod`. `uat` and `prod` MUST reject direct pushes, force pushes, deletion, and non-linear history.
- `uat` and `prod` MUST use the same packaging workflow with separate GitHub environments. Production publication MUST consume artifacts produced and smoke-tested by that workflow.
- Windows release archives MUST be `.zip`; Linux and macOS release archives MUST be `.tar.gz`.
- A production defect follows `fix/<scope> -> uat -> prod`; it MUST NOT bypass UAT validation.
- Ordinary work branches start from `dev`; a production defect or production rollback starts from current `prod` on `fix/<scope>`, then follows the same explicit UAT and production approval gates. The production starting point is an exception to the ordinary development baseline, not to validation.
- Before UAT merge, verify permanent `dev` can fast-forward to the expected accepted baseline. Preserve any divergent commits on a work branch and reconcile through a pull request; never force-update or silently discard permanent-branch history.

### 14. Grow regression coverage with every feature

- Every new observable behavior MUST include an automated regression test in the same change.
- Every bug fix MUST first add or identify a test that fails for the defect and passes after the fix.
- Use the narrowest sufficient layer: pure domain tests, application policy tests, shared Store conformance tests, or HTTP handler tests.
- The repository MUST maintain one named full-element scenario that grows with each accepted vertical slice and exercises the supported flow through real application services and Store adapters.
- UAT validation MUST run the full-element scenario against both SQLite and PostgreSQL before packaging; a missing, skipped, or partially selected database run is a failure.
- Existing regression coverage MUST NOT be removed while the behavior remains supported. Replacing a test requires equal or stronger evidence.

## Change discipline

- Update `CODEMAP.md` in the same change when paths, entry points, or ownership move.
- Update `docs/ARCHITECTURE.md` in the same change when a boundary, dependency, persistence rule, or deployment shape changes.
- Update `docs/PROJECT_PLAN.md` only when product scope or implementation phases change.
- Prefer the smallest implementation that preserves these invariants.
- Follow `docs/DEVELOPMENT_WORKFLOW.md` for branch creation, promotion, packaging, and rollback.
