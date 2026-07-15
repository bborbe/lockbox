---
status: verifying
tags:
    - dark-factory
    - spec
approved: "2026-07-15T21:14:34Z"
generating: "2026-07-15T21:14:35Z"
prompted: "2026-07-15T21:22:52Z"
verifying: "2026-07-15T21:31:27Z"
branch: dark-factory/search-name-compat
---

## Summary

- Lockbox's read API omits the human-readable secret **name**, so `teamvault-cli search` and `info` can only show bare keys for Lockbox-backed secrets.
- This spec authorizes **two additive** changes to the TeamVault-compatible read API: add `name` to the secret-detail body, and enrich each search result with `hashid`, `name`, `username`, `url` wrapped in a `{count, results}` envelope.
- Both changes are additive (no existing field removed or renamed) and move Lockbox **closer** to real TeamVault's response shape.
- The `docs/dod.md` "frozen contract" clause is explicitly waived for these two shapes; the `main_test.go` contract assertions must be updated in lockstep to assert the new fields.
- `name` / `username` / `url` are metadata, not secret values — password and file content stay encrypted-only behind the `crypto.Crypter` seam.

## Problem

Real TeamVault returns a secret's human-readable `name` in both its search results and its secret-detail body. Lockbox returns neither: the detail endpoint emits only `{username, url, current_revision}`, and search emits only a list of `{api_url}` entries. Because `name` never crosses the wire, `teamvault-cli search` and `teamvault-cli info` cannot display secret names for Lockbox-backed secrets — they fall back to bare keys, which are meaningless to a human scanning results. The companion teamvault-cli client fix (already merged) reads `name` when present; Lockbox must now emit it.

## Goal

After this work, a client querying Lockbox's read API sees the same name-bearing shape it would see from real TeamVault:

- `GET /api/secrets/{key}/` (and the `/api/v1` mirror) returns `name` alongside the existing `username`, `url`, `current_revision`.
- `GET /api/secrets/?search=q` returns a `{count, results}` envelope where each result carries `hashid`, `name`, `username`, `url`, and the pre-existing `api_url`.
- `teamvault-cli search` and `info` display Lockbox secret names without further client changes.
- The `docs/dod.md` frozen-contract check passes because these diffs are the spec's explicitly-intended change, and the `main_test.go` contract assertions assert the new fields.

## Non-goals

- Do NOT change any write-side endpoint (POST/PATCH create/update) request or response shape.
- Do NOT change teamvault-cli client code — the companion client fix is already merged.
- Do NOT implement full DRF `next`/`previous` pagination. A single page plus a `count` is sufficient; `next` and `previous` may be `null`.
- Do NOT remove, rename, or change the type of any field currently emitted by the detail or search endpoints (`api_url`, `username`, `url`, `current_revision` all stay).
- Do NOT return secret values (`password`, file content) in any metadata or search response — invariant; if a future consumer needs bulk secret retrieval that is a separate spec.

## Desired Behavior

1. The secret-detail body (`GET /api/secrets/{key}/` and `/api/v1/secrets/{key}/`) includes a `name` field set from the stored secret's name, in addition to the unchanged `username`, `url`, `current_revision`.
2. The search body (`GET /api/secrets/?search=q`) is a `{count, results}` envelope: `count` is the number of matches, `next` and `previous` are present and may be `null`, `results` is the array of matches.
3. Each search result object carries `hashid` (the secret key), `name`, `username`, `url`, and the pre-existing absolute `api_url`.
4. The store's search operation returns per-match records carrying key, name, username, and url — not a bare key list — reusing the decrypt it already performs to match on name/username/url/description.
5. The `SecretStore` counterfeiter mock reflects the new search return type so mock-based tests compile and drive the new fields.
6. The `main_test.go` TeamVault contract assertions assert `name` present in the detail body and assert the enriched search envelope and result fields.

## Constraints

- Frozen-contract waiver is **scoped to exactly** the two additive shapes in this spec (detail `name`; search envelope + per-result `hashid`/`name`/`username`/`url`). No other route, auth, or JSON shape changes. See `docs/dod.md` "TeamVault API compatibility (frozen contract)".
- No field is removed or renamed: `api_url` stays in each search result; `username`, `url`, `current_revision` stay in the detail body.
- The detail body's `current_revision` semantics are unchanged — it still ends at `.../secret-revisions/{key}/` with a trailing slash and no `/data` suffix.
- Secret values stay encrypted-only through the `crypto.Crypter` / `secret.Store` seam. `name`, `username`, `url` are metadata and are the only newly-exposed fields; `password` and file content are NOT exposed by these endpoints. See `docs/dod.md` "Secret handling".
- Error handling, logging, and factory-function purity conventions in `docs/dod.md` "Code Quality" continue to apply to any new/changed code.
- Tests use Ginkgo v2 / Gomega with Counterfeiter mocks in external `_test` packages.

## Failure Modes

