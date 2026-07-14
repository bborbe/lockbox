---
status: completed
spec: [002-keyring-key-rotation]
summary: Documented keyring, rotation-by-restart flow, and cmd/reencrypt in README.md, updated example.env with multi-key example, and appended docs bullet to CHANGELOG.md Unreleased section
execution_id: lockbox-exec-011-spec-002-docs-and-contract-check
dark-factory-version: dev
created: "2026-07-14T19:50:07Z"
queued: "2026-07-14T20:02:53Z"
started: "2026-07-14T20:23:05Z"
completed: "2026-07-14T20:25:06Z"
branch: dark-factory/keyring-key-rotation
---

<summary>
- Documents the new multi-key encryption configuration and how to rotate the master key by restarting.
- Documents the one-shot re-encrypt command operators run to migrate every secret onto the current key so old keys can be retired.
- Updates the example environment file to show both the single-key and the multi-key configuration options.
- Adds a changelog entry describing the keyring / key-rotation feature.
- Confirms the read/write HTTP handlers and their existing contract assertions were left untouched by the whole feature.
</summary>

<objective>
Document the keyring in `README.md` (the `LOCKBOX_ENCRYPTION_KEYS` variable, the rotation-by-restart flow, and the re-encrypt command), update `example.env`, add a `CHANGELOG.md` entry, and confirm the read/write handler contracts are unchanged.
</objective>

<context>
Read `/home/node/.claude/CLAUDE.md` and `/workspace/CLAUDE.md`.

Read these coding-plugin guides (in-container paths):
- `/home/node/.claude/plugins/marketplaces/coding/docs/changelog-guide.md` — entry format, prefixes, `## Unreleased` rules.

Read these source files fully before editing:
- `/workspace/README.md` — the `## Components` table (add the `cmd/reencrypt` row) and the `## Encryption at rest` section (extend it with the keyring, rotation, and re-encrypt content). The current text documents only `LOCKBOX_ENCRYPTION_KEY`.
- `/workspace/example.env` — the single `LOCKBOX_ENCRYPTION_KEY` export you extend with a commented `LOCKBOX_ENCRYPTION_KEYS` example.
- `/workspace/CHANGELOG.md` — the top of the file. If a `## Unreleased` section already exists (created by an earlier prompt in this spec), APPEND to it; otherwise create it above the latest released version entry.
- `/workspace/cmd/reencrypt/main.go` — the command shipped in prompt 3 (its env vars and behavior are what you document). If it is absent, prompt 3 has not shipped: STOP and report `Status: failed` with `"prompt 3 (cmd/reencrypt) not yet deployed"`.
- `/workspace/main.go` — the `LOCKBOX_ENCRYPTION_KEYS` field wired in prompt 2 (its `usage` string is the source of truth for the doc wording).
- `/workspace/pkg/handler/` — the read/write handlers; this prompt does NOT modify any of them. You only confirm they are untouched.

The spec Acceptance Criteria require: `grep -n 'LOCKBOX_ENCRYPTION_KEYS' README.md` ≥1, `grep -n -i 'rotat' README.md` ≥1, and the `CHANGELOG.md` top entry mentions key rotation / keyring. The read/write handler source files and the existing `main_test.go` contract assertion blocks must be byte-identical to master (AC 11).
</context>

<requirements>
1. **README — extend `## Encryption at rest`.** Keep the existing single-key paragraph and add content covering:
   - **The keyring / multi-key config:** `LOCKBOX_ENCRYPTION_KEYS` is a comma-separated list of base64 AES keys (16 or 32 raw bytes each), **primary first**. New secrets are sealed under the primary key and tagged with a key identifier; any secret is read back under the key that sealed it. Exactly one of `LOCKBOX_ENCRYPTION_KEY` (single-key shorthand) or `LOCKBOX_ENCRYPTION_KEYS` must be set; setting both or neither refuses start, as does any key that is not valid base64 decoding to 16 or 32 bytes, or a duplicate key.
   - **Back-compat:** the single `LOCKBOX_ENCRYPTION_KEY` still works and boots an equivalent one-entry keyring; existing deployments need no change. Legacy ciphertext written before the keyring stays readable in place with no rewrite.
   - **Rotation by restart:** to rotate the master key, PREPEND a new key to `LOCKBOX_ENCRYPTION_KEYS` (making it the new primary) and restart. New writes adopt the new key while every previously stored secret still decrypts under its original, still-present key — no downtime, no data loss, no rewrite at rotation time. (The word "rotate"/"rotation" MUST appear — AC requires `grep -i 'rotat' README.md` ≥1.)
   - **Re-encrypt sweep:** the one-shot `cmd/reencrypt` command reads every secret and rewrites it under the current primary key, so once it completes no secret references a retired key and the old keys can be removed from `LOCKBOX_ENCRYPTION_KEYS`. It opens the same `DATADIR` and reads the same `LOCKBOX_ENCRYPTION_KEY`/`LOCKBOX_ENCRYPTION_KEYS` as the server; it is idempotent and safe to re-run after an interruption. Show an invocation example, e.g.:
     ```bash
     DATADIR=/data LOCKBOX_ENCRYPTION_KEYS="$NEW,$OLD" go run ./cmd/reencrypt
     ```
     Document the operator flow as an ordered list: (1) prepend the new key, restart the server; (2) run `cmd/reencrypt` to migrate existing secrets onto the new key; (3) remove the retired key from `LOCKBOX_ENCRYPTION_KEYS`, restart.

