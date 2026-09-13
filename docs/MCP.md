# MCP setup and tool contract

For the 2026-09-12 UAT batch capabilities, changes and exclusions, see
[MCP release notes](MCP_RELEASE_NOTES.md). Product-model images ship as the
[Web configuration feature](model-images.md) plus three tools: the `get_model_image`
read, the `import_model_image_from_url` confirmed import and the bounded-content
`upload_model_image` replacement that needs no network.

AssetLoop exposes opt-in Streamable HTTP at `/mcp` in the existing Go server.
It uses the same application services, database and authorization rules as Web.
No model API key, second service or stdio process is needed. Development evidence
and remaining acceptance items are in [MCP implementation](MCP_IMPLEMENTATION.md).
The verified Windows Codex setup sequence, failure modes and native-tool lifecycle
evidence are in [Codex acceptance notes](CODEX_MCP_ACCEPTANCE.md).

## Enable locally

Set these non-secret values in your environment or ignored `.env`:

```dotenv
AUTH_MODE=local
HTTP_ADDR=127.0.0.1:8081
MCP_ENABLED=true
MCP_ISSUER=http://127.0.0.1:8081
MCP_CLIENT_ID=codex-local
MCP_REDIRECT_URI=http://127.0.0.1/callback
```

Run `assetloop serve`, or `go run ./cmd/assetloop serve` from the repository.
Use explicit absolute `DB_DSN` and `ATTACHMENT_LOCAL_ROOT` values when maintaining
more than one local instance. Do not point an acceptance instance at live data.
SQLite upgrades on startup; PostgreSQL requires the existing migration workflow.
Initialize the account through `/setup` if this is a new database, otherwise sign
in normally. MCP cannot be enabled with `AUTH_MODE=disabled`.

The issuer must be an origin without a path/query/fragment and must match the
address used by the client. Plain HTTP is accepted only for loopback IPs. This
guide does not authorize public deployment or changes to shared infrastructure.

## Connect Codex

With the server running, add the pre-registered client:

```sh
codex mcp add assetloop-dev --url http://127.0.0.1:8081/mcp --oauth-client-id codex-local --oauth-resource http://127.0.0.1:8081/mcp
codex mcp login assetloop-dev --scopes assets:read,assets:catalog,assets:lifecycle
```

