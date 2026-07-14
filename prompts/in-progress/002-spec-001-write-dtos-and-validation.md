---
status: approved
spec: [001-write-api-teamvault-compat]
created: "2026-07-14T16:01:00Z"
queued: "2026-07-14T14:10:42Z"
branch: dark-factory/write-api-teamvault-compat
---

<summary>
- Adds the request/response data shapes for the TeamVault-compatible write API: a create/update body envelope and the representation returned to clients.
- Accepts TeamVault's create body: content_type, name, optional username/url/description, and a write-only secret_data block.
- Models secret_data for both supported content types: password (with an accepted-but-ignored otp field) and file (base64 file content plus optional filename).
- Adds pure validation that maps a decoded body into the stored record, rejecting missing content_type, missing secret_data, unsupported content types (including credit-card), mismatched secret_data, and invalid base64 — each as a client error.
- No HTTP routes or handlers change here; this prompt only produces the DTOs plus a reusable validation function the create and update handlers will call.
</summary>

<objective>
Add the wire DTOs for the TeamVault write API (create/update body, `secret_data` password/file shapes, and the response representation) plus a pure validation function that maps a decoded body into a `secret.Secret`, returning client-facing 400 errors for malformed input.
</objective>

<context>
Read `/home/node/.claude/CLAUDE.md` and `/workspace/CLAUDE.md`.

Read these coding-plugin guides:
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-patterns.md`
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-testing-guide.md`
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-error-wrapping-guide.md`
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-json-error-handler-guide.md` (how `libhttp.WrapWithStatusCode` drives the HTTP status from an error)
- `/home/node/.claude/plugins/marketplaces/coding/docs/teamvault-conventions.md` (what a lookup key / hashid is)

Read these source files fully:
- `/workspace/pkg/api/response.go` — existing DTOs (`SecretMetadata`, `RevisionData`, `SearchResults`, `SearchResult`, `UpsertRequest`, `UpsertResult`). You will ADD to this file. Do NOT remove `UpsertRequest`/`UpsertResult` in this prompt — prompt 5 removes them.
- `/workspace/pkg/secret/secret.go` — the `Secret` struct as expanded by prompt 1 (now has `Name`, `Description`, `ContentType`, plus `ContentTypePassword`/`ContentTypeFile` constants). If those fields/constants are NOT present, prompt 1 has not shipped: STOP and report `Status: failed` with `"secret.ContentType / secret.ContentTypePassword not present — prompt 1 not yet deployed"`.

`libhttp.WrapWithStatusCode` is verified at `github.com/bborbe/http@v1.26.17/http_error-handler.go`:
```go
func WrapWithStatusCode(err error, code int) ErrorWithStatusCode
```
The `NewJSONErrorHandler` wrapper already installed in `main.go`'s `registerAPI` reads that status code and emits it (defaulting to 500 for plain errors). So a validation failure returned as `libhttp.WrapWithStatusCode(errors.Errorf(ctx, "..."), http.StatusBadRequest)` becomes an HTTP 400 JSON error automatically.
</context>

<requirements>
1. **Create a new file** `/workspace/pkg/api/write.go` (2026 BSD header; `package api`). Put ALL new write DTOs here (keep `response.go` for the existing read/upsert DTOs). GoDoc every exported type and field, and set the `json` tags to the exact TeamVault field names below.

2. **Create body DTO** (`CreateSecretRequest`) — the body of `POST /api/secrets/` and (reused) `PATCH /api/secrets/{hashid}/`:
   ```go
   type CreateSecretRequest struct {
       ContentType string      `json:"content_type"`
       Name        string      `json:"name"`
       Username    string      `json:"username"`
       URL         string      `json:"url"`
       Description string      `json:"description"`
       SecretData  *SecretData `json:"secret_data"`
   }
   ```
   `SecretData` is a pointer so validation can distinguish "absent" (`nil`) from "present but empty". GoDoc must note `secret_data` is write-only (never echoed in a representation).

