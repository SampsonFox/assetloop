# 3D resource management surface

Mode: Operate. Extend AssetLoop's existing server-rendered catalog, drawer, and asset-edit conventions.

## Direction contract

THESIS: Make shared resources and instance overrides understandable through explicit binding sources and reference counts.

OWN-WORLD: Existing semantic light/dark colors, accent choices, system type, table rows, native controls, and responsive drawers.

STORY: Find or upload a named GLB; bind it to a model, color-bearing specification, or physical item; see whether the item inherits or overrides.

FIRST VIEWPORT: Page heading and upload action, compact search controls, then a paged resource table with name, size, attribution, references, and actions. Binding editors show their current resource and inheritance state before selection or upload.

FORM: User-approved existing-world table/drawer extension; no concept seed required. Preview loads on demand; no automatic model rotation.

FINISH: Verify desktop/mobile, localization, inherited and overridden states, and reference-protected deletion. Record review evidence without changing the established visual system.

## Interaction decisions

- GLB bytes are immutable. Uploading a replacement creates a resource; editing attribution changes the shared resource description.
- Only an absent binding inherits. A failed selected model falls back to the existing image.
- Resource pickers fetch paged search results on demand. Do not preload the full resource library.
- Product color belongs to the specification; the resource table has no product color or capacity fields.
- A saved item can choose a dedicated resource or restore inheritance. Creation first saves the item.

## Built surface

- `/admin/3d` ships a searchable library with 25 resources per page, an upload drawer, and upload-and-bind selection with explicit binding/inheritance context.
- `/admin/3d/{id}` combines preview, shared attribution, reference links, reference-protected deletion, and deletion retry for pending resources.
- Existing theme colors, typography, native controls, and drawers are retained; the resource table becomes labeled rows on narrow screens, and preview/edit content stacks vertically.

## Validation notes

Evidence below is reported by the parent task; this documentation pass inspected the templates and stylesheet without rerunning validation.

- PASS: local Go packages (`cmd/...`, `internal/...`, `migrations/...`), reproducible sqlc hashes, and migration/full-element scenarios on both SQLite and PostgreSQL.
- PASS: PostgreSQL Store conformance repeated three times, each including 12 cross-connection races; all five Node viewer-mechanics tests passed.
- Manual checks at 1280×900 and 390×844 covered light/dark preview, keyboard rotation/zoom/reset, upload, binding, and unbinding without overflow.
- Reduced motion, unavailable WebGL, and selected-model load failure were checked in automated VM tests; physical touch remains untested.
- Mechanical detection is DEGRADED: regex results were `[]`, but the HTML parser was unavailable; computed contrast has not been proven.
- The original SQLite database used by the local app was upgraded after a verified backup; no demo database was used. Temporary resource and asset override were removed; original asset fields, nine events, and six transactions matched the backup.
- Specialized finish reviewer/documenter roles were unavailable; default substitutes were used. Independent finish review returned “ship” for the five-screenshot-and-samples scope, with no material fixes; this is not UAT or production approval.
- Ignored review evidence in `.impeccable/review/`: `desktop.png`, `mobile.png`, `desktop-dark.png`, `library-desktop.png`, and `library-mobile.png`.
