---
status: approved
spec: [001-write-api-teamvault-compat]
created: "2026-07-14T16:02:00Z"
queued: "2026-07-14T14:10:42Z"
branch: dark-factory/write-api-teamvault-compat
---

<summary>
- Adds the TeamVault create endpoint: POST /api/secrets/ (and /api/v1/secrets/), Basic-auth protected on both prefixes.
- A client posts a TeamVault create body; the server generates a fresh unique key, stores the secret encrypted, and responds 201 with a TeamVault-shaped representation containing the new hashid and api_url.
- Malformed bodies (missing/invalid content_type or secret_data, unsupported type, bad base64) get a 400; missing/wrong auth gets a 401; a storage failure gets a 500.
- Key generation retries on the astronomically-unlikely collision, and never overwrites an existing secret (check-and-set).
- A secret created via POST is immediately readable through the unchanged read endpoints.
- Wires the new route into both API prefixes without touching any existing read route.
</summary>

<objective>
Implement `POST /api/secrets/` (both prefixes, Basic auth): decode a TeamVault create body, validate it, generate a fresh unique server-side key, store the secret encrypted via the check-and-set `Create`, and respond HTTP 201 with a TeamVault-shaped `SecretRepresentation`. Wire the route into both `/api` and `/api/v1`.
</objective>

<context>
Read `/home/node/.claude/CLAUDE.md` and `/workspace/CLAUDE.md`.

Read these coding-plugin guides:
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-http-handler-refactoring-guide.md` (handlers live in `pkg/handler/`, `New*Handler` naming)
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-json-error-handler-guide.md`
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-error-wrapping-guide.md`
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-testing-guide.md`

Read these source files fully:
- `/workspace/pkg/handler/secret-upsert.go` — the closest existing handler; note it uses `libhttp.NewJSONHandler` which ALWAYS returns HTTP 200. Your create handler must NOT use that (it needs 201) — see requirement 2.
- `/workspace/pkg/handler/secret-metadata.go` and `/workspace/pkg/handler/url.go` — how `apiPrefix(req, suffix)` and `absoluteURL(req, path)` build prefix-preserving absolute URLs. You will reuse these to build `api_url`.
- `/workspace/pkg/handler/basic-auth.go` — the auth wrapper (already applied in `registerAPI`).
- `/workspace/main.go` — the `registerAPI(router, prefix, store)` method where routes are wired. You will add the POST route there. Note the current PUT route on `/secrets/{key}/`.
- `/workspace/main_test.go` — the contract-test harness (`newContractRouter`, `registerAPI` on both prefixes). Prompt 6 adds POST contract assertions; this prompt adds handler-level unit tests in `pkg/handler/`.
- `/workspace/pkg/api/write.go` (from prompt 2) — `CreateSecretRequest`, `SecretData`, `SecretRepresentation`, `CreateSecretRequest.Validate(ctx)`.
- `/workspace/pkg/secret/store.go` and `/workspace/pkg/secret/keygen.go` (from prompt 1) — `Store.Create(ctx, key, secret) error`, `ErrKeyExists`, `KeyGenerator`, `NewKeyGenerator(secret.DefaultKeyLength)`.

If `pkg/api/write.go` or `secret.Store.Create` / `secret.KeyGenerator` are missing, prompts 1-2 have not shipped: STOP and report `Status: failed` with `"prompt 1/2 not yet deployed"`.

Verified libhttp helpers for a 201 success (NOT `NewJSONHandler`):
```go
// github.com/bborbe/http@v1.26.17/http_send-json-response.go
func SendJSONResponse(ctx context.Context, resp http.ResponseWriter, data interface{}, statusCode int) error
// github.com/bborbe/http@v1.26.17/http_with-error.go
type WithError interface { ServeHTTP(ctx context.Context, resp http.ResponseWriter, req *http.Request) error }
type WithErrorFunc func(ctx context.Context, resp http.ResponseWriter, req *http.Request) error
// github.com/bborbe/http@v1.26.17/http_error-handler.go
func WrapWithStatusCode(err error, code int) ErrorWithStatusCode
```
The `registerAPI` auth wrapper already wraps every `libhttp.WithError` in `libhttp.NewJSONErrorHandler`, which reads `ErrorWithStatusCode` to pick the status and defaults to 500 otherwise.
</context>

