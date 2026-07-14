---
status: completed
spec: [001-write-api-teamvault-compat]
summary: Removed flat PUT upsert handler and DTOs, switched migrate-teamvault from PUT to POST /api/secrets/ with TeamVault create body
execution_id: lockbox-exec-005-spec-001-remove-put-and-switch-importer
dark-factory-version: dev
created: "2026-07-14T16:04:00Z"
queued: "2026-07-14T14:10:42Z"
started: "2026-07-14T14:29:14Z"
completed: "2026-07-14T14:31:58Z"
branch: dark-factory/write-api-teamvault-compat
---

<summary>
- Removes the legacy flat PUT upsert entirely: its route, its handler, and its request/response DTOs. The old PUT /api/secrets/{key}/ no longer creates secrets.
- Switches the migrate-teamvault importer from the flat PUT to the new TeamVault-compatible POST /api/secrets/ create endpoint.
- The importer now sends a TeamVault create body (content_type, name, username, url, secret_data) and reads back the server-generated hashid instead of choosing the key itself.
- Password and file secrets continue to migrate; credit-card secrets stay skipped exactly as before.
- Updates the importer's own test to expect a POST create body rather than a keyed PUT.
</summary>

<objective>
Delete the flat `PUT /api/secrets/{key}/` upsert (route, handler, `UpsertRequest`/`UpsertResult` DTOs) and switch the `migrate-teamvault` Lockbox client from that PUT to `POST /api/secrets/`, mapping each TeamVault source secret into the new create body.
</objective>

<context>
Read `/home/node/.claude/CLAUDE.md` and `/workspace/CLAUDE.md`.

Read these coding-plugin guides:
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-error-wrapping-guide.md`
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-testing-guide.md`
- `/home/node/.claude/plugins/marketplaces/coding/docs/changelog-guide.md`

Read these source files fully:
- `/workspace/pkg/handler/secret-upsert.go` and `/workspace/pkg/handler/secret-upsert_test.go` — the handler + test to DELETE.
- `/workspace/pkg/api/response.go` — remove `UpsertRequest` and `UpsertResult` from here.
- `/workspace/main.go` — remove the PUT route line in `registerAPI` (`router.Path(prefix + "/secrets/{key}/").Methods(http.MethodPut)...`).
- `/workspace/main_test.go` — the existing contract test has an `It("round-trips PUT then GET metadata and revision data", ...)` block that PUTs; this block must be removed or replaced. Prompt 6 rewrites the contract suite to use POST. In THIS prompt, remove the PUT-specific `It` block (and any `api.UpsertRequest`/`api.UpsertResult` references in `main_test.go`) so the code compiles after the DTOs are deleted; do NOT add the new POST contract assertions here (that is prompt 6). Leave the auth `It` blocks (401 tests) intact.
- `/workspace/pkg/migrate/lockbox.go` — the `LockboxClient` interface + `lockboxClient` impl that currently PUTs `api.UpsertRequest`. Rewrite to POST the create body.
- `/workspace/pkg/migrate/migrator.go` — `migrateOne` builds the `api.UpsertRequest`; change it to build the create body and call the renamed client method.
- `/workspace/pkg/migrate/model.go` — `TeamVaultSecret` (has `ContentType`, `Name`, `Username`, `URL`, `Hashid`) and the `ContentTypePassword`/`ContentTypeFile`/`ContentTypeCreditCard` constants.
- `/workspace/pkg/migrate/teamvault.go` — `GetRevisionData` returns `api.RevisionData{Password, File}`.
- `/workspace/pkg/migrate/migrator_test.go` — the importer test; its Lockbox mock server currently asserts `r.Method == http.MethodPut` and decodes `api.UpsertRequest`. Rewrite it to assert POST and decode `api.CreateSecretRequest`.
- `/workspace/cmd/migrate-teamvault/main.go` — its package GoDoc says "PUTs them into a running Lockbox"; update the wording to "creates them via POST".
- `/workspace/pkg/api/write.go` (from prompt 2) — `CreateSecretRequest`, `SecretData`, `SecretRepresentation`.

If `api.CreateSecretRequest` does not exist, prompt 2 has not shipped: STOP and report `Status: failed` with `"prompt 2 not yet deployed"`.
</context>

<requirements>
1. **Delete the flat PUT handler:** remove `/workspace/pkg/handler/secret-upsert.go` and `/workspace/pkg/handler/secret-upsert_test.go` entirely (`git rm`-equivalent — just delete the files; dark-factory handles git).

2. **Delete the upsert DTOs:** in `/workspace/pkg/api/response.go`, remove the `UpsertRequest` and `UpsertResult` struct declarations and their GoDoc. Leave `SecretMetadata`, `RevisionData`, `SearchResults`, `SearchResult` untouched.

