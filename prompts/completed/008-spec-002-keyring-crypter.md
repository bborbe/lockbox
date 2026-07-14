---
status: completed
spec: [002-keyring-key-rotation]
execution_id: lockbox-exec-008-spec-002-keyring-crypter
dark-factory-version: dev
created: "2026-07-14T19:50:07Z"
queued: "2026-07-14T20:02:53Z"
started: "2026-07-14T20:02:55Z"
completed: "2026-07-14T20:15:19Z"
branch: dark-factory/keyring-key-rotation
---

<summary>
- Adds a keyring: an ordered set of one or more AES keys, the first designated the primary.
- New secrets are sealed under the primary key and tagged with a small, content-derived key identifier so any future reader knows which key sealed each secret.
- Reading a secret picks the key its tag names; if the tag is missing, unknown, or does not authenticate, every configured key is tried and the one whose authentication tag matches wins.
- Secrets written before the keyring existed (no tag) keep decrypting unchanged, with no data rewrite.
- A wrong or dropped key never yields wrong plaintext or a panic — it always surfaces a clear error only after every configured key has failed.
- The key identifier is derived from the key material itself, so prepending a new primary key never reassigns an existing secret's tag.
- Full Ginkgo unit-test coverage: legacy read, multi-key decrypt-by-id, unknown-id error, and stability under key reordering.
</summary>

<objective>
Add a new `pkg/keyring` package whose `Keyring` type implements the `github.com/bborbe/crypto` `Crypter` interface by composing one single-key `crypto.Crypter` per configured key. It seals under the primary key with a content-derived key-id frame, decrypts by id with an authenticated legacy try-each fallback, and reads pre-keyring (un-framed) ciphertext in place. This is the foundation the config wiring (prompt 2) and re-encrypt sweep (prompt 3) compose.
</objective>

<context>
Read `/home/node/.claude/CLAUDE.md` and `/workspace/CLAUDE.md`.

Read these coding-plugin guides (in-container paths):
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-patterns.md` — public interface + private struct + `New*` constructor, counterfeiter, error wrapping.
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-error-wrapping-guide.md` — `github.com/bborbe/errors` API; never `fmt.Errorf`; never `context.Background()` in `pkg/`; sentinel errors with a `stderrors` alias.
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-testing-guide.md` — Ginkgo v2/Gomega, external `_test` package, coverage ≥80%, error-path tests.
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-security-linting.md` — gosec expectations.

Read these source files fully before implementing:
- `/workspace/pkg/secret/store.go` — the `Store` consumes a `crypto.Crypter` via `NewStore(db, crypter)`; the keyring is injected through this exact seam, so `Keyring` MUST satisfy `crypto.Crypter` and nothing in `store.go` changes.
- `/workspace/pkg/secret/store_test.go` — mirror this Ginkgo style: external `_test` package, `BeforeEach` with `context.Background()`, `memorykv`, fixed test keys.
- `/workspace/pkg/secret/keygen.go` — mirror the package/constructor/GoDoc/copyright-header style for a new `pkg/` file.

The underlying single-key crypter contract (READ, do not modify) is
`/home/node/go/pkg/mod/github.com/bborbe/crypto@v1.0.2/crypter.go`:

```go
type Crypter interface {
	Encrypt(ctx context.Context, value []byte) ([]byte, error)
	Decrypt(ctx context.Context, value []byte) ([]byte, error)
}
func NewCrypter(secretKey SecretKey) Crypter { ... }
```

and `/home/node/go/pkg/mod/github.com/bborbe/crypto@v1.0.2/secret-key.go`:

```go
type SecretKey []byte
func (s SecretKey) Bytes() []byte { return s }
```

CRITICAL — the single-key `Decrypt` (crypter.go line 66) does `nonce, ciphertext := value[:nonceSize], value[nonceSize:]` with NO length guard: a value shorter than the 12-byte GCM nonce panics with a slice-out-of-range. The keyring MUST never hand a too-short slice to a single-key crypter — guard the length before calling `Decrypt`, and treat a too-short candidate as "this key did not authenticate" (fall through), never a panic.
</context>

<requirements>
Create the package `github.com/bborbe/lockbox/pkg/keyring` with the 2026 BSD copyright header on every file (copy the 3-line header from `pkg/secret/keygen.go`). GoDoc on every exported symbol.

