---
version: 1
slug: "internal-web-templates-model-image-html"
primary_target: "internal/web/templates/model_image.html"
related_targets: ["internal/web/templates/catalog_drawers.html", "internal/web/templates/asset.html", "internal/web/templates/assets.html"]
---

THESIS: Product-model images give assets a recognizable visual without requiring 3D.
OWN-WORLD: Inherit existing server-rendered settings, subdued theme, and icon actions.
STORY: Model settings → inspect image → upload or detach → asset fallback.
FIRST VIEWPORT: Model name, preview and file constraints; one primary upload action.
FORM: Independent model image form; no nested forms or coupling with 3D bindings.
FINISH: Accessible names, bounded preview, responsive width and localized recovery.

## Observed implementation

- Settings → Item types → edit an existing model → Model image opens a dedicated
  page. One current default image is shared by the model's assets in detail/editor
  views and list thumbnails. It illustrates the model; color is for reference.
- Image configuration is independent of 3D. A ready 3D viewer takes precedence;
  absent or failed 3D leaves the image fallback visible. Without an image, the
  category icon remains. Replacement and clear leave 3D bindings and lifecycle
  events unchanged; clear detaches the image and retains historical files.
- The page shows the model name, current preview or empty message, a required file
  picker and optional source-page URL. Upload/replace accepts PNG, JPEG and WebP,
  up to 8 MiB and 16 million decoded pixels, with complete decoding required.
  Upload, conditional clear and disclosed HTTPS import use separate native forms.
- Import from URL expands a direct-image URL form with optional source attribution.
  The server stores downloaded bytes from a public HTTPS URL; a webpage, login or
  private-network link is unsupported. Localized errors explain retry; a failed
  upload requires file reselection, and failed import can recover via file upload.
- Back, upload, clear and import use existing icon actions with localized accessible
  names and titles. The asset detail caption is static, localized reference text,
  not an action. Detail back/edit actions remain separate from the illustration.
- Source styles bound the settings region to 680px and contain the preview image
  within its available width at 280px height. Detail images also use contain sizing;
  list thumbnails are 44px squares. These are observed local styles, not new tokens.

Sources: `internal/web/templates/model_image.html`, `asset.html`,
`catalog_drawers.html`, `internal/web/static/app.css`,
`internal/web/model_images_i18n.go` and `docs/model-images.md`, with `PRODUCT.md`
providing the existing operational and accessibility context.

## Review disposition and limits

Independent fresh reviewer disposition: **ship only the current asset detail at
631×560**. Other widths and configuration-form visuals remain unverified. Earlier
390/1280 viewport requests still produced a 631px viewport; they do not establish
independent mobile/desktop acceptance. This documentation pass used source review
only and performed no browser verification.

No documentation gate blocks this local merge. Broader visual acceptance needs
actual captures of the other widths and configuration form; live PostgreSQL
verification remains a separate required UAT gate in `docs/model-images.md`.
This brief adds no global design rules or trade-in implementation scope.
