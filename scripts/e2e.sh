#!/usr/bin/env bash
# Copyright (c) 2026 Benjamin Borbe All rights reserved.
# Use of this source code is governed by a BSD-style
# license that can be found in the LICENSE file.
#
# Hermetic end-to-end test: build the real lockbox + reencrypt binaries and
# drive them against a temp data dir (no live TeamVault, no network). Shares
# setup/assert helpers with the scenarios via scenarios/helper/lib.sh. Mirrors
# scenarios/001-core-api-e2e.md and scenarios/002-keyring-rotation-e2e.md.
set -uo pipefail

source "$(cd "$(dirname "$0")/.." && pwd)/scenarios/helper/lib.sh"

echo "e2e: building binaries"
build_binaries

######################################################################
# Scenario 001: core TeamVault-compatible API, both /api and /api/v1.
######################################################################
echo "e2e: scenario 001 — core API"

DATADIR="$WORK_DIR/data-001"
unset LOCKBOX_ENCRYPTION_KEYS
export LOCKBOX_ENCRYPTION_KEY
LOCKBOX_ENCRYPTION_KEY="$(gen_key)"
start_lockbox
echo "e2e: lockbox at $LOCKBOX_URL"

# Create a password secret.
CREATE_RESP="$(lb_curl "/api/secrets/" -X POST -H 'Content-Type: application/json' -d '{
	"content_type": "password",
	"name": "demo",
	"username": "demo-user",
	"url": "https://demo.example/login",
	"secret_data": {"password": "demo-pass-123"}
}')"
CREATE_STATUS="$(lb_status "/api/secrets/" -X POST -H 'Content-Type: application/json' -d '{
	"content_type": "password",
	"name": "demo2",
	"username": "demo-user2",
	"url": "https://demo.example/login2",
	"secret_data": {"password": "demo-pass-456"}
}')"
assert_eq "create secret status" "201" "$CREATE_STATUS"

KEY="$(echo "$CREATE_RESP" | jq -r .hashid)"
assert_contains "create response has hashid" "hashid" "$CREATE_RESP"

# Read metadata back.
META_RESP="$(lb_curl "/api/secrets/$KEY/")"
assert_eq "metadata username" "demo-user" "$(echo "$META_RESP" | jq -r .username)"
assert_eq "metadata url" "https://demo.example/login" "$(echo "$META_RESP" | jq -r .url)"

# Read the password revision data back (direct URL).
REV_RESP="$(lb_curl "/api/secret-revisions/$KEY/data")"
assert_eq "revision password" "demo-pass-123" "$(echo "$REV_RESP" | jq -r .password)"

# Read the password by FOLLOWING current_revision, exactly as a real TeamVault
# client does: it appends "data" to current_revision. current_revision must end
# at ".../{key}/" (no "/data"), else the client would request ".../datadata".
CR="$(echo "$META_RESP" | jq -r .current_revision)"
assert_contains "current_revision points at revision base" "/secret-revisions/$KEY/" "$CR"
case "$CR" in *"/data") assert_eq "current_revision has no /data suffix" "no-data-suffix" "HAS-DATA-SUFFIX" ;; esac
CR_PATH="/${CR#*://*/}"   # strip scheme+host, keep leading-slash path
REV_VIA_CR="$(lb_curl "${CR_PATH}data")"
assert_eq "revision password via current_revision+data" "demo-pass-123" "$(echo "$REV_VIA_CR" | jq -r .password)"

# Update a metadata field.
UPDATE_STATUS="$(lb_status "/api/secrets/$KEY/" -X PATCH -H 'Content-Type: application/json' \
	-d '{"content_type": "password", "url": "https://demo.example/updated"}')"
assert_eq "update status" "200" "$UPDATE_STATUS"
META_RESP2="$(lb_curl "/api/secrets/$KEY/")"
assert_eq "metadata url after update" "https://demo.example/updated" "$(echo "$META_RESP2" | jq -r .url)"

