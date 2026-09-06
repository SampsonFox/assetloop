# Specification-tag development acceptance

Date: 2026-09-06. Scope: the approved tag plan, amended to remove the old variant
hierarchy completely. This is a development checkpoint, not UAT or production.

| Requirement | Evidence |
|---|---|
| Category → model → item; optional typed descriptors | Domain selection policy, SpecificationService.SaveAsset and shared Store tests; model is required, all tag dimensions may be empty. |
| Reuse, normalized uniqueness, single/multiple dimensions | Domain specification tests and storetest/specifications.go; memory and storage remain separate dimensions. |
| Model allowances, disabled-history retention and references | Transactional SaveModel/SaveTag/SaveAsset policies; shared Store tests reject shrinking used allowances and new disabled selections. |
| Concurrent edits cannot bypass constraints | Shared tests race allowance removal against selection and appearance binding against resource deletion using independent connections. |
| Stable 3D defaults and candidate-only discovery | EffectiveForAsset/Candidates tests cover shared color across capacities, most-specific rules, ambiguity, no optional selection, explicit item overrides, unbinding and descriptive edits not rebinding. |
| Resource upload and deletion safety | Local/OSS Blob suites, application failure tests, Store media conformance and HTTP resource tests. |
| Old hierarchy removed | Paired 00014 migrations, removed runtime types/ports/queries/routes; HTTP rejects variant_id. Historical migrations remain immutable. |
| Upgrade preserves data | Migration fixtures cover text/color backfill, conflicts, old files and unknown-FK rollback/retry. Original SQLite preview upgraded 13→14 with automatic backup; retained asset fields, all 15 events, two tag links and resource metadata matched the backup exactly. GLB bytes/hash unchanged. |
| Management/entry interfaces | Web regression covers dictionary, model allowances, direct item forms, resource descriptions, appearance confirmation/upload, authorization, CSRF and error echo. |
| Existing features retained | Full Go regression and 50 Node tests cover lifecycle/cost, viewer framing/fallback, list queries and drawer navigation. |
| Frontend verification | Desktop and 390px dictionary captures, no horizontal overflow; active navigation and heading fixes confirmed. Live light theme, English UI and Escape closure checked; original zh-CN/system/violet preferences restored. Prior bounded UI review retained; no visual redesign. |
| Documentation | ARCHITECTURE.md, PROJECT_PLAN.md, CODEMAP.md, PRODUCT.md, DESIGN.md, surface brief, inline review and design sidecar updated. |
| Reproducible generated queries | sqlc generation repeated; all eight generated files unchanged. |

## Commands

```text
go test ./cmd/... ./internal/... ./migrations/... -count=1 -timeout=120s
node --test internal/web/*.test.mjs
sqlc generate
```

The explicit Go package set avoids unrelated ignored temporary work under tmp/.
SQLite executes the application/Store/full-element scenarios. PostgreSQL packages
compile, but DSN-dependent tests skip: the user's server is inaccessible and the
user explicitly deferred that execution. Never count those skips as validation.

## Gates and limitations

- PostgreSQL migrations, concurrency and dual-database full-element execution are
  required before UAT. No UAT PR, packaging or production deployment was started.
- The one design detector run fell back to regex because parser dependencies were
  unavailable. No regex findings does not prove computed contrast or accessibility.
- The finish review was inline, not independent. Its final `ship` verdict applies
  to the two listed visual fixes; see specification-ui-review.md.
- The existing tag type named “旧规格描述” contains preserved original text. It is
  an ordinary tag dimension, not a surviving ProductVariant or compatibility API.
  Its contents were not guessed into capacity/CPU dimensions.
- No new shipping raster assets were introduced; the existing poster and GLB were
  preserved. Review captures stay local and are not published.
