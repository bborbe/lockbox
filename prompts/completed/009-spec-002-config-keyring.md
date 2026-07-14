---
status: completed
spec: [002-keyring-key-rotation]
summary: Extended createCrypter to build a keyring from LOCKBOX_ENCRYPTION_KEYS (comma-separated base64, primary first) or the legacy LOCKBOX_ENCRYPTION_KEY, enforcing exactly-one-source and refusing invalid key material; added comprehensive table-driven tests for all valid/invalid cases.
execution_id: lockbox-exec-009-spec-002-config-keyring
dark-factory-version: dev
created: "2026-07-14T19:50:07Z"
queued: "2026-07-14T20:02:53Z"
started: "2026-07-14T20:15:20Z"
completed: "2026-07-14T20:17:39Z"
branch: dark-factory/keyring-key-rotation
---

<summary>
- Lets an operator configure multiple encryption keys as an ordered, comma-separated list, with the first key treated as the current (primary) key.
- Keeps the existing single-key setting working unchanged, so existing deployments and the example config boot with no changes.
- Requires exactly one of the two key settings to be present: neither-set and both-set are startup errors.
- Refuses to start on any invalid key material — bad base64, wrong length, empty list, or duplicate keys — so the server never runs on unusable keys.
- Wires the resulting keyring into the secret store through the existing crypter seam, so no handler, route, or store logic changes.
- Adds a table-driven config test proving each valid and each invalid startup case, plus a round-trip through a store built from a two-key list.
</summary>

<objective>
Extend the server's startup key configuration in `main.go` so `createCrypter` builds a `pkg/keyring` keyring from either the new `LOCKBOX_ENCRYPTION_KEYS` (comma-separated base64, primary first) or the existing single `LOCKBOX_ENCRYPTION_KEY`, enforcing exactly-one-source and refusing to start on any invalid key material. The single-key path continues to boot an equivalent one-entry keyring.
</objective>

<context>
Read `/home/node/.claude/CLAUDE.md` and `/workspace/CLAUDE.md`.

Read these coding-plugin guides (in-container paths):
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-error-wrapping-guide.md`
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-testing-guide.md`

Read these source files fully before implementing:
- `/workspace/main.go` — the `application` struct (note `EncryptionKey string ... required:"true" ... env:"LOCKBOX_ENCRYPTION_KEY"` at the struct-tag block) and the current `createCrypter(ctx) (crypto.Crypter, error)` that base64-decodes ONE key, checks 16/32 length, and returns `crypto.NewCrypter(...)`. You REPLACE the body of `createCrypter` and ADD a new struct field; you do NOT change any route wiring or `createHTTPServer`.
- `/workspace/main_test.go` — the existing `Describe("createCrypter", ...)` block (currently four `It`s over `EncryptionKey`) that you EXTEND, and `newContractRouter` / the contract `Describe` blocks that must stay green.
- `/workspace/pkg/keyring/keyring.go` — the keyring built in prompt 1. Its constructor is `func New(ctx context.Context, keys ...crypto.SecretKey) (keyring.Keyring, error)` and `keyring.Keyring` embeds `crypto.Crypter`. If this file does not exist, prompt 1 has not shipped: STOP and report `Status: failed` with `"prompt 1 (pkg/keyring) not yet deployed"`.
- `/workspace/example.env` — the single `LOCKBOX_ENCRYPTION_KEY` export (documented in prompt 4, not here).

Verify the keyring constructor signature before wiring it:

```
grep -n 'func New(' pkg/keyring/keyring.go
```

It must accept `(ctx context.Context, keys ...crypto.SecretKey)`. If the shipped signature differs, adapt the call site to the real signature (do not invent one).
</context>

<requirements>
1. **Add the multi-key config field** to the `application` struct in `/workspace/main.go`, immediately after the existing `EncryptionKey` field:
   ```go
   EncryptionKeys string `required:"false" arg:"encryption-keys" env:"LOCKBOX_ENCRYPTION_KEYS" usage:"comma-separated base64 AES keys (16 or 32 raw bytes each), primary first; mutually exclusive with LOCKBOX_ENCRYPTION_KEY" display:"length"`
   ```
   Change the EXISTING `EncryptionKey` field's tag from `required:"true"` to `required:"false"` (exactly one of the two is now required; the exactly-one rule is enforced in `createCrypter`, not by the struct tag). Keep its `arg`/`env`/`usage`/`display` unchanged.

2. **Rewrite `createCrypter`** in `/workspace/main.go` to build a keyring. Keep the signature `func (a *application) createCrypter(ctx context.Context) (crypto.Crypter, error)` (the return type stays `crypto.Crypter`; `keyring.Keyring` satisfies it). Import `github.com/bborbe/lockbox/pkg/keyring`. Logic:
   - Let `single := strings.TrimSpace(a.EncryptionKey)` and `list := strings.TrimSpace(a.EncryptionKeys)`.
   - **Exactly-one rule:** if `single == "" && list == ""` → return `nil, errors.New(ctx, "either LOCKBOX_ENCRYPTION_KEY or LOCKBOX_ENCRYPTION_KEYS must be set")`. If `single != "" && list != ""` → return `nil, errors.New(ctx, "LOCKBOX_ENCRYPTION_KEY and LOCKBOX_ENCRYPTION_KEYS are mutually exclusive; set exactly one")`.
   - Collect the raw base64 entries into `[]string`: if `list != ""`, split `a.EncryptionKeys` on `","` and `strings.TrimSpace` each part; if `single != ""`, use `[]string{a.EncryptionKey}`.
   - Reject an empty resulting list or any empty/whitespace entry (covers an empty `LOCKBOX_ENCRYPTION_KEYS` like `",,"` or a trailing comma) → wrapped error.
   - For each entry: `raw, err := base64.StdEncoding.DecodeString(entry)`; on error return a wrapped error naming the entry index. Then require `len(raw)` in {16, 32}, else a wrapped error naming the index and observed length. Append `crypto.SecretKey(raw)` to a `keys []crypto.SecretKey` slice (order preserved, primary first).
   - Build and return `keyring.New(ctx, keys...)`. Its constructor already rejects duplicates and empty input; propagate its error wrapped via `errors.Wrap(ctx, err, "build keyring failed")`. Do NOT re-implement duplicate detection here — the keyring owns it.
   - Update the GoDoc comment above `createCrypter` to describe the two sources, the exactly-one rule, and the refuse-start-on-invalid behavior. Keep it factual, no version claims.

