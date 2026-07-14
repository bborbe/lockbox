---
status: approved
spec: [001-write-api-teamvault-compat]
created: "2026-07-14T16:03:00Z"
queued: "2026-07-14T14:10:42Z"
branch: dark-factory/write-api-teamvault-compat
---

<summary>
- Adds the TeamVault update endpoint: PATCH /api/secrets/{hashid}/ (and /api/v1/...), Basic-auth protected on both prefixes.
- A client patches metadata (name, description, username, url) and/or replaces the secret value; the server updates the stored record and responds 200 with the updated representation.
- content_type is immutable on update: a content_type in the PATCH body is ignored.
- Patching a non-existent hashid returns 404; missing/wrong auth returns 401; a malformed secret_data returns 400.
- Wires the PATCH route into both API prefixes without touching any read route.
</summary>

<objective>
Implement `PATCH /api/secrets/{hashid}/` (both prefixes, Basic auth): load the existing secret (404 if absent), merge in the metadata fields present in the body and — when `secret_data` is present — the new value, keeping `content_type` immutable, persist it, and respond HTTP 200 with the updated `SecretRepresentation`. Wire the route into both prefixes.
</objective>

<context>
Read `/home/node/.claude/CLAUDE.md` and `/workspace/CLAUDE.md`.

Read these coding-plugin guides:
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-http-handler-refactoring-guide.md`
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-json-error-handler-guide.md`
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-error-wrapping-guide.md`
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-testing-guide.md`

Read these source files fully:
- `/workspace/pkg/handler/secret-create.go` (from prompt 3) — mirror its structure (WithErrorFunc, decode, validate, SendJSONResponse). PATCH returns 200, not 201.
- `/workspace/pkg/handler/secret-metadata.go` and `/workspace/pkg/handler/url.go` — `mux.Vars(req)["key"]`, `apiPrefix`, `absoluteURL`.
- `/workspace/main.go` — `registerAPI`; you add the PATCH route on the same `<prefix>/secrets/{key}/` path.
- `/workspace/pkg/api/write.go` (from prompt 2) — `CreateSecretRequest.ApplyUpdate(ctx, existing secret.Secret) (secret.Secret, error)` (content_type immutable, secret_data validated against existing type, 400 on malformed). Reuse it; do NOT re-implement merge logic.
- `/workspace/pkg/secret/store.go` — `Store.Get(ctx, key) (*Secret, error)` (returns error for a missing key) and `Store.Upsert(ctx, key, secret) error` (overwrite in place — correct for update, since the key already exists).

If `ApplyUpdate` or `secret-create.go` are missing, prompts 2-3 have not shipped: STOP and report `Status: failed` with `"prompt 2/3 not yet deployed"`.

Verified helpers (see prompt 3 context): `libhttp.SendJSONResponse(ctx, resp, data, statusCode)`, `libhttp.WrapWithStatusCode(err, code)`, `libhttp.WithErrorFunc`. The `NewJSONErrorHandler` wrapper in `registerAPI` maps `ErrorWithStatusCode` to the HTTP status (defaulting to 500).
</context>

<requirements>
1. **New handler file** `/workspace/pkg/handler/secret-update.go` (2026 BSD header, `package handler`). Define:
   ```go
   // NewSecretUpdateHandler serves PATCH /api/secrets/{hashid}/ — the TeamVault
   // update endpoint. It merges the metadata fields present in the body and,
   // when secret_data is present, the new value into the stored secret, keeps
   // content_type immutable, and responds 200 with the updated representation.
   func NewSecretUpdateHandler(store secret.Store) libhttp.WithError
   ```
   Return a `libhttp.WithErrorFunc`.

