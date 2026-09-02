# Development and Release Workflow

This is the required delivery workflow for AssetLoop. It is part of the project architecture, not a temporary team convention.

## Permanent branches

| Branch | Purpose | Direct push | Deleted |
|---|---|---:|---:|
| `dev` | Default baseline for new development branches | Allowed only for deliberate repository maintenance | Never |
| `uat` | Accepted release candidate under user testing | Never | Never |
| `prod` | Published production history | Never | Never |

The GitHub repository default branch is `prod`, so visitors and ordinary repository links resolve to published production history. `dev` remains the default baseline for creating development branches; changing the GitHub default branch does not authorize development or direct pushes on `prod`.

`uat` and `prod` are protected on GitHub. Both require a pull request and passing CI; force pushes and deletion are disabled.

## Public repository and secret safety

The GitHub repository is intentionally public. Assume that every commit, branch, pull request, Action log, artifact name, issue, and review comment is immediately visible outside the project.

Before every checkpoint push:

1. keep real local values only in ignored `.env` files;
2. keep `.env.example`, fixtures, migrations, screenshots, and logs free of real credentials;
3. inspect the staged diff for credentials and private infrastructure details;
4. let GitHub Push Protection reject supported secret patterns;
5. require the `secret-scan` CI job, which scans the complete fetched Git history with a checksum-pinned Gitleaks release.

GitHub Secret Protection and Push Protection must remain enabled. `secret-scan` is a required status check for both `uat` and `prod` alongside `test`.

If a secret may have been exposed, stop promotion. Revoke or rotate it first; then remove it from the working tree and Git history, re-run a full-history scan, and only then resume development. Deleting the current file alone is not remediation because prior commits remain public.

## Work branches

- `dev-<scope>`: a substantial product or architecture slice. A slash is intentionally not used because Git cannot store permanent `dev` and `dev/<scope>` refs at the same time.
- `feature/<scope>`: a focused feature added after the main slice exists.
- `fix/<scope>`: a defect correction.
- `release/<scope>`: a production candidate based on current `prod` that reconciles one accepted UAT tree.

Create a work branch from the latest permanent `dev` baseline. Keep its scope singular. Commit and push whenever a checkpoint is independently testable and safe to restore; do not wait for the whole feature, and do not commit failing intermediate states merely to create volume.

Examples:

```text
dev-asset-lifecycle
feature/csv-export
fix/sqlite-upgrade-lock
release/core-lifecycle
```

## Promotion

1. Run formatting, sqlc generation, tests, vet, secret scanning, the full-element scenario, and the relevant local smoke test.
2. Push the work branch and open a pull request to `uat`.
3. Squash merge after CI passes so UAT receives one coherent change.
4. Delete the short-lived branch.
5. Fast-forward permanent `dev` to the accepted `uat` commit before starting the next baseline.
6. Let the UAT packaging workflow build and smoke-test its artifacts.
7. Test the UAT artifact, not a separately built local binary.
8. Stop and present the exact UAT commit, workflow run, artifacts, and test evidence to the user. A green UAT workflow is not production authorization.
9. Only after the user explicitly authorizes that specific UAT result for production, create `release/<scope>` from the current `prod` and reconcile the accepted UAT commit into it. UAT is authoritative for application and project content; keep only explicit production release metadata as an allowed difference.
10. Verify the release branch against the accepted UAT commit, push it, and open `release/<scope> -> prod`. Record the UAT commit and artifact evidence in the pull request.
11. Squash merge after required checks so protected `prod` remains linear; the same workflow rebuilds, smoke-tests, and publishes the production artifacts.

The release branch exists because independently squash-merged permanent branches do not share commit ancestry even when their trees originally matched. Never resolve that history shape by force-pushing a permanent branch or disabling linear-history protection.

Parallel work branches may continue from an older `dev`, but they must incorporate the latest accepted baseline before promotion.

## Regression test ladder

Every feature changes production behavior and its automated evidence together. Choose the
narrowest layer that proves the behavior and keep the test after release:

1. domain tests for money, event, and invariant calculations;
2. application tests for validation, authorization, and use-case orchestration;
3. the shared Store conformance suite for every persistence behavior on SQLite and PostgreSQL;
4. `httptest` coverage for authentication, CSRF, routing, status codes, and rendered Web flows;
5. `TestFullElementScenario` for the cumulative supported user journey.

A bug fix begins with a failing regression test. A feature is incomplete if its test is missing,
skipped, or only exercises one supported database. Pull-request CI runs the normal suite; the UAT
packaging validation explicitly runs `TestFullElementScenario` with both database adapters before
any artifact is built. The packaged-binary smoke test remains mandatory after that in-process gate.

## UAT defects and production fixes

- A UAT defect uses `fix/<scope>` from the current UAT-aligned `dev`, then returns through UAT packaging.
- A production defect starts from the current `prod` source, is fixed on `fix/<scope>`, and is promoted to `uat` first. Only the tested UAT result may proceed to `prod`.
- No fix may be pushed directly to `prod`.

## Packaging and environment isolation

`.github/workflows/package.yml` is the only UAT/Prod packaging path. It:

1. runs the SQLite and PostgreSQL test suite;
2. verifies committed sqlc output;
3. builds Windows, Linux, and macOS artifacts;
4. packages Windows as `.zip`, packages Linux/macOS as `.tar.gz`, and rejects a mismatched format;
5. writes SHA-256 checksum files;
6. starts the packaged Linux binary against a temporary SQLite database and checks `/healthz`;
7. stores artifacts under the matching `uat` or `prod` GitHub Environment;
8. creates a GitHub Release only for explicitly authorized `prod` promotion.

Environment-specific secrets and variables belong in their matching GitHub Environment. They must never be copied between environments through repository files.

## Rollback

- Development rollback: revert the smallest checkpoint commit on the work branch.
- UAT rollback: revert the squash commit or promotion pull request, then rebuild UAT artifacts.
- Production rollback: revert through a new pull request to `prod`; do not rewrite production history.
- Database rollback: add a corrective forward migration. Never use a destructive down migration on user data.
