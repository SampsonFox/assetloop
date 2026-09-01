## Scope

Describe the single change this pull request promotes.

## Verification

- [ ] Formatting and sqlc generation are current
- [ ] `go test ./...` passes
- [ ] `go vet ./...` passes
- [ ] Relevant local or packaged smoke test passes
- [ ] Both database migrations were added when schema changed
- [ ] `CODEMAP.md` and architecture/plan documents were updated when required

## Promotion

- [ ] Target branch is `uat`, or this is an accepted `uat -> prod` promotion
- [ ] UAT was tested using the packaged artifact before production promotion
- [ ] The source work branch can be deleted after squash merge

