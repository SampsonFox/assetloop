# Code Map

Purpose: give agents and contributors the smallest useful reading set before they search the repository. Keep this file concise and update it whenever paths or ownership change.

Status: v0.1 foundation plus authentication/RBAC, asset catalog, append-only lifecycle, and product-model 3D media vertical slices are implemented. MCP is in development (see `docs/MCP_IMPLEMENTATION.md`); general attachments, market and scheduler remain later slices.

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
| `internal/web/` | HTTP transport; asset-list-first SSR UI, server-filtered/sorted/paged tables, detail-style asset create/edit pages, inherited GLB viewer, code-defined zh-CN/en language packs, account menu, semantic light/dark themes with user accent palettes, shared catalog drawers, inline custom lifecycle event types, and progressively disclosed FX evidence forms |
| `internal/web/resources.go`, `resource_tags.go`, `resources_i18n.go`, `templates/resources.html`, `templates/resource.html` | Paged 3D library with shared list backdrop and wide preview/edit drawer; atomic metadata/tag save, attribution, reference navigation and deletion retry |
| `internal/web/shared_rename.test.mjs` | Conditional shared-name confirmation, no-JS confirmation fallback and unified resource editor form coverage |
| `internal/web/templates/ui_icons.html`, `management_ui.test.mjs`, `resource_presentation_test.go` | Shared named action icons; consistent compact catalog/tag/resource/event-type row actions and drawer controls; accessible-label/layout regression checks |
| `internal/web/model_configuration.go`, `model_configuration.test.mjs`, `templates/catalog_drawers.html` | Unified model metadata/tag/appearance submission and transactional save; management drawers share `data-management-drawer` fixed heading actions and independent scrolling, covered by `management_ui.test.mjs` |
| `internal/mcp/` | Streamable HTTP semantic query adapter calling application services; SDK HTTP regression tests; not mounted until OAuth wiring is complete |
| `internal/scheduler/` | Refresh-job entry adapters |
| `internal/application/` | Authentication, catalog, model-media, lifecycle use cases, validation, and inward ports shared by Web and semantic MCP writes |
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
| both Store `model_media.go` adapters | Resource persistence, transactional bindings and guarded pending deletion; effective appearance policy lives in application | application ports, domain |
| `internal/blob/local/` | Local filesystem BlobStore | blob port |
| `internal/blob/aliyun/` | Aliyun OSS BlobStore | blob port, Aliyun SDK |
| `internal/blob/key_mapper.go` | Shared tenant-scoped logical object keys | application key-mapper port |
| `internal/market/onebound/` | OneBound request/response adapter | market port |
| `internal/market/manual/` | Manual/imported market observations | market port |
| `migrations/sqlite/` | SQLite forward migrations | none |
| `migrations/postgres/` | PostgreSQL forward migrations | none |

## Core ports

| Symbol | Expected location | Implementations |
|---|---|---|
| `BlobStore` | `internal/application/ports.go` | Local, Aliyun OSS |
| `ObjectKeyMapper` | `internal/blob/key_mapper.go` | one shared mapper |
| `MarketDataProvider` | `internal/application/ports.go` | OneBound, Manual |
| `FXProvider` | `internal/application/ports.go` | selected FX source |
| `AuthStore` | `internal/application/ports.go` | SQLite, PostgreSQL (implemented) |
| `CatalogStore` | `internal/application/ports.go` | SQLite, PostgreSQL (implemented) |
| `LifecycleStore` | `internal/application/ports.go` | SQLite, PostgreSQL (implemented) |
| `ModelMediaStore` | `internal/application/ports.go` | SQLite, PostgreSQL (implemented) |

## Regression spine

