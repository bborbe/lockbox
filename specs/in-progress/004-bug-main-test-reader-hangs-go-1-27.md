---
status: verifying
approved: "2026-09-03T18:15:05Z"
generating: "2026-09-03T18:15:06Z"
verifying: "2026-09-03T18:24:56Z"
branch: dark-factory/bug-main-test-reader-hangs-go-1-27
---

## Summary

- `make precommit` (specifically `make test`) hangs for 600s and exits 2 in any environment running Go 1.27 — including the `github-update-go-agent` sandbox, whose image toolchain is 1.27.0 — while the same commit is green on Go 1.26.6 in GitHub Actions.
- The hang is a busy loop inside the JSON decoder reading the request body of `POST /api/secrets/` contract tests.
- The trigger is the custom `reader` test helper in `main_test.go`: its `Read` method discards unconsumed bytes and never returns `io.EOF`, both violations of the `io.Reader` contract.
- Go 1.26's JSON decoder read whole bodies in one large read, so the defects never fired; Go 1.27's rewritten `jsontext` decoder reads in fixed 64-byte chunks, so a second read hits the broken helper and spins forever.
- Fixing the helper (preserve the remainder, return `io.EOF` when empty) makes the suite pass on both toolchains.

## Problem

`bborbe/lockbox` cannot be updated to Go 1.27.0. The `github-update-go-agent` reports `gate target "precommit" failed (exit 2) with no parseable findings` after burning 600s of compute per attempt, and refuses to retry. GitHub Actions is green at the identical SHA because CI pins to `go-version-file: go.mod` (1.26.6) while the agent image bakes Go 1.27.0. The moment the `go.mod` directive bumps to 1.27, CI will fail identically — this is a repo bug, not an environment gap, and it must be fixed in the repo before the fleet-wide update can land.

## Goal

`make test` (and therefore `make precommit`) completes normally under any Go toolchain from 1.26 onward, including 1.27.0. The TeamVault contract suite in `main_test.go` passes without hanging, and a future `go.mod` bump to 1.27.0 is gated only by the normal update flow, not by a test-helper hang.

## Non-goals

- Do NOT change production code or any handler/decode logic — the bug is confined to the test helper; production JSON decoding is unaffected.
- Do NOT pin, constrain, or work around the `go.mod` directive or any toolchain version to dodge the hang — the fix must make the suite pass, not hide the failure.
- Do NOT change JSON decoder settings, request bodies, or any `It(...)` / `DescribeTable(...)` assertion.
- Do NOT add a scenario — the contract suite is itself the test that exercises this behavior.

## Reproduction

Exact steps (verified 2026-09-03 on Go 1.27.1 darwin/arm64, and matching the agent Job output from 2026-08-27):

1. Check out `bborbe/lockbox` at `dbbb1be` (or any commit with the buggy helper).
2. Run the main package test under Go 1.27 with a short timeout:
   ```bash
   go test -mod=mod -timeout 60s -p 1 .
   ```
3. Observed: the test times out. Goroutine dump parks the test goroutine in `mux.(*Router).ServeHTTP` (`mux.go:212`) called from `doJSON` (`main_test.go:75`); the suite runner waits in `select`. Agent Job evidence (verbatim tail):
   ```
   github.com/bborbe/lockbox.doJSON(...)
           /tmp/github-update-go-d5496bdf-.../main_test.go:75 +0x33c
   ...
   FAIL    github.com/bborbe/lockbox    600.035s
   make: *** [Makefile.precommit:35: test] Error 1
   ```
4. Under the same commit with Go 1.26.6 the suite passes:
   ```bash
   GOTOOLCHAIN=go1.26.6 go test -mod=mod -timeout 60s -p 1 .   # ok 0.341s
   ```
5. CPU profile of the hang pins the spin (98% of samples):
   - `github.com/bborbe/lockbox.(*reader).Read` — 40%
   - `encoding/json/jsontext.(*decoderState).fetch` — 36%
   - `runtime.memmove` — 21%

## Expected vs Actual

**Expected:** `go test` on the main package completes in well under a second on any supported Go toolchain, as it does on Go 1.26.6 (`ok 0.341s`).

