---
status: approved
approved: "2026-07-14T13:34:47Z"
branch: dark-factory/write-api-teamvault-compat
---

## Summary

- Lockbox's READ API is 100% TeamVault-compatible; its WRITE API is not — v0.1.0 shipped a flat `PUT /api/secrets/{key}/` MVP shortcut that no real TeamVault client speaks.
- Replace that shortcut with TeamVault's real write API: `POST /api/secrets/` (create, server generates the key) and `PATCH /api/secrets/{hashid}/` (update metadata and/or store a new value).
- Accept TeamVault's create/update body shape (`content_type`, `name`, `username?`, `url?`, `description?`, `secret_data`) and return TeamVault-shaped representations including the new `hashid` and `api_url`.
- Scope to the `password` and `file` content types; encryption-at-rest, dual `/api` + `/api/v1` prefixes, and Basic auth all continue to apply. The read endpoints keep returning byte-for-byte the same shapes they do today.
- Switch the `migrate-teamvault` importer from the flat `PUT` to `POST /api/secrets/`, and remove the flat upsert entirely.

## Problem

The whole point of Lockbox is to be a drop-in TeamVault replacement: the same `teamvault-cli` clients must work against either server with only a base-URL change. Reads already satisfy that contract, but writes do not — Lockbox exposes a home-grown flat `PUT` that expects a caller-chosen key and a `{username,url,password,file}` body, whereas TeamVault clients create secrets with `POST /api/secrets/` (server assigns the key) and update them with `PATCH`, using a `content_type` + `secret_data` envelope. Until the write path matches, `teamvault-cli create` / `update` fail against Lockbox, so the compatibility promise is only half true and migrations depend on a non-standard endpoint.

## Goal

A TeamVault client can create and update secrets against Lockbox exactly as it does against TeamVault. `POST /api/secrets/` with TeamVault's create body stores the secret (encrypted at rest), assigns a fresh server-generated key, and returns a TeamVault-shaped representation containing that key. `PATCH /api/secrets/{hashid}/` updates the stored metadata and/or replaces the secret value. Both endpoints exist under `/api` and `/api/v1`, are Basic-auth protected, and cover the `password` and `file` content types. The read endpoints return the exact same JSON shapes as before. The flat `PUT` upsert no longer exists, and `migrate-teamvault` seeds Lockbox through `POST /api/secrets/`.

## Non-goals

- Do NOT implement the `cc` (credit-card) content type as a first-class write path — reject it with a 4xx; credit-card migration is already skipped by the importer. If a future consumer needs `cc` writes, that is a separate spec.
- Do NOT implement `POST /api/generate_password/`, OTP (`otp_key_data`), sharing (`shares`), `access_policy` enforcement, or `status` lifecycle semantics beyond the fields a create/read round-trip needs to echo.
- Do NOT add secret revision history — Lockbox stores only the current value; a "new revision" on update reduces to replacing the stored value. If versioned history is ever required, that is a separate spec.
- Do NOT change any READ endpoint's response shape, route, or auth. The existing read contract is frozen.
- The exact key/`hashid` format is **opaque to clients** — only URL-safety and uniqueness are contractual, NOT the charset or length. Do not try to replicate TeamVault's hashids algorithm.
- Do NOT build the `teamvault-cli create` / `update` client commands — that is the companion spec in the `teamvault-cli` repo, developed independently and in parallel.
- Do NOT make the generated-key length, charset, or the 201/400 status codes configurable — they are invariants of TeamVault compatibility; a future variation is a separate spec.

## Acceptance Criteria

Each criterion is binary and declares the artifact the verifier observes. Contract-level ACs are asserted by the Ginkgo contract suite in `main_test.go` (both `/api` and `/api/v1` prefixes) unless noted.

