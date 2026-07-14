---
status: completed
summary: Extended store.Search to match query against Name, URL, and Description (case-insensitive substring), added three new test cases, and recorded the change in CHANGELOG.md
execution_id: lockbox-exec-007-teamvault-compatible-search-name-url-description
dark-factory-version: dev
created: "2026-07-14T17:37:50Z"
queued: "2026-07-14T17:37:50Z"
started: "2026-07-14T17:38:07Z"
completed: "2026-07-14T17:39:19Z"
---
<summary>
- Secret search now finds secrets by their human-readable Name, matching how TeamVault's `?search=` works, not just by key and username.
- Search also matches on the secret's URL and Description, so a query hitting any of those fields returns the secret.
- Matching stays case-insensitive substring; an empty query still returns every secret; a query that matches nothing still returns an empty result.
- The HTTP search endpoint, its response shape, and all routes are unchanged — only what the store matches against changes.
- Test coverage is extended to prove matches via Name, URL, and Description, alongside the existing key and username cases.
- A CHANGELOG entry records the broadened search behavior.
</summary>

<objective>
Make Lockbox secret search TeamVault-compatible by extending `store.Search` to match the query against the secret `Name`, `URL`, and `Description` in addition to the key and `Username`, without touching the search handler, response shape, or routes.
</objective>

<context>
Read `/home/node/.claude/CLAUDE.md` and `/workspace/CLAUDE.md`.

Read these coding-plugin guides:
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-error-wrapping-guide.md`
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-testing-guide.md`
- `/home/node/.claude/plugins/marketplaces/coding/docs/changelog-guide.md`

Read these source files fully before editing:
- `/workspace/pkg/secret/store.go` — the `(s *store) Search(ctx context.Context, query string) (Keys, error)` method. It lowercases the query into `needle`, iterates `s.kv.Map`, decrypts each entry via `s.decrypt`, and appends the key when `needle == ""` OR `needle` is a substring of the lowercased `key.String()` OR the lowercased `secret.Username`. Errors are wrapped with `errors.Wrapf(ctx, ...)` from `github.com/bborbe/errors` (already imported). You extend ONLY the match condition.
- `/workspace/pkg/secret/secret.go` — the `Secret` struct. Fields are `Username`, `URL`, `Password`, `File`, `Name`, `Description`, `ContentType` (all `string`). Match against `Name`, `URL`, `Description` in addition to `Username`.
- `/workspace/pkg/secret/store_test.go` — the `Describe("Search", ...)` block (fixtures created in its `BeforeEach` via `store.Upsert`, then `It` blocks for empty-query, key match, username match, no-match). Ginkgo/Gomega, package `secret_test`. You extend this block.
- `/workspace/pkg/handler/secret-search.go` — `NewSecretSearchHandler`. DO NOT modify; read only to confirm it consumes `store.Search` and the `{results:[{api_url}]}` shape is unaffected.
- `/workspace/CHANGELOG.md` — add a bullet under `## Unreleased` (create that section directly under the header line if it does not yet exist; do not disturb `## v0.4.0`).
</context>

<requirements>
1. **Extend the match condition in `(s *store) Search`** in `/workspace/pkg/secret/store.go`. Replace the current two-clause condition:
   ```go
   if needle == "" ||
       strings.Contains(strings.ToLower(key.String()), needle) ||
       strings.Contains(strings.ToLower(secret.Username), needle) {
       result = append(result, key)
   }
   ```
   with a condition that also matches `secret.Name`, `secret.URL`, and `secret.Description`, all lowercased:
   ```go
   if needle == "" ||
       strings.Contains(strings.ToLower(key.String()), needle) ||
       strings.Contains(strings.ToLower(secret.Name), needle) ||
       strings.Contains(strings.ToLower(secret.Username), needle) ||
       strings.Contains(strings.ToLower(secret.URL), needle) ||
       strings.Contains(strings.ToLower(secret.Description), needle) {
       result = append(result, key)
   }
   ```
   Keep the `s.kv.Map` iteration, the per-entry `s.decrypt`, and both `errors.Wrapf(ctx, ...)` wraps exactly as they are. Do NOT introduce `fmt.Errorf`; error wrapping stays `github.com/bborbe/errors`. Empty query must still return all keys.

