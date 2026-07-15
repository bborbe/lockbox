---
status: completed
spec: [003-search-name-compat]
summary: Made Lockbox search endpoint TeamVault-compatible by changing Store.Search to return per-match SearchRecords, adding count/next/previous/results envelope, and updating mock, tests, contract test, scenario, README and CHANGELOG
execution_id: lockbox-search-name-exec-013-spec-003-search-envelope-and-records
dark-factory-version: dev
created: "2026-07-15T21:20:00Z"
queued: "2026-07-15T21:27:21Z"
started: "2026-07-15T21:28:52Z"
completed: "2026-07-15T21:31:27Z"
branch: dark-factory/search-name-compat
---
<summary>
- Search (`GET /api/secrets/?search=q`) now returns a `{count, next, previous, results}` envelope instead of a bare `{results}` list, matching real TeamVault's paginated shape (single page; `next`/`previous` are always null).
- Each search result now carries the secret's `hashid`, `name`, `username`, and `url` in addition to the pre-existing absolute `api_url`, so `teamvault-cli search` can display real secret names instead of bare keys.
- The store's search operation now returns per-match records (key, name, username, url) instead of a bare key list, reusing the decrypt it already performs — no extra decryption or lookups.
- The change is additive: `api_url` stays on every result, so old clients that only read `api_url` keep working; the new fields are extra keys they ignore.
- Zero matches returns `{count: 0, results: []}`; a decrypt failure during search still returns a non-200 error with no secret value in the body (existing behavior preserved).
- No secret value (`password`, file content) is exposed — only `name`/`username`/`url` metadata crosses the wire.
- The `SecretStore` mock, the store unit tests, the TeamVault contract test, the e2e scenario, README, and CHANGELOG are all updated in lockstep so the build and frozen-contract check stay green.
</summary>

<objective>
Make Lockbox's search endpoint TeamVault-compatible by (a) changing `store.Search` to return per-match records carrying key/name/username/url instead of a bare key list, (b) wrapping the HTTP search response in a `{count, next, previous, results}` envelope whose results each carry `hashid`, `name`, `username`, `url`, and the pre-existing `api_url`, and (c) regenerating the `SecretStore` mock, store tests, contract test, scenario, and docs so everything compiles and the frozen-contract check passes. The store signature change, the mock regeneration, and the handler that consumes the new records must land together in this one prompt or the build breaks.
</objective>

<context>
Read `/home/node/.claude/CLAUDE.md` and `/workspace/CLAUDE.md` (if present) for project conventions.

