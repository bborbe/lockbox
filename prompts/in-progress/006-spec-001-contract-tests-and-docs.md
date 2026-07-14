---
status: approved
spec: [001-write-api-teamvault-compat]
created: "2026-07-14T16:05:00Z"
queued: "2026-07-14T14:10:42Z"
branch: dark-factory/write-api-teamvault-compat
---

<summary>
- Adds full TeamVault write-API contract coverage to main_test.go, driven through the real router on BOTH the /api and /api/v1 prefixes.
- Proves the end-to-end journey a real client takes: POST creates a secret and returns a hashid + api_url; the created secret is immediately readable through the unchanged read endpoints; PATCH updates metadata and value; content_type is immutable; two identical POSTs yield two independent secrets.
- Proves the rejection contract: malformed POST bodies return 400, a missing hashid PATCH returns 404, and missing/wrong auth returns 401 on both write methods.
- Proves the old flat PUT no longer creates a secret.
- Documents the new POST/PATCH endpoints in the README API table and finalizes the CHANGELOG entry for the write-API replacement.
</summary>

<objective>
Add Ginkgo contract tests to `main_test.go` that exercise `POST /api/secrets/` and `PATCH /api/secrets/{hashid}/` through the real router on both prefixes (create → read round-trip, update, immutability, uniqueness, 400/401/404, old-PUT gone), and document the new endpoints in `README.md` and the `CHANGELOG.md` entry.
</objective>

<context>
Read `/home/node/.claude/CLAUDE.md` and `/workspace/CLAUDE.md`.

Read these coding-plugin guides:
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-testing-guide.md`
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-test-types-guide.md` (this is the integration/contract layer — it goes through the real dispatch path, not mocks)
- `/home/node/.claude/plugins/marketplaces/coding/docs/changelog-guide.md`
- `/home/node/.claude/plugins/marketplaces/coding/docs/readme-guide.md`

Read these source files fully:
- `/workspace/main_test.go` — the existing contract harness: `newContractRouter(user, pass)` builds a real `mux.Router` via `registerAPI` on both `/api` and `/api/v1` over an in-memory kv store; the `for _, prefix := range []string{"/api", "/api/v1"}` loop; the surviving 401 auth `It` blocks (after prompt 5). You ADD new `It` blocks inside that per-prefix `Describe`.
- `/workspace/main.go` — `registerAPI` now wires GET metadata, GET revision-data, GET search, POST create, PATCH update (no PUT). The create handler generates a REAL random key (via `secret.NewKeyGenerator`), so the returned `hashid` is not predictable — tests must read it from the POST response and reuse it, NOT hard-code it.
- `/workspace/pkg/api/write.go` — `CreateSecretRequest`, `SecretData`, `SecretRepresentation`.
- `/workspace/pkg/api/response.go` — `SecretMetadata` (`{username, url, current_revision}`), `RevisionData` (`{password, file}`), `SearchResults`.
- `/workspace/README.md` — the `## API` section with the endpoint table.
- `/workspace/CHANGELOG.md` — the `## Unreleased` entry (created in prompt 5; append if present).

If `POST`/`PATCH` are not wired in `main.go` (grep `MethodPost`/`MethodPatch`), prompts 3-5 have not shipped: STOP and report `Status: failed` with `"prompt 3/4/5 not yet deployed"`.

Because the create handler uses a real key generator, the tests are non-deterministic in the key value; always capture `hashid` from the decoded POST response and interpolate it into subsequent GET/PATCH request paths and expected `api_url` values.
</context>

