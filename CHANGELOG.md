# Changelog

All notable changes to this project will be documented in this file.

Please choose versions by [Semantic Versioning](http://semver.org/).

* MAJOR version when you make incompatible API changes,
* MINOR version when you add functionality in a backwards-compatible manner, and
* PATCH version when you make backwards-compatible bug fixes.

## v0.8.0

- feat: GET /api/secrets/{key}/ now returns the secret `name` alongside `username`, `url`, `current_revision` (TeamVault-compatible detail shape); `teamvault-cli info` can now show Lockbox secret names
- feat: GET /api/secrets/?search=q now returns a `{count, next, previous, results}` envelope where each result carries `hashid`, `name`, `username`, `url` alongside the existing `api_url` (TeamVault-compatible search shape); `teamvault-cli search` can now show Lockbox secret names

## v0.7.0

- build: also build and ship the `reencrypt` sweep binary in the Docker image (`/reencrypt`) so key rotation can retire old keys on the server via `docker run --entrypoint /reencrypt` (server `/main` entrypoint unchanged)

## v0.6.2

- fix: `current_revision` in GET /api/secrets/{key}/ now ends at the revision base (`.../secret-revisions/{key}/`, no `/data` suffix) — matches the TeamVault contract where the client appends `data`; previously a real client requested `.../datadata` (404) and could not read passwords/files. The contract test now follows `current_revision` (append `data`) instead of fetching it directly, so the format is enforced.

## v0.6.1

- test: add hermetic end-to-end scenarios (core API + keyring rotation) runnable via make e2e

## v0.6.0

- feat: add `pkg/keyring` package with `Keyring` type satisfying `crypto.Crypter`; encrypts under primary key with a content-derived SHA-256/4 key-id frame, decrypts by id with authenticated try-each fallback, and reads pre-keyring (un-framed) ciphertext unchanged
- feat: add `LOCKBOX_ENCRYPTION_KEYS` config (comma-separated base64 keys, primary first) alongside existing `LOCKBOX_ENCRYPTION_KEY`; exactly one must be set; the resulting keyring is wired into the secret store via `createCrypter` with full round-trip testing
- feat: add `secret.Store.ReEncrypt` method that rewrites every stored secret under the current primary key; idempotent and crash-safe; enables safe key rotation
- feat: add `cmd/reencrypt` one-shot command that re-encrypts all secrets under the current primary key from the same data directory and key env vars as the server
- refactor: extract `keyring.Parse` as the shared single source of truth for the exactly-one key env-var parsing rule used by both the server and the re-encrypt command
- test: add `pkg/secret/reencrypt_test.go` integration tests covering full primary-key conversion, idempotent re-runs, and legacy un-framed blob sweeping
- test: add `Describe("Parse", ...)` unit tests in `pkg/keyring/keyring_test.go` covering AC-6/7/8 cases
- docs: Document the keyring, rotation-by-restart flow, and cmd/reencrypt in README and example.env

## v0.5.0

- feat: search now matches secret name, url and description (in addition to key and username) for TeamVault-compatible `?search=` behavior

## v0.4.0

- fix: Map TeamVault secret `description` through migrate-teamvault into the create request so migrated secrets keep their description
- docs: Document POST /api/secrets/ and PATCH /api/secrets/{hashid}/ in the README API table
- feat: Replace flat PUT upsert with TeamVault-compatible write API: POST /api/secrets/ (create, server-generated hashid) and PATCH /api/secrets/{hashid}/ (update), on both /api and /api/v1
- refactor: Switch migrate-teamvault importer from the flat PUT to POST /api/secrets/; remove UpsertRequest/UpsertResult DTOs and the flat-PUT handler
- feat: add `POST /api/secrets/` (and `/api/v1/secrets/`) handler `NewSecretCreateHandler` that decodes a TeamVault create body, validates it, generates a fresh unique key, stores the secret encrypted via check-and-set, and responds HTTP 201 with a TeamVault-shaped representation containing the new hashid and api_url
- feat: expand `Secret` domain record with `Name`, `Description`, `ContentType` fields and add `ContentTypePassword`/`ContentTypeFile` constants for TeamVault write-API compatibility
- feat: add server-side `KeyGenerator` interface with `NewKeyGenerator` constructor producing URL-safe base62 keys of fixed 8-character length via `crypto/rand`
- feat: add check-and-set `Create` method to `Store` interface that returns `ErrKeyExists` if key already exists; used by create handler to enforce uniqueness
- feat: add TeamVault write-API DTOs (`CreateSecretRequest`, `SecretData`, `SecretRepresentation`) and a pure `Validate`/`ApplyUpdate` pair that maps a decoded create/update body into a `secret.Secret`, returning HTTP 400 for malformed input
- feat: add `PATCH /api/secrets/{hashid}/` (and `/api/v1/secrets/{hashid}/`) handler `NewSecretUpdateHandler` that loads the existing secret, merges in metadata fields and new secret_data from the body, keeps `content_type` immutable, persists via `Upsert`, and responds HTTP 200 with the updated `SecretRepresentation`

## v0.3.1

- Make `SENTRY_DSN` optional — an empty value disables Sentry; the server no longer refuses to start without it (verified: starts with empty DSN, `/healthz` returns 200)

## v0.3.0

- Encrypt stored secrets at rest: `pkg/secret.NewStore` now takes a `github.com/bborbe/crypto` `Crypter` and AES-GCM encrypts each secret (JSON-marshalled) before writing it to the kv bucket, decrypting on read; depends on the `Crypter` interface so the algorithm can be swapped later
- Add required `LOCKBOX_ENCRYPTION_KEY` config (base64-encoded 16- or 32-byte AES key); the server refuses to start if it is missing or the decoded key length is invalid
- No data migration: the store has no data in a real Lockbox deployment yet, so no migration path is provided for pre-existing plaintext records

## v0.2.0

- Add `cmd/migrate-teamvault`: one-shot API-to-API importer that reads all secrets from a running TeamVault instance (following DRF `next` pagination) and PUTs them into a running Lockbox instance
- Add `pkg/migrate`: `TeamVaultClient`, `LockboxClient` and `Migrator` — skips credit-card secrets and secrets whose data isn't readable, logs and continues past per-secret fetch/write failures, and returns a `Report` with migrated/skipped/failed counts

## v0.1.0

- Add TeamVault-compatible read API: `GET /api/secrets/{key}/`, `GET /api/secret-revisions/{key}/data`, `GET /api/secrets/?search=`
- Add TeamVault-compatible write API: `PUT /api/secrets/{key}/` (upsert), on both `/api` and `/api/v1`, Basic-auth protected
- kv-backed secret store on `bborbe/kv` (BoltDB), no Postgres
- Dual `/api/` + `/api/v1/` routing; HTTP Basic auth on the business API
- Drop Kafka from the service skeleton (Lockbox is not a consumer)
- Add Ginkgo tests: `pkg/secret` store (memorykv backend), all handlers (counterfeiter mocks), `NewBasicAuth`, and a full TeamVault-contract test in `main_test.go` covering PUT/GET round-trip, search, and auth on both API prefixes

## v0.0.1

- Initial commit
