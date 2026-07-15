---
status: completed
approved: "2026-07-14T19:48:12Z"
generating: "2026-07-14T19:48:29Z"
prompted: "2026-07-14T19:58:42Z"
verifying: "2026-07-15T21:13:24Z"
completed: "2026-07-15T21:39:46Z"
branch: dark-factory/keyring-key-rotation
---

## Summary

- Lockbox encrypts every secret at rest under a **single** AES-GCM key with no key identifier on the ciphertext, so swapping `LOCKBOX_ENCRYPTION_KEY` once the store holds data makes all secrets undecryptable — key rotation is currently destructive.
- Introduce a **keyring**: an ordered set of keys (primary first). New writes encrypt under the primary key and tag the ciphertext with a key identifier; reads select the decrypting key by that identifier.
- Existing single-key deployments keep working unchanged, and existing ciphertext (written before the keyring) stays readable with **no data rewrite required** to adopt the keyring.
- Rotation becomes a restart: prepend a new primary key, restart; new secrets use the new key while every old secret still decrypts under its still-present key.
- A re-encrypt sweep rewrites every stored secret under the current primary key so retired keys can be dropped afterward; the sweep is crash-safe and re-runnable.

## Problem

Lockbox is meant to hold a personal TeamVault-migrated password store, but its encryption-at-rest is bound to one master key with no way to rotate it. Every secret is sealed under the single `LOCKBOX_ENCRYPTION_KEY`, and the ciphertext carries no marker identifying which key sealed it, so changing the key after the store has data renders every secret permanently unreadable. This blocks the migration outright: filling the store under a key that can never be rotated is an unacceptable long-term posture for a secret manager. TeamVault itself solves this with a Fernet → MultiFernet keyring; Lockbox needs the equivalent so keys can rotate with no downtime and no data loss.

## Goal

Lockbox encrypts and decrypts stored secrets through a keyring of one or more AES-GCM keys. New secrets are sealed under the **primary** key and carry a key identifier; any secret is read back by selecting the key its identifier names, or — for ciphertext written before the keyring existed — by the previously-valid key, without rewriting the store. An operator rotates the master key by prepending a new primary key to the configuration and restarting: new writes adopt the new key while all previously stored secrets remain readable under their original, still-present keys. A one-shot re-encrypt sweep rewrites the entire store under the current primary key so that retired keys can be safely removed from the configuration. The single-key `LOCKBOX_ENCRYPTION_KEY` configuration continues to work as a one-entry keyring, and the server still refuses to start on invalid or absent key material. The HTTP API and its read/write handler contracts are unchanged.

## Non-goals

- Do NOT implement envelope encryption (a per-secret data key wrapped by a key-encryption key). At-rest secrets stay sealed directly under a keyring key. If per-secret DEKs are ever required, that is a separate spec.
- Do NOT integrate an external KMS or HSM. Keys stay in configuration/environment as today. A future KMS-backed keyring is a separate spec.
- Do NOT change the HTTP API shape, routes, auth, or the read/write handlers' request/response contracts. Only the at-rest encoding, the key configuration, and a new re-encrypt command change.
- Do NOT add secret revision history or any store semantics beyond re-encrypting existing values in place.
- Do NOT make the key-identifier length, the ciphertext framing, or the "invalid key material ⇒ refuse start" behavior configurable — they are correctness invariants; a future variation is a separate spec.
- Do NOT require a data rewrite to adopt the keyring — legacy ciphertext must remain readable in place.

## Acceptance Criteria

Each criterion is binary and declares the artifact the verifier observes. Encryption behavior is asserted by Ginkgo/Gomega unit tests over the new keyring crypter and the store (using an in-memory kv), unless noted.

