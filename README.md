# lockbox

A small, self-hosted secret manager in Go — API-compatible with [TeamVault](https://github.com/seibert-media/teamvault), backed by a key-value store ([bborbe/kv](https://github.com/bborbe/kv): BoltDB / Badger), no Postgres.

Drop-in replacement for a personal TeamVault server: existing [teamvault-cli](https://github.com/Seibert-Data/teamvault-cli) clients work with only a base-URL swap.

## Components

| Path | Description |
|------|-------------|
| `main.go` | Lockbox server — serves the TeamVault-compatible HTTP API, kv-backed |
| `cmd/lockbox-cli/` | Client CLI — read secrets by key |
| `cmd/migrate-teamvault/` | One-shot Postgres → kv importer (seeds from an existing TeamVault DB) |
| `cmd/reencrypt/` | One-shot re-encrypt sweep — rewrites every stored secret under the current primary key so retired keys can be removed |

## API

Mirrors the TeamVault API (see `cmd/fakevault` in teamvault-cli):

| Endpoint | Response |
|----------|----------|
| `POST /api/secrets/` | create a secret (server-generated key); returns `{hashid, api_url, content_type, name, username, url}` |
| `GET /api/secrets/{key}/` | `{name, username, url, current_revision}` |
| `GET /api/secret-revisions/{key}/data` | `{password, file}` |
| `GET /api/secrets/?search=q` | `{count, next, previous, results: [{hashid, api_url, name, username, url}]}` |
| `PATCH /api/secrets/{hashid}/` | update metadata and/or value; returns the secret representation |

Both `/api/…` and `/api/v1/…` route to the same handlers. HTTP Basic auth.

## Encryption at rest

Secrets are AES-GCM encrypted (via [bborbe/crypto](https://github.com/bborbe/crypto)) before they are written to the kv store; only ciphertext ever touches disk. Configure the key with `LOCKBOX_ENCRYPTION_KEY`: a base64-encoded 16- or 32-byte AES key. Generate one with `openssl rand -base64 32`. The server refuses to start if the key is missing or not 16/32 raw bytes.

### Keyring and key rotation

For multi-key operation and key rotation, set `LOCKBOX_ENCRYPTION_KEYS` — a comma-separated list of base64 AES keys (primary first). New secrets are sealed under the primary key and tagged with a content-derived key identifier; any secret is read back under the key that sealed it. Exactly one of `LOCKBOX_ENCRYPTION_KEY` (single-key shorthand) or `LOCKBOX_ENCRYPTION_KEYS` must be set; setting both or neither refuses start, as does any key that is not valid base64 decoding to 16 or 32 bytes, or a duplicate key.

The single `LOCKBOX_ENCRYPTION_KEY` still works unchanged and boots an equivalent one-entry keyring; existing deployments need no configuration change. Legacy ciphertext written before the keyring feature stays readable in place with no rewrite.

**Rotation by restart** — to rotate the master key, prepend a new key to `LOCKBOX_ENCRYPTION_KEYS` (making it the new primary) and restart the server. New writes adopt the new primary key; every previously stored secret still decrypts under its original key which remains in the keyring. No downtime, no data loss, no rewrite at rotation time.

Once the server has restarted with the new primary, run the one-shot re-encrypt sweep to migrate all existing secrets onto the new key:

```bash
DATADIR=/data LOCKBOX_ENCRYPTION_KEYS="$(cat /etc/lockbox/new-key.key),$(cat /etc/lockbox/old-key.key)" go run ./cmd/reencrypt
```

The sweep reads every stored secret and rewrites it under the current primary key, after which no secret references a retired key. The command is idempotent and crash-safe — re-running it after an interruption leaves every secret readable and unchanged.

**Operator key-rotation flow:**
1. Prepend the new key to `LOCKBOX_ENCRYPTION_KEYS` and restart the server.
2. Run `cmd/reencrypt` to migrate all existing secrets onto the new primary key.
3. Remove the retired key from `LOCKBOX_ENCRYPTION_KEYS` and restart the server.

## Run locally

```bash
make test
make run
```

## End-to-end tests

`make e2e` runs hermetic scenarios (`scenarios/001-core-api-e2e.md`,
`scenarios/002-keyring-rotation-e2e.md`) that build the real `lockbox` and
`cmd/reencrypt` binaries and drive them against a temp data dir over HTTP —
full TeamVault-compatible API coverage plus a live keyring-rotation +
reencrypt-sweep flow. No live TeamVault, no network.

## Deploy

```bash
make buca
```

## License

BSD-style license. See [LICENSE](LICENSE) file for details.
