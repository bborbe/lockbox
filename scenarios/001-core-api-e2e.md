---
status: active
---

# Scenario 001: core TeamVault-compatible API, end-to-end

Validates the real `lockbox` server, started hermetically against a temp data
dir, exercising the full TeamVault-compatible HTTP API via Basic-auth curl +
jq: create, read metadata, read revision data, update, search, auth
enforcement, and the `/api/v1` prefix mirror of `/api`. No live TeamVault, no
network dependency.

Setup/assert helpers live in `scenarios/helper/lib.sh` (same convention as
sm-teamvault-cli). CI runs the whole thing via `make e2e`; the fastest local
path is also `make e2e`.

Covered cases: create a password secret (201), read metadata, read revision
data, PATCH a metadata field, create a second secret and search for it,
401 without credentials, and the same create/read flow under `/api/v1`.

## Setup

```bash
source ~/Documents/workspaces/lockbox/scenarios/helper/lib.sh
build_binaries                          # builds lockbox + reencrypt to a temp dir
unset LOCKBOX_ENCRYPTION_KEYS
export LOCKBOX_ENCRYPTION_KEY="$(gen_key)"
DATADIR="$WORK_DIR/data-001" start_lockbox   # starts the server, waits for /healthz, exports LOCKBOX_URL
```

- [ ] `$LOCKBOX` and `$REENCRYPT` exist; the server is listening (`$LOCKBOX_URL` non-empty, `/healthz` returned 200)

## Action + Expected

```bash
CREATE_STATUS="$(lb_status "/api/secrets/" -X POST -H 'Content-Type: application/json' -d '{
	"content_type": "password", "name": "demo", "username": "demo-user",
	"url": "https://demo.example/login", "secret_data": {"password": "demo-pass-123"}
}')"
assert_eq "create secret status" "201" "$CREATE_STATUS"

CREATE_RESP="$(lb_curl "/api/secrets/" -X POST -H 'Content-Type: application/json' -d '{
	"content_type": "password", "name": "demo", "username": "demo-user",
	"url": "https://demo.example/login", "secret_data": {"password": "demo-pass-123"}
}')"
KEY="$(echo "$CREATE_RESP" | jq -r .hashid)"

META_RESP="$(lb_curl "/api/secrets/$KEY/")"
assert_eq "metadata username" "demo-user" "$(echo "$META_RESP" | jq -r .username)"
assert_eq "metadata url" "https://demo.example/login" "$(echo "$META_RESP" | jq -r .url)"

REV_RESP="$(lb_curl "/api/secret-revisions/$KEY/data")"
assert_eq "revision password" "demo-pass-123" "$(echo "$REV_RESP" | jq -r .password)"

UPDATE_STATUS="$(lb_status "/api/secrets/$KEY/" -X PATCH -H 'Content-Type: application/json' \
	-d '{"content_type": "password", "url": "https://demo.example/updated"}')"
assert_eq "update status" "200" "$UPDATE_STATUS"
META_RESP2="$(lb_curl "/api/secrets/$KEY/")"
assert_eq "metadata url after update" "https://demo.example/updated" "$(echo "$META_RESP2" | jq -r .url)"

SEARCH_CREATE_RESP="$(lb_curl "/api/secrets/" -X POST -H 'Content-Type: application/json' -d '{
	"content_type": "password", "name": "search-target-secret", "username": "search-user",
	"secret_data": {"password": "search-pass"}
}')"
SEARCH_KEY="$(echo "$SEARCH_CREATE_RESP" | jq -r .hashid)"
SEARCH_RESP="$(lb_curl "/api/secrets/?search=search-target")"
assert_contains "search results contain the right api_url" "/api/secrets/$SEARCH_KEY/" \
	"$(echo "$SEARCH_RESP" | jq -r '.results[].api_url' | tr '\n' ' ')"

NOAUTH_STATUS="$(curl -s -o /dev/null -w '%{http_code}' "$LOCKBOX_URL/api/secrets/$KEY/")"
assert_eq "no-auth status" "401" "$NOAUTH_STATUS"

V1_CREATE_RESP="$(lb_curl "/api/v1/secrets/" -X POST -H 'Content-Type: application/json' -d '{
	"content_type": "password", "name": "v1-demo", "username": "v1-user",
	"secret_data": {"password": "v1-pass-789"}
}')"
V1_KEY="$(echo "$V1_CREATE_RESP" | jq -r .hashid)"
assert_contains "v1 create response api_url uses /api/v1" "/api/v1/secrets/$V1_KEY/" \
	"$(echo "$V1_CREATE_RESP" | jq -r .api_url)"
V1_REV_RESP="$(lb_curl "/api/v1/secret-revisions/$V1_KEY/data")"
assert_eq "v1 revision password" "v1-pass-789" "$(echo "$V1_REV_RESP" | jq -r .password)"

stop_lockbox
scenario_done   # prints "e2e: PASS" and exits non-zero if any assertion failed
```

- [ ] All assertions print `ok:` and `scenario_done` reports `e2e: PASS`

## Cleanup

`scenarios/helper/lib.sh` installs an EXIT trap that kills the lockbox server
and removes `$WORK_DIR` — no manual cleanup needed.
