#!/usr/bin/env bash
# Copyright (c) 2026 Benjamin Borbe All rights reserved.
# Use of this source code is governed by a BSD-style
# license that can be found in the LICENSE file.
#
# Shared helpers for lockbox scenarios and `make e2e`. Mirrors the convention
# in sm-teamvault-cli (build_binaries + assert_* + scenario_done). Starts the
# real lockbox server (and, for rotation scenarios, the reencrypt sweep binary)
# against a temp data dir — no live TeamVault, no network.
#
# Usage:
#   source scenarios/helper/lib.sh
#   build_binaries
#   LOCKBOX_ENCRYPTION_KEY="$(gen_key)" start_lockbox
#   assert_eq "desc" "expected" "$actual"
#   assert_exit_nonzero "desc" some-command --with args
#   scenario_done

# Repo root, derived from this file's location (works in worktrees too).
LB_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
_FAIL=0
LB_PID=""

_cleanup() {
	[ -n "$LB_PID" ] && kill "$LB_PID" 2>/dev/null || true
	[ -n "${WORK_DIR:-}" ] && rm -rf "$WORK_DIR"
}
trap _cleanup EXIT

# build_binaries builds lockbox + reencrypt into a temp dir and sets $LOCKBOX /
# $REENCRYPT. WORK_DIR is created once and reused for the whole scenario run so
# DATADIR can persist across server restarts.
build_binaries() {
	WORK_DIR="$(mktemp -d)"
	go build -C "$LB_ROOT" -o "$WORK_DIR/lockbox" . || exit 1
	go build -C "$LB_ROOT" -o "$WORK_DIR/reencrypt" ./cmd/reencrypt || exit 1
	# shellcheck disable=SC2034 # used by the scenario script that sources this file
	LOCKBOX="$WORK_DIR/lockbox"
	# shellcheck disable=SC2034 # used by the scenario script that sources this file
	REENCRYPT="$WORK_DIR/reencrypt"
}

# gen_key prints a fresh base64-encoded 32-byte AES key, valid for
# LOCKBOX_ENCRYPTION_KEY / LOCKBOX_ENCRYPTION_KEYS.
gen_key() {
	head -c 32 /dev/urandom | base64
}

# start_lockbox launches the real lockbox server in the background. Honours
# LOCKBOX_ENCRYPTION_KEY / LOCKBOX_ENCRYPTION_KEYS already exported by the
# caller (exactly one must be set — same contract as main.go). DATADIR
# defaults to a fixed dir under $WORK_DIR so it persists across restarts
# (rotation scenarios stop/start against the same store); pass DATADIR="..."
# beforehand to override. Picks a random high port, waits for /healthz, and
# exports LOCKBOX_URL.
start_lockbox() {
	DATADIR="${DATADIR:-$WORK_DIR/data}"
	mkdir -p "$DATADIR"

	local port=$((20000 + RANDOM % 20000))
	local listen="127.0.0.1:$port"
	LOCKBOX_URL="http://$listen"

	DATADIR="$DATADIR" LISTEN="$listen" BASIC_AUTH_USER=test BASIC_AUTH_PASS=test \
		"$LOCKBOX" >"$WORK_DIR/lockbox.log" 2>&1 &
	LB_PID=$!

	local ok=""
	for _ in $(seq 1 100); do
		if curl -s -o /dev/null -w '%{http_code}' "$LOCKBOX_URL/healthz" 2>/dev/null | grep -q '^200$'; then
			ok=1
			break
		fi
		sleep 0.1
	done
	if [ -z "$ok" ]; then
		echo "lockbox did not start"
		cat "$WORK_DIR/lockbox.log"
		exit 1
	fi
}

# stop_lockbox kills the running server and waits for it to exit, so a
# following start_lockbox against the same DATADIR never races the old
# process's bolt file lock.
stop_lockbox() {
	[ -n "$LB_PID" ] || return 0
	kill "$LB_PID" 2>/dev/null || true
	wait "$LB_PID" 2>/dev/null || true
	LB_PID=""
}

# lb_curl runs curl with the Basic-auth test credentials against $LOCKBOX_URL.
# Usage: lb_curl <path> [curl-args...]  (path is appended to $LOCKBOX_URL)
lb_curl() {
	local path="$1"
	shift
	curl -s -u test:test "$@" "$LOCKBOX_URL$path"
}

# lb_status is lb_curl but prints only the HTTP status code (body discarded).
lb_status() {
	local path="$1"
	shift
	curl -s -o /dev/null -w '%{http_code}' -u test:test "$@" "$LOCKBOX_URL$path"
}

# assert_eq <desc> <expected> <actual>
assert_eq() {
	if [ "$2" = "$3" ]; then
		echo "  ok: $1"
	else
		echo "  FAIL: $1 — expected '$2' got '$3'"
		_FAIL=1
	fi
}

# assert_exit_nonzero <desc> <command...>
assert_exit_nonzero() {
	local desc="$1"
	shift
	if "$@" >/dev/null 2>&1; then
		echo "  FAIL: $desc — expected non-zero exit"
		_FAIL=1
	else
		echo "  ok: $desc"
	fi
}

# assert_contains <desc> <needle> <haystack>
assert_contains() {
	case "$3" in
	*"$2"*) echo "  ok: $1" ;;
	*)
		echo "  FAIL: $1 — '$2' not found in: $3"
		_FAIL=1
		;;
	esac
}

# scenario_done reports and exits non-zero if any assertion failed.
scenario_done() {
	if [ "$_FAIL" -eq 0 ]; then
		echo "e2e: PASS"
	else
		echo "e2e: FAIL"
		exit 1
	fi
}