Specification-tag refactor: `internal/domain/specifications.go` owns optional typed selections, retained disabled values, per-model appearance overrides and deterministic confirmed-rule matching. Paired `00013` SQL files define the direct asset/model relation and typed associations; `internal/store/specification_upgrade.go` executes their DDL and Unicode-aware legacy backfill in one Goose transaction. `specification_migration_test.go` covers rollback/retry, old colors/descriptions, conflicting GLBs and preserved history. `internal/application/specifications.go`, `specification_queries.go` and `specification_ports.go` provide transactional tag/model/asset/resource maintenance, references, candidates and effective appearance resolution. Both Store `specifications.go` adapters use generated `specification.sql` queries and the shared tenant write lock. `storetest/specifications.go` tests these application paths over both adapters (PostgreSQL execution needs its test DSN). Tag Web integration and migrations 13–14 are deployed to local 8080; development evidence is recorded in `docs/specification-acceptance.md`; PostgreSQL live validation remains a UAT gate.

Tag Web integration: `internal/web/specifications.go` and `templates/specifications.html` provide the type/value dictionary, references and shared-name confirmation. `model_tags.go` owns model allowance transport and appearance-rule endpoints; `asset_tags.go`, `static/asset-tags.js` and `asset_tags.test.mjs` implement direct model selection, optional tag inputs and confirmation before dropping incompatible selections. `resource_tags.go` edits descriptive resource associations without rebinding. `appearance.go` and `templates/appearance.html` provide confirmed defaults, category/tag candidates, manual resource selection, preview links, transactional upload/binding. The media application service uses the shared appearance resolver, not the old variant resolver. Tag UI is the only supported path; old variant-maintenance routes are removed. The original 8080 SQLite preview is now on migration 14 with verified backup/data/GLB preservation. Legacy specification input translation has been removed; `specifications_test.go` rejects obsolete parameters and tag-aware list search uses both generated query sets. The first browser QA and correction confirmation are complete; Final development evidence and explicit PostgreSQL deferral are recorded in `docs/specification-acceptance.md`.

Cost dashboard: `internal/application/cost_dashboard.go` reads the full authorized lifecycle; `internal/domain/cost_dashboard.go` owns exact net/daily cost, calendar-day duration, trend and expense grouping. `internal/web/cost_dashboard.go`, `templates/cost_dashboard.html` and `static/cost-timeline.js` render SVG reports and progressive timeline details without market estimates or persistence changes. Timeline list filters never scope dashboard calculations.

`internal/web/static/timeline-query.js` progressively enhances timeline GET forms with debounced search, separately applied advanced filters, cancellable result-only refresh, and outside/Escape dismissal. The model viewer and cost dashboard DOM remain untouched.

`internal/web/static/asset-query.js` enhances the asset toolbar with cancellable, debounced search and local filter/sort/page/view updates. It preserves the toolbar and focus, refreshes results/count/navigation from SSR, retains native GET fallback, and is covered by `internal/web/asset_query.test.mjs`.

Event types: `internal/application/event_types.go` owns paged management, rename/direction locks and enable/disable policy under the lifecycle write transaction. Both Store `event_types.go` adapters use sqlc queries; migration `00012` links historical events to tenant-scoped IDs and seeds immutable system types. `internal/store/event_type_upgrade.go` reports unmappable legacy records before migration. `internal/web/event_types.go`, `event_types_i18n.go` and `templates/event_types.html` provide the management page and shared creation entry point. `storetest/event_types.go` and `event_type_migration_test.go` cover compatibility and concurrency.