- [ ] **Legacy ciphertext decrypts unchanged.** A ciphertext produced by the pre-keyring single-key crypter (raw `bborbe/crypto` AES-GCM blob, no key-id) decrypts to the original plaintext when that same key is present in the keyring — evidence: a unit test seals bytes with `crypto.NewCrypter(k)`, then decrypts them through the keyring built from `[k]` and asserts byte-equality with the original.
- [ ] **Primary-key encrypt is identifiable.** Encrypting through a multi-key keyring produces ciphertext that (a) round-trips back to the plaintext and (b) is decryptable by a keyring containing only the primary key — evidence: unit test asserts round-trip equality, and asserts a keyring built from just the primary key decrypts it while a keyring built from only a non-primary key returns a non-nil error.
- [ ] **Decrypt-by-id across a multi-key ring.** Given a ring `[primary, old]`, a blob sealed under `old` and a blob sealed under `primary` both decrypt correctly through the ring — evidence: unit test seals two plaintexts under two different single-key crypters whose keys are both in the ring, decrypts both through the ring, asserts both match.
- [ ] **Unknown key-id is a clear error, not a panic and not silent wrong-plaintext.** A framed ciphertext whose key-id names a key absent from the ring returns a non-nil wrapped error and no plaintext — evidence: unit test seals under key A, decrypts through a ring built from key B only, asserts the returned error is non-nil and the returned bytes are nil.
- [ ] **Key-id is stable under list reordering.** A blob sealed while a key is primary still decrypts after that key is demoted (a new primary prepended). Concretely: seal under ring `[A]`, then decrypt the same blob under ring `[B, A]` — evidence: unit test asserts the blob still decrypts to the original plaintext after the reorder, proving the key-id is derived from key material, not list position.
- [ ] **Back-compat single-key config still boots.** Startup with only `LOCKBOX_ENCRYPTION_KEY` set (valid base64, 16 or 32 bytes) yields a working one-entry keyring — evidence: a `createCrypter`-level unit/config test returns a non-nil crypter and no error for the legacy single-key input, and round-trips a secret through the resulting store.
- [ ] **Ordered multi-key config parses, primary first.** Startup with `LOCKBOX_ENCRYPTION_KEYS` set to a comma-separated list of valid base64 keys yields a keyring whose primary is the first entry — evidence: config test parses a two-key list, encrypts a value, and asserts it decrypts under a ring of only the first (primary) key.
- [ ] **Invalid or absent key material refuses start.** Each of these returns a non-nil error from the crypter/config construction and never returns a usable crypter: (a) neither env var set, (b) both env vars set, (c) empty `LOCKBOX_ENCRYPTION_KEYS`, (d) a list entry that is not valid base64, (e) a list entry that decodes to a length other than 16 or 32 bytes, (f) two identical keys in the list — evidence: table-driven config test asserts a non-nil error for each case.
- [ ] **Re-encrypt sweep moves every secret to the primary key.** After seeding a store that contains secrets sealed under a non-primary (old) key and running the sweep, every stored secret decrypts under a ring containing only the primary key — evidence: integration test seals N secrets under `old`, runs the sweep with ring `[primary, old]`, then builds a store with ring `[primary]` and asserts all N secrets read back correctly.
- [ ] **Re-encrypt sweep is idempotent / re-runnable.** Running the sweep twice in a row leaves every secret readable and unchanged in value — evidence: integration test runs the sweep twice, then asserts all secrets still decrypt to their original plaintext under ring `[primary]`, and the sweep's second run returns no error.
- [ ] **Read/write handler contracts unchanged.** The read handler source files and the existing `main_test.go` GET/POST/PATCH contract assertions are not modified — evidence: `git diff master -- pkg/handler` shows no changes to the metadata/revision-data/search/create/update handler response shapes, and the pre-existing contract assertion blocks in `main_test.go` are byte-identical.
- [ ] **Docs describe rotation and re-encrypt.** `README.md` documents `LOCKBOX_ENCRYPTION_KEYS`, the rotation-by-restart flow, and the re-encrypt command; `CHANGELOG.md` has a new top entry describing the keyring — evidence: `grep -n 'LOCKBOX_ENCRYPTION_KEYS' README.md` returns ≥1, `grep -n -i 'rotat' README.md` returns ≥1, and the `CHANGELOG.md` top version entry mentions key rotation / keyring.
- [ ] **`make precommit` exits 0** — evidence: exit code 0 (runs format, generate, test, check, addlicense).

## Verification

### Container-executable (runs at prompt time)

```
make precommit
```

Plus the targeted greps named in the Acceptance Criteria:

```
git diff master -- pkg/handler          # expect no response-shape changes
grep -n 'LOCKBOX_ENCRYPTION_KEYS' README.md   # expect >=1
grep -n -i 'rotat' README.md                  # expect >=1
```

There is no operator-executable rung: the entire behavior is exercisable with in-memory kv and unit/integration tests; no deploy or cluster observation is required to prove it.

## Desired Behavior

