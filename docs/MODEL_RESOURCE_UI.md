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
