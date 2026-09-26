# CLAUDE.md

NanoFaktura — self-hosted invoicing for Czech sole traders and small companies.
Go API (huma v2 on chi, GORM) + React SPA in `web/`.

**`docs/SPEC.md` is the binding contract** (data model, API, conventions). Read it before
changing anything; record any deviation in its "Otevřené otázky" section.
Never mention other invoicing products anywhere in the repo (code, docs, commits).

## Commands

```bash
make build          # CGO_ENABLED=0 go build -o bin/nanofaktura ./cmd/server/
make run            # build + run on :8080 (SQLite nanofaktura.db)
make dev-backend    # go run ./cmd/server/
make dev-frontend   # Vite on :5173, proxies /api → :8080
make test           # go vet + go test ./... + web tsc (if web/node_modules exists)
make test-api       # go test ./internal/api/... -v
make gen-types      # regenerate web/src/api/schema.gen.ts from OpenAPI — run after any API change
go test ./internal/api/ -run TestAccount -v -count=1   # single test
```

Config is env only (`NANOFAKTURA_*`, see `internal/config` / SPEC §5). OpenAPI at `/api/openapi.json`, docs at `/api/docs`.

## Layout

```
cmd/server/       config → db.Open/Migrate → api.New → http.Server (+ SPA from NANOFAKTURA_STATIC_DIR)
cmd/gen-schema/   prints OpenAPI JSON (api.New with nil DB)
internal/config/  env config
internal/db/      Open(driver, dsn) (glebarez pure-Go SQLite or Postgres) + Migrate (AutoMigrate model.All())
internal/model/   GORM structs + enum constants; no API concerns
internal/auth/    bcrypt, sessions, API tokens, huma middlewares, auth.UserFrom/AccountFrom/RoleFrom/RequireOwner
internal/api/     api.New + one file per resource (DTOs next to handlers) + *_test.go (package api_test)
```

## Conventions (details in SPEC §2)

- Money `int64` minor units, quantity decimal string in API / `QuantityMilli` in DB, VAT `*_bps`, dates `YYYY-MM-DD` strings, instants `time.Time`.
- GORM models are never huma input/output. DTO naming in `internal/api`: `Subject` (output),
  `SubjectCreate` (create body, optional fields `omitempty`), `SubjectPatch` (pointers, nil = unchanged),
  `toSubject(*model.Subject) Subject` (converter). Output slices get `nullable:"false"` and must never be nil.
- Errors: `notFound("subject")` (404, also for other accounts' records), `conflict(msg)` (409),
  `invalid(field, msg)` (422), `dbErr(err, "subject")` maps GORM errors (not found → 404, duplicate key → 409, else 500).
  DB is opened with `TranslateError`, so unique violations are `gorm.ErrDuplicatedKey`.
- SQLite uses a single connection: inside `Transaction(func(tx) …)` use only `tx`, never `s.db` (would deadlock).
- AutoMigrate never drops columns; delete the local DB after incompatible model changes.

## Adding an account-scoped endpoint

Routes live in three huma groups built in `api.New` (see the doc comment in `internal/api/api.go`):
`public`, `authed` (401 without session cookie / `Authorization: Bearer nf_…`) and `account`
(`/api/accounts/{slug}` prefix; non-member → 404; `{slug}` is added to OpenAPI automatically — don't declare it).

1. Create `internal/api/subjects.go`:

```go
func (s *server) registerSubjects(g huma.API) {
	huma.Get(g, "/subjects", s.listSubjects)
	huma.Post(g, "/subjects", s.createSubject, status(http.StatusCreated))
	huma.Get(g, "/subjects/{id}", s.getSubject)
}

func (s *server) listSubjects(ctx context.Context, in *struct {
	PageParams
	Query string `query:"query"`
}) (*Out[ListResponse[Subject]], error) {
	q := s.scoped(ctx).Order("name") // always scoped → WHERE subjects.account_id = current account
	if in.Query != "" {
		q = q.Where("LOWER(name) LIKE ?", "%"+strings.ToLower(in.Query)+"%")
	}
	return paginate(q, in.PageParams, toSubject)
}

func (s *server) getSubject(ctx context.Context, in *struct {
	ID uint `path:"id"`
}) (*Out[Subject], error) {
	var m model.Subject
	if err := s.scoped(ctx).First(&m, in.ID).Error; err != nil {
		return nil, dbErr(err, "subject")
	}
	return &Out[Subject]{Body: toSubject(&m)}, nil
}
```

   New records set `AccountID: auth.AccountFrom(ctx).ID`. In transactions use `tx.Scopes(inAccount(ctx))`.
   Owner-only operations start with `if err := auth.RequireOwner(ctx); err != nil { return nil, err }`.
   Outputs: `Out[T]` (JSON body), `NoContent` + `status(http.StatusNoContent)` for deletes.
2. Register it in `api.New`: `s.registerSubjects(account)`.
3. Test it in `internal/api/subjects_test.go` (harness in `testutil_test.go`: in-memory SQLite, fixed clock `ts.now`):

```go
func TestSubjects(t *testing.T) {
	ts := newTestServer(t)
	a := ts.signup("a@example.cz", "Firma A") // registered + logged in, a.slug = its account
	b := ts.signup("b@example.cz", "Firma B")

	created := doJSON[api.Subject](a, http.StatusCreated, "POST", a.acct("/subjects"), api.SubjectCreate{Name: "ACME"})
	list := doJSON[api.ListResponse[api.Subject]](a, http.StatusOK, "GET", a.acct("/subjects"), nil)
	if list.Total != 1 { t.Fatalf("list: %+v", list) }

	// tenant isolation is mandatory: b must not see a's record
	res, body := b.do("GET", fmt.Sprintf("%s/%d", b.acct("/subjects"), created.ID), nil)
	assertError(t, res, body, http.StatusNotFound, "subject not found")
}
```

   Helpers: `c.do(method, path, body)`, `c.mustDo(status, …) []byte`, `doJSON[T](c, status, …) T`,
   `decodeJSON[T](t, b)`, `assertError(t, res, body, status, substr)`, `ts.anon()`, `ts.db` for direct setup,
   `&client{ts: ts, token: "nf_…"}` for bearer auth.
4. `make test` and `make gen-types`.
