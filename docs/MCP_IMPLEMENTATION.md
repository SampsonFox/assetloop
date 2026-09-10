# MCP implementation and acceptance

Approved scope: HTTP Streamable MCP in the existing Go process, backed by the
same application services as Web. Work branch `dev-mcp` starts at accepted UAT
`99f733c`, in a separate worktree. The frontend preview and its database are not
part of this worktree's development runtime.

## Delivery checklist

- [x] Pin the official Go MCP SDK and implement a thin read adapter.
- [x] Exercise real HTTP discovery, context, scoped denial, input validation and
  tenant isolation with the SDK client and migrated SQLite.
- [x] Add application-owned OAuth grants, authorization codes and hashed tokens,
  with paired SQLite/PostgreSQL migrations, sqlc adapters and upgrade tests.
- [x] Implement same-account consent, authorization-code PKCE, metadata discovery,
  exact registered loopback callback matching (variable port), rotating refresh,
  revocation and authorized-client management in Web. Web cookies are not MCP
  bearer credentials. Recheck current roles and grant state on every request.
- [x] Wire the optional HTTP endpoint into the existing server. Default off;
  disabled Web authentication must not imply MCP Owner access. Validate request
  origins/host, token audience/resource and granted scope.
- [x] Add shared application-layer durable idempotency for management writes;
  require `request_key` for every MCP mutation. Reuse lifecycle receipts. Replay
  returns the committed result; conflicting payloads fail, failed transactions
  leave no successful receipt, concurrent requests do not duplicate mutations.
- [x] Expose category/model, typed tag, asset, lifecycle event-type, record and
  append-only correction management; exact cost queries; existing 3D metadata,
  references, binding, appearance defaults and deletion. Add the supporting
  configuration/reference queries needed to select valid existing IDs.
- [x] Verify all input/output schemas, stable safe errors, pagination, permissions,
  exact integer money and fixed-point FX. Return only relevant public metadata,
  never credentials or physical storage paths.
- [x] Extend the cumulative full-element scenario and run both supported stores,
  migration upgrades and relevant Web/MCP regressions.
- [x] User-approved 2026-09-10 native Codex MCP-only lifecycle acceptance:
  OAuth login, discovery, read, create/purchase/repair/correct/sale, durable retry,
  conflict refusal, preserved history and exact cost/aggregate verification.
- [ ] Remaining separate operational checks: live per-client revocation and cleanup.
- [x] Document configuration, client setup, scope/tool contracts and direct evidence.
- [ ] Commit/push independently verified checkpoints; verify each secret-scan.

## Contract and boundaries

The account's current role intersects client scopes `assets:read`,
`assets:catalog`, and `assets:lifecycle`. Tool arguments cannot select the acting
tenant or user. Application use cases remain the business authorization and
transaction boundary; the transport does not call SQL, storage SDKs or its own
Web endpoints. No duplicated calculation or validation policy.

Creating an asset and recording its purchase are two separately committed,
explicit commands, not one atomic workflow. The Harness confirms screenshot
fields before issuing writes. No additional pending-review state is introduced.
Corrections void and replace historical events through the existing use case.

First phase excludes stdio, public deployment, dynamic public client registration,
file upload, general attachments, market integration and MCP member/permission
administration. Pre-register the local Codex OAuth client. Existing Web file
upload remains available; MCP manages only existing resources.

Completion means the entire checklist is proved, not merely that the read adapter
compiles. UAT packaging/promotion and production remain separate user approvals.

## Development evidence

2026-09-10 approved follow-up: `import_3d_resource_from_url` extends discovery to
40 tools. Public HTTPS downloading is infrastructure behind an application port;
the application reuses shared GLB/Blob upload and management receipts, with I/O
outside transactions. Import does not bind. Downloader tests cover special IPs,
DNS answer pinning/mixed-address denial, redirects, size, compression and timeout.
The dual-Store-defined import scenario covers authorization, bad input, receipt
failure rollback/cleanup, simultaneous same-key import and replay without download.
The named full-element scenario now imports its resource through MCP using a
deterministic download-port fixture, then exercises existing binding/deletion.
Local SQLite regressions pass; PostgreSQL execution and native Codex import/preview
remain to be verified for this new increment. Earlier 39-tool evidence is historical.
The new binary is running on the retained 8081 database/blob directories and
existing native MCP `get_context` still succeeds. New-tool discovery in the
current task requires a client tool-catalog reload; no fresh OAuth login is needed.