2. **Do NOT change** the search handler (`pkg/handler/secret-search.go` / `NewSecretSearchHandler`), the `{results:[{api_url}]}` response shape, or any route in `main.go`.

3. **Extend the `Describe("Search", ...)` block** in `/workspace/pkg/secret/store_test.go`:
   a. In the block's `BeforeEach`, give the `GitHubToken` fixture distinctive `Name`, `URL`, and `Description` values so they can be matched independently of the key and username. Update it to:
      ```go
      store.Upsert(ctx, secret.Key("GitHubToken"), secret.Secret{
          Name:        "Production GitHub",
          Username:    "octocat",
          URL:         "https://github.example.com",
          Description: "CI deploy token for the release pipeline",
      })
      ```
      Keep the `AwsKey` (username `root`) and `DbPassword` (username `admin`) fixtures; you may leave them with only a username, but ensure their fields do NOT contain the new query substrings used below (so the match assertions stay unambiguous — `ConsistOf(secret.Key("GitHubToken"))`).
   b. Keep the existing `It` blocks: empty query returns all 3 keys; `"github"` matches the key case-insensitively; `"OCTOCAT"` matches the username case-insensitively; `"nope"` returns empty.
   c. Add an `It` asserting a query matches via `Name` case-insensitively, e.g. `store.Search(ctx, "production github")` → `ConsistOf(secret.Key("GitHubToken"))`. Pick a substring present only in the `Name` (not in the key `GitHubToken` — note `"github"` alone would also match the key, so use a Name-only token like `"production"`).
   d. Add an `It` asserting a query matches via `URL` case-insensitively, e.g. `store.Search(ctx, "GITHUB.EXAMPLE.COM")` → `ConsistOf(secret.Key("GitHubToken"))`.
   e. Add an `It` asserting a query matches via `Description` case-insensitively, e.g. `store.Search(ctx, "release pipeline")` → `ConsistOf(secret.Key("GitHubToken"))`.
   Use Gomega `ConsistOf` / `HaveLen` / `BeEmpty` consistent with the existing block. No bare `interface{}`; use `any` if needed.

4. **CHANGELOG** — add a bullet under `## Unreleased` in `/workspace/CHANGELOG.md` (per `changelog-guide.md`, prefix `feat:`, be specific), e.g.:
   `- feat: search now matches secret name, url and description (in addition to key and username) for TeamVault-compatible \`?search=\` behavior`

5. **README** — if `/workspace/README.md` describes what the search endpoint matches on (beyond the endpoint table row `GET /api/secrets/?search=q`), update that prose to state search now also matches name, url, and description. If the README only lists the endpoint without describing match fields, skip the README (do not invent new prose).
</requirements>

<constraints>
- Change ONLY the match condition in `store.Search`; leave the `s.kv.Map` iteration, `s.decrypt` call, and both `errors.Wrapf` wraps structurally unchanged.
- Do NOT modify the search handler, the `{results:[{api_url}]}` response shape, or any route.
- Empty query must still return all keys; a non-matching query must still return an empty result.
- Wrap errors with `github.com/bborbe/errors`; never `fmt.Errorf`.
- This repo does NOT vendor (`/vendor` is gitignored, Makefile uses `-mod=mod`); never run `go mod vendor` and never pass `-mod=vendor`.
- No cross-repo writes; edit only files under `/workspace`.
- Container-autonomous: file edits + `make` only. No `kubectl`, no deploy, no `gh`, no PR steps.
- Do NOT commit — dark-factory handles git.
- Every existing test must still pass.
</constraints>

<verification>
Run in `/workspace`:

```
make test
make precommit
```

Both must exit 0. Then confirm the new matching is exercised:

```
go test -coverprofile=/tmp/cover.out ./pkg/secret/... && go tool cover -func=/tmp/cover.out | grep Search
```

(Do NOT add `-mod=vendor`.) Also confirm the store change and the new test cases are present:

```
grep -n 'secret.Name\|secret.URL\|secret.Description' pkg/secret/store.go   # expect the 3 new match clauses
grep -n 'Production GitHub\|github.example.com\|release pipeline' pkg/secret/store_test.go   # expect the new fixtures/cases
grep -n 'Unreleased' CHANGELOG.md
```

Confirm the handler and response shape are untouched: `git diff --stat` (informational only, do NOT commit) must show no changes under `pkg/handler/` or to `main.go`.
</verification>