3. **`SecretData` DTO** — the polymorphic value block. TeamVault sends different keys per content type; model the superset and validate per type:
   ```go
   type SecretData struct {
       // Password is the secret value for a password secret.
       Password string `json:"password"`
       // OtpKeyData is accepted for TeamVault compatibility but NOT stored.
       OtpKeyData string `json:"otp_key_data"`
       // FileContent is the base64-encoded file payload for a file secret.
       FileContent string `json:"file_content"`
       // Filename is the optional original filename for a file secret.
       Filename string `json:"filename"`
   }
   ```
   GoDoc must state that `OtpKeyData` and `Filename` are accepted but not persisted (spec Desired Behavior 1: `otp_key_data` accepted but not stored; only current value is kept).

4. **Representation DTO** (`SecretRepresentation`) — the TeamVault-shaped body returned by `POST` (201) and `PATCH` (200). GoDoc every field:
   ```go
   type SecretRepresentation struct {
       Hashid      string `json:"hashid"`
       APIURL      string `json:"api_url"`
       ContentType string `json:"content_type"`
       Name        string `json:"name"`
       Username    string `json:"username"`
       URL         string `json:"url"`
       Description string `json:"description"`
   }
   ```
   Do NOT include `secret_data` in the representation (write-only). Do NOT include `otp`, `shares`, `access_policy`, or `status` (spec Non-goals).

5. **Validation function** — add to `/workspace/pkg/api/write.go`:
   ```go
   // Validate checks a CreateSecretRequest against the TeamVault write contract
   // and returns the secret.Secret it maps to. On any contract violation it
   // returns an error already wrapped with HTTP 400 via
   // libhttp.WrapWithStatusCode, so the JSON error handler emits a 400.
   func (r CreateSecretRequest) Validate(ctx context.Context) (secret.Secret, error)
   ```
   Import `"github.com/bborbe/lockbox/pkg/secret"`, `"github.com/bborbe/errors"`, `libhttp "github.com/bborbe/http"`, `"encoding/base64"`, `"net/http"`, `"context"`.
   Validation rules (each failure returns `secret.Secret{}` plus `libhttp.WrapWithStatusCode(errors.Errorf(ctx, "<message>"), http.StatusBadRequest)`):
   - (a) `ContentType == ""` → error "content_type is required".
   - (b) `ContentType` not in `{secret.ContentTypePassword, secret.ContentTypeFile}` (this rejects `"cc"` and any other value) → error "unsupported content_type %q" .
   - (c) `SecretData == nil` → error "secret_data is required".
   - (d) When `ContentType == secret.ContentTypePassword` and `SecretData.Password == ""` → error "secret_data.password is required for a password secret".
   - (e) When `ContentType == secret.ContentTypeFile` and `SecretData.FileContent == ""` → error "secret_data.file_content is required for a file secret".
   - (f) When `ContentType == secret.ContentTypeFile`, `SecretData.FileContent` must decode as valid base64: call `base64.StdEncoding.DecodeString(r.SecretData.FileContent)`; on error return a 400 wrapping it ("secret_data.file_content is not valid base64"). Store the ORIGINAL base64 string in `Secret.File` (the revision-data read echoes the base64 form — see prompt 1 read shape; the file read returns the base64 string), NOT the decoded bytes.
   - On success, return:
     ```go
     secret.Secret{
         Name:        r.Name,
         Description: r.Description,
         Username:    r.Username,
         URL:         r.URL,
         ContentType: r.ContentType,
         Password:    passwordValue, // r.SecretData.Password when content_type==password, else ""
         File:        fileValue,     // r.SecretData.FileContent when content_type==file, else ""
     }
     ```
     For a password secret, `File` is `""`; for a file secret, `Password` is `""`. Never carry `OtpKeyData` or `Filename` into `secret.Secret`.