1. **Keyring crypter (in-lockbox wrapper).** A new keyring type wraps one `bborbe/crypto` `Crypter` per key and itself satisfies the `crypto.Crypter` interface (`Encrypt`/`Decrypt(ctx, []byte)`), so `secret.NewStore` keeps taking a `crypto.Crypter` unchanged. The keyring holds an ordered set of keys with the first designated **primary**. (The cross-repo alternative — extending `bborbe/crypto` with a multi-key crypter — is explicitly deferred in favor of the in-lockbox wrapper.)
2. **Framed ciphertext with a content-derived key-id.** `Encrypt` seals the plaintext under the primary key and prepends a self-describing frame carrying a **key identifier derived from the key material** (a short deterministic fingerprint of the key bytes), NOT from the key's position in the list. Deriving the id from key material is load-bearing: prepending a new primary must never reassign an existing key's id, or previously stored ciphertext would point at the wrong key and be lost.
3. **Decrypt by ordered precedence, authenticated at every step.** `Decrypt` resolves plaintext through one fixed ordered path and returns an error only after **every** step fails — it never short-circuits to an error while a configured key could still decrypt the blob:
   1. Parse the frame. If it parses **and** names a key present in the ring, attempt AES-GCM decryption under that key; if the tag authenticates, return the plaintext.
   2. Otherwise — the frame does not parse, names a key-id absent from the ring, or fails authentication — fall through to the **legacy try-each** path (§4): attempt each configured key and accept the one whose AES-GCM tag authenticates.
   3. If no configured key authenticates, return a wrapped error and no plaintext.

   Discrimination is always by AES-GCM authentication, never by the frame marker alone. So a legacy blob that coincidentally resembles a frame — including one whose bytes imply an **unknown** key-id — is never turned into an error while a key that can actually decrypt it is still configured; and an unknown-id framed blob whose real key was dropped falls through and only then errors (no silent wrong plaintext, ever).
4. **Legacy ciphertext read in place.** Ciphertext written before the keyring (a bare `bborbe/crypto` AES-GCM blob with no frame) is decrypted by trying the configured keys and accepting the one whose AES-GCM tag authenticates. No stored data is rewritten to adopt the keyring; the previously-single key simply becomes one keyring entry.
5. **Two config sources, one authoritative.** Configuration accepts `LOCKBOX_ENCRYPTION_KEYS` (comma-separated base64, primary first) as the keyring, and keeps `LOCKBOX_ENCRYPTION_KEY` (single base64 key) as a one-entry-keyring shorthand. Exactly one of the two must be non-empty; neither-set and both-set are startup errors. Every key must base64-decode to 16 or 32 bytes and be distinct, or startup fails — the server never runs on invalid or empty key material.
6. **Rotation by restart.** Prepending a new key to `LOCKBOX_ENCRYPTION_KEYS` (making it primary) and restarting causes all new writes to use the new key while every previously stored secret decrypts under its original, still-present key. No downtime, no data loss, no rewrite at rotation time.
7. **Re-encrypt sweep.** A sweep operation over the store reads every secret and rewrites it sealed under the current primary key, so that once it completes no stored secret references a non-primary key and retired keys can be removed from the configuration. It is invoked by a one-shot command under `cmd/` (mirroring `cmd/migrate-teamvault`) that opens the same data directory and builds the keyring from the same environment. The sweep is idempotent (re-encrypting an already-primary secret is harmless) and crash-safe (an interrupted run leaves every secret readable under the still-present keys; re-running completes the conversion).

## Constraints

- The HTTP API, its routes, auth, and the read/write handlers' request/response JSON shapes are frozen and must keep passing the existing `main_test.go` contract assertions. Only the at-rest encoding, key configuration, and the new re-encrypt command change.
- `secret.NewStore` continues to accept a `github.com/bborbe/crypto` `Crypter`; the keyring is provided through that same seam so the store's encrypt-before-write / decrypt-on-read logic is otherwise unchanged. Only ciphertext ever touches the kv store.
- The single-key `LOCKBOX_ENCRYPTION_KEY` path remains valid and boots an equivalent one-entry keyring — existing deployments and the `example.env` default keep working without config changes.
- Repo conventions hold: errors wrapped via `github.com/bborbe/errors` (never `fmt.Errorf`); no `time.Now` / `context.Background` in business logic; Ginkgo/Gomega tests in external `_test` packages; counterfeiter-generated mocks; GoDoc on every exported symbol; 2026 BSD copyright headers; `make precommit` green.
- Underlying primitive stays `bborbe/crypto` AES-GCM (`Encrypt`/`Decrypt` produce/consume `nonce+ciphertext`); the keyring composes these single-key crypters rather than reimplementing the cipher.

## Failure Modes