| Path | Coverage |
|---|---|
| `internal/application/*_test.go` | validation, exact money/FX conversion, auth, catalog, lifecycle, and role policy |
| `internal/store/storetest/` | shared dual-database auth/catalog/model-media/lifecycle behavior, custom event types, bulk relation loading, server-side list query contracts, aggregate summaries, FX evidence, append-only correction, locks, and tenant isolation |
| `internal/store/migration_upgrade_test.go` | previous-schema upgrades, including lifecycle-table expansion, without data loss |
| `internal/store/model_media_migration_test.go`, `storetest/model_media.go` | Color splitting and legacy media preservation, transactional migration rollback/retry, resource reuse and cross-connection bind/delete races on both databases |
| `internal/store/schema.go`, `migration_lock*.go` | read-only schema compatibility checks and cross-process SQLite/PostgreSQL migration locks |
| `internal/store/migration_safety_test.go`, `migration_lock_test.go` | concurrent upgrades, unchanged-schema backups, version rejection, recovery backups, and crash-released locks |
| `internal/application/lifecycle_write.go`, both Store `lifecycle_write.go` adapters | transaction-bound lifecycle policy and durable tenant/user-scoped idempotency receipts |
| `internal/store/storetest/lifecycle_safety.go`, `internal/web/lifecycle_retry_test.go` | concurrent mutations, independent-connection retries, receipt rollback, and Web form replay |
| `internal/web/i18n.go` | registered locales, stable message keys, browser/cookie locale matching, and zh-CN fallback |
| `internal/web/*_test.go` | auth, CSRF, locale/theme preferences, role-scoped account menu, asset-list states, shared drawers, catalog, GLB upload/read and fallback, progressive FX evidence, correction, totals, and role denial |
| `internal/web/viewer_mechanics.test.mjs` | Dependency-free Node test harness for viewer framing, keyboard controls, reduced motion, idle rendering and failure fallback |
| `internal/integration/full_element_test.go` | cumulative auth → persisted preferences → typed model allowances → direct items with different capacity tags sharing an appearance GLB → dedicated override/inheritance → foreign purchase → repair correction → sale scenario on both databases |

## Read paths by task

Asset 3D selection: `templates/asset_form.html` and `static/asset-tags.js` provide
a native searchable resource choice and child preview. `SaveSpecificationAsset.ResourceID`
commits the optional override alongside metadata/tags through the specification transaction;
nil preserves existing bindings and empty restores inheritance. Shared Store conformance
covers atomic rollback; Web transport retains ordinary form and drawer save paths.

| Task | Start here | Then read only |
|---|---|---|
| Change money or FX behavior | `internal/domain/` | relevant application use case and Store mapping |
| Add lifecycle event | `internal/domain/lifecycle.go` | `internal/application/lifecycle.go`, both lifecycle adapters and migration pair |
| Change asset catalog | `internal/application/catalog.go` | domain asset types, both catalog adapters, Web catalog templates |
| Add database field | both migration directories | both sqlc query directories, Store conformance tests |
| Add attachment behavior | blob port and key mapper | local and Aliyun adapters, attachment application service |
| Change product 3D media | `internal/application/model_media.go` | resource library, model/appearance/asset bindings, Blob adapters, both Store mappings, Web asset/catalog/resource templates |
| Add market provider | market port | provider adapter plus shared normalization pipeline |
| Change MCP tool | `internal/mcp/` | called application service; never inspect Store unless service contract changes |
| Change Web screen | `internal/web/server.go` | affected template under `templates/`, then `static/app.css` or local `static/app.js`; called application service only when behavior changes |
| Change locale or theme | `internal/web/i18n.go` | affected templates, semantic variables in `static/app.css`, then Web locale/theme tests |
| Change deployment config | `internal/config/` | `.env.example`, README deployment section |
| Change architecture | `AGENTS.md` | `docs/ARCHITECTURE.md`, then affected code |
| Change Git/release workflow | `docs/DEVELOPMENT_WORKFLOW.md` | `AGENTS.md`, delivery architecture, GitHub workflows |

## Core execution flows

Retired foundation path: the unused `AssetService`, implicit category/model/variant
upsert Store API, and its generated queries are removed. `ids.go` retains the shared
ID generator; typed `SaveAsset` conformance now proves asset round trips, explicit
model reuse and cross-space reads instead of testing the retired implicit creation.

