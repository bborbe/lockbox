---
status: completed
spec: [001-write-api-teamvault-compat]
summary: Expanded Secret domain record with Name/Description/ContentType fields, added KeyGenerator with crypto/rand base62 implementation, and added check-and-set Create method to Store
execution_id: lockbox-exec-001-spec-001-domain-record-and-keygen
dark-factory-version: dev
created: "2026-07-14T16:00:00Z"
queued: "2026-07-14T14:10:42Z"
started: "2026-07-14T14:10:44Z"
completed: "2026-07-14T14:15:56Z"
branch: dark-factory/write-api-teamvault-compat
---

<summary>
- Expands the stored secret record so it can round-trip the TeamVault create/update metadata (name, description, content type) alongside the existing username/url/password/file values.
- Adds a server-side key generator that produces short, URL-safe, alphanumeric secret keys (the TeamVault "hashid"), so callers never choose the key.
- Adds a create operation to the secret store that only writes when the key does not already exist (check-and-set), so a create can never overwrite another secret.
- Leaves every existing read shape and existing store behavior untouched — the metadata read still returns username/url/current_revision and the revision-data read still returns password/file.
- Foundation prompt: later prompts (create handler, update handler) depend on these building blocks but nothing here changes any HTTP route yet.
</summary>

<objective>
Expand the `pkg/secret` domain record with the fields TeamVault create/update needs to echo (`Name`, `Description`, `ContentType`), add a server-side unique key generator, and add a check-and-set `Create` operation to the store — without changing any existing read behavior or route.
</objective>

<context>
Read `/home/node/.claude/CLAUDE.md` and `/workspace/CLAUDE.md` for project conventions.

Read these coding-plugin guides before implementing:
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-patterns.md` (interface + private struct + `New*` constructor, counterfeiter, error wrapping)
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-testing-guide.md` (Ginkgo/Gomega, counterfeiter mocks, coverage ≥80%)
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-error-wrapping-guide.md` (`github.com/bborbe/errors`, never `fmt.Errorf`, never `context.Background()` in pkg)
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-security-linting.md` (gosec, `#nosec` with reasons)

Read these source files fully before editing:
- `/workspace/pkg/secret/secret.go` — the domain `Secret` struct and `Key` type you will extend.
- `/workspace/pkg/secret/store.go` — the `Store` interface and `store` impl. Note it uses `libkv.NewStore[Key, []byte]` whose typed methods are `Add` (upsert — overwrites), `Get`, `Exists(ctx, key) (bool, error)`, `Remove`, `Map`. The `Add` method OVERWRITES an existing key; there is no atomic put-if-absent, so `Create` must `Exists`-then-`Add`.
- `/workspace/pkg/secret/store_test.go` — existing Ginkgo test style (memorykv backend, `crypto.NewCrypter`).
- `/workspace/pkg/secret/secret_suite_test.go` — suite entry point.

The libkv typed store interface (verified at `github.com/bborbe/kv@v1.19.6/kv_store.go`) exposes:
```go
Add(ctx context.Context, key KEY, object OBJECT) error     // overwrites if present
Get(ctx context.Context, key KEY) (*OBJECT, error)
Exists(ctx context.Context, key KEY) (bool, error)
```
</context>

<requirements>
1. **Expand the domain record** in `/workspace/pkg/secret/secret.go`. Add three exported string fields to the `Secret` struct, each with a GoDoc comment:
   - `Name string` — the human-readable secret name (TeamVault `name`).
   - `Description string` — free-text description; may be empty.
   - `ContentType string` — the TeamVault content type; one of `"password"` or `"file"`.
   Keep the existing `Username`, `URL`, `Password`, `File` fields unchanged and in place. Do NOT add JSON tags (this struct is JSON-marshalled by the store only to be encrypted; field names are internal).

2. **Add content-type constants** in `/workspace/pkg/secret/secret.go` (exported, GoDoc each):
   - `ContentTypePassword = "password"`
   - `ContentTypeFile = "file"`
   Do NOT add a `cc` constant here — `cc` is explicitly a non-goal for the write path.

3. **Add a key generator** in a new file `/workspace/pkg/secret/keygen.go` (2026 BSD header). Define:
   ```go
   //counterfeiter:generate -o ../../mocks/key-generator.go --fake-name KeyGenerator . KeyGenerator

   // KeyGenerator produces fresh, URL-safe, short alphanumeric secret keys.
   type KeyGenerator interface {
       // Generate returns a new random Key. The returned key is a non-empty,
       // URL-safe alphanumeric string; it makes no uniqueness guarantee against
       // the store (the store's Create enforces uniqueness via check-and-set).
       Generate(ctx context.Context) (Key, error)
   }

   // NewKeyGenerator returns a KeyGenerator producing keys of length keyLength
   // from the base62 alphabet [A-Za-z0-9], drawing randomness from crypto/rand.
   func NewKeyGenerator(keyLength int) KeyGenerator
   ```
   Implementation notes:
   - Use `crypto/rand` (`crypto/rand`.`Read` into a byte slice, or `rand.Int`) to select characters from the alphabet `const alphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789"`.
   - `keyLength` must be a fixed compile-time-chosen value; do NOT expose it as configuration (spec Non-goal: key length is NOT configurable). Callers pass a constant. Define `const DefaultKeyLength = 8` in `keygen.go` and have the create handler (a later prompt) use it via `NewKeyGenerator(secret.DefaultKeyLength)`.
   - Every generated character must come from `alphabet`; the result is always URL-safe (no `+`, `/`, `=`, or padding). To avoid modulo bias, either read one random byte per character and reject bytes `>= 256 - (256 % len(alphabet))`, OR use `crypto/rand.Int(rand.Reader, big.NewInt(int64(len(alphabet))))` per character. Wrap any `crypto/rand` error with `errors.Wrapf(ctx, err, "read random bytes failed")`.
   - On a randomness read failure, return `("", error)` — the caller (create handler) will surface a 500.