Read these coding-plugin guides (in-container paths):
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-error-wrapping-guide.md`
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-testing-guide.md`
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-mocking-guide.md`
- `/home/node/.claude/plugins/marketplaces/coding/docs/changelog-guide.md`
- `/home/node/.claude/plugins/marketplaces/coding/docs/teamvault-conventions.md`

Read `/workspace/docs/dod.md` — the "TeamVault API compatibility (frozen contract)" and "Secret handling" clauses. This spec's waiver is scoped to exactly the search envelope + per-result `hashid`/`name`/`username`/`url`.

Read these source files fully before editing:

- `/workspace/pkg/secret/store.go` — contains the counterfeiter directive and the `Store` interface. The relevant pieces:
  ```go
  //counterfeiter:generate -o ../../mocks/secret-store.go --fake-name SecretStore . Store

  type Store interface {
      Upsert(ctx context.Context, key Key, secret Secret) error
      Create(ctx context.Context, key Key, secret Secret) error
      Get(ctx context.Context, key Key) (*Secret, error)
      // Search returns the keys whose key or username contains query
      // (case-insensitive). An empty query matches every secret.
      Search(ctx context.Context, query string) (Keys, error)
      ReEncrypt(ctx context.Context) error
  }
  ```
  The `(s *store) Search` implementation lowercases the query into `needle`, iterates `s.kv.Map`, decrypts each entry via `s.decrypt` into `secret`, and on a match does `result = append(result, key)`. It matches on key, `secret.Name`, `secret.Username`, `secret.URL`, `secret.Description` (all lowercased). `ReEncrypt` calls `s.Search(ctx, "")` and iterates the returned value with `for _, key := range keys` then `s.Get(ctx, key)`.

- `/workspace/pkg/secret/secret.go` — the `Secret` struct. Existing `string` fields: `Username`, `URL`, `Password`, `File`, `Name`, `Description`, `ContentType`. `Key` is a `string` type with a `String()` method. No change to this file.

- `/workspace/pkg/handler/secret-search.go` — `NewSecretSearchHandler`. It reads `query := req.URL.Query().Get("search")`, calls `keys, err := store.Search(ctx, query)`, wraps errors with `errors.Wrapf(ctx, err, "search secrets for %q failed", query)`, builds `prefix := apiPrefix(req, "secrets/")`, then for each key appends `api.SearchResult{APIURL: absoluteURL(req, prefix+"secrets/"+key.String()+"/")}` and returns `api.SearchResults{Results: results}`. You rewrite the loop to consume records and build the enriched envelope.

- `/workspace/pkg/handler/url.go` — helpers `apiPrefix(req, suffix)` and `absoluteURL(req, path)`. Reuse them unchanged; do NOT modify this file.

- `/workspace/pkg/api/response.go` — the `SearchResults` and `SearchResult` DTOs:
  ```go
  // SearchResults is the body of GET /api/secrets/?search=q.
  type SearchResults struct {
      Results []SearchResult `json:"results"`
  }

  type SearchResult struct {
      APIURL string `json:"api_url"`
  }
  ```
  You extend `SearchResults` into a `{count, next, previous, results}` envelope and extend `SearchResult` with `hashid`, `name`, `username`, `url` (keeping `api_url`).

- `/workspace/pkg/api/write.go` — the `SecretRepresentation` DTO orders fields `hashid, api_url, content_type, name, username, url, description`. Follow that field-order convention for the new `SearchResult` fields.

- `/workspace/mocks/secret-store.go` — the counterfeiter-generated `SecretStore` mock. `make precommit` regenerates it via `make generate` (`go generate -mod=mod ./...`). When `Store.Search`'s return type changes, regeneration updates the mock's `SearchReturns`/`SearchStub`/`searchReturns` to the new type. Read the `Search*` methods to understand what changes.

- `/workspace/pkg/handler/secret-search_test.go` — the handler unit test (external `handler_test`, `mocks.SecretStore`). It currently does `store.SearchReturns(secret.Keys{"AbC123", "DeF456"}, nil)` and asserts `body.Results[0].APIURL`. You rewrite it to stub the new record type and assert the enriched envelope + fields.

- `/workspace/pkg/secret/store_test.go` — the `Describe("Search", ...)` block. Its `BeforeEach` upserts `GitHubToken` (Name "Production GitHub", Username "octocat", URL "https://github.example.com", Description "..."), `AwsKey` (Username "root"), `DbPassword` (Username "admin"). The existing `It` blocks assert `keys` via `HaveLen`, `ConsistOf(secret.Key("GitHubToken"))`, `BeEmpty`. You update these to the new record return type and add an assertion on a record's name/username/url.

- `/workspace/main_test.go` — the TeamVault contract test. There is currently NO search assertion block. You ADD a search `Context` that POSTs a secret with a distinctive name, GETs `/api/secrets/?search=<term>`, and asserts the `{count, results}` envelope and a result's `name`. Follow the existing `doJSON(router, method, path, auth, body)` helper and `prefix` variable used throughout the file.

- `/workspace/scenarios/001-core-api-e2e.md` — the e2e scenario. Its search block POSTs a secret named `search-target-secret` and asserts `.results[].api_url` contains the key. You add a jq assertion on `.results[].name`.

- `/workspace/README.md` — the endpoint table row `| \`GET /api/secrets/?search=q\` | \`{results: [{api_url}]}\` |`. Update to the new envelope shape.

