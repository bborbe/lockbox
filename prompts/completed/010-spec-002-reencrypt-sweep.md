---
status: completed
spec: [002-keyring-key-rotation]
summary: Added ReEncrypt sweep method to secret.Store, shared keyring.Parse parser, and cmd/reencrypt one-shot command with full integration test coverage
execution_id: lockbox-exec-010-spec-002-reencrypt-sweep
dark-factory-version: dev
created: "2026-07-14T19:50:07Z"
queued: "2026-07-14T20:02:53Z"
started: "2026-07-14T20:17:40Z"
completed: "2026-07-14T20:23:04Z"
branch: dark-factory/keyring-key-rotation
---

<summary>
- Adds a one-shot re-encrypt sweep that rewrites every stored secret under the current primary key.
- After a full sweep, no stored secret references a retired key, so old keys can be safely dropped from the configuration.
- The sweep is safe to re-run: running it twice leaves every secret readable and unchanged in value.
- The sweep is crash-safe: an interrupted run leaves every secret still decryptable under the still-present keys, and re-running finishes the job.
- The sweep runs as a standalone admin command against the data directory — no network endpoint, no new HTTP attack surface.
- The command builds its keyring from the same environment variables as the server, so rotation config is shared through one parser.
- Integration tests prove full conversion to the primary key and idempotent re-runs.
</summary>

<objective>
Add a re-encrypt sweep over the secret store that reads every secret and rewrites it sealed under the current primary key, plus a one-shot `cmd/reencrypt` command (mirroring `cmd/migrate-teamvault`) that opens the same data directory and builds the keyring from the same environment through a single shared parser. The sweep is idempotent and crash-safe so retired keys can be removed once it completes.
</objective>

<context>
Read `/home/node/.claude/CLAUDE.md` and `/workspace/CLAUDE.md`.

Read these coding-plugin guides (in-container paths):
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-cli-guide.md` — one-shot command structure.
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-error-wrapping-guide.md`
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-testing-guide.md` — integration tests over an in-memory kv.
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-context-cancellation-in-loops.md` — the sweep loops over every secret; add the non-blocking context check.

Read these source files fully before implementing:
- `/workspace/pkg/secret/store.go` — the `Store` interface and private `store` struct. Existing methods: `Upsert`, `Create`, `Get`, `Search`, and the private `decrypt`. The store already encrypts-on-`Upsert` and decrypts-on-`Get` via its injected `crypto.Crypter`. You ADD one method to the interface and the struct.
- `/workspace/pkg/secret/store_test.go` — mirror this Ginkgo style for the new integration tests (external `_test` package, `memorykv`, fixed test keys).
- `/workspace/cmd/migrate-teamvault/main.go` — the one-shot command template: `service.MainCmd(context.Background(), &application{})`, struct-tag config, `Run(ctx) error`. Mirror its structure for `cmd/reencrypt`. `service.MainCmd(ctx context.Context, app run.Runnable) int` requires the app to implement `Run(ctx context.Context) error`.
- `/workspace/main.go` — how `createCrypter` (as rewritten in prompt 2) parses the two key env vars and builds the keyring, and how the boltkv DB is opened: `libboltkv.OpenDir(ctx, a.DataDir)` then `defer db.Close()`, then `secret.NewStore(db, crypter)`. The command reuses this exact open/build sequence.
- `/workspace/pkg/keyring/keyring.go` — `keyring.New(ctx, keys...)` from prompt 1.

If `pkg/keyring/keyring.go` or the prompt-2 keyring wiring in `main.go` (`grep -n 'keyring.New' main.go`) is absent, prompts 1-2 have not shipped: STOP and report `Status: failed` with `"prompt 1/2 not yet deployed"`.

The secret `Store.Search(ctx, "")` already returns every key. Build the sweep on the existing `Store` methods (`Search` + `Get` + `Upsert`) so re-encryption reuses the store's own encrypt-on-write path — do not reach past the `crypto.Crypter` seam.
</context>