4. **Add a check-and-set `Create` method** to the `Store` interface and `store` impl in `/workspace/pkg/secret/store.go`:
   - Add to the `Store` interface (with GoDoc):
     ```go
     // Create stores secret under key only if key does not already exist.
     // If key is already present it returns ErrKeyExists and does not modify
     // the stored secret.
     Create(ctx context.Context, key Key, secret Secret) error
     ```
   - Define an exported sentinel error in `store.go` using the stderrors alias pattern from `go-error-wrapping-guide.md`:
     ```go
     import stderrors "errors"
     // ErrKeyExists is returned by Create when the target key is already present.
     var ErrKeyExists = stderrors.New("key already exists")
     ```
   - Implement `Create`: call `s.kv.Exists(ctx, key)`; if it reports the key exists, return `errors.Wrapf(ctx, ErrKeyExists, "create secret %s failed", key)`; otherwise JSON-marshal + encrypt (reuse the exact same marshal/encrypt sequence as `Upsert`, including the existing `#nosec G117` comment) and call `s.kv.Add(ctx, key, encrypted)`, wrapping errors with `errors.Wrapf(ctx, err, "create secret %s failed", key)`.
   - Add a GoDoc note on `Create` that this is check-and-set at the store level and that collision at Lockbox scale is astronomically unlikely; a caller regenerating on `ErrKeyExists` (later prompt) closes the residual gap.
   - Keep `Upsert`, `Get`, `Search` exactly as they are — do NOT delete `Upsert` in this prompt (prompt 5 removes the flat-PUT handler; the store `Upsert` method may remain until then and is fine to keep).

5. **Regenerate mocks.** The store mock at `/workspace/mocks/secret-store.go` must gain `Create`, and a new `/workspace/mocks/key-generator.go` must be generated for `KeyGenerator`. Run the project's generate target (`make generate`, invoked by `make precommit`) so counterfeiter regenerates them. Do NOT hand-write mocks.

6. **Tests** (Ginkgo, external `package secret_test`, following `store_test.go` style):
   - In `/workspace/pkg/secret/store_test.go`, add a `Describe("Create", ...)` block asserting:
     - `Create` stores a new key and `Get` returns the exact `Secret` (including the new `Name`, `Description`, `ContentType` fields).
     - `Create` on an already-existing key returns an error that `errors.Is(err, secret.ErrKeyExists)` is true, and the stored value is unchanged (the first value, not the second).
   - Add `/workspace/pkg/secret/keygen_test.go` (new file, `package secret_test`) asserting:
     - `Generate` returns a non-empty key whose length equals `secret.DefaultKeyLength` and whose every character is in `[A-Za-z0-9]` (assert via a regexp `^[A-Za-z0-9]+$` match).
     - Two successive `Generate` calls return different keys (probabilistic uniqueness; with length 8 base62 a collision is negligible).
   - Import `stderrors "errors"` or use `github.com/bborbe/errors`'s `Is` — match whatever the repo already uses; `errors.Is` from stdlib works on the sentinel.
</requirements>

<constraints>
- Errors via `github.com/bborbe/errors` (`errors.Wrapf(ctx, err, ...)`, `errors.Errorf(ctx, ...)`); never `fmt.Errorf`; never `context.Background()` inside `pkg/`. Use the `stderrors "errors"` alias only for `stderrors.New` sentinels and (optionally) `errors.Is`.
- GoDoc on every exported symbol (`Create`, `ErrKeyExists`, `KeyGenerator`, `NewKeyGenerator`, `Generate`, `DefaultKeyLength`, `ContentTypePassword`, `ContentTypeFile`, and each new `Secret` field).
- No bare `interface{}` — use `any` or concrete types. (counterfeiter-generated files are exempt.)
- Ginkgo/Gomega tests; counterfeiter-generated mocks only (never hand-written).
- 2026 BSD copyright header on every new file (copy the exact 3-line header from `pkg/secret/secret.go`).
- Do NOT expose key length, charset, or any generation parameter as runtime configuration (spec Non-goal).
- Do NOT change any READ behavior, route, or the `SecretMetadata`/`RevisionData` response shapes.
- Do NOT commit — dark-factory handles git.
- Existing tests must still pass.
</constraints>

<verification>
Run in `/workspace`:

```
make test
make precommit
```

`make precommit` must exit 0 (runs format, generate, test, check, addlicense). Then confirm the new surface exists:

```
grep -n 'func NewKeyGenerator\|type KeyGenerator\|DefaultKeyLength' pkg/secret/keygen.go
grep -n 'Create(ctx context.Context' pkg/secret/store.go
grep -n 'ErrKeyExists' pkg/secret/store.go
grep -n 'Name string\|Description string\|ContentType string' pkg/secret/secret.go
grep -rn 'func (fake \*KeyGenerator) Generate\|func (fake \*SecretStore) Create' mocks/
```

Each grep must return at least one line. Confirm coverage of the `secret` package is ≥80% for the new code:

```
go test -coverprofile=/tmp/cover.out ./pkg/secret/... && go tool cover -func=/tmp/cover.out
```
</verification>