- `/workspace/CHANGELOG.md` — add a bullet under `## Unreleased` (create it below the preamble and above `## v0.7.0` if prompt 1 did not already create it; if it exists, append).
</context>

<requirements>
1. **Add a store record type and change the `Store.Search` return type** in `/workspace/pkg/secret/store.go`.
   a. Add a record type and its list type near the existing `Keys` type. Name the record `SearchRecord` and the list `SearchRecords`:
      ```go
      // SearchRecord is one search match: the secret's Key plus the metadata
      // fields (Name, Username, URL) needed to render a TeamVault search result.
      // It deliberately carries no secret value (Password/File).
      type SearchRecord struct {
          Key      Key
          Name     string
          Username string
          URL      string
      }

      // SearchRecords is a list of search matches.
      type SearchRecords []SearchRecord
      ```
   b. Change the `Store` interface `Search` signature and doc comment:
      ```go
      // Search returns a record (key, name, username, url) for every secret whose
      // key, name, username, url, or description contains query (case-insensitive).
      // An empty query matches every secret. No secret value is returned.
      Search(ctx context.Context, query string) (SearchRecords, error)
      ```
   c. Rewrite the `(s *store) Search` body to build `SearchRecords`. Keep the `needle`, `s.kv.Map`, `s.decrypt`, the exact multi-clause match condition (key/Name/Username/URL/Description), and both `errors.Wrapf(ctx, ...)` wraps. On a match, append a record built from the decrypted secret:
      ```go
      needle := strings.ToLower(query)
      result := SearchRecords{}
      err := s.kv.Map(ctx, func(ctx context.Context, key Key, encrypted []byte) error {
          secret, err := s.decrypt(ctx, encrypted)
          if err != nil {
              return errors.Wrapf(ctx, err, "decrypt secret %s failed", key)
          }
          if needle == "" ||
              strings.Contains(strings.ToLower(key.String()), needle) ||
              strings.Contains(strings.ToLower(secret.Name), needle) ||
              strings.Contains(strings.ToLower(secret.Username), needle) ||
              strings.Contains(strings.ToLower(secret.URL), needle) ||
              strings.Contains(strings.ToLower(secret.Description), needle) {
              result = append(result, SearchRecord{
                  Key:      key,
                  Name:     secret.Name,
                  Username: secret.Username,
                  URL:      secret.URL,
              })
          }
          return nil
      })
      if err != nil {
          return nil, errors.Wrapf(ctx, err, "search secrets for %q failed", query)
      }
      return result, nil
      ```
   d. Update `(s *store) ReEncrypt` — it calls `keys, err := s.Search(ctx, "")` then `for _, key := range keys { ... s.Get(ctx, key) ... }`. Since `Search` now returns `SearchRecords`, iterate records and use `record.Key`:
      ```go
      records, err := s.Search(ctx, "")
      if err != nil {
          return errors.Wrapf(ctx, err, "re-encrypt: list secrets failed")
      }
      for _, record := range records {
          select {
          case <-ctx.Done():
              return errors.Wrapf(ctx, ctx.Err(), "re-encrypt cancelled")
          default:
          }
          sec, err := s.Get(ctx, record.Key)
          if err != nil {
              return errors.Wrapf(ctx, err, "re-encrypt: read secret %s failed", record.Key)
          }
          if err := s.Upsert(ctx, record.Key, *sec); err != nil {
              return errors.Wrapf(ctx, err, "re-encrypt: rewrite secret %s failed", record.Key)
          }
      }
      return nil
      ```
      Keep the `select`/`ctx.Done()` cancellation check. Do NOT remove the `Keys` type — leave it in place; other code and the mock's `Create`/`Get`/`Upsert` args still reference `Key`, and `Keys` may be used elsewhere. (Only `Search` changes its return type.)