2. **README — `## Components` table.** Add a row for the new command:
   ```
   | `cmd/reencrypt/` | One-shot re-encrypt sweep — rewrites every stored secret under the current primary key so retired keys can be removed |
   ```

3. **example.env.** Below the existing `LOCKBOX_ENCRYPTION_KEY` export, add a commented block showing the multi-key alternative and the exactly-one rule, e.g.:
   ```
   # Alternatively, configure a keyring of ordered keys (primary first) for
   # rotation. Set EXACTLY ONE of LOCKBOX_ENCRYPTION_KEY or LOCKBOX_ENCRYPTION_KEYS.
   # export LOCKBOX_ENCRYPTION_KEYS=<new-base64-key>,<old-base64-key>
   ```
   Leave the existing single-key `export LOCKBOX_ENCRYPTION_KEY=...` line active (do not comment it out) so the default single-key setup keeps working.

4. **CHANGELOG.** Add (or append to) the `## Unreleased` section at the top with prefixed bullets per `changelog-guide.md`. The top entry MUST mention key rotation / keyring. Suggested bullets (adjust to what the earlier prompts already recorded; do not duplicate existing bullets):
   - `- feat: Encrypt stored secrets through a keyring of ordered AES keys (primary first) with a content-derived key-id per ciphertext, enabling zero-downtime encryption-key rotation`
   - `- feat: Accept LOCKBOX_ENCRYPTION_KEYS (comma-separated base64 keys, primary first) alongside the single-key LOCKBOX_ENCRYPTION_KEY; exactly one must be set or the server refuses to start`
   - `- feat: Add cmd/reencrypt one-shot sweep that rewrites every stored secret under the current primary key so retired keys can be removed`
   - `- docs: Document the keyring, rotation-by-restart flow, and cmd/reencrypt in README and example.env`

5. **Confirm handler contracts untouched (AC 11).** Do NOT edit anything under `/workspace/pkg/handler/`, and do NOT edit the read/write contract assertion blocks in `/workspace/main_test.go`. This prompt only touches `README.md`, `example.env`, and `CHANGELOG.md`. Verify (see `<verification>`) that no handler source file was modified across the whole feature branch.
</requirements>

<constraints>
- Do NOT change the HTTP API shape, routes, auth, or the read/write handlers' request/response contracts (spec Non-goals; AC 11). This prompt is docs-only plus a no-change verification.
- README MUST document `LOCKBOX_ENCRYPTION_KEYS`, the rotation flow (the token "rotat" must appear), and the re-encrypt command. CHANGELOG top entry MUST mention the keyring / key rotation.
- Keep the existing single-key `LOCKBOX_ENCRYPTION_KEY` documentation and `example.env` export intact — the single-key path is still fully supported.
- Follow `changelog-guide.md`: every bullet has a prefix (`feat:`/`docs:`), is specific (names the env vars / command), and is not the prompt filename. If `## Unreleased` already has bullets from prompts 1-3, append rather than replace.
- Do NOT commit — dark-factory handles git.
- Every existing test must still pass (this prompt changes only Markdown/env; `make precommit` should be unaffected by the doc edits themselves).
</constraints>

<verification>
Run in `/workspace`:

```
grep -n 'LOCKBOX_ENCRYPTION_KEYS' README.md   # expect >=1 (AC)
grep -n -i 'rotat' README.md                  # expect >=1 (AC)
grep -n 'cmd/reencrypt' README.md             # command documented
grep -n 'LOCKBOX_ENCRYPTION_KEYS' example.env # multi-key example present
grep -n -iE 'keyring|rotat' CHANGELOG.md      # top entry mentions keyring/rotation
grep -n 'Unreleased' CHANGELOG.md             # unreleased section present
```

Confirm no handler source and no contract assertion block was touched by this feature. The feature branch is `dark-factory/keyring-key-rotation`; confirm the handler package files are byte-identical to master by checking none were modified since the branch point:

```
find pkg/handler -name '*.go' -newer main_test.go   # expect: any listed file is only a TEST touched by an unrelated change; handler *.go source must NOT appear
```

Then run the full gate:

```
make test
make precommit
```

`make precommit` must exit 0.

Operator-side confirmation (run outside the container, not required for prompt pass): `git diff master -- pkg/handler` shows no changes, and the pre-existing contract assertion blocks in `main_test.go` are byte-identical.
</verification>
