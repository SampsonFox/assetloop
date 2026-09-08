# MCP implementation and acceptance

Approved scope: HTTP Streamable MCP in the existing Go process, backed by the
same application services as Web. Work branch `dev-mcp` starts at accepted UAT
`99f733c`, in a separate worktree. The frontend preview and its database are not
part of this worktree's development runtime.

## Delivery checklist

- [x] Pin the official Go MCP SDK and implement a thin read adapter.
- [x] Exercise real HTTP discovery, context, scoped denial, input validation and
  tenant isolation with the SDK client and migrated SQLite.
- [ ] Add application-owned OAuth grants, authorization codes and hashed tokens,
  with paired SQLite/PostgreSQL migrations, sqlc adapters and upgrade tests.
- [ ] Implement same-account consent, authorization-code PKCE, metadata discovery,
  exact registered loopback callback matching (variable port), rotating refresh,
  revocation and authorized-client management in Web. Web cookies are not MCP
  bearer credentials. Recheck current roles and grant state on every request.
- [ ] Wire the optional HTTP endpoint into the existing server. Default off;
  disabled Web authentication must not imply MCP Owner access. Validate request
  origins/host, token audience/resource and granted scope.
- [ ] Add shared application-layer durable idempotency for management writes;
  require `request_key` for every MCP mutation. Reuse lifecycle receipts. Replay
  returns the committed result; conflicting payloads fail, failed transactions
  leave no successful receipt, concurrent requests do not duplicate mutations.
- [ ] Expose category/model, typed tag, asset, lifecycle event-type, record and
  append-only correction management; exact cost queries; existing 3D metadata,
  references, binding, appearance defaults and deletion. Add the supporting
  configuration/reference queries needed to select valid existing IDs.
- [ ] Verify all input/output schemas, stable safe errors, pagination, permissions,
  exact integer money and fixed-point FX. Return only relevant public metadata,
  never credentials or physical storage paths.
- [ ] Extend the cumulative full-element scenario and run both supported stores,
  migration upgrades and relevant Web/MCP regressions.
- [ ] Accept through actual local Codex OAuth login, discovery, read, confirmed
  create/record/correct, Web visibility, retry and per-client revocation.
- [ ] Document configuration, client setup, scope/tool contracts and final evidence.
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