<requirements>
1. **New handler file** `/workspace/pkg/handler/secret-create.go` (2026 BSD header, `package handler`). Define:
   ```go
   // NewSecretCreateHandler serves POST /api/secrets/ — the TeamVault create
   // endpoint. It decodes a TeamVault create body, validates it, generates a
   // fresh unique key, stores the secret encrypted, and responds 201 with a
   // TeamVault-shaped representation containing the new hashid and api_url.
   func NewSecretCreateHandler(store secret.Store, keyGen secret.KeyGenerator) libhttp.WithError
   ```
   Return a `libhttp.WithErrorFunc(func(ctx, resp, req) error { ... })` (NOT `NewJSONHandler`, because success must be 201).

2. **Handler body:**
   - Decode the JSON body into `api.CreateSecretRequest`. On decode error, return `libhttp.WrapWithStatusCode(errors.Wrap(ctx, err, "decode create request failed"), http.StatusBadRequest)` (malformed JSON is a client error → 400).
   - Call `value, err := body.Validate(ctx)`. `Validate` already wraps its errors with `WrapWithStatusCode(..., 400)`; return the error as-is (do NOT re-wrap and lose the status).
   - Generate a unique key with bounded retry on collision:
     ```go
     const maxKeyAttempts = 10
     var key secret.Key
     for attempt := 0; attempt < maxKeyAttempts; attempt++ {
         if err := ctx.Err(); err != nil { return errors.Wrap(ctx, err, "context cancelled during key generation") }
         candidate, err := keyGen.Generate(ctx)
         if err != nil { return errors.Wrap(ctx, err, "generate key failed") } // → 500
         if err := store.Create(ctx, candidate, value); err != nil {
             if stderrors.Is(err, secret.ErrKeyExists) { continue } // collision → regenerate
             return errors.Wrapf(ctx, err, "create secret %s failed", candidate) // backend error → 500
         }
         key = candidate
         break
     }
     if key == "" { return errors.Errorf(ctx, "could not generate a unique key after %d attempts", maxKeyAttempts) } // → 500
     ```
     Import `stderrors "errors"` for `stderrors.Is`.
   - Build the representation and respond 201:
     ```go
     prefix := apiPrefix(req, "secrets/")
     repr := api.SecretRepresentation{
         Hashid:      key.String(),
         APIURL:      absoluteURL(req, prefix+"secrets/"+key.String()+"/"),
         ContentType: value.ContentType,
         Name:        value.Name,
         Username:    value.Username,
         URL:         value.URL,
         Description: value.Description,
     }
     return libhttp.SendJSONResponse(ctx, resp, repr, http.StatusCreated)
     ```
     NOTE on `apiPrefix`: the create route is mounted at `<prefix>/secrets/` with NO trailing key, so the request path is e.g. `/api/v1/secrets/`. `apiPrefix(req, "secrets/")` returns `/api/v1/` (it trims the suffix `secrets/` off the path). Then `absoluteURL(req, "/api/v1/secrets/<key>/")` yields the correct prefix-preserving `api_url`. Confirm this against `pkg/handler/url.go` before writing.

3. **Route wiring** in `/workspace/main.go` `registerAPI`. Add, alongside the existing routes, on the SAME prefix:
   ```go
   router.Path(prefix + "/secrets/").Methods(http.MethodPost).
       Handler(auth(handler.NewSecretCreateHandler(store, secret.NewKeyGenerator(secret.DefaultKeyLength))))
   ```
   Because gorilla/mux matches method-specific routes, this coexists with the existing `GET <prefix>/secrets/` (search) route on the same path. Do NOT remove or alter the GET search route or any other existing route in this prompt. Keep the existing PUT route for now (prompt 5 removes it). Import `"github.com/bborbe/lockbox/pkg/secret"` in `main.go` if not already imported (it is — used by `secret.NewStore`).
   - `registerAPI` currently takes `(router *mux.Router, prefix string, store secret.Store)`. Do NOT change its signature: construct the `KeyGenerator` inline in the handler wiring as shown (a fresh generator per prefix is fine and stateless). This keeps `newContractRouter` in `main_test.go` and the two `registerAPI` call sites in `createHTTPServer` working unchanged. Verify there are exactly these call sites with `grep -rn 'registerAPI(' /workspace` and confirm none break.

