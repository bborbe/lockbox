---
status: completed
spec: [004-bug-main-test-reader-hangs-go-1-27]
summary: 'Fixed the reader test helper in main_test.go to satisfy the io.Reader contract (preserve unconsumed bytes via r.data=r.data[n:] and return io.EOF when exhausted), added a deterministic unit test pinning both behavior rows, and added a fix:-prefixed ## Unreleased CHANGELOG entry; the TeamVault contract suite now completes in ~0.02s on both Go 1.26.6 and Go 1.27.1 with no busy-loop hang.'
execution_id: lockbox-reader-fix-exec-014-spec-004-fix-main-test-reader
dark-factory-version: dev
created: "2026-09-03T18:21:24Z"
queued: "2026-09-03T18:22:07Z"
started: "2026-09-03T18:22:09Z"
completed: "2026-09-03T18:24:56Z"
---
<summary>
- The TeamVault contract suite in `main_test.go` no longer hangs: `make test` (and therefore `make precommit`) completes in well under a minute on any Go toolchain from 1.26 up to 1.27.x, instead of burning the full 600s test timeout and exiting 2.
- The root cause — a test helper that violated the `io.Reader` contract — is fixed: the `reader` helper now preserves unconsumed bytes across reads and signals `io.EOF` once its buffer is exhausted.
- The fix is confined to the test file: no production code, no handler/decode logic, no `go.mod` change, no JSON decoder setting, and no existing `It(...)` / `DescribeTable(...)` assertion is modified, removed, or skipped.
- The existing contract suite passes identically under Go 1.26.6 and Go 1.27.x (dual-toolchain test runs), matching the spec's acceptance criteria.
- A small new unit test in `main_test.go` locks the helper's contract behavior (partial reads keep the remainder; exhaustion returns `io.EOF`) deterministically on any toolchain, so the two specific failure modes cannot silently regress.
- A `## Unreleased` CHANGELOG bullet with a conventional `fix:` prefix documents the change, keeping the repo releasable per its changelog convention.
</summary>

<objective>
Make the `reader` test helper in `main_test.go` comply with the `io.Reader` contract so the TeamVault contract suite completes in well under a second on Go 1.26.6 AND Go 1.27.x (no busy-loop hang, no test-timeout failure), and add a `## Unreleased` CHANGELOG bullet documenting the fix. Nothing outside `main_test.go` + `CHANGELOG.md` changes.
</objective>

<context>
Read `/home/node/.claude/CLAUDE.md` and `/workspace/CLAUDE.md` (if present) for project conventions.

Read these coding-plugin guides (in-container paths):
- `/home/node/.claude/plugins/marketplaces/coding/docs/changelog-guide.md` — `## Unreleased` placement and conventional-prefix rules
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-testing-guide.md` — Ginkgo/Gomega style (already the file's style; the new test follows it)

Read `/workspace/docs/dod.md` — specifically the "Documentation" clause: `## Unreleased` goes below the preamble block and above the newest `## vX.Y.Z` section (never between the `# Changelog` title and the preamble).

Read `/workspace/main_test.go` fully. The buggy helper is the `reader` type near the top of the file (used as the request body by `doJSON` and two inline `httptest.NewRequest` calls):

```go
// reader wraps a byte slice as a ReadCloser for http.Request body.
type reader struct{ data []byte }

func (r *reader) Read(p []byte) (n int, _ error) {
	n = copy(p, r.data)
	r.data = r.data[:0]
	return n, nil
}

func (r *reader) Close() error { return nil }
```

Two contract violations: `r.data = r.data[:0]` discards bytes that did not fit into `p` (a correct reader advances `r.data = r.data[n:]`), and it never returns `io.EOF` once the buffer is empty — returning `(0, nil)` makes a decoder spin forever. Go 1.26's JSON decoder read whole small bodies in one large read, so the defects never fired; Go 1.27's `jsontext` decoder reads in fixed 64-byte chunks, so a second read hits the broken helper and busy-loops for the full test timeout.

The file's stdlib import block currently is (lines 7-14):

```go
import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/bborbe/crypto"
	...
)
```

`"io"` is NOT yet imported — you will add it (gofmt/golangci sorts stdlib imports alphabetically; `"io"` belongs between `"encoding/json"` and `"net/http"`).

Also read `/workspace/CHANGELOG.md`: it has NO `## Unreleased` section yet — the newest versioned section is `## v0.9.0`. Per `docs/dod.md`, create `## Unreleased` directly below the preamble (the last preamble line is `* PATCH version when you make backwards-compatible bug fixes.`) and above `## v0.9.0`.
</context>

