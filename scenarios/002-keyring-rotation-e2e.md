---
status: active
---

# Scenario 002: keyring rotation + reencrypt sweep, end-to-end

Validates the full operator key-rotation flow documented in the README:
prepend a new primary key and restart (old secrets still decrypt under the
retired key), run the `cmd/reencrypt` sweep (rewrites everything under the new
primary), then restart with the retired key fully removed and prove every
secret — old and new — still reads. Hermetic: real `lockbox` + `reencrypt`
binaries, temp data dir, no live TeamVault, no network.

Setup/assert helpers live in `scenarios/helper/lib.sh`. CI runs the whole
thing via `make e2e`; the fastest local path is also `make e2e`.

Covered cases: single-key boot, prepend-new-primary rotation via restart,
old-key secret still readable post-rotation, new secret sealed under the new
primary, the reencrypt sweep exits 0, and — with the retired key fully
removed from config — both the pre-rotation and post-rotation secrets still
decrypt (proving the sweep actually rewrote the pre-rotation secret).

## Setup

```bash
source ~/Documents/workspaces/lockbox/scenarios/helper/lib.sh
build_binaries                 # builds lockbox + reencrypt to a temp dir
KEY_A="$(gen_key)"
KEY_B="$(gen_key)"
DATADIR="$WORK_DIR/data-002"
unset LOCKBOX_ENCRYPTION_KEYS
export LOCKBOX_ENCRYPTION_KEY="$KEY_A"
start_lockbox                  # single primary key A, fresh DATADIR
```

- [ ] `$LOCKBOX` and `$REENCRYPT` exist; the server is listening under key A only

## Action + Expected

```bash
# Create S1 under primary A, confirm it reads back.
S1_CREATE_RESP="$(lb_curl "/api/secrets/" -X POST -H 'Content-Type: application/json' -d '{
	"content_type": "password", "name": "rotation-s1", "username": "s1-user",
	"secret_data": {"password": "s1-pass"}
}')"
S1_KEY="$(echo "$S1_CREATE_RESP" | jq -r .hashid)"
assert_eq "S1 readable under key A" "s1-pass" \
	"$(lb_curl "/api/secret-revisions/$S1_KEY/data" | jq -r .password)"
stop_lockbox

# Restart with B prepended as the new primary (A retained) — same DATADIR.
unset LOCKBOX_ENCRYPTION_KEY
export LOCKBOX_ENCRYPTION_KEYS="$KEY_B,$KEY_A"
start_lockbox
assert_eq "S1 still readable after rotation (decrypts under retired key A)" "s1-pass" \
	"$(lb_curl "/api/secret-revisions/$S1_KEY/data" | jq -r .password)"

# Create S2 — sealed under the new primary B.
S2_CREATE_RESP="$(lb_curl "/api/secrets/" -X POST -H 'Content-Type: application/json' -d '{
	"content_type": "password", "name": "rotation-s2", "username": "s2-user",
	"secret_data": {"password": "s2-pass"}
}')"
S2_KEY="$(echo "$S2_CREATE_RESP" | jq -r .hashid)"
assert_eq "S2 readable under new primary B" "s2-pass" \
	"$(lb_curl "/api/secret-revisions/$S2_KEY/data" | jq -r .password)"
stop_lockbox

# Run the reencrypt sweep — rewrites everything under primary B.
DATADIR="$DATADIR" LOCKBOX_ENCRYPTION_KEYS="$KEY_B,$KEY_A" "$REENCRYPT"
assert_eq "reencrypt sweep exit 0" "0" "$?"

# Restart with ONLY B — A fully retired. Both S1 and S2 must still read.
unset LOCKBOX_ENCRYPTION_KEYS
export LOCKBOX_ENCRYPTION_KEY="$KEY_B"
start_lockbox
assert_eq "S1 readable with A fully retired (proves reencrypt worked)" "s1-pass" \
	"$(lb_curl "/api/secret-revisions/$S1_KEY/data" | jq -r .password)"
assert_eq "S2 still readable under B-only keyring" "s2-pass" \
	"$(lb_curl "/api/secret-revisions/$S2_KEY/data" | jq -r .password)"
stop_lockbox

scenario_done   # prints "e2e: PASS" and exits non-zero if any assertion failed
```

- [ ] All assertions print `ok:` and `scenario_done` reports `e2e: PASS`

## Cleanup

`scenarios/helper/lib.sh` installs an EXIT trap that kills any running
lockbox server and removes `$WORK_DIR` — no manual cleanup needed.