- [ ] `POST /api/secrets/` with a valid password body (`{content_type:"password", name, username, url, secret_data:{password}}`) returns HTTP **201** and a JSON body whose `hashid` is a non-empty string and whose `api_url` equals `http://<host><prefix>/secrets/<hashid>/` — evidence: HTTP status 201 + response JSON keys `hashid`, `api_url` asserted in contract test.
- [ ] A secret created via `POST` is immediately readable through the unchanged read endpoints: `GET <prefix>/secrets/<hashid>/` returns 200 with body keys exactly `{username, url, current_revision}`, and `GET <prefix>/secret-revisions/<hashid>/data` returns 200 with `{password, file}` echoing the posted value — evidence: HTTP 200 + exact key-set and value assertions in contract test.
- [ ] `POST /api/secrets/` with `content_type:"file"` and `secret_data:{file_content:<base64>, filename}` stores the payload so the revision-data read returns it in the `file` field with `password` empty — evidence: HTTP 201 then `GET .../data` body `file` equals the posted base64, `password` == "".
- [ ] The `POST` response representation contains the TeamVault-shaped keys `hashid`, `api_url`, `content_type` (string equal to the posted value), `name`, `username`, `url` — evidence: response JSON key/value assertions in contract test.
- [ ] `POST` rejects malformed input with HTTP **400**: (a) missing `content_type`, (b) missing `secret_data`, (c) `content_type` not in `{"password","file"}` (including `"cc"`), (d) `password` body whose `secret_data` lacks `password`, (e) `file` body whose `secret_data` lacks `file_content` — evidence: HTTP 400 for each case asserted in contract test.
- [ ] `PATCH /api/secrets/<hashid>/` with `{name, url}` returns 200 and a later `GET .../secrets/<hashid>/` reflects the new `url`; `PATCH` with a new `secret_data` changes the value returned by the revision-data read — evidence: HTTP 200 + before/after value assertions in contract test.
- [ ] `content_type` is immutable on update: `PATCH` of a password secret with `content_type:"file"` in the body leaves the representation's `content_type` as `"password"` — evidence: the create `POST` representation and the update `PATCH` 200 response body both report `content_type:"password"` (the `"file"` in the PATCH body is ignored).
- [ ] Two `POST` calls with identical bodies produce two different `hashid`s, and both remain independently readable — evidence: contract test asserts the two `hashid`s are unequal and each `GET .../data` returns its own value.
- [ ] The flat `PUT` upsert is gone: `PUT <prefix>/secrets/<anykey>/` no longer routes to an upsert handler, and `git grep -n 'UpsertRequest\|UpsertResult\|NewSecretUpsertHandler' -- ':!specs'` returns 0 matches — evidence: negative grep returns empty + contract test shows `PUT` no longer creates a secret (405 or 404, whichever the router emits).
- [ ] `migrate-teamvault` writes to Lockbox via `POST`: `grep -n 'http.MethodPost' pkg/migrate/lockbox.go` returns ≥1 line and `grep -n 'http.MethodPut' pkg/migrate/lockbox.go` returns 0 — evidence: grep line counts + the migrator unit test asserts the create call carries the TeamVault create body.
- [ ] Bad or missing Basic auth on `POST` and `PATCH` returns HTTP **401** — evidence: HTTP 401 asserted in contract test for both methods.
- [ ] `README.md` documents `POST /api/secrets/` and `PATCH /api/secrets/{hashid}/` in the API table, and `CHANGELOG.md` has a new top version entry describing the write-API replacement — evidence: `grep -n 'POST /api/secrets/' README.md` returns ≥1 and `grep -n 'PATCH' README.md` returns ≥1; `CHANGELOG.md` top entry mentions the write API.
- [ ] The existing READ endpoints are not modified — evidence: `git diff master -- <read-handler source files>` (secret-metadata, revision-data, secret-search handlers) is empty, and the pre-existing GET-contract assertions in `main_test.go` are unchanged (byte-identical block).
- [ ] `make precommit` exits 0 — evidence: exit code 0 (runs format, generate, test, check, addlicense).

## Verification

### Container-executable (runs at prompt time)

```
make precommit
```

Plus the targeted greps named in the Acceptance Criteria:

```
git grep -n 'UpsertRequest\|UpsertResult\|NewSecretUpsertHandler' -- ':!specs'   # expect no output
grep -n 'http.MethodPost' pkg/migrate/lockbox.go                                  # expect >=1
grep -n 'http.MethodPut'  pkg/migrate/lockbox.go                                  # expect 0
grep -n 'POST /api/secrets/' README.md                                            # expect >=1
```