# Create a second secret, then search for it by name substring.
SEARCH_CREATE_RESP="$(lb_curl "/api/secrets/" -X POST -H 'Content-Type: application/json' -d '{
	"content_type": "password",
	"name": "search-target-secret",
	"username": "search-user",
	"secret_data": {"password": "search-pass"}
}')"
SEARCH_KEY="$(echo "$SEARCH_CREATE_RESP" | jq -r .hashid)"
SEARCH_RESP="$(lb_curl "/api/secrets/?search=search-target")"
assert_contains "search results contain the right api_url" "/api/secrets/$SEARCH_KEY/" \
	"$(echo "$SEARCH_RESP" | jq -r '.results[].api_url' | tr '\n' ' ')"

# Auth required.
NOAUTH_STATUS="$(curl -s -o /dev/null -w '%{http_code}' "$LOCKBOX_URL/api/secrets/$KEY/")"
assert_eq "no-auth status" "401" "$NOAUTH_STATUS"

# The same create/read flow works under the /api/v1 prefix too.
V1_CREATE_RESP="$(lb_curl "/api/v1/secrets/" -X POST -H 'Content-Type: application/json' -d '{
	"content_type": "password",
	"name": "v1-demo",
	"username": "v1-user",
	"secret_data": {"password": "v1-pass-789"}
}')"
V1_KEY="$(echo "$V1_CREATE_RESP" | jq -r .hashid)"
assert_contains "v1 create response api_url uses /api/v1" "/api/v1/secrets/$V1_KEY/" \
	"$(echo "$V1_CREATE_RESP" | jq -r .api_url)"
V1_REV_RESP="$(lb_curl "/api/v1/secret-revisions/$V1_KEY/data")"
assert_eq "v1 revision password" "v1-pass-789" "$(echo "$V1_REV_RESP" | jq -r .password)"

stop_lockbox

######################################################################
# Scenario 002: keyring rotation + reencrypt sweep.
######################################################################
echo "e2e: scenario 002 — keyring rotation"

DATADIR="$WORK_DIR/data-002"
KEY_A="$(gen_key)"
KEY_B="$(gen_key)"

# Start with a single primary key A; create S1.
unset LOCKBOX_ENCRYPTION_KEYS
export LOCKBOX_ENCRYPTION_KEY="$KEY_A"
start_lockbox
S1_CREATE_RESP="$(lb_curl "/api/secrets/" -X POST -H 'Content-Type: application/json' -d '{
	"content_type": "password",
	"name": "rotation-s1",
	"username": "s1-user",
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

S2_CREATE_RESP="$(lb_curl "/api/secrets/" -X POST -H 'Content-Type: application/json' -d '{
	"content_type": "password",
	"name": "rotation-s2",
	"username": "s2-user",
	"secret_data": {"password": "s2-pass"}
}')"
S2_KEY="$(echo "$S2_CREATE_RESP" | jq -r .hashid)"
assert_eq "S2 readable under new primary B" "s2-pass" \
	"$(lb_curl "/api/secret-revisions/$S2_KEY/data" | jq -r .password)"
stop_lockbox

# Run the reencrypt sweep with the same keyring — rewrites everything under
# primary B so retired key A can be dropped.
DATADIR="$DATADIR" LOCKBOX_ENCRYPTION_KEYS="$KEY_B,$KEY_A" "$REENCRYPT"
REENCRYPT_RC=$?
assert_eq "reencrypt sweep exit 0" "0" "$REENCRYPT_RC"

# Restart with ONLY B — A is now retired. Both S1 and S2 must still read,
# proving the sweep rewrote S1 (originally sealed under A) onto B.
unset LOCKBOX_ENCRYPTION_KEYS
export LOCKBOX_ENCRYPTION_KEY="$KEY_B"
start_lockbox
assert_eq "S1 readable with A fully retired (proves reencrypt worked)" "s1-pass" \
	"$(lb_curl "/api/secret-revisions/$S1_KEY/data" | jq -r .password)"
assert_eq "S2 still readable under B-only keyring" "s2-pass" \
	"$(lb_curl "/api/secret-revisions/$S2_KEY/data" | jq -r .password)"
stop_lockbox

scenario_done
