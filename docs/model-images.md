# Product-model images

## Configuration and scope

Open Settings → Item types → edit a model → Model image. Upload a PNG, JPEG or
WebP, optionally record its public source page, or expand Import from URL for a
public HTTPS image direct link. The server downloads and stores the bytes; remote
URLs are not permanent rendering dependencies. Each model has one current image.
The model image is an illustration, not a claim about an individual item's color.

Assets inherit this image in their detail/editor view and list thumbnails. A ready
3D viewer takes precedence; unavailable 3D leaves the image fallback visible.
Replacement and removal do not modify 3D bindings or economic events. Removal
detaches the current image but retains revisions and blobs; restoration and garbage
collection UI are not implemented. Web and MCP share this use case: `get_model_image`
reads the current revision, `import_model_image_from_url` performs a confirmed,
receipt-backed HTTPS import of the same validated image, and `upload_model_image`
applies the same validated replacement from at most 8 MiB of bounded base64 content
with no network access. Trade-in linking remains a separate proposed feature.

## Import constraints and recovery

- 8 MiB maximum and 16 million decoded pixels; complete decode required.
- File extensions and remote MIME declarations are not trusted. Actual PNG, JPEG
  and WebP bytes determine the stored content type.
- URL imports reuse the existing public HTTPS downloader with bounded redirects,
  timeouts, pinned public DNS answers and no inherited credentials/proxy.
- Reserved/fake-IP DNS results are rejected. If a browser can open a public image
  but the server cannot retrieve it, download in the browser and use file upload
  (Web) or the bounded `upload_model_image` content path (MCP).
  Do not relax private-address or TLS checks to make a preview work.
- A public source page is attribution, not a credential-bearing download URL.
- Content upload accepts only base64 PNG, JPEG or WebP bytes with the same size,
  format and decode limits; an impossible encoded length is rejected before decoding.
  It accepts no local path, fetch URL or credential and performs no network I/O.

## Real-world findings

Before downloading for import, open the original candidate image and visually
check the exact model, color, whole-product framing and clarity. If temporary
download is necessary to inspect it, it is not yet approved for upload. Do not
substitute multi-color lineups, color swatches or cropped detail shots for a
matching product photo. Front/back views of the same color are acceptable.
After saving, re-read the binding and inspect the actual asset display. Consider
other assets sharing this model before replacing a shared image.

The initial multi-color illustration was rejected; a complete white front/back
product image was subsequently inspected and uploaded, with the asset view
verified. No economic records changed. That acceptance was Web-based; the MCP import
tool reuses the same validated upload use case with a durable request receipt.

The Xiaomi 15 specifications page presents a multi-color model illustration. Its
color swatches are tiny images, not product photos. Inspect the actual image and
dimensions before importing. The bare PNG URL retrieved during local verification
had a valid header but failed complete decoding; the browser's observed WebP
variant decoded successfully and was saved through the normal upload use case.
Never treat a filename suffix or a successful HTTP response as a valid image.

The local preview was restarted with its original database and BlobStore paths;
the existing purchase and service events were not altered. No customer screenshot,
order identifier, account credential or downloaded product image is committed.

## Validation

Automated coverage includes three-format decoding, invalid/truncated/oversized
inputs, authorization before download, download failure recovery, public URL and
response-size checks, CSRF, upload/read/replace/clear, tenant isolation, revision
retention, storage-default switching and old-schema upgrades. The named full-element
scenario includes image operations for both Store implementations. Receipt-backed MCP
import is covered for both databases and over HTTP: absent-image null, replay without
re-downloading, changed-payload conflict, concurrent identical import with one surviving
revision, failed-receipt rollback, role/scope denial and the public metadata projection.
The bounded content upload is covered over the same MCP HTTP session and both databases:
input validation (bad base64, empty, non-image, oversize), no downloader call, same-key
replay, changed-content conflict, one surviving concurrent revision, denial by scope and
role, no content/path/metadata leakage, and an older-key replay that must not replace a
later active image.
Local Go tests, Web interaction tests and SQLite scenario passed. Live PostgreSQL verification
remains required before UAT; a locally skipped PostgreSQL test is not a pass.

The current local browser showed the saved image on the detail and list views.
Explicit 390/1280 viewport overrides did not change its observed 631-pixel viewport;
do not interpret the captures as independent mobile/desktop acceptance.
