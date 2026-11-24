# ADR-006: Race testing is CI-only on this host

- Status: accepted
- Date: 2026-11-25

## Context

The Windows development host has no C compiler/CGO, so `go test -race` cannot
run locally. CI runs Linux + race. The first CI run exposed a test-only data
race in `fakeUpstream.callName`; it was fixed with a mutex and by copying the
tools slice before sort/filter.

## Decision

- CI always runs non-race then `-race`, using `go-version-file: go.mod`.
- Test doubles must be concurrency-safe when a test calls them concurrently.
- Do not claim local race coverage; link the CI run.

## Consequences

- A green local `go test` is necessary but not sufficient.
- Any new concurrent test must be reviewed for shared writes.