| Trigger | Expected behavior | Recovery | Detection |
|---------|-------------------|----------|-----------|
| Stored secret has empty `Name` | Response includes `"name": ""` (field always present, value empty) | None needed — empty name is valid | Client shows blank name / falls back to key |
| Search matches zero secrets | Envelope `{count: 0, results: []}` with `next`/`previous` null | None needed | `count == 0` |
| Decrypt fails for one secret during search | Search returns an error (existing behavior preserved) — no partial plaintext leak, no secret value in error string | Handler returns non-200; a grep of the error response body returns zero secret-value substrings (negative evidence) | Non-200 status surfaced from handler |
| Client relies on old bare-`{api_url}`-only search shape | Additive change: `api_url` still present, so old clients keep working; new fields are extra keys | None needed | Old client ignores unknown keys |

## Security / Abuse Cases

- Attacker controls the `search` query string. It is used only for case-insensitive substring matching (existing behavior); no new injection surface is introduced.
- Trust boundary: the newly-exposed `name`/`username`/`url` are metadata already stored per secret. No secret value (`password`, file content, encryption key) crosses the boundary — those remain served only from the revision-data endpoint behind the crypter seam.
- No new unbounded loop, retry, or external call is added; search still iterates the existing bucket once.

## Acceptance Criteria

- [ ] The detail DTO carries a `name` JSON field — evidence: `grep -n 'json:"name"' pkg/api/response.go` returns a line inside the `SecretMetadata` type (≥1 match).
- [ ] The detail handler sets `name` from the loaded secret — evidence: `grep -n 'found.Name' pkg/handler/secret-metadata.go` returns ≥1 line.
- [ ] The `main_test.go` detail contract assertion asserts `name` present and the body has 4 keys — evidence: `grep -n 'HaveKey("name")' main_test.go` returns ≥1 line AND the adjacent `HaveLen(...)` for the metadata map asserts `4`.
- [ ] The search body is a `{count, results}` envelope with `next`/`previous` keys — evidence: `grep -n 'json:"count"' pkg/api/response.go` returns ≥1 line inside the search-envelope type.
- [ ] Each search result carries `hashid`, `name`, `username`, `url`, `api_url` — evidence: `grep -nE 'json:"(hashid|name|username|url|api_url)"' pkg/api/response.go` returns 5 matches within the search-result type.
- [ ] The store search returns per-match records with key/name/username/url instead of bare keys — evidence: a Ginkgo store test asserts a returned record's name/username/url equal the stored secret's, and `make precommit` exits 0.
- [ ] The `SecretStore` mock matches the new search signature — evidence: `go build ./...` exits 0 (mock in `mocks/secret-store.go` compiles against the new interface).
- [ ] The `main_test.go` search contract assertion asserts the envelope `count` and a result's `name` — evidence: `grep -nE 'results|count' main_test.go` shows a search block asserting `name` present on a result object.
- [ ] The e2e scenario `scenarios/001-core-api-e2e.md` asserts `name` present in a search result — evidence: `grep -n 'name' scenarios/001-core-api-e2e.md` shows a jq assertion on `.results[].name` (or equivalent).
- [ ] `README.md` documents the name-bearing search/detail shape and `CHANGELOG.md` has an `## Unreleased` entry — evidence: `grep -n 'Unreleased' CHANGELOG.md` returns ≥1 line placed below the preamble and above the newest `## vX.Y.Z`.
- [ ] Full check passes — evidence: `make precommit` exits 0.

## Verification

```
go build ./...
make precommit
```

Expected: both exit 0. The updated `main_test.go` contract test passes with `name` asserted in the detail body (4 keys) and in each search result, and the enriched search envelope asserted.

## Suggested Decomposition

| # | Prompt focus | Covers DBs | Covers ACs | Depends on |
|---|---|---|---|---|
| 1 | DETAIL `name` only: `api.SecretMetadata` gains `Name`; `secret-metadata.go` sets it from `found.Name`; detail contract assertion goes `HaveLen(3)`→`HaveLen(4)` + `HaveKey("name")`. No signature change. | 1 | 1, 2, 3 | — |
| 2 | SEARCH envelope + store records: `{count, results}` envelope with per-result `hashid`/`name`/`username`/`url`; `store.Search` returns records instead of `Keys`; `SecretStore` mock regen; search-envelope contract block in `main_test.go`; store test; scenario 001 `name` assertion. | 2, 3, 4, 5, 6 | 4, 5, 6, 7, 8, 9 | — |

Both prompts touch AC 10 (README/CHANGELOG) and AC 11 (`make precommit`): each adds its own CHANGELOG bullet and must leave `make precommit` green.

Rationale: prompts 1 and 2 are independent — prompt-1 touches only the detail path (`pkg/api/response.go` `SecretMetadata`, `secret-metadata.go`, the detail block in `main_test.go`), prompt-2 touches only the search path (`SearchResults`/`SearchResult`, `secret-search.go`, `store.Search`, the mock, the search block in `main_test.go`, scenario 001). They share no files, so the separable detail-`name` win is not blocked by a search-side compile break, and the two can land in either order. Within prompt-2 the `store.Search` signature change, its mock regeneration, and the handler that consumes it must land atomically or the build breaks.

## Do-Nothing Option

If we skip this, `teamvault-cli search` and `info` continue to show bare keys for Lockbox secrets, making the already-merged client fix inert for Lockbox backends. Users must memorize opaque keys to identify secrets. The current shape is not acceptable given the goal of drop-in TeamVault compatibility, and the companion client change already assumes Lockbox will emit `name`.