**Actual:** on Go 1.27 the JSON decoder and the test helper busy-loop for the full test timeout (Go's default 10 minutes — the observed 600s), then `go test` reports `FAIL ... 600.035s` and `make test` exits 2. The agent's gate sees `exit 2` with no parseable lint findings and reports an unhelpful escalation.

## Why this is a bug

`main_test.go`'s `reader` claims to implement `io.Reader` but violates its contract twice:

```go
func (r *reader) Read(p []byte) (n int, _ error) {
	n = copy(p, r.data)
	r.data = r.data[:0]   // (1) discards unconsumed remainder
	return n, nil         // (2) never signals EOF
}
```

- **(1) Remainder discarded:** `r.data = r.data[:0]` throws away bytes that did not fit into `p`. A correct reader advances `r.data = r.data[n:]`. Go 1.26's JSON decoder read the whole (≤512B) body in one call, so the discard never mattered. Go 1.27's `jsontext` decoder reads in fixed 64-byte chunks — a 75-byte create body needs a second read, which finds nothing left and truncates the JSON.
- **(2) No EOF:** returning `(0, nil)` signals "no progress, no error", which the reader contract forbids. `jsontext.(*decoderState).fetch` treats it as "try again" and loops forever. (A reader returning `(0, io.EOF)` instead is what `bytes.Reader` does, and decodes cleanly.)

Both defects must be fixed together: EOF alone (with the discard still in place) turns the hang into a fast `unexpected EOF` 400 on the create tests; the remainder fix alone (with `(0,nil)` kept) leaves the spin.

## Acceptance Criteria

- [ ] `main_test.go`'s `reader.Read` no longer violates the `io.Reader` contract: it preserves unconsumed bytes across calls and returns `io.EOF` once its buffer is exhausted. — evidence: `git diff main_test.go` shows the remainder-preserving advance (`r.data = r.data[n:]`) and an `io.EOF` return path on exhaustion (file-diff shape).
- [ ] `go test -mod=mod -timeout 120s -p 1 .` on the fixed commit completes under **both** Go 1.26.6 and Go 1.27.x, all `TestContractSuite` specs passing. — evidence: two green test runs (one per toolchain), timings < a few seconds.
- [ ] `make precommit` completes for the fixed commit under Go 1.27. — evidence: full precommit run green (this is the gate the agent runs).
- [ ] A `## Unreleased` CHANGELOG entry documents the fix. — evidence: CHANGELOG diff.

## Verification

```bash
# From repo root, after the fix lands (run on the fix branch):
GOTOOLCHAIN=go1.26.6 go test -mod=mod -timeout 120s -p 1 .     # expect: ok
GOTOOLCHAIN=go1.27.1 go test -mod=mod -timeout 120s -p 1 .     # expect: ok (no 600s hang)
make precommit                                                 # expect: green, "ready to commit"
```

## Desired Behavior

1. The contract suite in `main_test.go` completes in well under a minute on Go 1.27 — no test-timeout hang, no busy loop.
2. `reader.Read` behaves like a standard reader: returns data in chunks, preserves any unconsumed remainder for the next call, and returns `io.EOF` once exhausted.
3. All existing `TestContractSuite` specs keep passing — no assertions changed, no tests removed or skipped.
4. The behavior is toolchain-independent: identical results on Go 1.26.6 and Go 1.27.x.
5. The fix is confined to the test helper (and the CHANGELOG); no production code, API shape, or test semantics change.

## Constraints

- The fix must be confined to `main_test.go` (the `reader` type) plus a `## Unreleased` CHANGELOG bullet. Do not touch production code, `go.mod`, or any other test.
- All existing `It(...)` / `DescribeTable(...)` assertions must pass unchanged.
- The helper must remain an `io.ReadCloser` (it is passed to `httptest.NewRequest` as the request body).
- Repo convention: `goimports` / `gofmt` clean; BSD license header preserved; no `fmt.Errorf` (use `errors` wrapping only if an error path is added — none should be needed).

## Do-Nothing Option

Leaving the bug unfixed is not acceptable: `bborbe/lockbox` stays pinned on Go 1.26.6 while the rest of the fleet moves to 1.27.0; every `github-update-go-agent` attempt burns 600s of compute and escalates with the same unhelpful `exit 2, no parseable findings` message; and the failure is only a `go.mod` bump away from also breaking CI. The fix is a two-line test-helper change with no production risk.

## Failure Modes

| Trigger | Expected behavior | Recovery |
|---|---|---|
| Helper returns EOF while data remains in the buffer | Decode returns `unexpected EOF`; create tests 400 — signals the remainder-preservation fix is missing | Fix `r.data = r.data[n:]`, re-run tests |
| Helper returns `(0,nil)` at exhaustion | Decoder fetch spins; 600s hang returns | Add the `io.EOF` return, re-run tests |
| Chosen fix changes test semantics (assertions/skips) | Auditor/prompt review flags the drift | Keep the diff strictly to `reader.Read` + CHANGELOG |