4. **Tests** — `/workspace/pkg/handler/secret-create_test.go` (new file, `package handler_test`, Ginkgo). Use the counterfeiter `mocks.SecretStore` and `mocks.KeyGenerator` (from prompt 1). Follow the style of `pkg/handler/secret-upsert_test.go` and `secret-metadata_test.go`. Wrap the handler in `libhttp.NewJSONErrorHandler` in the test (mirroring `registerAPI`) so the status codes surface, then drive it with `httptest`. Assert:
   - **201 happy path (password):** valid password body → `store.Create` called once with the mapped `secret.Secret`; response code 201; response JSON has non-empty `hashid`, an `api_url` ending in `/secrets/<hashid>/`, and `content_type=="password"`, `name`, `username`, `url` echoing the body.
   - **201 happy path (file):** valid file body (real base64) → 201; representation `content_type=="file"`.
   - **400 cases:** missing content_type; `content_type:"cc"`; missing secret_data; password body missing password; file body with invalid base64 → each returns 400 and `store.Create` is NEVER called (assert `store.CreateCallCount() == 0`).
   - **Collision retry:** stub the mock `store.CreateStub` to return `secret.ErrKeyExists` on the first call and `nil` on the second → handler returns 201 and `store.CreateCallCount() == 2`; `keyGen.GenerateCallCount() == 2`.
   - **Exhausted retries → 500:** `store.CreateStub` always returns `secret.ErrKeyExists` → response code 500 (via the error handler) and `store.CreateCallCount() == maxKeyAttempts` (10).
   - **Backend error → 500:** `store.CreateStub` returns a plain non-`ErrKeyExists` error → 500, called once.
   - **Keygen error → 500:** `keyGen.GenerateReturns("", someErr)` → 500, `store.Create` never called.
   - Set the `keyGen` mock to return deterministic keys (`keyGen.GenerateReturns(secret.Key("AbC12345"), nil)` or a stub cycling values) so `api_url` assertions are stable.
</requirements>

<constraints>
- Handler lives in `pkg/handler/` (never inline in `main.go`); constructor named `New*Handler` per `go-http-handler-refactoring-guide.md`.
- Success is HTTP 201 via `libhttp.SendJSONResponse(ctx, resp, repr, http.StatusCreated)` — do NOT use `libhttp.NewJSONHandler` (it hardcodes 200).
- Errors via `github.com/bborbe/errors`; client errors carry their status via `libhttp.WrapWithStatusCode`. Never `fmt.Errorf`; never `context.Background()` in `pkg/`. Use `stderrors "errors"` only for `stderrors.Is`.
- The generated key must be URL-safe and unique via check-and-set; never overwrite an existing secret; bounded retry (10 attempts) on `ErrKeyExists`, then 500.
- Basic auth unchanged — the route is wrapped by the existing `auth(...)` closure in `registerAPI`; do NOT add a second auth layer.
- Do NOT change any READ route or the read response shapes. Do NOT change the `registerAPI` signature. Do NOT remove the PUT route (prompt 5).
- GoDoc on the exported handler constructor. No bare `interface{}`.
- 2026 BSD header on new files. ≥80% coverage of the new handler including every error branch.
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
grep -n 'func NewSecretCreateHandler' pkg/handler/secret-create.go
grep -n 'SendJSONResponse' pkg/handler/secret-create.go
grep -n 'MethodPost' main.go
grep -rn 'registerAPI(' main.go main_test.go
go test -coverprofile=/tmp/cover.out ./pkg/handler/... && go tool cover -func=/tmp/cover.out
```

Each grep must return ≥1 line; the `registerAPI(` grep must show the wiring in `main.go` plus the two call sites in `createHTTPServer` and the two in `main_test.go` still intact; coverage of the new handler ≥80%.
</verification>
