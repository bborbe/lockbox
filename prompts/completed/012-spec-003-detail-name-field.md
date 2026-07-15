---
status: completed
spec: [003-search-name-compat]
summary: Added `name` field to GET /api/secrets/{key}/ detail response body — SecretMetadata DTO now returns {name, username, url, current_revision}, sourced from stored secret.Name, with handler, unit test, contract test, README, and CHANGELOG updated.
execution_id: lockbox-search-name-exec-012-spec-003-detail-name-field
dark-factory-version: dev
created: "2026-07-15T21:20:00Z"
queued: "2026-07-15T21:27:21Z"
started: "2026-07-15T21:27:23Z"
completed: "2026-07-15T21:28:51Z"
branch: dark-factory/search-name-compat
---
<summary>
- The secret-detail endpoint (`GET /api/secrets/{key}/` and its `/api/v1` mirror) now returns the human-readable secret `name` alongside the existing username, url, and current_revision.
- This lets `teamvault-cli info` show a real secret name for Lockbox-backed secrets instead of falling back to the bare key.
- The change is purely additive: no existing field is removed, renamed, or retyped; `current_revision` still ends at `.../secret-revisions/{key}/` with no `/data` suffix.
- Only metadata is exposed — the password and file content stay behind the encryption seam and are never returned by this endpoint.
- A secret with an empty stored name returns `"name": ""` (field always present, value empty) — not a missing field.
- The frozen-contract check is satisfied because the TeamVault contract test in `main_test.go` is updated in lockstep to assert the new `name` key and a 4-key detail body.
- README and CHANGELOG record the name-bearing detail shape.
</summary>

<objective>
Add a `name` field to the secret-detail response body so `GET /api/secrets/{key}/` (and the `/api/v1` mirror) returns `{name, username, url, current_revision}`, sourced from the stored secret's `Name`, moving Lockbox's detail shape closer to real TeamVault. No signature changes; only the detail DTO, the detail handler, the detail contract assertion, and docs change.
</objective>

<context>
Read `/home/node/.claude/CLAUDE.md` and `/workspace/CLAUDE.md` (if present) for project conventions.

Read these coding-plugin guides (in-container paths):
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-error-wrapping-guide.md`
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-testing-guide.md`
- `/home/node/.claude/plugins/marketplaces/coding/docs/changelog-guide.md`
- `/home/node/.claude/plugins/marketplaces/coding/docs/teamvault-conventions.md`

Read `/workspace/docs/dod.md` — the "TeamVault API compatibility (frozen contract)" and "Secret handling" clauses. This spec's waiver is scoped to exactly the detail `name` field; no other route, auth, or JSON shape may change.

Read these source files fully before editing:

- `/workspace/pkg/api/response.go` — the `SecretMetadata` DTO. It is currently:
  ```go
  // SecretMetadata is the body of GET /api/secrets/{key}/.
  // current_revision is an absolute URL pointing at the revision-data endpoint.
  type SecretMetadata struct {
      Username        string `json:"username"`
      URL             string `json:"url"`
      CurrentRevision string `json:"current_revision"`
  }
  ```
  You add a `Name string \`json:"name"\`` field to this struct. Do NOT touch `RevisionData`, `SearchResults`, or `SearchResult` in this prompt — those belong to prompt 2.

- `/workspace/pkg/handler/secret-metadata.go` — `NewSecretMetadataHandler`. It loads the secret via `store.Get(ctx, key)` into `found` (a `*secret.Secret`), builds the absolute `revision` URL, and returns:
  ```go
  return api.SecretMetadata{
      Username:        found.Username,
      URL:             found.URL,
      CurrentRevision: revision,
  }, nil
  ```
  You add `Name: found.Name,` to this literal. Do NOT change the `revision` URL construction — `current_revision` must still end at `.../secret-revisions/{key}/` (trailing slash, no `/data`).

- `/workspace/pkg/secret/secret.go` — the `Secret` struct. `Name` is an existing `string` field (the human-readable TeamVault name; may be empty). No change to this file.

- `/workspace/pkg/handler/secret-metadata_test.go` — the existing handler unit test. Read it to match its Ginkgo/Gomega style (external `handler_test` package, counterfeiter `mocks.SecretStore`). You extend it to assert `name`.

- `/workspace/main_test.go` — the TeamVault contract test. The detail assertion block lives inside `Context("POST /secrets/ (password)", ...)` and currently reads (around the "GET metadata" comment):
  ```go
  Expect(metadata).To(HaveKey("username"))
  Expect(metadata).To(HaveKey("url"))
  Expect(metadata).To(HaveKey("current_revision"))
  Expect(metadata).To(HaveLen(3))
  Expect(metadata["username"]).To(Equal("alice"))
  Expect(metadata["url"]).To(Equal("https://example.com"))
  ```
  The POST body in this context sets `Name: "gh"` (see `postBody`). You update this block to also assert the `name` key, a length of 4, and value `"gh"`. Also update the "body must be exactly {username, url, current_revision}" comment above it to include `name`.

- `/workspace/README.md` — the endpoint table row `| \`GET /api/secrets/{key}/\` | \`{username, url, current_revision}\` |`. Update this row's shape to include `name`.

- `/workspace/CHANGELOG.md` — no `## Unreleased` section exists yet; you create one directly below the preamble and above `## v0.7.0`.
</context>