2. **Extend the search DTOs** in `/workspace/pkg/api/response.go`.
   a. Turn `SearchResults` into the envelope. `next`/`previous` are always null single-page; use `*string` so they marshal to JSON `null`:
      ```go
      // SearchResults is the body of GET /api/secrets/?search=q. It is a single
      // page: count is the number of matches, next and previous are always null
      // (Lockbox does not paginate), results is the array of matches.
      type SearchResults struct {
          Count    int            `json:"count"`
          Next     *string        `json:"next"`
          Previous *string        `json:"previous"`
          Results  []SearchResult `json:"results"`
      }
      ```
   b. Extend `SearchResult` with `hashid`, `name`, `username`, `url`, keeping `api_url`. Order fields per the `SecretRepresentation` convention (hashid, api_url, name, username, url):
      ```go
      // SearchResult is one entry in a SearchResults page. It carries the secret's
      // metadata (name, username, url) plus its hashid and the absolute api_url of
      // its metadata endpoint. It carries no secret value.
      type SearchResult struct {
          Hashid   string `json:"hashid"`
          APIURL   string `json:"api_url"`
          Name     string `json:"name"`
          Username string `json:"username"`
          URL      string `json:"url"`
      }
      ```

3. **Rewrite the search handler** in `/workspace/pkg/handler/secret-search.go` to consume `SearchRecords` and build the envelope. Keep the `query` read, the `store.Search` call, the error wrap, and `apiPrefix`/`absoluteURL`:
   ```go
   query := req.URL.Query().Get("search")
   records, err := store.Search(ctx, query)
   if err != nil {
       return nil, errors.Wrapf(ctx, err, "search secrets for %q failed", query)
   }
   prefix := apiPrefix(req, "secrets/")
   results := make([]api.SearchResult, 0, len(records))
   for _, record := range records {
       results = append(results, api.SearchResult{
           Hashid:   record.Key.String(),
           APIURL:   absoluteURL(req, prefix+"secrets/"+record.Key.String()+"/"),
           Name:     record.Name,
           Username: record.Username,
           URL:      record.URL,
       })
   }
   return api.SearchResults{
       Count:   len(results),
       Results: results,
   }, nil
   ```
   Leave `Next`/`Previous` as their zero value (`nil` `*string`), which marshals to `null`. On zero matches, `results` is a non-nil empty slice (`make(..., 0, 0)`), so the body is `{"count":0,"next":null,"previous":null,"results":[]}`.

4. **Regenerate the `SecretStore` mock** in `/workspace/mocks/secret-store.go` so it matches the new `Search` signature. Run `make generate` (which does `go generate -mod=mod ./...`) — do NOT hand-edit the generated file. After regeneration, confirm the mock's `Search` returns `(secret.SearchRecords, error)` and `SearchReturns` accepts `secret.SearchRecords`. `make precommit` also runs `generate`, but regenerate early so `make test` compiles.

5. **Update the store unit test** `Describe("Search", ...)` in `/workspace/pkg/secret/store_test.go`. The `Search` call now returns `SearchRecords`:
   a. Update the existing `It` blocks to the new type. For counts use `HaveLen`; for the "matches GitHubToken" cases assert on the record's `Key` — e.g.:
      ```go
      records, err := store.Search(ctx, "github")
      Expect(err).To(BeNil())
      Expect(records).To(HaveLen(1))
      Expect(records[0].Key).To(Equal(secret.Key("GitHubToken")))
      ```
      Keep the empty-query (all 3), key/username/name/url/description match, and no-match (`BeEmpty`) cases. Adjust `ConsistOf(secret.Key("GitHubToken"))` assertions to record-based equivalents.
   b. Add an `It` (or extend an existing match `It`) asserting the returned record carries the stored metadata (spec AC): for the `GitHubToken` match, assert
      ```go
      Expect(records[0].Name).To(Equal("Production GitHub"))
      Expect(records[0].Username).To(Equal("octocat"))
      Expect(records[0].URL).To(Equal("https://github.example.com"))
      ```
   Use `any`, never bare `interface{}`.

