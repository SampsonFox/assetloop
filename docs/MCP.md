# MCP setup and tool contract

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
cannot supply the acting tenant or user. All 39 tools remain discoverable, but
unauthorized calls fail. Ask for only the scopes needed for the intended workflow.

| Scope | Tools |
|---|---|
| `assets:read` — catalog | `get_context`, `list_assets`, `get_asset`, `list_categories`, `search_product_models`, `get_product_model` |
| `assets:read` — configuration | `list_tag_types`, `search_specification_tags`, `get_model_configuration`, `get_resource_configuration`, `get_specification_references` |
| `assets:read` — lifecycle | `list_events`, `get_event`, `list_event_types`, `get_asset_cost`, `get_portfolio_summary` |
| `assets:read` — 3D | `list_3d_resources`, `get_3d_resource`, `get_3d_references`, `get_asset_appearance`, `get_3d_binding`, `search_appearance_candidates` |
| `assets:catalog` | `create_category`, `update_category`, `create_product_model`, `save_tag_type`, `save_specification_tag`, `save_model_configuration`, `save_asset`, `save_appearance_default`, `delete_appearance_default`, `save_3d_resource_metadata`, `bind_3d_resource`, `delete_3d_resource` |
| `assets:lifecycle` | `create_event_type`, `update_event_type`, `set_event_type_enabled`, `record_event`, `correct_event` |

Paged queries use `page` and `page_size`, normalize invalid/nonpositive values,
and cap page size at 200. Read all required pages before selecting IDs. Complete
configuration queries return the full association set; do not replace it with a
partial search result. An absent appearance override inherits the tag type's
default; explicit `false` overrides it. Resource descriptions/candidates never
automatically bind a resource. Uploads remain in Web.

## Confirmed mutations and retries

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
  economic event. It does not change the original asset or event type.
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
general attachment upload, or market integration.