<requirements>
1. **Add the `Name` field to `SecretMetadata`** in `/workspace/pkg/api/response.go`. Place `name` first (matching real TeamVault's field ordering and the `SecretRepresentation` DTO in `pkg/api/write.go`, which orders name before username/url), with a doc comment:
   ```go
   // SecretMetadata is the body of GET /api/secrets/{key}/.
   // current_revision is an absolute URL pointing at the revision-data endpoint.
   type SecretMetadata struct {
       // Name is the human-readable secret name; may be empty.
       Name            string `json:"name"`
       Username        string `json:"username"`
       URL             string `json:"url"`
       CurrentRevision string `json:"current_revision"`
   }
   ```
   Do NOT modify `RevisionData`, `SearchResults`, or `SearchResult` in this file — they are prompt 2's scope.

2. **Set `Name` in the detail handler** in `/workspace/pkg/handler/secret-metadata.go`. Add `Name: found.Name,` to the returned `api.SecretMetadata` literal:
   ```go
   return api.SecretMetadata{
       Name:            found.Name,
       Username:        found.Username,
       URL:             found.URL,
       CurrentRevision: revision,
   }, nil
   ```
   Do NOT change the `store.Get` call, the error wrap (`errors.Wrapf(ctx, err, "get secret %s failed", key)` from `github.com/bborbe/errors`), the `apiPrefix`/`absoluteURL` calls, or the `revision` URL (it must still end at `.../secret-revisions/{key}/`). If `found.Name` is empty, the response naturally carries `"name": ""` — that is the required behavior (field always present, value may be empty).

3. **Extend the handler unit test** in `/workspace/pkg/handler/secret-metadata_test.go`. In the scenario where `store.Get` returns a secret, ensure the returned `*secret.Secret` has a non-empty `Name` (e.g. `Name: "gh"`), and add an assertion that the decoded response body's `name` equals that value. Follow the existing test's decode style (it decodes into `api.SecretMetadata` or a `map[string]any` — match whichever the existing test uses). Keep all existing assertions passing. Use `any`, never bare `interface{}`.

4. **Update the detail contract assertion** in `/workspace/main_test.go` inside `Context("POST /secrets/ (password)", ...)`. The `postBody` already sets `Name: "gh"`. Change the metadata block to:
   ```go
   // GET metadata — body must be exactly {name, username, url, current_revision}
   ...
   Expect(metadata).To(HaveKey("name"))
   Expect(metadata).To(HaveKey("username"))
   Expect(metadata).To(HaveKey("url"))
   Expect(metadata).To(HaveKey("current_revision"))
   Expect(metadata).To(HaveLen(4))
   Expect(metadata["name"]).To(Equal("gh"))
   Expect(metadata["username"]).To(Equal("alice"))
   Expect(metadata["url"]).To(Equal("https://example.com"))
   ```
   Update the `HaveLen(3)` to `HaveLen(4)` and the "body must be exactly {username, url, current_revision}" comment to include `name`. Do NOT change the revision-data block (it still asserts `{password, file}` with `HaveLen(2)`), the `current_revision` suffix assertion, or any other Context in the file.

5. **Update the README endpoint table** in `/workspace/README.md`. Change the detail row's shape from `{username, url, current_revision}` to `{name, username, url, current_revision}`. Do not alter other rows in this prompt (the search row is prompt 2's scope).

6. **Add a CHANGELOG entry**. Create a `## Unreleased` section in `/workspace/CHANGELOG.md` directly below the preamble (the lines ending at "...PATCH version when you make backwards-compatible bug fixes.") and above `## v0.7.0`. Add a `feat:` bullet, e.g.:
   `- feat: GET /api/secrets/{key}/ now returns the secret \`name\` alongside \`username\`, \`url\`, \`current_revision\` (TeamVault-compatible detail shape); \`teamvault-cli info\` can now show Lockbox secret names`
</requirements>

<constraints>
- Frozen-contract waiver is scoped to exactly the detail `name` field. No other route, auth, or JSON shape may change. See `docs/dod.md` "TeamVault API compatibility (frozen contract)".
- Do NOT remove, rename, or retype any existing field: `username`, `url`, `current_revision` all stay unchanged in the detail body.
- `current_revision` semantics are unchanged — it still ends at `.../secret-revisions/{key}/` with a trailing slash and no `/data` suffix.
- Do NOT return secret values: `password` and file content are NOT exposed by the detail endpoint. Only `name` (metadata) is newly exposed. See `docs/dod.md` "Secret handling".
- Do NOT touch the search path (`SearchResults`, `SearchResult`, `pkg/handler/secret-search.go`, `store.Search`, the `SecretStore` mock, scenario 001) — that is prompt 2's scope.
- Wrap errors with `github.com/bborbe/errors`; never `fmt.Errorf`.
- Tests use Ginkgo v2 / Gomega with Counterfeiter mocks in external `_test` packages.
- This repo does NOT vendor (`/vendor` is gitignored, Makefile uses `-mod=mod`); never run `go mod vendor` and never pass `-mod=vendor`.
- Container-autonomous: file edits + `make` only. No `kubectl`, no `docker`, no `gh`, no PR/deploy steps.
- Do NOT commit — dark-factory handles git.
- Every existing test must still pass.
</constraints>

<verification>
Run in `/workspace`:

```
make test
make precommit
```

Both must exit 0 (`make precommit` runs generate + full lint + trivy). Then confirm the additive detail shape landed:

```
grep -n 'json:"name"' pkg/api/response.go        # expect ≥1 match inside the SecretMetadata type
grep -n 'found.Name' pkg/handler/secret-metadata.go   # expect ≥1 match
grep -n 'HaveKey("name")' main_test.go            # expect ≥1 match in the detail block
grep -n 'HaveLen(4)' main_test.go                 # expect the detail metadata map length is now 4
grep -n 'Unreleased' CHANGELOG.md                 # expect the new section
```

Confirm the search path was NOT touched by this prompt:

```
grep -n 'json:"count"\|json:"hashid"' pkg/api/response.go   # expect NO match (prompt 2 adds these)
```

(Do NOT add `-mod=vendor`. Do NOT run any `git` command — `.git` is masked in this container.)
</verification>