6. **Rewrite the search handler unit test** in `/workspace/pkg/handler/secret-search_test.go`:
   a. In the success case, stub records instead of keys:
      ```go
      store.SearchReturns(secret.SearchRecords{
          {Key: "AbC123", Name: "prod", Username: "alice", URL: "https://a.example"},
          {Key: "DeF456", Name: "stage", Username: "bob", URL: "https://b.example"},
      }, nil)
      ```
   b. Decode into `api.SearchResults` and assert the envelope: `Expect(body.Count).To(Equal(2))`, `Expect(body.Next).To(BeNil())`, `Expect(body.Previous).To(BeNil())`, `Expect(body.Results).To(HaveLen(2))`.
   c. Assert per-result fields for the first result: `Hashid == "AbC123"`, `APIURL == "http://example.com/api/secrets/AbC123/"`, `Name == "prod"`, `Username == "alice"`, `URL == "https://a.example"`.
   d. Keep the store-args assertion (`_, query := store.SearchArgsForCall(0); Expect(query).To(Equal("ab"))`).
   e. Keep the "store fails → non-200" case; it stubs `store.SearchReturns(nil, errors.New("boom"))` — `nil` is a valid `secret.SearchRecords`, so only update the type if the compiler requires it.
   f. Add a zero-match case: `store.SearchReturns(secret.SearchRecords{}, nil)`, then assert the JSON body serializes `results` as `[]` and NOT `null` — decode into `api.SearchResults` and `Expect(body.Count).To(Equal(0))` + `Expect(body.Results).NotTo(BeNil())` + `Expect(body.Results).To(HaveLen(0))`, AND assert the raw response body contains the substring `"results":[]` (not `"results":null`). This locks the non-nil-empty-slice contract from failure-mode row 2 against a future refactor that returns a nil slice.

7. **Add a search contract block to `/workspace/main_test.go`.** Add a new `Context("GET /secrets/?search=q", ...)` inside the same `Describe` the other contract contexts live in (use the `router`, `prefix`, and `doJSON` helper already in the file). It must: POST a password secret with a distinctive `Name` (e.g. `"searchable-name"`) and `Username`, capture its `hashid`, GET `prefix+"/secrets/?search=searchable"`, decode the body into `map[string]any`, and assert the envelope + a result's name:
   ```go
   Expect(searchBody).To(HaveKey("count"))
   Expect(searchBody).To(HaveKey("next"))
   Expect(searchBody).To(HaveKey("previous"))
   Expect(searchBody).To(HaveKey("results"))
   Expect(searchBody["count"]).To(BeNumerically(">=", 1))
   results, ok := searchBody["results"].([]any)
   Expect(ok).To(BeTrue())
   // find the created secret's result by hashid and assert its name
   ```
   Iterate `results`, find the object whose `hashid` equals the created key, and assert its `name` equals `"searchable-name"`, its `api_url` has suffix `"/secrets/<hashid>/"`, and it has a `username` and `url` key. Do NOT assert secret-value keys (`password`, `file`) are present — assert they are ABSENT from each result object (negative evidence that no secret value leaks). Use `any`, never bare `interface{}`.

8. **Update scenario 001** `/workspace/scenarios/001-core-api-e2e.md`. The search block creates a secret named `search-target-secret` and asserts `.results[].api_url`. Add a jq assertion that `.results[].name` contains `search-target-secret`, e.g. after the existing `api_url` assertion:
   ```bash
   assert_contains "search results contain the secret name" "search-target-secret" \
       "$(echo "$SEARCH_RESP" | jq -r '.results[].name' | tr '\n' ' ')"
   ```
   Keep the existing `api_url` assertion. Do NOT change unrelated scenario steps.

9. **Update the README endpoint table** in `/workspace/README.md`. Change the search row shape from `{results: [{api_url}]}` to the new envelope, e.g. `{count, next, previous, results: [{hashid, api_url, name, username, url}]}`. Do not alter the detail row (prompt 1 owns it) or other rows.