2026-09-10 current outcome: the user accepted a complete MCP-only business flow as
the desktop acceptance criterion. The current conversation loaded all 39 tools
and directly completed category/model/item creation, CNY 1000 purchase, CNY 100
repair corrected to CNY 80, and CNY 700 sale. Exact final cost was CNY 380;
original repair remained voided, retries returned original identities, conflicting
payloads failed, and portfolio deltas matched. No Web/REST/SQL/SDK business
operations substituted for the native tools in this run. Setup, pitfalls and
evidence are recorded in [Codex acceptance notes](CODEX_MCP_ACCEPTANCE.md).
This supersedes historical statements below that current-conversation acceptance
was incomplete, not their separate revocation/cleanup caveats. Test data remains.
No UAT promotion or production release is implied.

2026-09-10 OAuth interoperability follow-up: live diagnostics confirmed that
Codex sends `scope` during refresh. The HTTP adapter previously rejected every
non-empty scope before reaching the grant policy. Refresh now accepts an
explicit repeat of the original normalized scope set; changes remain rejected
with `invalid_scope` before consuming the refresh credential. Application-owned
grant, resource, current-role and rotation checks remain unchanged. The HTTP
regression failed with 400 before the fix and now covers same-scope refresh,
duplicate scope normalization, omitted scope, escalation/unknown-scope denial,
rotation, refreshed bearer authentication and revocation. Application, MCP and
Web package tests pass. Native consent forms also now use `same-origin`
Referrer-Policy, retaining strict Origin validation without an opaque POST
Origin. Temporary diagnostics were removed from application code.

The disposable local instance was initialized through Web on 2026-09-09.
Codex CLI OAuth login succeeded; a separately authorized official SDK client
called all 39 tools on that same live instance, replayed mutations and checked
persisted values. Web confirmed the corrected CNY cost; GLB upload used Web,
not an MCP tool. At that checkpoint current-conversation direct tool acceptance,
final revocation and cleanup were incomplete. Do not treat the earlier cleanup record below
as the state of this retained September 9 test instance.

Contract audit: `internal/mcp/contract_test.go` checks all discovered input/output
envelopes, every mutation's required string request key (nested for correction),
and integer money/FX schema types. Real SDK HTTP error tests cover all mapped
business error codes, retryability and absence of internal driver/path text.
The MCP package passes. Together with the payload, exact-money, public-media,
paging, role/scope and dual-Store scenarios recorded below, this closes the
automated contract check; it does not close actual desktop acceptance.