## Desired Behavior

1. **Create.** `POST /api/secrets/` (both prefixes, Basic auth) accepts the TeamVault create body — `content_type` ∈ {`password`,`file`}, `name`, optional `username`/`url`/`description`, and a write-only `secret_data` (password → `{password, otp_key_data?}` with `otp_key_data` accepted but not stored; file → `{file_content:<base64>, filename?}`). It generates a fresh server-side key, stores the secret encrypted at rest, and responds 201 with a TeamVault-shaped representation containing the new `hashid` and `api_url`.
2. **Server-generated key.** The key is a non-empty, URL-safe, short alphanumeric string, unique among existing secrets. It is returned as `hashid` and is immediately usable in every read endpoint. Creation never overwrites an existing key.
3. **Update.** `PATCH /api/secrets/{hashid}/` (both prefixes, Basic auth) updates the standard metadata fields (`name`, `description`, `username`, `url`) that are present in the body and, when `secret_data` is present, replaces the stored value. It responds 200 with the updated representation. `content_type` in a `PATCH` body is ignored (immutable after create).
4. **Read shapes frozen.** The domain record gains the fields needed to round-trip create/update (`name`, `description`, `content_type`) alongside the existing value fields. The metadata read still returns exactly `{username, url, current_revision}` and the revision-data read still returns exactly `{password, file}`.
5. **Validation.** Missing `content_type`, missing `secret_data`, an unsupported `content_type` (including `cc`), or a `secret_data` shape that does not match its `content_type` each yields HTTP 400 with a JSON error and no stored secret.
6. **Flat PUT removed.** The `PUT /api/secrets/{key}/` upsert route, its handler, and its `UpsertRequest`/`UpsertResult` DTOs are deleted. Requests to the old route no longer create secrets.
7. **Importer switched.** `migrate-teamvault`'s Lockbox client creates each migrated secret via `POST /api/secrets/`, mapping the TeamVault source secret's `content_type`, name, username, url and value into the TeamVault create body. Password and file secrets migrate; credit-card secrets stay skipped as today.

## Constraints

- The read endpoints (`GET /api/secrets/{key}/`, `GET /api/secret-revisions/{key}/data`, `GET /api/secrets/?search=q`), their routes, response JSON shapes, auth, and the dual `/api` + `/api/v1` prefixing are frozen and must keep passing the existing `main_test.go` contract assertions.
- Encryption at rest is preserved: every stored secret is JSON-marshalled and encrypted via the existing `github.com/bborbe/crypto` `Crypter` before it reaches the kv store; only ciphertext touches disk.
- The exact TeamVault write-API shapes are defined by the Django source at `teamvault/teamvault/apps/secrets/api/serializers.py` (`SecretSerializer`, `SecretDetailSerializer`, `_extract_data`, `STANDARD_FIELDS`), `api/urls.py`, and `models.py`. Implementers read these as the source of truth rather than inventing field names. (Candidate for a new `docs/teamvault-write-api.md` — see report.)
- Repo conventions hold: errors via `github.com/bborbe/errors`, GoDoc on every exported symbol, no bare `interface{}` (use `any` / concrete types), Ginkgo/Gomega tests, counterfeiter-generated mocks, 2026 BSD copyright headers, `make precommit` green.
- `POST` returns 201 and validation failures return 400 to match TeamVault/DRF; these status codes are not configurable.

## Failure Modes