<requirements>
1. **Add a `ReEncrypt` method to `secret.Store`.** In `/workspace/pkg/secret/store.go`:
   - Add to the `Store` interface:
     ```go
     // ReEncrypt rewrites every stored secret sealed under the current primary
     // key. It reads each secret through the configured crypter and Upserts it
     // back, so any secret still sealed under a non-primary (or pre-keyring) key
     // is re-sealed under the primary. It is idempotent (re-encrypting an
     // already-primary secret is harmless) and crash-safe (each secret is a
     // single-key overwrite; an interrupted run leaves every secret readable
     // under the still-present keys and re-running completes the conversion).
     ReEncrypt(ctx context.Context) error
     ```
   - Implement it on the private `store` struct:
     - Collect every key: `keys, err := s.Search(ctx, "")` (empty query matches every secret); wrap the error.
     - Loop over `keys`. At the top of the loop body add a non-blocking context-cancellation check (per `go-context-cancellation-in-loops.md`): `select { case <-ctx.Done(): return errors.Wrapf(ctx, ctx.Err(), "re-encrypt cancelled") default: }`.
     - `sec, err := s.Get(ctx, key)` — decrypts under whatever key sealed it (framed-by-id or legacy try-each). On error return `errors.Wrapf(ctx, err, "re-encrypt: read secret %s failed", key)` (spec Failure Modes: "the sweep returns a wrapped error naming which key failed").
     - `if err := s.Upsert(ctx, key, *sec); err != nil { return errors.Wrapf(ctx, err, "re-encrypt: rewrite secret %s failed", key) }` — Upsert re-encrypts under the primary key (spec Failure Modes: "a failed write leaves the prior ciphertext intact and readable").
     - Return nil after the loop.
   - A failed read or write returns immediately with the wrapped error; already-converted secrets stay readable, so re-running is safe (spec: idempotent/crash-safe).

2. **Regenerate the `SecretStore` mock.** The `Store` interface gained a method, so `mocks/secret-store.go` must be regenerated via the counterfeiter directive already present above the `Store` interface. Run the project's generate target; do NOT hand-edit the mock.

3. **Extract one shared key parser into `pkg/keyring`.** Add an exported function to the keyring package so both the server and the command build the keyring from the same rule (removing duplication and keeping a single source of truth for the exactly-one rule):
   ```go
   // Parse builds a Keyring from the two mutually-exclusive key configuration
   // sources: single (LOCKBOX_ENCRYPTION_KEY, one base64 key) and list
   // (LOCKBOX_ENCRYPTION_KEYS, comma-separated base64 keys, primary first).
   // Exactly one of single/list must be non-empty; every key must base64-decode
   // to 16 or 32 bytes and be distinct, or Parse returns a wrapped error.
   func Parse(ctx context.Context, single string, list string) (Keyring, error)
   ```
   Move the base64/length/exactly-one validation currently in `main.go`'s `createCrypter` (written in prompt 2) into `Parse`, delegating duplicate/empty-ring detection to `New`. Then rewrite `main.go`'s `createCrypter` body to a one-liner: `return keyring.Parse(ctx, a.EncryptionKey, a.EncryptionKeys)`. The prompt-2 `createCrypter` tests in `main_test.go` must keep passing unchanged (same inputs, same error/non-error outcomes); if any assertion depended on wording, relax it to `Expect(err).NotTo(BeNil())`. Add a focused `Describe("Parse", ...)` unit test in `pkg/keyring/keyring_test.go` covering the AC-8 invalid cases (neither set, both set, empty list, bad base64, wrong length, duplicate keys) and the AC-6/AC-7 valid cases (single key boots; two-key list has the first entry as primary — verify by decrypting a `Parse`-produced ciphertext under a `New(ctx, firstKey)` ring).

4. **Create the one-shot command `cmd/reencrypt/main.go`.** Mirror `cmd/migrate-teamvault/main.go`:
   - Package doc comment describing the sweep as a one-shot re-encrypt of every secret under the current primary key.
   - `func main() { os.Exit(service.MainCmd(context.Background(), &application{})) }`.
   - `type application struct` with struct-tag config fields so it opens the same store from the same environment:
     ```go
     DataDir        string `required:"true"  arg:"datadir"         env:"DATADIR"                usage:"data directory"`
     EncryptionKey  string `required:"false" arg:"encryption-key"  env:"LOCKBOX_ENCRYPTION_KEY" usage:"base64-encoded AES key (16 or 32 raw bytes)" display:"length"`
     EncryptionKeys string `required:"false" arg:"encryption-keys" env:"LOCKBOX_ENCRYPTION_KEYS" usage:"comma-separated base64 AES keys, primary first" display:"length"`
     ```
   - `Run(ctx context.Context) error`:
     - `crypter, err := keyring.Parse(ctx, a.EncryptionKey, a.EncryptionKeys)`; on error `return errors.Wrap(ctx, err, "build keyring failed")`.
     - `db, err := libboltkv.OpenDir(ctx, a.DataDir)`; wrap error; `defer db.Close()`.
     - `store := secret.NewStore(db, crypter)`.
     - `glog.V(0).Infof("re-encrypting all secrets under the current primary key in %s", a.DataDir)`.
     - `if err := store.ReEncrypt(ctx); err != nil { return errors.Wrap(ctx, err, "re-encrypt sweep failed") }`.
     - `glog.V(0).Infof("re-encrypt sweep finished")`; return nil.
   - Import `libboltkv "github.com/bborbe/boltkv"`, `"github.com/bborbe/errors"`, `"github.com/bborbe/service"`, `"github.com/golang/glog"`, `"github.com/bborbe/lockbox/pkg/keyring"`, `"github.com/bborbe/lockbox/pkg/secret"` (mirror the import aliases used in `main.go`/`cmd/migrate-teamvault/main.go`). 2026 BSD copyright header; GoDoc on the package.