3. **Extend the `createCrypter` tests** in `/workspace/main_test.go` inside the existing `Describe("createCrypter", ...)` block. Keep the four existing `It`s (legacy single-key valid 32-byte, valid 16-byte, invalid length, non-base64) passing unchanged — they exercise the back-compat single-key path (AC 6). Add a table-driven set (`DescribeTable`/`Entry`, or additional `It`s) covering:
   - **Back-compat single-key still boots (AC 6):** an `application{EncryptionKey: base64(validKey)}` returns a non-nil crypter and no error; additionally build a `secret.NewStore(memorykv, crypter)` from it and round-trip a secret (Upsert then Get equal) to prove the resulting keyring is usable end-to-end.
   - **Ordered multi-key config parses, primary first (AC 7):** `application{EncryptionKeys: base64(k1)+","+base64(k2)}` (two distinct valid keys) returns a non-nil crypter; encrypt a value through it, then assert it decrypts under a keyring built from ONLY `k1` (the primary/first entry). Use `keyring.New(ctx, crypto.SecretKey(k1))` to build the primary-only ring and confirm it decrypts the value the config crypter produced.
   - **Invalid or absent key material refuses start (AC 8):** a `DescribeTable` asserting a NON-NIL error (and never a usable crypter) for each of: (a) neither env var set (`application{}`); (b) BOTH set (`EncryptionKey` and `EncryptionKeys` both non-empty); (c) empty `EncryptionKeys` (`","` or `""` after trim with only whitespace); (d) a list entry that is not valid base64 (`"!!!"`); (e) a list entry that decodes to a length other than 16 or 32 (base64 of `"short"`); (f) two identical keys in the list (`base64(k1)+","+base64(k1)`).
   Build the test keys as fixed byte patterns like `store_test.go`'s `testEncryptionKey`; import `github.com/bborbe/lockbox/pkg/keyring` and `github.com/bborbe/lockbox/pkg/secret` in the test as needed. `main_test.go` is `package main`, so `createCrypter` is directly callable.
</requirements>

<constraints>
- The single-key `LOCKBOX_ENCRYPTION_KEY` path MUST remain valid and boot an equivalent one-entry keyring — existing deployments and `example.env` keep working without config changes. Do NOT break the four pre-existing `createCrypter` tests.
- Exactly one of `LOCKBOX_ENCRYPTION_KEY` / `LOCKBOX_ENCRYPTION_KEYS` must be non-empty; neither-set and both-set are startup errors (spec Failure Modes: "Startup with invalid key material ⇒ refuses to start").
- Every key must base64-decode to 16 or 32 bytes and be distinct, or startup fails — the server never runs on invalid or empty key material. Duplicate detection is delegated to `keyring.New` (do not duplicate that logic).
- Do NOT change the HTTP API, routes, auth, `createHTTPServer`, `registerAPI`, or any handler — only the key configuration and `createCrypter` change (spec Constraints; AC 11).
- `createCrypter` still returns `crypto.Crypter` so `createHTTPServer(... crypter ...)` and `secret.NewStore(db, crypter)` are unchanged.
- Errors wrapped via `github.com/bborbe/errors` (`errors.New`/`errors.Wrap`/`errors.Wrapf`/`errors.Errorf` with `ctx`) — never `fmt.Errorf`, never `context.Background()` in business logic. `main.go`'s `createCrypter` already receives a `ctx`; use it.
- Ginkgo/Gomega tests; keep the 2026 BSD headers; no bare `interface{}` (use `any`).
- Do NOT commit — dark-factory handles git.
- Every existing test must still pass (especially the contract suite in `main_test.go`, which must remain byte-identical in its read/write assertion blocks — AC 11).
</constraints>

<verification>
Run in `/workspace`:

```
make test
make precommit
```

`make precommit` must exit 0. Then confirm the config surface:

```
grep -n 'LOCKBOX_ENCRYPTION_KEYS' main.go            # new env wired
grep -n 'required:"false"' main.go | grep -i encryption   # both key fields optional at struct level
grep -n 'keyring.New' main.go                        # keyring built at startup
grep -n 'mutually exclusive' main.go                 # both-set rejected
```

Confirm the config tests exercise every AC-8 invalid case and the AC-6/AC-7 valid cases:

```
grep -c 'createCrypter' main_test.go   # existing four + new cases
```

Run the config test package with coverage on the changed code:

```
go test -coverprofile=/tmp/cover.out . && go tool cover -func=/tmp/cover.out | grep createCrypter
```
</verification>
