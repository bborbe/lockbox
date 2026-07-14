# lockbox

A small, self-hosted secret manager in Go — API-compatible with [TeamVault](https://github.com/seibert-media/teamvault), backed by a key-value store ([bborbe/kv](https://github.com/bborbe/kv): BoltDB / Badger), no Postgres.

Drop-in replacement for a personal TeamVault server: existing [teamvault-cli](https://github.com/Seibert-Data/teamvault-cli) clients work with only a base-URL swap.

## Components

| Path | Description |
|------|-------------|
| `main.go` | Lockbox server — serves the TeamVault-compatible HTTP API, kv-backed |
| `cmd/lockbox-cli/` | Client CLI — read secrets by key |
| `cmd/migrate-teamvault/` | One-shot Postgres → kv importer (seeds from an existing TeamVault DB) |

## API

Mirrors the TeamVault read API (see `cmd/fakevault` in teamvault-cli):

| Endpoint | Response |
|----------|----------|
| `GET /api/secrets/{key}/` | `{username, url, current_revision}` |
| `GET /api/secret-revisions/{key}/data` | `{password, file}` |
| `GET /api/secrets/?search=q` | `{results: [{api_url}]}` |

Both `/api/…` and `/api/v1/…` route to the same handlers. HTTP Basic auth.

## Encryption at rest

Secrets are AES-GCM encrypted (via [bborbe/crypto](https://github.com/bborbe/crypto)) before they are written to the kv store; only ciphertext ever touches disk. Configure the key with `LOCKBOX_ENCRYPTION_KEY`: a base64-encoded 16- or 32-byte AES key. Generate one with `openssl rand -base64 32`. The server refuses to start if the key is missing or not 16/32 raw bytes.

## Run locally

```bash
make test
make run
```

## Deploy

```bash
make buca
```

## License

BSD-style license. See [LICENSE](LICENSE) file for details.
