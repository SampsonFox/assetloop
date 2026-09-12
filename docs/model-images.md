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
collection UI are not implemented. This is a Web slice, not a completed MCP image
tool contract. Trade-in linking remains a separate proposed feature.

## Import constraints and recovery

- 8 MiB maximum and 16 million decoded pixels; complete decode required.
- File extensions and remote MIME declarations are not trusted. Actual PNG, JPEG
  and WebP bytes determine the stored content type.
- URL imports reuse the existing public HTTPS downloader with bounded redirects,
  timeouts, pinned public DNS answers and no inherited credentials/proxy.
- Reserved/fake-IP DNS results are rejected. If a browser can open a public image
  but the server cannot retrieve it, download in the browser and use file upload.
  Do not relax private-address or TLS checks to make a preview work.
- A public source page is attribution, not a credential-bearing download URL.

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
verified. No economic records changed. This was Web acceptance, not MCP upload.

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
scenario includes image operations for both Store implementations. Local Go tests,
Web interaction tests and SQLite scenario passed. Live PostgreSQL verification
remains required before UAT; a locally skipped PostgreSQL test is not a pass.

The current local browser showed the saved image on the detail and list views.
Explicit 390/1280 viewport overrides did not change its observed 631-pixel viewport;
do not interpret the captures as independent mobile/desktop acceptance.