| Trigger | Expected behavior | Recovery | Detection | Reversibility | Concurrency |
|---------|-------------------|----------|-----------|---------------|-------------|
| Framed ciphertext names a key-id absent from the ring (retired key dropped too early) | `Decrypt` returns a wrapped error, no plaintext; the read fails with a server error | Operator re-adds the missing key to `LOCKBOX_ENCRYPTION_KEYS` and restarts; the secret is readable again | Read returns 500 + `bborbe/errors`-wrapped log line naming the unknown key-id | Reversible — nothing was mutated; the ciphertext is intact | n/a — read-only |
| No key can decrypt a legacy (un-framed) blob (its key was dropped) | `Decrypt` returns a wrapped error; no plaintext, no silent wrong value | Operator re-adds the original key and restarts | Read returns 500 + wrapped log line | Reversible — ciphertext intact | n/a |
| Startup with invalid key material (bad base64, wrong length, empty, both/neither env set, duplicate keys) | Server refuses to start; construction returns a wrapped error before any request is served | Operator fixes the env and restarts | Process exits non-zero with a wrapped startup error | Reversible — no data touched | n/a |
| Corrupt / truncated frame prefix on a stored blob | Framed interpretation fails AES-GCM authentication; falls through to the legacy try-each path; if that also fails, returns a wrapped error (never a panic, never wrong plaintext) | Operator investigates the corrupt record; re-encrypt cannot fix a genuinely corrupt blob | Read returns 500 + wrapped log line; a panic would be a defect | Depends — genuine corruption is irreversible, but discrimination logic never makes it worse | n/a |
| Re-encrypt sweep interrupted mid-run (crash, SIGKILL, power loss) | Partially converted store: some secrets under primary, some under old keys; every secret remains decryptable because all keys are still configured | Re-run the sweep; it converts the remainder and re-frames already-primary secrets harmlessly | Operator re-runs and observes the command completes with no error | Reversible — each write is a single-key overwrite; no half-written secret is left undecryptable | Sweep is a one-shot admin command; it must not run concurrently with itself — the second invocation simply re-does work, it does not corrupt |
| Retired key removed from config before the sweep completed | Secrets still referencing the retired id fail to read (unknown key-id, row above) | Re-add the key, run the sweep to completion, then remove the key | 500 on the affected reads | Reversible by re-adding the key | Removing a key is config-only; no data mutation |
| kv backend write fails during the sweep | The failing secret is reported; the sweep returns a wrapped error naming which key failed | Operator fixes the backend and re-runs; already-converted secrets are skipped-safe (re-encrypting them is harmless) | Command exits non-zero with a wrapped error | Reversible — a failed write leaves the prior ciphertext intact and readable | Re-run is safe |

## Security / Abuse Cases

- **Key material is the trust boundary.** Keys arrive only through `LOCKBOX_ENCRYPTION_KEY` / `LOCKBOX_ENCRYPTION_KEYS` environment configuration, never from request input. The at-rest frame carries only a non-secret key **identifier** (a fingerprint), never key bytes — the fingerprint must not leak enough to reconstruct the key.
- **No wrong-plaintext on mismatch.** Because AES-GCM is authenticated, a wrong key (whether via a stale key-id or the legacy try-each path) fails the tag check and returns an error; the keyring must never return unauthenticated bytes as plaintext.
- **Bounded decrypt work.** The legacy try-each path is bounded by the (small) number of configured keys; there is no unbounded retry or input-controlled loop. Frame parsing rejects truncated input rather than reading out of bounds.
- **No new HTTP surface.** The re-encrypt sweep is a one-shot command run by the operator against the data directory, not a network endpoint, so it adds no unauthenticated attack surface. (If a future admin endpoint is wanted, that is a separate spec.)

## Suggested Decomposition

Prompts generated in this order — each row is one prompt.

| # | Prompt focus | Covers DBs | Covers ACs | Depends on |
|---|---|---|---|---|
| 1 | Keyring crypter: content-derived key-id, framed encrypt under primary, decrypt-by-id, legacy authenticated try-each fallback; unit tests for legacy/multi-key/unknown-id/reorder | 1, 2, 3, 4 | 1, 2, 3, 4, 5 | — |
| 2 | Config parsing in `main.go`: `LOCKBOX_ENCRYPTION_KEYS` + back-compat `LOCKBOX_ENCRYPTION_KEY`, exactly-one-source rule, validation & refuse-start; config tests | 5, 6 | 6, 7, 8 | 1 |
| 3 | Re-encrypt sweep over the store + one-shot `cmd/` command; idempotent/crash-safe integration tests | 7 | 9, 10 | 1, 2 |
| 4 | README rotation + re-encrypt docs, `example.env`, CHANGELOG; confirm handler contracts untouched | — | 11, 12 | 1, 2, 3 |

Rationale: the keyring crypter (1) is the foundation everything else composes, and its correctness properties (legacy read, id stability) are the highest-risk part, so it lands first with the bulk of the tests. Config (2) wires the keyring into startup and can only be validated once the crypter exists. The sweep (3) needs both a working keyring and a store fed by it. Docs and the frozen-contract check (4) close out once behavior is in place.

## Do-Nothing Option

If we skip this, Lockbox's encryption key can never be rotated once the store holds data — any key change is catastrophic data loss. That is an unacceptable posture for a secret manager and it directly blocks the personal TeamVault → Lockbox migration, since filling the store commits to an unrotatable key forever. The current single-key design is fine only for an empty or throwaway store; for a durable personal vault it is a standing liability, so leaving it as-is is not acceptable.