<requirements>
1. **Add write contract tests to `main_test.go`** inside the existing `Describe("prefix "+prefix, ...)` block (so every assertion runs for BOTH `/api` and `/api/v1`). Use the real `router` from `newContractRouter(user, pass)` and `httptest`. Add these `It` blocks:

   a. **POST create → read round-trip (password):** POST `<prefix>/secrets/` with body
      `api.CreateSecretRequest{ContentType:"password", Name:"gh", Username:"alice", URL:"https://example.com", SecretData:&api.SecretData{Password:"s3cr3t"}}` and Basic auth →
      - response code **201**;
      - decode into a map (or `api.SecretRepresentation`); assert `hashid` is a non-empty string; `api_url == "http://example.com"+prefix+"/secrets/"+hashid+"/"`; `content_type=="password"`, `name=="gh"`, `username=="alice"`, `url=="https://example.com"`.
      - Then `GET <prefix>/secrets/<hashid>/` → 200 with body keys EXACTLY `{username, url, current_revision}` (assert `HaveKey` for those three and `HaveLen(3)` on the decoded map), `username=="alice"`, `url=="https://example.com"`, `current_revision=="http://example.com"+prefix+"/secret-revisions/"+hashid+"/data"`.
      - Then `GET <prefix>/secret-revisions/<hashid>/data` → 200 with body keys EXACTLY `{password, file}` (`HaveLen(2)`), `password=="s3cr3t"`, `file==""`.

   b. **POST create (file):** POST with `ContentType:"file"`, `SecretData:&api.SecretData{FileContent: base64.StdEncoding.EncodeToString([]byte("filecontent")), Filename:"f.txt"}` → 201; capture `hashid`; `GET .../data` → `file` equals the posted base64 string, `password==""`.

   c. **POST 400 cases** (each a separate `It` or a table): missing `content_type`; `content_type:"cc"`; missing `secret_data` (nil); password body missing `password`; file body missing `file_content` (or invalid base64 `"!!!"`). Each → response code **400**. To send a body with a missing/omitted field, marshal a `map[string]any` rather than the struct so you can omit keys.

   d. **PATCH update metadata + value:** POST-create a password secret, capture `hashid`; PATCH `<prefix>/secrets/<hashid>/` with `{name:"gh2", url:"https://new.example.com"}` → 200; then `GET <prefix>/secrets/<hashid>/` → `url=="https://new.example.com"`. Then PATCH the same hashid with `{secret_data:{password:"rotated"}}` → 200; then `GET .../data` → `password=="rotated"`.

   e. **content_type immutable on PATCH:** POST-create a password secret; PATCH it with `{content_type:"file"}` → 200 and the PATCH response body `content_type=="password"` (the `"file"` in the PATCH body is ignored). Also `GET .../secrets/<hashid>/` still resolves and the revision-data read still returns the password value.

   f. **Two identical POSTs yield distinct, independent secrets:** POST the same password body twice → two 201s with different `hashid`s (assert unequal); `GET .../data` for each hashid returns its own value (both readable).

   g. **PATCH on non-existent hashid → 404:** PATCH `<prefix>/secrets/doesnotexist/` with any valid body → response code **404**.

   h. **401 on write without/with wrong auth:** POST `<prefix>/secrets/` with NO Basic auth → 401; PATCH `<prefix>/secrets/anykey/` with WRONG Basic auth → 401. (The GET-side 401 blocks already exist from earlier; keep them.)

   i. **Old flat PUT no longer creates a secret:** `PUT <prefix>/secrets/somekey/` with a JSON body and valid auth → response code is NOT 2xx (assert `resp.Code >= 400`; gorilla/mux returns 405 since GET/PATCH remain on that path). Then `GET <prefix>/secrets/somekey/` → NOT 200 (the PUT did not create anything; the store has no such key so the read returns a non-200). Assert `resp.Code != http.StatusOK`.

   Implementation notes for the block:
   - Add a small local helper inside the `Describe` (or a package-level test helper func) `doJSON(method, path string, auth bool, body any) *httptest.ResponseRecorder` that marshals `body` (skip marshalling when `body == nil`), builds the request with `httptest.NewRequest`, sets Basic auth when `auth` is true, serves via `router`, and returns the recorder. This keeps the many requests terse. Alternatively inline — but do not copy 15 lines per request.
   - Always decode the POST response to read `hashid` before issuing dependent requests; never hard-code a key.
   - Keep `format.TruncatedDiff = false` (already set in `TestContractSuite`).

2. **README** — in `/workspace/README.md`, update the `## API` section table to add the write endpoints. The table currently lists only the three GET endpoints. Add rows:
   ```
   | `POST /api/secrets/` | create a secret (server-generated key); returns `{hashid, api_url, content_type, name, username, url}` |
   | `PATCH /api/secrets/{hashid}/` | update metadata and/or value; returns the secret representation |
   ```
   Also update the surrounding prose if it says the write API is a flat PUT (the current README describes only reads plus the intro; ensure it now says the write API is TeamVault-compatible `POST`/`PATCH`). The spec AC requires `grep -n 'POST /api/secrets/' README.md` ≥1 and `grep -n 'PATCH' README.md` ≥1.

3. **CHANGELOG** — ensure `/workspace/CHANGELOG.md` has a top entry (under `## Unreleased`, created in prompt 5 — append if it exists, create if somehow missing) describing the write-API replacement. The top entry must mention the new write API (`POST`/`PATCH`). Follow `changelog-guide.md`: prefix each bullet (`feat:`/`refactor:`), be specific. Do NOT duplicate a bullet prompt 5 already added; if prompt 5's bullets already cover POST/PATCH and the importer switch, this prompt only needs to confirm they are present and add a `feat:`/`docs:` bullet for the README + contract-test coverage if not already there, e.g.:
   - `- docs: Document POST /api/secrets/ and PATCH /api/secrets/{hashid}/ in the README API table`
</requirements>

<constraints>
- Contract tests run through the REAL router (`newContractRouter` → `registerAPI`) for BOTH `/api` and `/api/v1` — this is the integration boundary the spec requires; do NOT mock the store or handlers here.
- Never hard-code a `hashid` — the key generator is real and random; capture it from each POST response.
- The read-endpoint assertions must confirm the read shapes are FROZEN: metadata body is exactly `{username, url, current_revision}` (3 keys) and revision-data body is exactly `{password, file}` (2 keys). Do NOT modify any read handler or its existing assertions.
- Do NOT weaken or delete the existing 401 auth `It` blocks retained from prompt 5.
- README must document POST and PATCH; CHANGELOG top entry must mention the write API.
- Ginkgo/Gomega; no bare `interface{}` in new test code (use `any`/concrete). 2026 BSD header is already on `main_test.go`; keep it.
- Do NOT commit — dark-factory handles git.
- Every existing test must still pass.
</constraints>

<verification>
Run in `/workspace`:

```
make test
make precommit
```

`make precommit` must exit 0 (this is the AC that gates the whole spec). Then:

```
grep -n 'POST /api/secrets/' README.md   # expect >=1
grep -n 'PATCH' README.md                # expect >=1
grep -c 'MethodPost\|MethodPatch' main_test.go   # expect >=1 (write requests present)
grep -n 'Unreleased' CHANGELOG.md        # expect >=1 and the entry mentions the write API
```

Confirm the write contract assertions run for both prefixes: `make test` output should show the new `It` descriptions under both `prefix /api` and `prefix /api/v1`. Confirm no regression in the read contract: the metadata/revision-data key-set assertions still pass.
</verification>
