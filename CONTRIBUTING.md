# Contributing Changes

Contributions should preserve accurate, data-driven client behavior and follow the existing project structure.

## Before opening a change

- Inspect the relevant implementation, inputs, and reference behavior before editing.
- Keep the change focused and avoid unrelated refactors.
- Preserve existing comments and document confirmed findings separately from hypotheses.
- Do not add hardcoded map, asset, entity, or protocol exceptions when the behavior can be derived from project data or the original client.
- Run `gofmt` on changed Go files and the narrowest relevant tests.
- Run `go test ./...` and `go build ./...` when the project structure supports them.

## Pull requests

Describe the observed problem, the evidence supporting the change, the files modified, and the checks performed. Clearly identify any runtime or visual verification that remains outstanding.