6. **Add an update-merge helper** for PATCH semantics (prompt 4 uses it) — add to `/workspace/pkg/api/write.go`:
   ```go
   // ApplyUpdate returns a copy of existing with the metadata fields present in
   // r overlaid, and — when r.SecretData is non-nil — the value replaced.
   // content_type is immutable: r.ContentType is ignored. secret_data is
   // validated against existing.ContentType (not r.ContentType). Returns a 400
   // (via libhttp.WrapWithStatusCode) if a present secret_data is malformed for
   // the existing content type.
   func (r CreateSecretRequest) ApplyUpdate(ctx context.Context, existing secret.Secret) (secret.Secret, error)
   ```
   Behavior:
   - Start from a copy of `existing`.
   - Overlay metadata fields from `r` that are present in the body. Because JSON-absent and JSON-empty-string are indistinguishable for plain `string` fields, treat a NON-empty `r.Name`/`r.Description`/`r.Username`/`r.URL` as an update to that field and leave the existing value when the incoming field is empty. (This matches "updates the fields present in the body"; empty-string clears are out of scope for this spec.)
   - Never change `ContentType` (`existing.ContentType` is preserved; `r.ContentType` ignored).
   - If `r.SecretData != nil`, validate it against `existing.ContentType` using the same per-type rules as `Validate` (password → require `Password`; file → require valid base64 `FileContent`), then set `Password`/`File` on the result accordingly (password sets `Password`, clears nothing else; file sets `File`). If `r.SecretData == nil`, leave the value fields unchanged.
   - Return the merged `secret.Secret` and `nil`, or a 400-wrapped error.
   - Factor the shared per-type secret_data validation into an unexported helper (e.g. `validateSecretData(ctx, contentType string, data *SecretData) (password, file string, err error)`) called by both `Validate` and `ApplyUpdate` — do NOT duplicate the base64 logic.

7. **Tests** — `/workspace/pkg/api/write_test.go` (new file, `package api_test`, Ginkgo). Create the suite file `/workspace/pkg/api/api_suite_test.go` if one does not already exist (mirror `pkg/secret/secret_suite_test.go`). This is the boundary test the write path crosses — call `Validate`/`ApplyUpdate` DIRECTLY on representative bodies (do NOT rely only on struct-shape assertions):
   - `Validate` on a valid password body → returns a `secret.Secret` with `ContentType=="password"`, `Password` set, `File==""`, and metadata mapped; `err` is nil.
   - `Validate` on a valid file body (real base64, e.g. `base64.StdEncoding.EncodeToString([]byte("hello"))`) → `File` equals the posted base64, `Password==""`, err nil.
   - `Validate` returns a 400 for each of: missing content_type; content_type `"cc"`; an arbitrary unsupported content_type `"bogus"`; `secret_data` nil; password body missing `password`; file body missing `file_content`; file body with `file_content: "!!!not base64!!!"`. For each, assert the error is non-nil AND that `errors.As(err, &libhttp.ErrorWithStatusCode)` yields `StatusCode() == http.StatusBadRequest`.
   - `ApplyUpdate`: starting from an existing password secret, a body with `{name:"new", url:"https://new"}` and nil `secret_data` returns a secret with updated name+url, same password, same content_type; a body with a new `secret_data.password` replaces the password; a body carrying `content_type:"file"` is IGNORED (result content_type stays `"password"`); a body with a malformed `secret_data` for the existing type returns a 400.
   Assert the 400 status via:
   ```go
   var coded libhttp.ErrorWithStatusCode
   Expect(errors.As(err, &coded)).To(BeTrue())
   Expect(coded.StatusCode()).To(Equal(http.StatusBadRequest))
   ```
   where `errors` is `stderrors "errors"` (stdlib `errors.As`) — the `bborbe/errors` package wraps compatibly with `errors.As`.
</requirements>

<constraints>
- Errors via `github.com/bborbe/errors` for construction (`errors.Errorf(ctx, ...)`) and `libhttp.WrapWithStatusCode` for attaching the 400. Never `fmt.Errorf`; never `context.Background()` inside `pkg/`.
- Every exported type, field, method, and function gets a GoDoc comment.
- No bare `interface{}` — use `any` or concrete types.
- Ginkgo/Gomega tests; ≥80% coverage for the new `pkg/api` write code including every 400 branch.
- 2026 BSD header on every new file.
- Do NOT remove `UpsertRequest`/`UpsertResult` from `response.go` (prompt 5 does that).
- Do NOT implement `cc` as a stored type, and do NOT persist `otp_key_data` or `filename` (spec Non-goals / Desired Behavior 1).
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
grep -n 'CreateSecretRequest\|SecretData\|SecretRepresentation' pkg/api/write.go
grep -n 'func (r CreateSecretRequest) Validate\|func (r CreateSecretRequest) ApplyUpdate' pkg/api/write.go
grep -n 'WrapWithStatusCode' pkg/api/write.go
go test -coverprofile=/tmp/cover.out ./pkg/api/... && go tool cover -func=/tmp/cover.out
```

Each grep must return ≥1 line; coverage of the new `pkg/api` write code must be ≥80%.
</verification>