2. **Handler body:**
   - `key := secret.Key(mux.Vars(req)["key"])`.
   - Load the existing secret: `existing, err := store.Get(ctx, key)`. On error, treat it as "not found" and return `libhttp.WrapWithStatusCode(errors.Wrapf(ctx, err, "secret %s not found", key), http.StatusNotFound)` → 404. (The store's `Get` returns an error for a missing key; there is no separate "not found" sentinel, so a Get error on PATCH maps to 404. Document this mapping in a code comment.)
   - Decode the JSON body into `api.CreateSecretRequest`. On decode error, return `libhttp.WrapWithStatusCode(errors.Wrap(ctx, err, "decode update request failed"), http.StatusBadRequest)` → 400.
   - Merge: `updated, err := body.ApplyUpdate(ctx, *existing)`. `ApplyUpdate` already wraps malformed `secret_data` with `WrapWithStatusCode(..., 400)` and keeps `content_type` immutable; return its error as-is on failure.
   - Persist with `store.Upsert(ctx, key, updated)` (the key already exists, so overwrite-in-place is correct here — this is an update, not a create). On error return `errors.Wrapf(ctx, err, "update secret %s failed", key)` → 500.
   - Build the representation and respond 200:
     ```go
     prefix := apiPrefix(req, "secrets/"+key.String()+"/")
     repr := api.SecretRepresentation{
         Hashid:      key.String(),
         APIURL:      absoluteURL(req, prefix+"secrets/"+key.String()+"/"),
         ContentType: updated.ContentType,
         Name:        updated.Name,
         Username:    updated.Username,
         URL:         updated.URL,
         Description: updated.Description,
     }
     return libhttp.SendJSONResponse(ctx, resp, repr, http.StatusOK)
     ```
     NOTE: the PATCH route path includes the key (`<prefix>/secrets/<key>/`), so use `apiPrefix(req, "secrets/"+key.String()+"/")` exactly as `secret-metadata.go` does — verify against `pkg/handler/url.go` and `secret-metadata.go`.

3. **Route wiring** in `/workspace/main.go` `registerAPI`, on the SAME `<prefix>/secrets/{key}/` path (method-specific, coexists with the existing GET metadata route and the PUT route):
   ```go
   router.Path(prefix + "/secrets/{key}/").Methods(http.MethodPatch).
       Handler(auth(handler.NewSecretUpdateHandler(store)))
   ```
   Do NOT alter or remove the existing GET metadata route or the PUT route (prompt 5 removes PUT). Do NOT change the `registerAPI` signature.

4. **Tests** — `/workspace/pkg/handler/secret-update_test.go` (new file, `package handler_test`, Ginkgo, counterfeiter `mocks.SecretStore`). Wrap the handler in `libhttp.NewJSONErrorHandler` and drive with `httptest`; set `mux.Vars` by routing through a `mux.Router` with the path `/secrets/{key}/` (mirror how `secret-metadata_test.go` supplies the `key` var). Assert:
   - **200 metadata update:** `store.GetReturns(&secret.Secret{ContentType:"password", Name:"old", URL:"https://old", Password:"p"}, nil)`; PATCH body `{name:"new", url:"https://new"}` → 200; `store.Upsert` called once with a secret whose `Name=="new"`, `URL=="https://new"`, `Password=="p"` (unchanged), `ContentType=="password"`; response JSON `url=="https://new"`, `content_type=="password"`.
   - **200 value replacement:** PATCH body `{secret_data:{password:"newpw"}}` on the same existing password secret → 200; upserted secret `Password=="newpw"`.
   - **content_type immutable:** existing is a password secret; PATCH body `{content_type:"file"}` → 200; upserted secret and response `content_type` both remain `"password"`; `store.Upsert` secret `ContentType=="password"`.
   - **404:** `store.GetReturns(nil, someErr)` → response code 404; `store.Upsert` NEVER called (`UpsertCallCount()==0`).
   - **400 malformed secret_data:** existing password secret; PATCH body `{secret_data:{}}` (password empty) → 400; `store.Upsert` never called.
   - **400 malformed JSON body:** send invalid JSON → 400 (note: `store.Get` may or may not be called first depending on ordering; per requirement 2 the load happens before decode, so `Get` is called then decode fails — assert 400 and `UpsertCallCount()==0`).
   - **500 upsert failure:** valid PATCH but `store.UpsertReturns(someErr)` → 500.
</requirements>

<constraints>
- Handler in `pkg/handler/`; `New*Handler` naming.
- PATCH success is HTTP 200 via `libhttp.SendJSONResponse(ctx, resp, repr, http.StatusOK)`.
- 404 on a missing hashid (Get error), 400 on malformed body/secret_data, 401 on auth (handled by the existing wrapper), 500 on persist failure.
- `content_type` is immutable on update — a `content_type` in the PATCH body is ignored (enforced by `ApplyUpdate`; do NOT re-derive content_type from the body).
- Reuse `api.CreateSecretRequest.ApplyUpdate` for merge + validation; do NOT duplicate merge logic in the handler.
- Errors via `github.com/bborbe/errors`; statuses via `libhttp.WrapWithStatusCode`. Never `fmt.Errorf`; never `context.Background()` in `pkg/`.
- Do NOT change any READ route or read response shape, and do NOT change the `registerAPI` signature. Do NOT remove the PUT route (prompt 5).
- GoDoc on the exported constructor. No bare `interface{}`. 2026 BSD header. ≥80% coverage including every branch.
- Do NOT commit — dark-factory handles git.
- Existing tests must still pass.
</constraints>

<verification>
Run in `/workspace`:

```
make test
make precommit
```

`make precommit` must exit 0. Then:

```
grep -n 'func NewSecretUpdateHandler' pkg/handler/secret-update.go
grep -n 'MethodPatch' main.go
grep -n 'ApplyUpdate' pkg/handler/secret-update.go
go test -coverprofile=/tmp/cover.out ./pkg/handler/... && go tool cover -func=/tmp/cover.out
```

Each grep must return ≥1 line; new-handler coverage ≥80%.
</verification>
