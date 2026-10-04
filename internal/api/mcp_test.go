package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/qwerin/nanofaktura/internal/api"
	"github.com/qwerin/nanofaktura/internal/model"
)

// bearerTransport adds the API token to every MCP request.
type bearerTransport struct{ token string }

func (b bearerTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	if b.token != "" {
		r.Header.Set("Authorization", "Bearer "+b.token)
	}
	return http.DefaultTransport.RoundTrip(r)
}

// mcpSession connects a real MCP client to the API with token.
func mcpSession(t *testing.T, ts *testServer, token string) *mcp.ClientSession {
	t.Helper()
	srv := httptest.NewServer(ts.handler)
	t.Cleanup(srv.Close)
	cl := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "1"}, nil)
	sess, err := cl.Connect(context.Background(), &mcp.StreamableClientTransport{
		Endpoint: srv.URL + "/api/mcp", HTTPClient: &http.Client{Transport: bearerTransport{token}},
	}, nil)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = sess.Close() })
	return sess
}

// apiToken creates an API token of c's user.
func apiToken(c *client) string {
	return doJSON[api.CreatedAPIToken](c, http.StatusCreated, "POST", "/api/auth/tokens", api.APITokenCreate{Name: "mcp"}).Token
}

// callTool calls an MCP tool and returns its text and error flag.
func callTool(t *testing.T, s *mcp.ClientSession, name string, args map[string]any) (string, bool) {
	t.Helper()
	res, err := s.CallTool(context.Background(), &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	var sb strings.Builder
	for _, c := range res.Content {
		if tc, ok := c.(*mcp.TextContent); ok {
			sb.WriteString(tc.Text)
		}
	}
	return sb.String(), res.IsError
}

// mustTool is callTool expecting success, decoding the JSON result into T.
func mustTool[T any](t *testing.T, s *mcp.ClientSession, name string, args map[string]any) T {
	t.Helper()
	text, isErr := callTool(t, s, name, args)
	if isErr {
		t.Fatalf("%s(%v): error %s", name, args, text)
	}
	return decodeJSON[T](t, []byte(text))
}

func TestMCPTools(t *testing.T) {
	ts := newTestServer(t)
	a := ts.signup("a@example.cz", "Firma A")
	s := mcpSession(t, ts, apiToken(a))

	tools, err := s.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	byName := map[string]*mcp.Tool{}
	for _, tool := range tools.Tools {
		byName[tool.Name] = tool
		// only reads and safe writes: nothing deletes, sends or changes settings
		for _, bad := range []string{"delete", "send", "remind", "settings", "member", "webhook", "token"} {
			if strings.Contains(tool.Name, bad) {
				t.Errorf("tool %s must not be exposed", tool.Name)
			}
		}
		if tool.Annotations == nil || tool.Annotations.DestructiveHint == nil || *tool.Annotations.DestructiveHint {
			t.Errorf("tool %s: annotations %+v", tool.Name, tool.Annotations)
		}
	}
	for _, name := range []string{"list_accounts", "list_invoices", "create_invoice", "add_invoice_payment", "create_subject", "get_vat_report"} {
		if byName[name] == nil {
			t.Fatalf("tool %s missing; have %d tools", name, len(tools.Tools))
		}
	}
	if !byName["list_invoices"].Annotations.ReadOnlyHint || byName["create_invoice"].Annotations.ReadOnlyHint {
		t.Fatal("read-only hints")
	}
	// the input schema comes from OpenAPI: account + query params, body with $defs
	schema, _ := json.Marshal(byName["create_invoice"].InputSchema)
	for _, want := range []string{`"account"`, `"body"`, `"$ref":"#/$defs/InvoiceCreate"`, `"subject_id"`, `"required":["account","body"]`} {
		if !strings.Contains(string(schema), want) {
			t.Fatalf("create_invoice schema lacks %s: %s", want, schema)
		}
	}
	schema, _ = json.Marshal(byName["list_invoices"].InputSchema)
	if !strings.Contains(string(schema), `"status"`) || strings.Contains(string(schema), `"slug"`) {
		t.Fatalf("list_invoices schema: %s", schema)
	}

	accs := mustTool[api.ListResponse[api.Account]](t, s, "list_accounts", nil)
	if accs.Total != 1 || accs.Items[0].Slug != a.slug {
		t.Fatalf("accounts: %+v", accs)
	}

	subj := mustTool[api.Subject](t, s, "create_subject", map[string]any{
		"account": a.slug, "body": map[string]any{"name": "ACME s.r.o."},
	})
	inv := mustTool[api.Invoice](t, s, "create_invoice", map[string]any{
		"account": a.slug,
		"body": map[string]any{"subject_id": subj.ID, "lines": []map[string]any{
			{"name": "Konzultace", "quantity": "2", "unit_price": 150000},
		}},
	})
	if inv.Number != "2026-0001" || inv.Total != 300000 || inv.Status != "open" {
		t.Fatalf("invoice: %+v", inv.InvoiceSummary)
	}
	// nothing is e-mailed through MCP
	if n := len(ts.mail.Messages()); n != 0 {
		t.Fatalf("%d e-mails sent", n)
	}

	unpaid := mustTool[api.ListResponse[api.InvoiceSummary]](t, s, "list_invoices", map[string]any{"account": a.slug, "status": "unpaid", "per_page": 10})
	if unpaid.Total != 1 || unpaid.PerPage != 10 {
		t.Fatalf("unpaid: %+v", unpaid)
	}
	paid := mustTool[api.PaymentResult](t, s, "add_invoice_payment", map[string]any{
		"account": a.slug, "id": inv.ID, "body": map[string]any{"paid_on": "2026-03-15"},
	})
	if paid.Invoice.Status != "paid" {
		t.Fatalf("payment: %+v", paid.Invoice.InvoiceSummary)
	}
	got := mustTool[api.Invoice](t, s, "get_invoice", map[string]any{"account": a.slug, "id": inv.ID})
	if got.Status != "paid" || len(got.Payments) != 1 {
		t.Fatalf("get: %+v", got.InvoiceSummary)
	}
	// the payment is recorded as the token's user, like a REST call
	var ev model.Event
	if err := ts.db.Where("name = ?", "invoice.paid").First(&ev).Error; err != nil || ev.UserID == nil {
		t.Fatalf("event: %+v %v", ev, err)
	}

	// errors are tool errors carrying the problem+json
	if text, isErr := callTool(t, s, "get_invoice", map[string]any{"account": a.slug, "id": 9999}); !isErr || !strings.Contains(text, "HTTP 404") {
		t.Fatalf("missing invoice: %v %s", isErr, text)
	}
	if text, isErr := callTool(t, s, "create_subject", map[string]any{"account": a.slug, "body": map[string]any{"name": ""}}); !isErr || !strings.Contains(text, "HTTP 422") {
		t.Fatalf("invalid subject: %v %s", isErr, text)
	}
	if text, isErr := callTool(t, s, "get_invoice", map[string]any{"id": inv.ID}); !isErr || !strings.Contains(text, "account") {
		t.Fatalf("missing account: %v %s", isErr, text)
	}
	if text, isErr := callTool(t, s, "get_invoice", map[string]any{"account": a.slug}); !isErr || !strings.Contains(text, "missing id") {
		t.Fatalf("missing id: %v %s", isErr, text)
	}
}

// TestMCPIsolationAndRoles: MCP calls are subject to membership and roles like REST.
func TestMCPIsolationAndRoles(t *testing.T) {
	ts := newTestServer(t)
	a := ts.signup("a@example.cz", "Firma A")
	b := ts.signup("b@example.cz", "Firma B")
	acme := doJSON[api.Subject](a, http.StatusCreated, "POST", a.acct("/subjects"), api.SubjectCreate{Name: "ACME"})

	sb := mcpSession(t, ts, apiToken(b))
	if text, isErr := callTool(t, sb, "get_subject", map[string]any{"account": a.slug, "id": acme.ID}); !isErr || !strings.Contains(text, "HTTP 404") {
		t.Fatalf("foreign account: %v %s", isErr, text)
	}
	if text, isErr := callTool(t, sb, "list_subjects", map[string]any{"account": "../" + a.slug}); !isErr || !strings.Contains(text, "HTTP 404") {
		t.Fatalf("slug escape: %v %s", isErr, text)
	}

	acc := ts.memberOf(a, "ucetni@example.cz", model.RoleAccountant)
	sa := mcpSession(t, ts, apiToken(acc))
	if text, isErr := callTool(t, sa, "create_subject", map[string]any{"account": a.slug, "body": map[string]any{"name": "X"}}); !isErr || !strings.Contains(text, "HTTP 403") {
		t.Fatalf("accountant write: %v %s", isErr, text)
	}
	if text, isErr := callTool(t, sa, "get_overview_report", map[string]any{"account": a.slug, "year": 2026}); isErr {
		t.Fatalf("accountant may read reports: %s", text)
	}
	mem := ts.memberOf(a, "clen@example.cz", model.RoleMember)
	sm := mcpSession(t, ts, apiToken(mem))
	if text, isErr := callTool(t, sm, "get_overview_report", map[string]any{"account": a.slug}); !isErr || !strings.Contains(text, "HTTP 403") {
		t.Fatalf("member report: %v %s", isErr, text)
	}
}

// TestMCPAuth: only a valid API token opens the endpoint (no cookies).
func TestMCPAuth(t *testing.T) {
	ts := newTestServer(t, withRateLimit)
	a := ts.signup("a@example.cz", "Firma A")
	init := map[string]any{"jsonrpc": "2.0", "id": 1, "method": "initialize", "params": map[string]any{
		"protocolVersion": "2025-06-18", "capabilities": map[string]any{}, "clientInfo": map[string]any{"name": "x", "version": "1"},
	}}

	res, body := a.do("POST", "/api/mcp", init) // session cookie only
	if res.StatusCode != http.StatusUnauthorized || !strings.HasPrefix(res.Header.Get("WWW-Authenticate"), "Bearer") {
		t.Fatalf("cookie: %d %s", res.StatusCode, body)
	}
	if s := mcpSession(t, ts, apiToken(a)); s.InitializeResult().ServerInfo.Name != "nanofaktura" {
		t.Fatalf("initialize: %+v", s.InitializeResult())
	}
	bad := &client{ts: ts, token: "nf_invalid"}
	for i := range 20 {
		if res, body := bad.do("POST", "/api/mcp", init); res.StatusCode != http.StatusUnauthorized {
			t.Fatalf("attempt %d: %d %s", i, res.StatusCode, body)
		}
	}
	if res, _ := bad.do("POST", "/api/mcp", init); res.StatusCode != http.StatusTooManyRequests || res.Header.Get("Retry-After") == "" {
		t.Fatalf("rate limit: %d %v", res.StatusCode, res.Header)
	}
}

// TestMCPEveryTool calls every tool once (lookup_ares has its own fake below).
func TestMCPEveryTool(t *testing.T) {
	ts := newTestServer(t)
	a := ts.signup("a@example.cz", "Firma A")
	s := mcpSession(t, ts, apiToken(a))
	acct := func(extra map[string]any) map[string]any {
		m := map[string]any{"account": a.slug}
		for k, v := range extra {
			m[k] = v
		}
		return m
	}

	sup := mustTool[api.Subject](t, s, "create_subject", acct(map[string]any{"body": map[string]any{"name": "Dodavatel", "type": "supplier"}}))
	if upd := mustTool[api.Subject](t, s, "update_subject", acct(map[string]any{"id": sup.ID, "body": map[string]any{"email": "d@example.cz"}})); upd.Email != "d@example.cz" {
		t.Fatalf("update_subject: %+v", upd)
	}
	exp := mustTool[api.Expense](t, s, "create_expense", acct(map[string]any{"body": map[string]any{
		"subject_id": sup.ID, "category": "Software", "lines": []map[string]any{{"name": "Licence", "unit_price": 50000}},
	}}))
	if res := mustTool[api.ExpensePaymentResult](t, s, "add_expense_payment", acct(map[string]any{"id": exp.ID, "body": map[string]any{}})); res.Expense.Status != "paid" {
		t.Fatalf("add_expense_payment: %+v", res.Expense)
	}
	mustTool[api.Todo](t, s, "create_todo", acct(map[string]any{"body": map[string]any{"text": "Zavolat účetní"}}))

	for name, args := range map[string]map[string]any{
		"get_account":             acct(nil),
		"search":                  acct(map[string]any{"q": "Dodavatel"}),
		"get_dashboard":           acct(map[string]any{"year": 2026}),
		"list_subjects":           acct(map[string]any{"type": "supplier"}),
		"get_subject":             acct(map[string]any{"id": sup.ID}),
		"list_expenses":           acct(map[string]any{"status": "paid"}),
		"get_expense":             acct(map[string]any{"id": exp.ID}),
		"list_expense_categories": acct(nil),
		"list_price_items":        acct(map[string]any{"archived": false}),
		"get_overview_report":     acct(nil),
		"list_todos":              acct(nil),
	} {
		if text, isErr := callTool(t, s, name, args); isErr || !strings.HasPrefix(text, "{") {
			t.Errorf("%s: %v %s", name, isErr, text)
		}
	}
	if text, _ := callTool(t, s, "list_todos", acct(nil)); !strings.Contains(text, "Zavolat účetní") {
		t.Fatalf("todos: %s", text)
	}
	// VAT report needs a VAT payer: the tool passes the domain error through
	if text, isErr := callTool(t, s, "get_vat_report", acct(nil)); !isErr || !strings.Contains(text, "not_vat_payer") {
		t.Fatalf("get_vat_report: %v %s", isErr, text)
	}
}

func TestMCPLookupARES(t *testing.T) {
	ts := newTestServer(t)
	a := ts.signup("a@example.cz", "Firma A")
	withARES(ts, fakeARES{"27074358": {RegistrationNo: "27074358", Name: "Firma a.s."}})
	s := mcpSession(t, ts, apiToken(a))
	if text, isErr := callTool(t, s, "lookup_ares", map[string]any{"ico": "27074358"}); isErr || !strings.Contains(text, "Firma a.s.") {
		t.Fatalf("lookup_ares: %v %s", isErr, text)
	}
}