<requirements>
1. **Add the `"io"` import** to `/workspace/main_test.go`'s stdlib import block. Add `"io"` so the block reads `"encoding/json"`, `"io"`, `"net/http"` (gofmt will order it — do not fight the formatter). Do NOT add any other import.

2. **Replace the `reader.Read` method** in `/workspace/main_test.go` so it satisfies the `io.Reader` contract. Replace the entire body of the existing method (signature unchanged — keep the named returns `(n int, _ error)`):

   Old:
   ```go
   func (r *reader) Read(p []byte) (n int, _ error) {
   	n = copy(p, r.data)
   	r.data = r.data[:0]
   	return n, nil
   }
   ```

   New:
   ```go
   func (r *reader) Read(p []byte) (n int, _ error) {
   	if len(r.data) == 0 {
   		return 0, io.EOF
   	}
   	n = copy(p, r.data)
   	r.data = r.data[n:]
   	return n, nil
   }
   ```

   This implements exactly the spec's Desired Behavior: return data in chunks, preserve the unconsumed remainder for the next call (`r.data = r.data[n:]`), and return `(0, io.EOF)` once exhausted (matching what `bytes.Reader` does). Do NOT touch `reader`'s `data` field, the `reader` struct, or the existing `Close` method — the helper must remain an `io.ReadCloser` because it is passed to `httptest.NewRequest` as the request body. The `reader` type is only used inside `main_test.go` (three call sites: `doJSON` at line 66, the PATCH-401 case at line 522, and the PUT case at line 538) — no other file references it, so there are no sibling call sites to update.

3. **Add a unit test that pins the `io.Reader` contract deterministically on any toolchain.** In `/workspace/main_test.go`, add a new top-level block following the existing Ginkgo/Gomega style (the file already dot-imports `ginkgo/v2` and `gomega`, and the `Describe` blocks are all `var _ = Describe(...)`). Append a new block at the end of the file:

   ```go
   var _ = Describe("reader", func() {
   	It("preserves unconsumed bytes across reads and returns io.EOF when exhausted", func() {
   		r := &reader{data: []byte("abcdefghij")}
   		buf := make([]byte, 4)

   		n, err := r.Read(buf)
   		Expect(n).To(Equal(4))
   		Expect(err).To(BeNil())
   		Expect(string(buf)).To(Equal("abcd"))

   		n, err = r.Read(buf)
   		Expect(n).To(Equal(4))
   		Expect(err).To(BeNil())
   		Expect(string(buf)).To(Equal("efgh"))

   		n, err = r.Read(buf)
   		Expect(n).To(Equal(2))
   		Expect(err).To(BeNil())
   		Expect(string(buf[:2])).To(Equal("ij"))

   		n, err = r.Read(buf)
   		Expect(n).To(Equal(0))
   		Expect(err).To(Equal(io.EOF))
   	})
   })
   ```

   This test simulates exactly what Go 1.27's `jsontext` decoder does (repeated small-chunk reads): the first two reads must return a full chunk with a `nil` error and preserve the remainder (locks failure-mode row 1 — "EOF while data remains" would fail this test), and the final read after exhaustion must return `(0, io.EOF)` (locks failure-mode row 2 — the old `(0, nil)` spin would fail this test). The existing `TestContractSuite` specs remain the integration-level proof through the real `json.Decoder`/`mux.Router` path. Do NOT modify, remove, or skip any existing `It(...)` / `DescribeTable(...)` spec.

4. **Add a `## Unreleased` CHANGELOG entry** in `/workspace/CHANGELOG.md`. The section does not exist yet. Create it directly below the preamble (after `* PATCH version when you make backwards-compatible bug fixes.`) and above `## v0.9.0`, with exactly one bullet using a `fix:` prefix (patch bump), following the repo's descriptive lower-case bullet style (see the `v0.6.2` `fix:` entry). For example:

   `- fix: make the \`reader\` test helper in \`main_test.go\` satisfy the \`io.Reader\` contract (preserve unconsumed bytes across reads, return \`io.EOF\` when exhausted) so the TeamVault contract suite no longer hangs under Go 1.27's chunked JSON decoder — \`make test\` now completes on both Go 1.26 and 1.27`

   Do NOT move, delete, or insert anything above or inside the preamble block; do NOT touch any `## vX.Y.Z` section; do NOT bump any version string.
</requirements>

