# Changelog

All notable changes to this project will be documented in this file.

Please choose versions by [Semantic Versioning](http://semver.org/).

* MAJOR version when you make incompatible API changes,
* MINOR version when you add functionality in a backwards-compatible manner, and
* PATCH version when you make backwards-compatible bug fixes.

## Unreleased

- feat: add `POST /api/secrets/` (and `/api/v1/secrets/`) handler `NewSecretCreateHandler` that decodes a TeamVault create body, validates it, generates a fresh unique key, stores the secret encrypted via check-and-set, and responds HTTP 201 with a TeamVault-shaped representation containing the new hashid and api_url
- feat: expand `Secret` domain record with `Name`, `Description`, `ContentType` fields and add `ContentTypePassword`/`ContentTypeFile` constants for TeamVault write-API compatibility
- feat: add server-side `KeyGenerator` interface with `NewKeyGenerator` constructor producing URL-safe base62 keys of fixed 8-character length via `crypto/rand`
- feat: add check-and-set `Create` method to `Store` interface that returns `ErrKeyExists` if key already exists; used by create handler to enforce uniqueness
- feat: add TeamVault write-API DTOs (`CreateSecretRequest`, `SecretData`, `SecretRepresentation`) and a pure `Validate`/`ApplyUpdate` pair that maps a decoded create/update body into a `secret.Secret`, returning HTTP 400 for malformed input

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