| Trigger | Expected behavior | Recovery | Detection | Concurrency |
|---------|-------------------|----------|-----------|-------------|
| `POST` body missing/invalid `content_type` or `secret_data`, or `secret_data` mismatched to type | Reject with HTTP 400 JSON error; nothing stored | Caller fixes body and retries; no partial write to clean up | Client sees 400 status + error body | n/a |
| Unsupported `content_type` (`cc`) on `POST` | Reject with HTTP 400 | Caller uses TeamVault for `cc`, or migration skips it as today | 400 status | n/a |
| Generated key collides with an existing key | Regenerate and retry key generation (bounded attempts) until unique; never overwrite | If bound is exhausted, return 500 JSON error; caller retries | 500 status is observable; collision at Lockbox scale is astronomically unlikely | Create is check-and-set: only writes when the key is absent, so two concurrent creates cannot clobber each other |
| `PATCH` on a non-existent `hashid` | Return HTTP 404; nothing stored | Caller creates via `POST` first | 404 status | n/a |
| Two concurrent `POST`s | Each gets its own unique key and its own record | none needed | both readable afterward | Distinct keys guaranteed by check-and-set write; identical bodies still yield distinct records |
| kv store write fails mid-create (backend error) | Return HTTP 500 JSON error; no key returned to caller | Caller retries; because the key is only surfaced after a successful write, no dangling half-secret is advertised | 500 status + server log via `github.com/bborbe/errors` wrap | Reversible — a failed create leaves no readable secret |
| Oversized/invalid base64 `file_content` | Reject with HTTP 400 before storing | Caller sends valid base64 | 400 status | n/a |

## Security / Abuse Cases

- **Attacker-controlled input:** the full create/update body (`content_type`, `name`, `secret_data`, `file_content`) crosses the trust boundary. Validate `content_type` against the `{password,file}` allowlist and require `secret_data` to match it; reject anything else with 400 rather than storing opaque garbage.
- **base64 validation:** `file_content` must decode as valid base64 before storage; malformed input is rejected with 400, not stored raw.
- **Key generation:** the generated key must be URL-safe (usable directly in the read-endpoint path) and unpredictable enough not to collide; it must never let a caller overwrite another secret (check-and-set on create).
- **Auth boundary unchanged:** `POST` and `PATCH` sit behind the same Basic-auth wrapper as the read endpoints; unauthenticated or wrong-credential writes return 401 and never mutate state.
- **No unbounded work:** decode/validate the body once; there is no retry-forever path except bounded key-regeneration on collision.

## Suggested Decomposition

Prompts generated in this order — each row is one prompt.

| # | Prompt focus | Covers DBs | Covers ACs | Depends on |
|---|---|---|---|---|
| 1 | Expand the domain record (`name`, `description`, `content_type`) + add server-side key generation (unique, URL-safe, check-and-set create in the store); read shapes unchanged | 2, 4 | (enables 1,2,8) | — |
| 2 | Add create/update wire DTOs (TeamVault create body, `secret_data` password/file shapes, representation) + validation rules | 1, 5 | 4, 5 | 1 |
| 3 | `POST /api/secrets/` create handler + route wiring (both prefixes, Basic auth), 201/400/401 | 1, 5 | 1, 2, 3, 5, 11 | 1, 2 |
| 4 | `PATCH /api/secrets/{hashid}/` update handler + route (metadata + new value, `content_type` immutable, 200/404/401) | 3 | 6, 7, 11 | 1, 2, 3 |
| 5 | Remove the flat `PUT` upsert (route, handler, DTOs) and switch `migrate-teamvault` Lockbox client to `POST` | 6, 7 | 9, 10 | 3 |
| 6 | Contract-test write coverage (`main_test.go` both prefixes) + README API table + CHANGELOG entry | — | 8, 9, 12, 13 | 3, 4, 5 |

Rationale: the store/model and DTO layers (1, 2) are the foundation both handlers need, so they come first and can be split cleanly. Create (3) precedes update (4) because update reuses the create representation and needs existing records to patch. Removal + importer switch (5) must land after `POST` exists so migration has a target. Tests and docs (6) close out once all behavior is in place; keeping them last avoids re-writing assertions against half-built handlers.

## Do-Nothing Option

If we skip this, Lockbox stays only half-compatible: `teamvault-cli create` / `update` keep failing against it, and any migration or write tooling must special-case Lockbox's flat `PUT`. That undermines the drop-in-replacement promise and means Lockbox can never fully stand in for a personal TeamVault server. The current flat `PUT` is an MVP shortcut that no standard client speaks, so leaving it is not an acceptable long-term state.
