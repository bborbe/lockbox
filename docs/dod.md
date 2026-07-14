# Definition of Done

After completing your implementation, review your own changes against each criterion below. These are quality checks you perform by inspecting your work — not commands to run (linting and tests already ran via `validationCommand`). Report any unmet criterion as a blocker.

## Code Quality

- Exported types, functions, and interfaces have doc comments
- Error handling uses `github.com/bborbe/errors` with context wrapping — never `fmt.Errorf`
- No `time.Now()` or `context.Background()` in business logic — inject a clock / propagate the request `context.Context`
- No debug output (`fmt.Print*`, `println`) — use structured logging
- Factory functions (`New*`) are pure composition — no conditionals, no I/O, no `context.Background()`
- Follow Interface → Constructor → Struct → Method pattern

## Secret handling (this is a secret manager)

- Secret values are only ever persisted **encrypted** through the `crypto.Crypter` seam — no plaintext secret bytes reach the kv store
- No secret value (password, file content, encryption key) is ever logged, returned in an error string, or written to stdout/stderr
- At-rest encoding changes stay behind the `secret.Store` / `crypto.Crypter` boundary; the AES-GCM primitive is composed from `github.com/bborbe/crypto`, not reimplemented

## TeamVault API compatibility (frozen contract)

- The read/write HTTP API — routes, Basic auth, and the request/response JSON shapes on both `/api` and `/api/v1` — is **100% TeamVault-compatible and frozen**. Do not change it unless the prompt explicitly says so.
- The `main_test.go` contract assertions must keep passing byte-for-byte; a diff to `pkg/handler/` request/response shapes is a blocker unless intended

## Testing

- New code has good test coverage (target >= 80%)
- Changes to existing code have tests covering at least the changed behavior
- Tests use Ginkgo v2 / Gomega with Counterfeiter mocks, in external `_test` packages
- Handler and store changes assert the boundary (JSON body shape / encrypted-at-rest round-trip), not just internal state

## Build / install

- `go build ./...` succeeds; no `exclude` or `replace` directives in `go.mod` (they break remote install)
- No new dependency with a known vulnerability (trivy / osv-scanner are part of `make precommit`)

## Documentation

- `README.md` is updated if the change affects usage, configuration, or setup
- `CHANGELOG.md` has an entry under `## Unreleased`. If that section does not exist yet, create it **below** the preamble block (the `All notable changes…` line and the `* MAJOR / MINOR / PATCH` lines) and **above** the newest `## vX.Y.Z` section — never between the `# Changelog` title and the preamble. Final order: `# Changelog` → preamble → `## Unreleased` → `## vX.Y.Z` (newest first).