<constraints>
- The fix must be confined to `/workspace/main_test.go` (the `reader` type and the new `Describe("reader", ...)` block) plus a `## Unreleased` CHANGELOG bullet. Do NOT touch production code, `go.mod`, `go.sum`, or any other test file.
- All existing `It(...)` / `DescribeTable(...)` assertions must pass unchanged — do NOT modify, remove, or skip any existing spec, and do NOT change JSON decoder settings or any request body.
- The helper must remain an `io.ReadCloser` (it is passed to `httptest.NewRequest` as the request body) — keep the `reader` struct and its `Close` method as-is.
- Do NOT pin, constrain, or work around the `go.mod` directive or any toolchain version — the fix must make the suite pass, not hide the failure. Do NOT edit `go.mod`.
- Do NOT add a scenario — the spec explicitly non-goals it; the contract suite in `main_test.go` is itself the test that exercises this behavior.
- This repo does NOT vendor (`/vendor` is gitignored, Makefile uses `-mod=mod`); never run `go mod vendor` and never pass `-mod=vendor`.
- Repo convention: `gofmt`/`goimports` clean (handled by `make format` inside `make precommit`); BSD license header preserved; do NOT use `fmt.Errorf` (no error path is added here — the fix returns `io.EOF`, a sentinel value, not a wrapped error).
- Container-autonomous: file edits + `make` only. No `kubectl`, no `docker`, no `gh`, no PR/deploy steps.
- Do NOT commit — dark-factory handles git.
- Every existing test must still pass.
</constraints>

<verification>
Run in `/workspace`:

1. **Fast-fail main-package test under the container's default toolchain.** This is the direct hang reproduction — if the fix is wrong, this command burns the full 120s timeout instead of completing in well under a second:

```
time go test -mod=mod -timeout 120s -p 1 .
# expect: ok, exit 0, real time well under 120s (the buggy helper hangs the whole 120s)
```

2. **Toolchain-proof runs.** Both toolchains must pass. On first use the requested toolchain downloads to the Go module cache from the Go proxy; if the download fails (proxy unreachable), report that explicitly and rely on step 1 plus `make test`:

```
GOTOOLCHAIN=go1.26.6 go test -mod=mod -timeout 120s -p 1 .   # expect: ok
GOTOOLCHAIN=go1.27.1 go test -mod=mod -timeout 120s -p 1 .   # expect: ok, no 600s hang
```

3. **Full gates.** Both must exit 0:

```
make test
make precommit
```

4. **Filesystem confirmations** (no `git` in this container — the diff-shape evidence in the spec's acceptance criteria is checked operator-side):

```
grep -n 'io.EOF' main_test.go                      # expect: the Read method EOF return AND the new unit test
grep -n 'r.data\[n:\]' main_test.go                # expect: the remainder-preserving advance
grep -n '"io"' main_test.go                        # expect: the new stdlib import
grep -n 'Describe("reader"' main_test.go           # expect: the new contract-pinning unit test
grep -n 'Unreleased' CHANGELOG.md                  # expect: the new section above ## v0.9.0
```

(Do NOT add `-mod=vendor`. Do NOT run any `git` command — git handling is dark-factory's job.)
</verification>

<!-- REVIEWER NOTES (audit-time only, not instructions):
1. The spec's acceptance criterion #2 asks for dual-toolchain proof (Go 1.26.6 AND Go 1.27.x). Step 2 of verification runs this in-container via GOTOOLCHAIN, which downloads the 1.27.1 toolchain on first use from the Go proxy. If the YOLO container has no Go proxy access, step 2 fails on download and the container must rely on step 1 + `make test`. The spec's own Verification ladder already re-runs both legs operator-side, so an in-container download failure is not a blocker — but the fix is only fully proven if at least one leg runs Go 1.27.
2. The container's default toolchain is unknown to the prompt author. If the image already ships Go 1.27, step 1 alone is the hang proof; if it ships 1.26.6, step 2's GOTOOLCHAIN=go1.27.1 leg is the hang proof. Both legs are included so the proof holds either way.
3. The new `Describe("reader", ...)` unit test is a deliberate addition beyond the spec's literal "two-line change". It is justified by spec Desired Behavior #2 and locks the two failure-mode rows (remainder discard, missing EOF) deterministically on any toolchain — the existing contract suite only exercises the multi-read path on Go 1.27. If the auditor prefers strictly-minimal diff, this block can be dropped without affecting the fix.
4. Changelog prefix chosen as `fix:` (patch bump) rather than `test:` — the spec frames this as a bug in the repo that blocks the Go 1.27 update; both prefixes are patch and either is defensible.
-->