Direct tool walkthrough: the full-element OAuth HTTP connection now successfully
calls all 39 discovered tools, not just tools/list. Every mutation is replayed
with its original request key; returned identities/results remain unchanged.
Catalog, typed configuration, assets, custom event types, appearance rules,
resource metadata, binding, reference reads and deletion are checked against
their persisted results. Foreign purchase/correction retains the original
economic evidence and exact cost; authenticated Web visibility and revocation
remain part of the scenario. An uncalled discovered tool fails the scenario.
SQLite passed locally. Commit `f295493` passed the same walkthrough with both
SQLite and PostgreSQL 17 in [CI run 34238352252](https://github.com/SampsonFox/assetloop/actions/runs/34238352252),
with `REQUIRE_POSTGRES_TEST=true`. Full Go tests, the separately executed named
full-element scenario, all 133 Web tests, sqlc cleanliness, vet and secret-scan
passed. No UAT packaging or promotion was requested or run. Temporary databases,
blob directories and HTTP servers are cleaned up by the test harness. GLB upload
is a fixture because MCP intentionally has no upload tool. This is direct SDK
HTTP tool acceptance, not proof of the Codex desktop browser login.

User-approved 8081 browser acceptance found repeated identical resource indicators
from Codex and native submitter values being lost when the submit button became
disabled. Regression tests first failed, then passed after narrowly fixing both.
Consent CSP now permits only the validated callback origin/port, not arbitrary
loopback ports; invalid callbacks and unrelated pages retain strict boundaries.
The consent page was visually inspected, but browser submission still reported
ERR_BLOCKED_BY_CLIENT and no client grant was created. Desktop login remains
unverified; do not infer the remaining browser failure's cause from the code fixes.
The temporary account/database, binary, logs, generated protocol helper and
assetloop-acceptance client configuration were removed; CLI logout confirmed no
stored OAuth credentials. The 8081 process was stopped. 8080 was not modified.
The setup/tool guide is available in `docs/MCP.md`.

Dual-database verification is now executed, not deferred: commit `7328a92` passed
[CI run 34229709652](https://github.com/SampsonFox/assetloop/actions/runs/34229709652).
The test job used its isolated PostgreSQL 17 service with `TEST_POSTGRES_DSN` and
`REQUIRE_POSTGRES_TEST=true`; Go tests passed for integration, MCP, migration,
SQLite/PostgreSQL stores and other packages. The named full-element scenario
also passed separately, all 132 Web interaction tests passed, and sqlc generation,
generated-tree cleanliness and vet passed. `secret-scan` passed. No packaging,
UAT pull request, promotion or production release was invoked. Historical notes
below describe earlier checkpoints and their then-unavailable PostgreSQL checks.

Historical desktop acceptance preparation: an isolated opt-in server ran on
`127.0.0.1:8081`, with its own `data/mcp-acceptance` database/blob directory. The
existing 8080 instance was not changed. Discovery metadata was reachable and
advertised issuer-bound OAuth callbacks. That isolated runtime has been removed.
Actual desktop consent/tool acceptance
is still open; automated HTTP SDK success is not desktop success.

The named `TestFullElementScenario` now includes `MCP OAuth lifecycle` on each
supported Store: actual Web consent with session/CSRF, HTTP PKCE code exchange,
SDK discovery and authenticated asset creation, separately committed foreign
purchase, durable retries, append-only correction preserving the original FX,
authenticated Web visibility and Web client revocation. The rejected MCP token
does not invalidate the Web session. The SQLite scenario passes locally.
`[full-test]` opts this development checkpoint into the existing isolated CI
PostgreSQL test job, not UAT packaging or promotion. CI results must be inspected
before claiming the PostgreSQL and full-suite gates. This automated SDK scenario
does not replace actual Codex desktop acceptance.

Public media output audit: model detail/search/create and resource detail/list/
effective appearance now use explicit transport projections rather than serializing
storage-bearing domain values. SDK HTTP tests assert resource identity is retained
while store IDs, object keys and checksums are absent in direct and nested reads.
`search_appearance_candidates` reuses the existing application selection policy,
returns the same safe resource projection, and requires confirmation before binding.
Candidate tests cover appearance overrides, matches, pagination and scope/tenant
denial. Discovery contains 39 tools; MCP, integration and application tests pass.
This does not claim the remaining complete-schema/error audit or live acceptance.

Management concurrency: two independent connections concurrently submit eight
same-key creates and observe one persisted category and one shared result ID.
Different payloads racing for one key produce one success and one conflict;
eight same-key deletions complete without residual metadata/blob, and a newly
constructed service replays the durable intent. The narrow SQLite integration
scenario passes five repeated runs. PostgreSQL executes this same scenario when
configured but remains an outstanding live gate. Reference query paging also
uses explicit stable ordering, with first/second/out-of-range page assertions.

Supporting edit queries: `get_model_configuration` returns all allowed tag IDs,
explicit appearance overrides (including false, distinct from absent), and
confirmed appearance rules. `get_resource_configuration` returns descriptive
tag/category associations; `get_specification_references` pages tag/type usage.
All three delegate to application projections of the existing tenant-scoped
specification snapshot, not transport-owned business queries. SDK discovery now
contains 38 tools. `mcp_configuration_test.go` verifies actual saved configuration,
rule identity/conditions, explicit false overrides, resource associations,
reference totals/paging and denied scopes/other tenants. The MCP, integration and
application package tests pass with SQLite; this is not live PostgreSQL evidence.

Recoverable deletion checkpoint: `delete_3d_resource` reuses the media service's
guarded preparation and cleanup. Its management receipt atomically records the
pending deletion and immutable object identity; success is returned only after
cleanup, and retries resume cleanup even if metadata was already removed.
No schema addition or blob I/O inside a SQL transaction. Fault-injected tests in
`mcp_media_test.go` cover receipt failure before blob I/O, blob failure with a
retained pending record, rejection of binding a pending resource, metadata
failure after blob deletion, completed retries, conflicting keys, current-role
denial and missing resources without a matching receipt. HTTP tests cover scope
denial and deletion/replay; discovery now contains 35 tools. SQLite/application
tests pass; live PostgreSQL, concurrent independent-connection management replay
and actual client acceptance remain outstanding.

3D binding checkpoint: `bind_3d_resource` and `get_3d_binding` reuse the existing
media application use cases. Binding and its management receipt commit together;
the query returns explicit/effective IDs without blob locations. The SDK discovers
34 tools. `internal/integration/mcp_media_test.go` uploads a real GLB through the
existing local BlobStore, verifies rollback on receipt failure, replay after a
later clear, model inheritance and asset overrides, referenced-delete refusal,
HTTP scope/current-role denial and cross-tenant rejection. The MCP, integration
and application packages pass on SQLite; PostgreSQL remains unexecuted without
its test DSN. Recoverable MCP resource deletion remains pending and must not put
irreversible blob deletion inside the management database transaction.

Initial adapter: `go test ./internal/mcp -count=1` passes. Seventeen read tools
are discovered over actual Streamable HTTP. A nil/empty verifier fails closed on
GET/POST/DELETE; authenticated calls carry the resolved identity and enforce read
scope; the second tenant cannot read the first tenant's category. This is not
OAuth acceptance, write support, PostgreSQL evidence or Codex desktop acceptance.
The adapter is deliberately not mounted in the application until authorization
is implemented.

OAuth application policy is implemented in `internal/application/oauth.go`, with
transactional test-double coverage for S256, exact code-exchange callback binding,
registered loopback ports, audience/client checks, expiration, current roles,
hash-only persistence, write rollback, code/refresh replay revocation, concurrent
code exchange and client-scoped token revocation. Scope constants are shared with
the MCP adapter. `go test ./internal/application ./internal/mcp -count=1` passes.
Both Store implementations now use generated `oauth.sql` queries and paired
migration 00015. SQLite real persistence and independent-connection exchange/replay
tests pass. Schema-14 upgrade tests prove rollback after a DDL collision, retry
and retained account data. The existing specification migration regression now
expects latest schema 15 without removing its history-preservation assertions.
PostgreSQL compiles but its live test is skipped because no TEST_POSTGRES_DSN is
configured; the dual-database checklist item is deliberately still open. HTTP
consent/token endpoints remain incomplete. These policy and SQLite tests are not
a substitute for PostgreSQL execution.

HTTP protocol adapters now provide authorization/resource metadata, code exchange,
refresh, revocation, bearer resolution and discovery challenges. Their guard
rejects unconfigured Host/cross-origin requests; form parsing rejects duplicate
parameters, oversized payloads, query credentials and unsupported client secrets.
Real SQLite tests cover exchange through HTTP, bearer identity and revocation.
`go test ./internal/mcp ./internal/application -count=1` passes. Consent UI,
runtime mounting, configuration and actual Codex acceptance remain pending.

Configuration parsing is now covered by `internal/config/mcp_test.go`:
`MCP_ENABLED` defaults to false; enabling requires `AUTH_MODE=local`, an explicit
`MCP_ISSUER` origin (HTTPS or HTTP loopback IP), a nonempty `MCP_CLIENT_ID` and a
valid `MCP_REDIRECT_URI`. Default client is `codex-local` with loopback `/callback`.
No live environment has been changed. The fields are parsed but not yet wired
into server startup, pending the consent UI. Login now preserves a validated
local `return_to` through failed attempts, successful login and existing sessions.
`internal/web/login_return_test.go` first reproduced the lost continuation and
now covers it plus external/encoded redirect rejection. The hidden field does not
change the existing login page's visual layout. Consent integration remains open.

Consent/runtime checkpoint: `MCP_ENABLED=true` now constructs the OAuth service
and mounts metadata, token, revocation and MCP routes in the same `serve` process.
Web exposes `/oauth/authorize` and `/account/clients`, with the latter linked from
the account menu only when enabled. Consent uses the existing authenticated
session and CSRF protection, validates the registered client before any redirect,
shows requested permissions, and returns code/error with state and issuer.
The authorization management list only contains the acting user's grants.
`TestOAuthConsentAndRevocation` covers login continuation, rendered permissions,
CSRF refusal, allow, deny, token exchange, grant listing and immediate revocation.
No preview instance has been restarted. Browser visual QA and actual Codex OAuth
acceptance are still pending; the checklist remains open until those are verified.

Lifecycle write checkpoint: `record_event` and `correct_event` now reuse the
existing application services and durable request receipts. Tool discovery has
17 read tools and 2 write tools. Every mutation requires a nonempty request key;
timestamps have explicit zones, money uses integer minor units and FX uses the
existing 100,000,000 scale. Correction preserves the original asset/type rather
than accepting fields the use case would ignore. Real SDK HTTP tests prove
replay identity, payload conflict, append-only three-row correction history and
scope/role denial. `go test ./internal/mcp -count=1` passes. Catalog/asset/tag/type
and 3D mutations plus shared management idempotency are still unimplemented.

Management idempotency prerequisite: catalog create/update/read queries previously
opened the database handle even when called on a transaction-scoped Store. Both
adapters now use `queries()`. `TestCatalogTransactionRollback` first failed with
a SQLite connection wait, then passed after the fix: category update/create and
model creation see their own writes and roll back together. The PostgreSQL test
is defined but skipped without its DSN. This fixes transaction composition; it
does not yet provide the management request receipt or management MCP tools.

Management receipt checkpoint: paired migration 00016 and sqlc adapters persist
tenant/user/key fingerprints and original JSON results. `ManagementService`
wraps existing category create/update and model-create use cases in the same
transaction as the receipt. Tests cover replay identity, payload conflict,
required keys, current-role checks and an injected receipt failure rolling back
the business mutation. Three new MCP tools call these services; real HTTP tests
cover category/model creation retries and category updates. There are now 22
tools (17 reads, 5 writes). Existing schema-14 upgrade tests cross migrations
15–16 while retaining account data. PostgreSQL runtime validation remains open,
as do management concurrency coverage and the remaining write families.

Specification management checkpoint: the management port now includes the existing
specification Store contract. Nested specification writes reuse the outer
management transaction only for its locked tenant. Wrapper methods cover assets,
types, tags, model configurations, appearance defaults and resource metadata.
Seven matching MCP tools bring discovery to 29 tools. HTTP tests exercise type/tag
creation, model allowance save and item create/update with replay; Store tests
prove nested item rollback when receipt persistence fails and reject a different
tenant inside the outer transaction. `go test ./internal/mcp ./internal/integration
./internal/application -count=1` passes (PostgreSQL still skipped without DSN).
3D tool happy-path coverage, supporting configuration/reference queries, event-type
mutations, resource binding/deletion, concurrency and final client acceptance are
still pending; tool registration alone does not complete those acceptance items.

Event-type management checkpoint: three semantic tools create, update and enable
custom event types through existing lifecycle use cases and shared management
receipts. Nested lifecycle writes reuse only the locked management tenant.
HTTP tests prove replay, rename, cashflow-direction locking after use and built-in
type protection. Receipt-failure injection proves event-type creation rolls back.
Discovery now exposes 32 tools. Resource binding/deletion, configuration reads,
3D happy-path coverage, concurrency, PostgreSQL and Codex acceptance remain open.