Approved follow-up: retire the old variant hierarchy completely. The specification
asset command and draft no longer translate legacy IDs; Web rejects obsolete
`variant_id` submissions. The retired translator test is replaced by HTTP rejection
coverage in `specifications_test.go` and direct model/tag Store conformance.
Paired migration 00014 and `internal/store/specification_contract.go` retire
schema-13 variant tables/mappings and asset variant/color fields. Unknown foreign
key dependencies roll back the contract migration and permit retry. Runtime domain,
Store ports, sqlc queries and Web have one direct-model/tag path; `TagSummary` is
computed from selected tags. Migration tests preserve existing item pins and GLBs.
The existing 8080 preview is on schema 14. The automatic schema-13 backup was compared against the upgraded asset, all 15 lifecycle rows, selected tags and resource metadata; every retained field and GLB hash matched.

Tag dictionary pagination: both Store `specification_lists.go` adapters use the
generated count/page queries in `specification.sql`; the application normalizes
search text and checks access. `storetest.RunSpecifications` covers normalized
literal search, type/status filtering, paging and data-space isolation.

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

3D resources and bindings:
Admin Web -> ModelMediaService -> verified Blob + resource metadata and binding transaction
Asset detail -> asset / confirmed appearance / model default resolver -> registry[store_id] -> BlobStore.Open
Resource deletion -> reject references -> pending deletion -> BlobStore.Delete -> remove metadata
```

## Maintenance rule

Do not list every file. List stable entry points, ports, and ownership boundaries. Once code exists, use `rg` inside the selected path rather than loading the whole repository.

## Unified settings navigation

Associated details use `internal/web/drawer_transport.go` (`X-Assetloop-Drawer`
fragment negotiation and semantic save responses) and `static/drawer-stack.js`
(ordered modal stack, independent submissions, in-place related-field updates,
cancel/race guards and responsive push). Existing application authorization and
CSRF checks remain mandatory. `drawer_catalog.go`, `drawer_assets.go` and
`drawer_resources.go` provide category details, asset save transport and target
3D binding details; `templates/resource_binding.html` is the binding editor, not
a management-list drawer. `drawer_*_test.go` and `drawer_stack.test.mjs` cover
the negotiated endpoints and stack behavior. Viewers expose container-scoped
initialization and receive `drawer:visibility` / `drawer:dispose` events.

`internal/web/settings.go` owns the authenticated `/settings` landing and module
classification. Existing catalog, tag, 3D and lifecycle-type routes share the
settings shell in `templates/base.html`; members remain separate.
`static/settings.js` replaces only root management content, preserves per-tab
list/binding context, handles history/retry and guards dirty/uploading forms.
Preview detail routes use native navigation so viewer modules keep their own
lifecycle. `static/app.js` idempotently initializes replaced drawers through
`settings:loaded`. Tests: `settings_test.go`, `settings_navigation.test.mjs`.

## Linked model tag transfers

`static/tag-transfer.js` progressively enhances `catalog_drawers.html` into two
linked transfers: allowed tag values and model-specific appearance dimensions.
`tag-transfer-state.mjs` owns pure selection/default state; native named controls
remain the submission source and no-JavaScript fallback. `tag_transfer.test.mjs`
covers dynamic types, bulk changes, inactive/disabled values, override pruning
and restoring defaults. No schema or application API change.
Transfers use checkbox-free selectable rows, Ctrl/Cmd and Shift selection,
double-click/Enter moves and central arrow actions; touch taps toggle selection.
Temporary highlights never mutate the named form fields or dirty state.
The appearance heading has one reset-all action that clears model overrides,
without changing allowed tags. A row's Default marker identifies only tag types
whose default is appearance-affecting, independently of the model override.

Shared drawer dismissal in `static/app.js` compares named form values with the
opening snapshot, routes Cancel/X/backdrop/Escape through one close guard, and
uses a page-owned confirmation dialog instead of browser-native confirm.
`dialog_close.test.mjs` covers unchanged/reverted values, secondary forms,
retaining/discarding drafts, upload changes and duplicate close requests.
Standalone form pages use the same visible discard prompt for ordinary same-tab
links (including Return/Cancel); confirmed navigation bypasses the duplicate
native unload warning. Browser refresh/tab close retains native unload protection.
