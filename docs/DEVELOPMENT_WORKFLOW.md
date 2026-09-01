# Development and Release Workflow

This is the required delivery workflow for AssetLoop. It is part of the project architecture, not a temporary team convention.

## Permanent branches

| Branch | Purpose | Direct push | Deleted |
|---|---|---:|---:|
| `dev` | Default baseline for new development branches | Allowed only for deliberate repository maintenance | Never |
| `uat` | Accepted release candidate under user testing | Never | Never |
| `prod` | Published production history | Never | Never |

`uat` and `prod` are protected on GitHub. Both require a pull request and passing CI; force pushes and deletion are disabled.

## Work branches

- `dev/<scope>`: a substantial product or architecture slice.
- `feature/<scope>`: a focused feature added after the main slice exists.
- `fix/<scope>`: a defect correction.

Create a work branch from the latest permanent `dev` baseline. Keep its scope singular. Commit and push whenever a checkpoint is independently testable and safe to restore; do not wait for the whole feature, and do not commit failing intermediate states merely to create volume.

Examples:

```text
dev/asset-lifecycle
feature/csv-export
fix/sqlite-upgrade-lock
```

## Promotion

1. Run formatting, sqlc generation, tests, vet, and the relevant local smoke test.
2. Push the work branch and open a pull request to `uat`.
3. Squash merge after CI passes so UAT receives one coherent change.
4. Delete the short-lived branch.
5. Fast-forward permanent `dev` to the accepted `uat` commit before starting the next baseline.
6. Let the UAT packaging workflow build and smoke-test its artifacts.
7. Test the UAT artifact, not a separately built local binary.
8. Open a pull request from `uat` to `prod` only after UAT acceptance.
9. Merge after required checks; the same workflow rebuilds, smoke-tests, and publishes the production artifacts.

Parallel work branches may continue from an older `dev`, but they must incorporate the latest accepted baseline before promotion.

## UAT defects and production fixes

- A UAT defect uses `fix/<scope>` from the current UAT-aligned `dev`, then returns through UAT packaging.
- A production defect starts from the current `prod` source, is fixed on `fix/<scope>`, and is promoted to `uat` first. Only the tested UAT result may proceed to `prod`.
- No fix may be pushed directly to `prod`.

## Packaging and environment isolation

`.github/workflows/package.yml` is the only UAT/Prod packaging path. It:

1. runs the SQLite and PostgreSQL test suite;
2. verifies committed sqlc output;
3. builds Windows, Linux, and macOS artifacts;
4. writes SHA-256 checksum files;
5. starts the packaged Linux binary against a temporary SQLite database and checks `/healthz`;
6. stores artifacts under the matching `uat` or `prod` GitHub Environment;
7. creates a GitHub Release only for `prod`.

Environment-specific secrets and variables belong in their matching GitHub Environment. They must never be copied between environments through repository files.

## Rollback

- Development rollback: revert the smallest checkpoint commit on the work branch.
- UAT rollback: revert the squash commit or promotion pull request, then rebuild UAT artifacts.
- Production rollback: revert through a new pull request to `prod`; do not rewrite production history.
- Database rollback: add a corrective forward migration. Never use a destructive down migration on user data.