1. **Content-derived key identifier.** Add an unexported helper `keyID(key crypto.SecretKey) []byte` that returns a short, deterministic fingerprint of the key BYTES (not the key's list position): compute `sha256.Sum256(key.Bytes())` and return the first `keyIDLen` bytes as a fresh `[]byte`. Declare `const keyIDLen = 4`. Deriving the id from key material is load-bearing — prepending a new primary must never reassign an existing key's id. The id is non-secret and must not leak the key: a truncated one-way SHA-256 satisfies this; do NOT store or expose key bytes in the frame.

2. **Frame format + parser.** Define the at-rest frame as a fixed prefix followed by the raw `bborbe/crypto` AES-GCM blob:
   - `const frameVersion byte = 0x01`
   - `var frameMagic = []byte{0x4C, 0x4B, 0x42, 0x31}` — the ASCII bytes `LKB1` (a self-describing non-secret marker). GoDoc-comment it.
   - Frame layout: `frameMagic (4 bytes) || frameVersion (1 byte) || keyID (keyIDLen bytes) || cryptoBlob (rest)`.
   - Declare `const frameHeaderLen = len(frameMagic) + 1 + keyIDLen` computed at init time (use a `var frameHeaderLen = len(frameMagic) + 1 + keyIDLen` since `len` on a var is not a constant expression).
   - Add an unexported `buildFrame(id []byte, blob []byte) []byte` that concatenates `frameMagic`, `frameVersion`, `id`, and `blob`.
   - Add an unexported `parseFrame(value []byte) (id []byte, blob []byte, ok bool)`: returns `ok == false` (never panics, never reads out of bounds) when `len(value) < frameHeaderLen`, or the leading `len(frameMagic)` bytes do not equal `frameMagic`, or the version byte is not `frameVersion`. On success returns the `keyIDLen`-byte id slice and the remaining blob. Frame parsing rejects truncated input rather than reading out of bounds (spec Security: "Frame parsing rejects truncated input").

3. **Keyring type.** Public interface + private struct + `New` constructor:
   ```go
   // Keyring is an ordered set of AES keys satisfying crypto.Crypter. The first
   // key is the primary: Encrypt always seals under it. Decrypt resolves any
   // secret sealed under any configured key (or a pre-keyring un-framed blob).
   type Keyring interface {
       crypto.Crypter
   }
   ```
   - Constructor signature: `func New(ctx context.Context, keys ...crypto.SecretKey) (Keyring, error)` (ctx is the FIRST parameter so errors can wrap via `github.com/bborbe/errors`).
   - Validation, all returning a wrapped error and a nil Keyring:
     - `len(keys) == 0` → error "keyring requires at least one key".
     - any `key.Bytes()` length not in {16, 32} → error naming the offending index and the observed length.
     - any two keys equal by bytes, OR whose `keyID` collide (compare with `bytes.Equal`) → error "duplicate key in keyring".
   - On success build, in list order, a private slice `entries []entry` where `type entry struct { id []byte; crypter crypto.Crypter }`, `crypter = crypto.NewCrypter(key)`, `id = keyID(key)`. Index 0 is the primary. Store `entries` on the private `keyring` struct and return it.

4. **Encrypt under primary + frame.** `Encrypt(ctx, value)`:
   - `blob, err := k.entries[0].crypter.Encrypt(ctx, value)`; on error return `nil, errors.Wrap(ctx, err, "keyring encrypt failed")`.
   - return `buildFrame(k.entries[0].id, blob), nil`.

5. **Decrypt by ordered precedence, authenticated at every step.** `Decrypt(ctx, value)` follows ONE fixed path and returns an error ONLY after every configured key has failed. Declare `const gcmMinLen = 12` (the GCM nonce size; a candidate shorter than this can never be a valid crypto blob). Implement exactly these steps in order:
   1. `id, blob, ok := parseFrame(value)`. If `ok`, find the entry whose `bytes.Equal(entry.id, id)`; if such an entry exists, attempt `plaintext, derr := entry.crypter.Decrypt(ctx, blob)` (guard `len(blob) >= gcmMinLen` first); if `derr == nil`, return `plaintext, nil`.
   2. If `ok`, iterate every entry in order over `blob`: guard `len(blob) >= gcmMinLen`, attempt `entry.crypter.Decrypt(ctx, blob)`; the first entry with no error returns its plaintext. (This handles a framed blob whose id byte was corrupted but whose key is still present.)
   3. Iterate every entry in order over the ORIGINAL `value` (this is the legacy un-framed path — a pre-keyring blob has no frame): guard `len(value) >= gcmMinLen`, attempt `entry.crypter.Decrypt(ctx, value)`; the first entry with no error returns its plaintext.
   4. If no step returned, return `nil, errors.Errorf(ctx, "keyring decrypt failed: no configured key authenticates the ciphertext")`. No plaintext, no panic, no silent wrong value.
   - Discrimination is ALWAYS by AES-GCM authentication (Decrypt returning no error), NEVER by the frame marker alone. A too-short candidate is a skipped attempt, never a panic.

6. **Counterfeiter mock.** Add the generate directive above the `Keyring` interface, exactly like `pkg/secret/store.go` does for `Store`:
   ```go
   //counterfeiter:generate -o ../../mocks/keyring.go --fake-name Keyring . Keyring
   ```
   Then run the project's generate target so `mocks/keyring.go` is produced. Inspect `mocks/mocks.go` and `pkg/secret/store.go` to see how existing mocks are wired and mirror that wiring; do NOT hand-write the mock.

7. **Tests** — create `pkg/keyring/keyring_test.go` (external `package keyring_test`) and a Ginkgo suite file `pkg/keyring/keyring_suite_test.go` (mirror `pkg/secret/secret_suite_test.go`). Use fixed 32-byte and 16-byte test keys built like `store_test.go`'s `testEncryptionKey` (distinct byte patterns per key). Cover every acceptance criterion:
   a. **Legacy ciphertext decrypts unchanged (AC 1):** seal bytes with `crypto.NewCrypter(k)` directly (raw AES-GCM, no frame), build a keyring via `keyring.New(ctx, k)`, Decrypt, assert byte-equality with the original plaintext.
   b. **Primary-key encrypt is identifiable (AC 2):** build `keyring.New(ctx, primary, old)`, Encrypt a plaintext; assert (i) round-trip equality through the same ring; (ii) a ring of ONLY `primary` decrypts it; (iii) a ring of ONLY `old` returns a non-nil error and nil bytes.
   c. **Decrypt-by-id across a multi-key ring (AC 3):** seal plaintext A through `keyring.New(ctx, old)` and plaintext B through `keyring.New(ctx, primary)`; decrypt BOTH ciphertexts through `keyring.New(ctx, primary, old)`; assert both match their originals.
   d. **Unknown key-id is a clear error, not panic/wrong-plaintext (AC 4):** seal through `keyring.New(ctx, A)`, then Decrypt through `keyring.New(ctx, B)`; assert returned error is non-nil AND returned bytes are nil. Add an explicit truncated-frame case: build bytes shaped like a frame prefix but cut to 6 bytes (shorter than `frameHeaderLen`) and assert Decrypt returns a non-nil error and does NOT panic.
   e. **Key-id stable under reordering (AC 5):** Encrypt a plaintext through `keyring.New(ctx, A)`, capturing the ciphertext; Decrypt the SAME ciphertext through `keyring.New(ctx, B, A)`; assert it still decrypts to the original — proving the id is derived from key material, not list position.
   f. **Constructor validation:** `keyring.New(ctx)` with no keys → non-nil error; a 15-byte key → error; two identical keys → error.
   Every error-path assertion must confirm the returned plaintext/keyring is nil. Do not assert on wrapped-error message text beyond `NotTo(BeNil())` unless a specific substring is load-bearing.
</requirements>

<constraints>
- `Keyring` MUST satisfy `github.com/bborbe/crypto` `Crypter` (`Encrypt`/`Decrypt(ctx, []byte) ([]byte, error)`) so `secret.NewStore` keeps taking a `crypto.Crypter` unchanged. Do NOT modify `pkg/secret/store.go` or the `crypto` library.
- The underlying primitive stays `bborbe/crypto` AES-GCM; the keyring COMPOSES single-key `crypto.NewCrypter` instances — do NOT reimplement AES-GCM.
- Discrimination between candidate keys is ALWAYS by AES-GCM authentication (Decrypt returning no error), never by the frame marker alone. A wrong key must never yield plaintext (spec Security: "No wrong-plaintext on mismatch").
- Never panic on malformed/truncated input: guard every slice against length before indexing; a too-short candidate is a failed try-each attempt, not a crash.
- The key-id in the frame is a non-secret fingerprint (truncated SHA-256); never place key bytes in the frame.
- Errors wrapped via `github.com/bborbe/errors` (`errors.New`/`errors.Wrap`/`errors.Wrapf`/`errors.Errorf` with `ctx`) — never `fmt.Errorf`, never `context.Background()` inside `pkg/`.
- Ginkgo/Gomega tests in an external `_test` package; counterfeiter-generated mock (never hand-written); GoDoc on every exported symbol; 2026 BSD copyright header on every new file.
- Do NOT commit — dark-factory handles git.
- Every existing test must still pass.
</constraints>

<verification>
Run in `/workspace`:

```
make generate   # regenerates mocks/keyring.go from the counterfeiter directive
make test
make precommit
```

`make precommit` must exit 0 (runs format, generate, test, check, addlicense). Then confirm the new artifacts exist:

```
grep -n 'func New(ctx context.Context, keys ...crypto.SecretKey)' pkg/keyring/keyring.go   # constructor present
grep -n 'sha256' pkg/keyring/keyring.go                                                    # content-derived id
grep -rn 'Describe' pkg/keyring/keyring_test.go                                            # tests present
ls mocks/keyring.go                                                                        # counterfeiter mock generated
```

Run the keyring package tests in isolation and confirm coverage ≥80%:

```
go test -coverprofile=/tmp/cover.out ./pkg/keyring/... && go tool cover -func=/tmp/cover.out | tail -1
```
</verification>