3. **Remove the PUT route** in `/workspace/main.go` `registerAPI`: delete the line
   ```go
   router.Path(prefix + "/secrets/{key}/").Methods(http.MethodPut).
       Handler(auth(handler.NewSecretUpsertHandler(store)))
   ```
   Leave the GET metadata, GET revision-data, GET search, POST create (prompt 3), and PATCH update (prompt 4) routes intact. After removal, a `PUT <prefix>/secrets/<key>/` request no longer routes to any handler; because the GET/PATCH methods are still registered on that path, gorilla/mux returns 405 Method Not Allowed for PUT — that is the intended behavior.

4. **Fix `main_test.go` to compile:** remove the `It("round-trips PUT then GET ...")` block and any `api.UpsertRequest` / `api.UpsertResult` usage in `main_test.go`. Keep the `It("rejects wrong Basic auth with 401")` and `It("rejects missing Basic auth with 401")` blocks. Do NOT add POST contract tests here (prompt 6). The suite must still build and pass with the remaining assertions.

5. **Rewrite the migrator Lockbox client** in `/workspace/pkg/migrate/lockbox.go`:
   - Change the `LockboxClient` interface method from `Upsert(ctx, hashid, api.UpsertRequest) error` to:
     ```go
     // Create sends a TeamVault create request to Lockbox's POST /api/secrets/
     // endpoint and returns the server-generated hashid on success.
     Create(ctx context.Context, req api.CreateSecretRequest) (string, error)
     ```
     Keep the `//counterfeiter:generate -o ../../mocks/lockbox-client.go --fake-name LockboxClient . LockboxClient` annotation so the mock regenerates.
   - Implement `Create` on `lockboxClient`:
     - Marshal `req` (keep the existing `#nosec G117` comment on the marshal, as `secret_data` is a secret being written).
     - `url := l.baseURL + "/api/secrets/"` (POST create — no key in the path; the server assigns it).
     - `http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))`.
     - Set Basic auth, `Content-Type: application/json`, `Accept: application/json` (as today).
     - On transport error: `errors.Wrap(ctx, err, "post secret failed")`.
     - On `resp.StatusCode/100 != 2`: `errors.Wrapf(ctx, errUnexpectedStatus, "POST %s returned status %d", url, resp.StatusCode)` (reuse the existing `errUnexpectedStatus` sentinel from `pkg/migrate/errors.go`).
     - Decode the response into `api.SecretRepresentation` and return `result.Hashid, nil`. If the decode fails: `return "", errors.Wrap(ctx, err, "decode create response failed")`.
   - Update the `LockboxClient` GoDoc: it now "creates secrets in a running Lockbox instance via POST /api/secrets/".

6. **Update the migrator** in `/workspace/pkg/migrate/migrator.go` `migrateOne`: build the create body and call `Create`:
   ```go
   contentType := secret.ContentType // where the source's content type maps to the create content_type
   ```
   Actually map from the TeamVault source secret's `ContentType` field. Build:
   ```go
   req := api.CreateSecretRequest{
       ContentType: secret.ContentType,   // "password" or "file" (cc is already skipped upstream)
       Name:        secret.Name,
       Username:    secret.Username,
       URL:         secret.URL,
       SecretData:  &api.SecretData{},
   }
   switch secret.ContentType {
   case ContentTypePassword:
       req.SecretData.Password = data.Password
   case ContentTypeFile:
       req.SecretData.FileContent = data.File
   }
   if _, err := m.sink.Create(ctx, req); err != nil {
       return errors.Wrapf(ctx, err, "create secret %s in lockbox failed", secret.Hashid)
   }
   ```
   Note: `data` is the `api.RevisionData{Password, File}` fetched from `m.source.GetRevisionData`. The `File` field already holds base64 (TeamVault serves the file field base64-encoded), so it maps straight into `FileContent`, which the create validator base64-checks. The source `Hashid` is used only for logging now (the new key is server-assigned); the return hashid can be ignored (`_`). Keep the credit-card skip in `Run` exactly as-is (it never reaches `migrateOne`).

7. **Regenerate the LockboxClient mock** at `/workspace/mocks/lockbox-client.go` via `make generate` (invoked by `make precommit`) so it exposes `Create` instead of `Upsert`. Do NOT hand-edit the mock.

8. **Update `/workspace/cmd/migrate-teamvault/main.go`** package/`Run` GoDoc: replace "PUTs them into a running Lockbox instance" with "creates them via POST /api/secrets/ in a running Lockbox instance". No behavioral change needed there (it just wires `NewLockboxClient` → `NewMigrator`, which is unchanged).