The add command may start OAuth immediately; a separate login is needed only
when authentication is still required. Follow the browser consent page and check
the account and permissions before allowing access. Do not paste bearer tokens
or Web cookies into configuration. Confirm the callback printed by Codex matches
the registered path. AssetLoop advertises issuer-bound callbacks, includes `iss`
in the authorization response, and permits variable ports for the registered
loopback callback. The client ID is not a secret. See the official
[Codex MCP guide](https://learn.chatgpt.com/docs/extend/mcp?surface=cli).

Use `codex mcp list` to inspect configuration. The client must load the new server
before tools become available; an already-running task's tool catalog is not
proof that the server has been refreshed. Desktop acceptance is tracked separately
from automated SDK tests.

Each authorization lasts at most 30 days. Access tokens last at most 10 minutes;
refresh tokens rotate. Reusing a consumed code/refresh token revokes its grant.
The current account role is checked on every request; a role change can require
reauthorization. Revoke a client under the Web account menu, **MCP clients**
(`/account/clients`). This does not sign the user out of Web. Removing a local
Codex configuration is not a substitute for server-side revocation.

## Permissions and tools

Client scopes never grant more authority than the signed-in account. Tool inputs
cannot supply the acting tenant or user. All 53 tools remain discoverable, but
unauthorized calls fail. Ask for only the scopes needed for the intended workflow.

| Scope | Tools |
|---|---|
| `assets:read` — catalog | `get_context`, `list_assets`, `get_asset`, `list_categories`, `search_product_models`, `get_product_model` |
| `assets:read` — configuration | `list_tag_types`, `search_specification_tags`, `get_model_configuration`, `get_resource_configuration`, `get_specification_references` |
| `assets:read` — lifecycle | `list_events`, `get_event`, `list_event_types`, `get_asset_cost`, `get_portfolio_summary` |
| `assets:read` — 3D | `list_3d_resources`, `get_3d_resource`, `get_3d_references`, `get_asset_appearance`, `get_3d_binding`, `search_appearance_candidates` |
| `assets:catalog` | `create_category`, `update_category`, `create_product_model`, `save_tag_type`, `save_specification_tag`, `save_model_configuration`, `save_asset`, `save_appearance_default`, `delete_appearance_default`, `save_3d_resource_metadata`, `bind_3d_resource`, `delete_3d_resource` |
| `assets:lifecycle` | `create_event_type`, `update_event_type`, `set_event_type_enabled`, `record_event`, `correct_event` |
| `assets:catalog` — URL import | `import_3d_resource_from_url`, `import_model_image_from_url` |
| `assets:read` — market | `list_market_items`, `get_market_item`, `get_asset_market_price` |
| `assets:read` — model image | `get_model_image` |
| `assets:catalog` — model image | `upload_model_image` (bounded base64 content, no network) |
| `assets:catalog` — market preparation | `search_market_products`, `select_market_product`, `preview_market_price` (temporary drafts, no business writes) |
| `assets:catalog` — market writes | `create_market_item`, `update_market_item`, `bind_asset_market`, `refresh_market_price` |

Paged queries use `page` and `page_size`, normalize invalid/nonpositive values,
and cap page size at 200. Read all required pages before selecting IDs. Complete
configuration queries return the full association set; do not replace it with a
partial search result. An absent appearance override inherits the tag type's
default; explicit `false` overrides it. Resource descriptions/candidates never
automatically bind a resource. Binary uploads stay in Web except the bounded
base64 `upload_model_image`; only the listed URL-import tools retrieve content
server-side.

## Secondhand prices

The combined development version adds 10 market tools plus the three model-image
tools (53 total: 29 read-only annotations and 24 persistent writes). Existing
OAuth scopes are reused; catalog permission is required for external discovery/preview,
market management, model-image import and model-image content upload.
Saved prices remain readable without configured provider credentials. Configure
`ZHUANZHUAN_MCP_TOKEN` through the same ignored configuration/environment as Web;
no token, provider metric, page token, lease token or raw provider evidence is
returned by these tools.

1. Search with `search_market_products`; select the returned `draft_id` and
   zero-based candidate `index` using `select_market_product`. Asking prices only
   identify candidates and are never saved as reference prices.
2. Preview the draft's returned `keyword` and `filter_criteria` with
   `preview_market_price`. Alternatively omit `draft_id` for direct model lookup.
3. Show actual product specifications and `matched_model` separately. A broad
   model quote does not prove precise capacity/color/condition matching. Obtain
   explicit acceptance, then call `create_market_item` with `accept_scope=true`,
   a name and stable `request_key`. Saving rechecks specifications and market model.
4. Explicitly bind with `bind_asset_market`; an empty `market_item_id` unbinds.
   `save_asset` does not silently change an existing market binding.
5. Read via `get_asset_market_price` or `get_market_item` (daily history and bound
   asset IDs). Refresh one enabled series with `refresh_market_price`. Rename or
   enable/disable it using `update_market_item`; query changes create a new series.

All prices are integer minor units. `max_minor` means the provider's latest-period
maximum, not the highest trade today. `base_minor=null` means FX is pending; original
currency/amount and nullable source date/sample count stay explicit. These tools
never change lifecycle cashflows or the holding-cost dashboard. A failed refresh
preserves the previous successful quote. Successful same-key retries return the
original result without provider calls, including after draft expiry/restart;
new deliberate refreshes need a new key. Concurrent requests use the same fenced
leases as Web, CLI and startup collection.

Drafts expire after 30 minutes or restart and belong to the current user/tenant.
`market.draft_expired`, `market.selection_changed`, `market.model_changed`,
`market.accept_scope`, and `market.product_gone` require renewed selection/review;
`market.busy`, `market.temporary`, and `market.rate_limit` are retryable.
Provider configuration/authentication errors require configuration repair.
A read-only or Viewer grant cannot use discovery or mutate saved prices.

## Confirmed mutations and retries

### Event type versus event notes / 事件类型与备注

These meanings are shipped in the MCP `tools/list` descriptions and JSON Schema
field descriptions, so a new client does not need prior chat history or a personal
skill to discover them.

- `type_id` identifies a reusable lifecycle category, including built-in purchase
  (买入), repair (维修), sale (卖出), and user-confirmed custom cost types.
  Query `list_event_types` and reuse a matching enabled type.
- `notes` describes the individual event: the product or service name and context.
  For example, an extended warranty uses a reusable 服务 expense type, with
  延保服务 in `notes`; do not create a separate type for each purchased product.
- `create_event_type.name` names a reusable action category, not a purchased item.
  Create a custom type only if no suitable enabled type exists and the user
  explicitly confirms adding that category. Permission to record a purchase is
  not permission to expand the type catalog.
- `correct_event` preserves the original asset and historical record. Omit its
  optional top-level `type_id` to retain the original type. An explicit different
  type must be an enabled tenant-owned custom type with the same cash-flow
  direction, or a neutral type when both original and replacement amounts are zero.
  Built-in targets are rejected. The last acquisition cannot be reclassified away
  while a repair or sale depends on it. Never rename a shared type to correct one
  record. Type changes use the same atomic void, replacement and replay receipt.
- The built-in purchase represents acquiring the item and is unique per item.
  Services and accessories use user-confirmed custom expense types. A free gift
  uses a custom neutral type with amount 0. Every expense or income requires a
  positive magnitude. After a sale no new built-in
  purchase, repair or sale is accepted, while custom post-sale cost events remain
  recordable.

中文：事件类型回答“发生了什么行为”，备注回答“具体买了什么、有什么补充”。
手机本体使用唯一的“买入”；服务、配件使用可复用的自定义支出类型，
赠品使用无金额类型。具体商品名仍写在备注，不要一单一类型。

### Server-side GLB import

`import_3d_resource_from_url` requires `assets:catalog` and catalog capability.
Inputs: `url`, `name`, `source_url`, `author`, `license`, `request_key`.
Only supply user-confirmed publicly downloadable HTTPS links and permitted models.
`source_url` is public attribution, not a credential-bearing download link.
No Cookie/header/file path input is accepted. The server downloads at most 25 MiB,
with a 30-second network deadline, up to three redirects, HTTPS port 443 only,
no environment proxy and no private/special IP destinations (including DNS answers).
Two imports may run concurrently; saturation returns retryable `unavailable`.

Content is checked by the existing GLB validator, including external resource URI
restrictions. A successful result is ordinary public resource metadata, not a
BlobStore path. Import does not create an appearance rule or binding: query the
resource and confirm asset/model/rule scope before invoking existing binding tools.
Same-key successful retries return the same result without downloading again;
changing any input needs a new command key, not a retry. Bad URL/size/encoding/GLB
errors are `invalid_input`; network/status/timeouts return sanitized `unavailable`.
No ZIP, webpage extraction, client file upload or authenticated-site downloading.
HTTP security primitives follow [Go net/http](https://pkg.go.dev/net/http#Transport).

### Server-side model-image import

`import_model_image_from_url` requires `assets:catalog` and catalog capability.
Inputs: `model_id`, `url`, `source_url`, `request_key`. Before importing, visually
verify that the image shows the correct model and color, and confirm the shared model
scope: the image belongs to the product model, so every item of that model uses it.
Only supply user-confirmed publicly downloadable HTTPS image links; `source_url` is
attribution, not a credential-bearing download link.
The server downloads at most 8 MiB using the same public-address, redirect, deadline and
no-credential policy as GLB import, and the actual PNG/JPEG/WebP bytes determine the
stored type. A successful call replaces the model's active image and returns only safe
metadata (`id`, `model_id`, `content_type`, `size_bytes`, `sha256`, `source_url`).
Same-key successful retries return the original revision without downloading again or
replacing a later image; a changed payload conflicts. Authorization and the request
fingerprint are checked before any network access, and the download never holds a
database transaction. The tool does not touch 3D resources, bindings or lifecycle
events. `get_model_image` reads the current metadata and returns null when the model has
no image. No storage path, store ID or tenant ID is returned, and no clear or arbitrary
binary-upload tool exists; confirmed content replacement uses `upload_model_image`.

### Model-image content upload

`upload_model_image` requires `assets:catalog` and catalog capability. Inputs:
`model_id`, `content_base64`, optional `source_url`, `request_key`. It applies the same
validated replacement as the URL import without any download, which is the fallback when
the local DNS resolver cannot reach a public image host — never a reason to weaken the
downloader, DNS or proxy policy. Visually verify the exact model and color and confirm the
shared model scope before calling; the image belongs to the product model, so every item
of that model uses it. At most 8 MiB of decoded PNG/JPEG/WebP content is accepted; an
impossible encoded length is rejected before decoding and the existing validator still
enforces the size, format and pixel limits. No local path, fetch URL, credential, tenant
or store input is accepted, and the result returns only the same safe metadata
(`id`, `model_id`, `content_type`, `size_bytes`, `sha256`, `source_url`). Authorization,
the request-key check and the content fingerprint run before any blob I/O; the verified
blob and the receipt commit atomically. Same-key retries return the original revision
without writing again or replacing a later image, changed content under the same key
conflicts, and concurrent identical uploads leave one active revision. The tool does not
touch 3D resources, bindings or lifecycle events.

### Existing write rules

- Confirm screenshot-derived fields with the user before writing.
- Every mutation requires a nonempty printable `request_key` of at most 128
  characters; `correct_event` places it inside `replacement`.
- Reuse exactly the same key and payload after a timeout or uncertain result.
  A different payload with the same key fails with `validation.request_conflict`.
  A new key means a new command, not a retry. Old retries do not overwrite later
  edits. Authorization is still checked before replay.
- Creating an asset and recording its purchase are separate commits. Successful
  creation followed by a failed purchase leaves the asset; retry the purchase,
  not creation with a new key. Core data has no second pending-review stage.
- `correct_event` appends a void and replacement while preserving the original
  economic event and asset. The optional top-level `type_id` applies the scoped
  custom-type reclassification described above; omission keeps the original type.
- Saves replace complete tag/category selections. Read current configuration
  first. Omit an asset's `resource_id` to retain its binding; use an empty string
  to clear its override and restore inheritance.
- Resource deletion is irreversible and rejects active references. Confirm the
  resource first. A failed cleanup remains pending and rejects new bindings;
  retry with the same request key to finish. The deletion-intent receipt alone
  is not a success response.

Amounts are nonnegative integer minor units, never decimal major units; event
cashflow determines the sign. For example, CNY 12.34 is `1234`. Foreign records
also require `fx_rate_scaled` (base currency per original currency unit multiplied
by `100000000`), `fx_rate_date` (`YYYY-MM-DD`), source and explicit confirmation.
`occurred_at` is RFC3339 with a timezone. Client code must preserve integer
precision; do not convert monetary values through floating-point arithmetic.

## Results and failures

Successful structured results have a `data` member. Business results retain their
existing field names (for example `ID` and `BaseAmountMinor`); explicit MCP
configuration/media projections use snake_case. Read the returned tool contract
and result shape rather than applying an implicit casing conversion. Public media
results contain resource IDs and descriptive metadata, not blob store IDs, object
keys, physical paths or credentials.

Business failures set `isError` and return `error.code`, `error.message` and
`error.retryable`. Codes include `invalid_input`, `forbidden`, `not_found`,
`referenced`, `conflict` and `unavailable`; input messages are stable validation
keys. Schema/JSON-RPC failures and HTTP authentication failures may occur before
the business wrapper. Missing/expired/revoked credentials require login, not a new
mutation key. Unexpected infrastructure errors do not expose raw internal details.

No tools provide arbitrary SQL, filesystem access, account/member administration,
general attachment upload, or hypothetical sale/portfolio market valuation.