5. **Integration tests** — add a `Describe("ReEncrypt", ...)` block in a new `pkg/secret/reencrypt_test.go` (`package secret_test`) covering:
   - **Sweep moves every secret to the primary key (AC 9):** build a store whose crypter is `keyring.New(ctx, old)` (single-key ring = `old`), seed N=3 secrets via `Upsert`. Then build a NEW store over the SAME `db` whose crypter is `keyring.New(ctx, primary, old)` and call `ReEncrypt(ctx)`. Then build a THIRD store over the same `db` whose crypter is `keyring.New(ctx, primary)` ONLY and assert all N secrets `Get` back to their original values under the primary-only ring. (Proves every secret was re-sealed under `primary` and no longer needs `old`.)
   - **Sweep is idempotent / re-runnable (AC 10):** on the ring-`[primary, old]` store, run `ReEncrypt(ctx)` TWICE in a row; assert the second call returns no error, then assert (via a primary-only-ring store over the same db) all secrets still decrypt to their original plaintext unchanged.
   - **Legacy blobs are swept too:** seed a secret through a store whose crypter is the RAW single-key `crypto.NewCrypter(k)` (a pre-keyring un-framed blob), then build a store over the same db with ring `keyring.New(ctx, primary, k)`, `ReEncrypt(ctx)`, and confirm it reads back under a ring `keyring.New(ctx, primary)`.
   - Use `memorykv.OpenMemory` for `db`; import `github.com/bborbe/lockbox/pkg/keyring`. Use distinct fixed 32-byte test keys for `primary`, `old`, `k`.
</requirements>

<constraints>
- The sweep is invoked by a one-shot command under `cmd/` (mirroring `cmd/migrate-teamvault`), NOT a network endpoint — it adds no HTTP surface (spec Security: "No new HTTP surface").
- The sweep must be idempotent (re-encrypting an already-primary secret is harmless) and crash-safe (each write is a single-key overwrite; an interrupted run leaves every secret readable under the still-present keys; re-running completes the conversion) — spec Failure Modes rows for interrupted sweep and kv write failure.
- Re-encryption reuses the store's own encrypt-on-`Upsert` path through the injected `crypto.Crypter`; only ciphertext ever touches the kv store. Do NOT bypass the crypter seam or write plaintext.
- The sweep loop MUST include the non-blocking context-cancellation check (`go-context-cancellation-in-loops.md`).
- The command and the server build the keyring from the SAME environment via the SAME `keyring.Parse` function — one exactly-one rule, one validation path. Do NOT introduce a second, divergent parser.
- Do NOT change the HTTP API, routes, auth, or handlers (AC 11). The only store change is the added `ReEncrypt` method.
- Errors wrapped via `github.com/bborbe/errors` with `ctx` — never `fmt.Errorf`, never `context.Background()` in `pkg/`. The `cmd` `main` may use `context.Background()` at the `service.MainCmd` call (mirroring `cmd/migrate-teamvault`), but `Run` and `pkg/secret`/`pkg/keyring` code use the passed `ctx`.
- Ginkgo/Gomega integration tests in the external `_test` package; regenerate the counterfeiter mock (never hand-edit); GoDoc on every exported symbol; 2026 BSD header on every new file.
- Do NOT commit — dark-factory handles git.
- Every existing test must still pass.
</constraints>

<verification>
Run in `/workspace`:

```
make generate   # regenerates mocks/secret-store.go for the new ReEncrypt method
make test
make precommit
```

`make precommit` must exit 0. Then confirm the new artifacts:

```
grep -n 'ReEncrypt(ctx context.Context) error' pkg/secret/store.go   # interface method present
grep -n 'func Parse(ctx context.Context' pkg/keyring/keyring.go      # shared parser present
grep -n 'keyring.Parse' main.go                                      # server delegates to shared parser
ls cmd/reencrypt/main.go                                             # command exists
grep -n 'store.ReEncrypt' cmd/reencrypt/main.go                      # command invokes the sweep
grep -rn 'Describe("ReEncrypt"' pkg/secret/                          # integration tests present
```

Run the affected packages with coverage on the changed code:

```
go test -coverprofile=/tmp/cover.out ./pkg/secret/... ./pkg/keyring/... && go tool cover -func=/tmp/cover.out | grep -iE 'reencrypt|Parse'
```
</verification>