9. **Rewrite the migrator test** `/workspace/pkg/migrate/migrator_test.go`:
   - The in-test Lockbox mock server handler for `/api/secrets/` must now assert `r.Method == http.MethodPost` (not PUT), decode the body into `api.CreateSecretRequest`, and respond with an `api.SecretRepresentation` carrying a server-assigned `hashid` (e.g. derive a fake hashid per call, or echo a fixed one — the migrator ignores the returned hashid, so any non-empty string works). Record the received `CreateSecretRequest`s keyed by something stable (e.g. by `req.Name` or `req.Username`) since the client no longer sends the key in the URL.
   - Update the assertions that previously checked `lockboxPuts["aaaa1111"]` etc. Because the URL no longer carries the source hashid, key the recorded map by a field the body carries — use `req.Name` (the source secrets have distinct names: "password secret", "file secret", "page2 secret"). Assert:
     - the password secret's create body has `ContentType=="password"`, `Username=="pwuser"`, `URL=="https://password.example.com"`, `SecretData.Password=="s3cr3t"`, and empty `SecretData.FileContent`.
     - the file secret's create body has `ContentType=="file"`, `SecretData.FileContent=="ZmlsZWNvbnRlbnQ="`, empty `SecretData.Password`.
     - the page-2 secret was created (`SecretData.Password=="page2pw"`).
     - credit-card, unreadable, and error secrets were NOT created (their names never appear).
   - Keep the pagination, count, and skip assertions (`report.Migrated==3`, `SkippedCC==1`, `SkippedUnreadable==1`, `Failed==1`) — those are unchanged. The mock must still assert Lockbox Basic auth (`lb-user`/`lb-pass`).
   - Import `github.com/bborbe/lockbox/pkg/api` (already imported) for `CreateSecretRequest`/`SecretData`/`SecretRepresentation`.

10. **CHANGELOG** — add a `## Unreleased` entry at the top of `/workspace/CHANGELOG.md` (immediately after implementing, before `make precommit`). Follow `changelog-guide.md`. Example bullets (adjust wording to what you actually did):
    - `- feat: Replace flat PUT upsert with TeamVault-compatible write API: POST /api/secrets/ (create, server-generated hashid) and PATCH /api/secrets/{hashid}/ (update), on both /api and /api/v1`
    - `- refactor: Switch migrate-teamvault importer from the flat PUT to POST /api/secrets/; remove UpsertRequest/UpsertResult DTOs and the flat-PUT handler`
    Note: prompt 6 also touches CHANGELOG/README; if `## Unreleased` already exists, append to it rather than replacing.
</requirements>

<constraints>
- After this prompt, `UpsertRequest`, `UpsertResult`, and `NewSecretUpsertHandler` must NOT appear anywhere outside `specs/` — the spec asserts `git grep -n 'UpsertRequest\|UpsertResult\|NewSecretUpsertHandler' -- ':!specs'` returns 0 matches.
- After this prompt, `pkg/migrate/lockbox.go` must use `http.MethodPost` and must NOT use `http.MethodPut`.
- The flat PUT route is gone; a PUT to the old path returns whatever gorilla/mux emits (405 given GET/PATCH remain on that path) — do NOT add a custom PUT handler to force a specific code.
- Password and file secrets migrate; credit-card secrets stay skipped (unchanged). Do NOT change the skip logic in `Run`.
- Errors via `github.com/bborbe/errors`; never `fmt.Errorf`; never `context.Background()` in `pkg/`.
- Regenerate mocks via `make generate`; never hand-edit `mocks/`.
- Do NOT change any READ route or read response shape.
- GoDoc on the changed exported symbols (`LockboxClient`, `Create`). No bare `interface{}`. 2026 BSD header on any file you create (none expected — this prompt edits/deletes).
- Do NOT commit — dark-factory handles git.
- Existing non-PUT tests must still pass.
</constraints>

<verification>
Run in `/workspace`:

```
make test
make precommit
```

`make precommit` must exit 0. Then confirm the removals and switch (non-git greps, matching the spec's ACs):

```
grep -rn 'UpsertRequest\|UpsertResult\|NewSecretUpsertHandler' --include='*.go' . ; echo "exit=$?"   # expect NO matches (grep exit 1)
grep -n 'http.MethodPost' pkg/migrate/lockbox.go   # expect >=1
grep -n 'http.MethodPut'  pkg/migrate/lockbox.go   # expect 0 (no output)
grep -n 'MethodPut' main.go ; echo "exit=$?"        # expect NO PUT route left (grep exit 1)
grep -rn 'func (fake \*LockboxClient) Create' mocks/lockbox-client.go   # expect >=1
go test -coverprofile=/tmp/cover.out ./pkg/migrate/... && go tool cover -func=/tmp/cover.out
```

The two `grep ... ; echo "exit=$?"` lines must report `exit=1` (no match found). `grep http.MethodPut pkg/migrate/lockbox.go` must produce no output. Migrator coverage ≥80%.
</verification>