10. **Add a CHANGELOG entry** under `## Unreleased` in `/workspace/CHANGELOG.md` (create the section below the preamble and above `## v0.7.0` if absent; otherwise append). Use a `feat:` bullet, e.g.:
    `- feat: GET /api/secrets/?search=q now returns a \`{count, next, previous, results}\` envelope where each result carries \`hashid\`, \`name\`, \`username\`, \`url\` alongside the existing \`api_url\` (TeamVault-compatible search shape); \`teamvault-cli search\` can now show Lockbox secret names`
</requirements>

<constraints>
- Frozen-contract waiver is scoped to exactly the search envelope + per-result `hashid`/`name`/`username`/`url`. No other route, auth, or JSON shape may change. See `docs/dod.md` "TeamVault API compatibility (frozen contract)".
- Do NOT remove, rename, or retype any existing field: `api_url` stays in every search result; the detail body (prompt 1's scope) is untouched here.
- Do NOT implement real `next`/`previous` pagination — they are always `null` (single page); `count` is the match count.
- Do NOT return secret values: no `password` or file content in any search result or error body. `name`/`username`/`url` are the only newly-exposed fields. See `docs/dod.md` "Secret handling".
- Preserve existing search error behavior: a decrypt failure during search still surfaces a non-200 from the handler with no secret value in the error string (the existing `errors.Wrapf` wraps carry only the key, not plaintext).
- Preserve the `ReEncrypt` context-cancellation `select`/`ctx.Done()` check when switching it to iterate records.
- The `store.Search` signature change, the mock regeneration (`make generate`), and the handler + all callers must land together in THIS prompt — a partial change breaks the build.
- Do NOT hand-edit `mocks/secret-store.go`; regenerate it via `make generate`.
- Wrap errors with `github.com/bborbe/errors`; never `fmt.Errorf`.
- Tests use Ginkgo v2 / Gomega with Counterfeiter mocks in external `_test` packages.
- This repo does NOT vendor (`/vendor` is gitignored, Makefile uses `-mod=mod`); never run `go mod vendor` and never pass `-mod=vendor`.
- Container-autonomous: file edits + `make` only. No `kubectl`, no `docker`, no `gh`, no PR/deploy steps. The e2e scenario runs on the operator side via `make e2e`, NOT inside this container — only edit the scenario markdown; do not run it here.
- Do NOT commit — dark-factory handles git.
- Every existing test must still pass.
</constraints>

<verification>
Run in `/workspace`:

```
make generate
make test
make precommit
```

All must exit 0 (`make precommit` re-runs generate + full lint + trivy). Confirm the store record type and signature change:

```
grep -n 'SearchRecords\|SearchRecord' pkg/secret/store.go            # expect the new types + updated Search
grep -n 'record.Key\|for _, record := range records' pkg/secret/store.go   # ReEncrypt iterates records
```

Confirm the search envelope and enriched result:

```
grep -n 'json:"count"' pkg/api/response.go                           # expect ≥1 inside SearchResults
grep -nE 'json:"(hashid|name|username|url|api_url)"' pkg/api/response.go   # expect 5 within SearchResult
```

Confirm the mock matches the new signature (build compiles against it):

```
go build ./...                                                        # exits 0
grep -n 'secret.SearchRecords' mocks/secret-store.go                 # regenerated Search return type
```

Confirm the contract test and scenario assert `name`:

```
grep -nE 'results|count' main_test.go                                # search block asserting envelope + a result name
grep -n 'name' scenarios/001-core-api-e2e.md                         # jq assertion on .results[].name
grep -n 'Unreleased' CHANGELOG.md
```

(Do NOT add `-mod=vendor`. Do NOT run any `git` command — `.git` is masked in this container. Do NOT run `make e2e` — the scenario runs operator-side.)
</verification>
