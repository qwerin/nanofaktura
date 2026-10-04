package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"maps"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/danielgtaylor/huma/v2"
	"github.com/go-chi/chi/v5"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/qwerin/nanofaktura/internal/auth"
)

// MCP server (SPEC §7.17): POST /api/mcp speaks the Model Context Protocol
// (Streamable HTTP, stateless) to AI assistants. Every request carries an API
// token (`Authorization: Bearer nf_…`); cookies are not accepted.
//
// A tool is a curated REST operation: its input schema is derived from the
// OpenAPI operation (path/query parameters + request body) and a call is
// dispatched in-process to the API router with the caller's token, so
// authentication, roles, validation, events and rate limits are exactly those
// of the REST API. Only reads and safe writes are exposed (no e-mails,
// deletions, settings).

// mcpPath is where the MCP endpoint is mounted.
const mcpPath = "/api/mcp"

// mcpMaxBody bounds a JSON-RPC message (tool bodies are small JSON documents).
const mcpMaxBody = 1 << 20

// mcpTool describes one MCP tool backed by a REST operation.
type mcpTool struct {
	Name        string
	Title       string
	Description string
	Method      string // http.MethodGet …
	Path        string // relative to /api/accounts/{slug} (account tools) or absolute ("/api/…")
	Write       bool   // false = read-only
	Force       map[string]any // body fields always set, whatever the model sends
}

// account tools need the account slug; absolute paths do not.
func (t mcpTool) account() bool { return !strings.HasPrefix(t.Path, "/api/") }

func (t mcpTool) openAPIPath() string {
	if t.account() {
		return "/api/accounts/{slug}" + t.Path
	}
	return t.Path
}

// mcpTools is the catalogue exposed over MCP. Keep descriptions in English
// (models follow them best); the data itself is Czech.
var mcpTools = []mcpTool{
	{Name: "list_accounts", Title: "Účty", Method: http.MethodGet, Path: "/api/accounts",
		Description: "List the accounts (companies) the token's user is a member of, with the user's role. Every other tool takes the `account` slug from here."},
	{Name: "get_account", Title: "Nastavení účtu", Method: http.MethodGet, Path: "",
		Description: "Company details of the account: name, IČO, DIČ, address, VAT payer status and VAT period, default currency and due days."},
	{Name: "search", Title: "Hledání", Method: http.MethodGet, Path: "/search",
		Description: "Global search across invoices, expenses, subjects (clients/suppliers) and price items."},
	{Name: "get_dashboard", Title: "Přehled", Method: http.MethodGet, Path: "/dashboard",
		Description: "Dashboard of a year: revenue, expenses, unpaid and overdue totals, monthly figures."},
	{Name: "list_invoices", Title: "Faktury", Method: http.MethodGet, Path: "/invoices",
		Description: "List issued documents (invoices, proformas, corrections, tax documents) with filters. status=unpaid returns receivables, status=overdue those after due date."},
	{Name: "get_invoice", Title: "Detail faktury", Method: http.MethodGet, Path: "/invoices/{id}",
		Description: "Full invoice detail including lines, VAT summary, payments and attachments."},
	{Name: "create_invoice", Title: "Připravit fakturu", Method: http.MethodPost, Path: "/invoices", Write: true,
		Force: map[string]any{"draft": true},
		Description: "Prepare a new document as a DRAFT (always): it has no number yet, counts nowhere and is not sent. The user reviews and issues it in the web app. Look the client up first (list_subjects / search) and pass subject_id."},
	{Name: "add_invoice_payment", Title: "Zapsat úhradu faktury", Method: http.MethodPost, Path: "/invoices/{id}/payments", Write: true,
		Description: "Record a received payment of an invoice (full or partial). The invoice becomes paid when payments cover its total."},
	{Name: "list_subjects", Title: "Kontakty", Method: http.MethodGet, Path: "/subjects",
		Description: "List clients and suppliers (subjects) of the account."},
	{Name: "get_subject", Title: "Detail kontaktu", Method: http.MethodGet, Path: "/subjects/{id}",
		Description: "Subject (client/supplier) detail."},
	{Name: "create_subject", Title: "Nový kontakt", Method: http.MethodPost, Path: "/subjects", Write: true,
		Description: "Create a client or supplier. For Czech companies prefer filling the data from lookup_ares (by IČO)."},
	{Name: "update_subject", Title: "Upravit kontakt", Method: http.MethodPatch, Path: "/subjects/{id}", Write: true,
		Description: "Change fields of a subject; omitted fields stay unchanged."},
	{Name: "lookup_ares", Title: "Hledání v ARES", Method: http.MethodGet, Path: "/api/ares/{ico}",
		Description: "Look up a Czech company in the ARES business register by IČO (8 digits): name, address, DIČ."},
	{Name: "list_expenses", Title: "Náklady", Method: http.MethodGet, Path: "/expenses",
		Description: "List received invoices / expenses with filters (status=unpaid = payables)."},
	{Name: "get_expense", Title: "Detail nákladu", Method: http.MethodGet, Path: "/expenses/{id}",
		Description: "Expense detail including lines, VAT and payments."},
	{Name: "list_expense_categories", Title: "Kategorie nákladů", Method: http.MethodGet, Path: "/expenses/categories",
		Description: "Expense categories already used in the account (use them for consistent categorisation)."},
	{Name: "create_expense", Title: "Zapsat náklad", Method: http.MethodPost, Path: "/expenses", Write: true,
		Description: "Record a received invoice / expense."},
	{Name: "add_expense_payment", Title: "Zapsat úhradu nákladu", Method: http.MethodPost, Path: "/expenses/{id}/payments", Write: true,
		Description: "Record a payment of an expense (full or partial)."},
	{Name: "list_price_items", Title: "Ceník", Method: http.MethodGet, Path: "/price-items",
		Description: "Price list items (products/services with prices, VAT rates, stock)."},
	{Name: "get_vat_report", Title: "DPH", Method: http.MethodGet, Path: "/reports/vat",
		Description: "VAT summary of a period (output and input VAT by rate). Accountant, admin and owner only."},
	{Name: "get_overview_report", Title: "Výsledky roku", Method: http.MethodGet, Path: "/reports/overview",
		Description: "Yearly overview: revenue, expenses and profit by month. Accountant, admin and owner only."},
	{Name: "list_todos", Title: "Úkoly", Method: http.MethodGet, Path: "/todos",
		Description: "To-dos of the account (manual and automatic, e.g. overdue invoices to remind)."},
	{Name: "create_todo", Title: "Nový úkol", Method: http.MethodPost, Path: "/todos", Write: true,
		Description: "Create a manual to-do."},
}

const mcpInstructions = `NanoFaktura is an invoicing app for Czech sole traders and small companies.
Start with list_accounts and pass the account slug as "account" to the other tools.
Conventions: money is an integer in minor units (haléře: 12100 = 121,00 Kč), VAT rates in basis points
(2100 = 21 %), quantities are decimal strings ("1.5"), dates "YYYY-MM-DD". Texts of the data are Czech.
Invoices are created as drafts only; the user issues them (assigns the number) in the web app.
Other writing tools create real records (subjects, expenses, payments) – confirm with the user first.
Nothing is ever issued, e-mailed, deleted or reconfigured through these tools; point the user to the web app for that.`

// mcpReq carries what the dispatcher needs from the outer HTTP request.
type mcpReq struct {
	remoteAddr string
	header     http.Header
}

type mcpReqKey struct{}

// mcpHandler builds the MCP endpoint dispatching tool calls to router.
func (s *server) mcpHandler(router http.Handler, api huma.API) http.Handler {
	srv := mcp.NewServer(&mcp.Implementation{Name: "nanofaktura", Title: "NanoFaktura", Version: "1.0.0"},
		&mcp.ServerOptions{Instructions: mcpInstructions})
	oapi := api.OpenAPI()
	for _, t := range mcpTools {
		srv.AddTool(t.tool(oapi), s.mcpCall(router, t))
	}
	h := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return srv }, &mcp.StreamableHTTPOptions{
		Stateless: true, JSONResponse: true,
		// DNS rebinding cannot steal anything: every request needs a bearer token,
		// and the check would reject reverse proxies on localhost.
		DisableLocalhostProtection: true,
	})
	return s.mcpAuth(h)
}

// mcpAuth requires a valid API token (401 with WWW-Authenticate otherwise;
// failed attempts are rate limited per IP) and remembers the request for the
// dispatcher.
func (s *server) mcpAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		unauthorized := func(msg string) {
			w.Header().Set("WWW-Authenticate", `Bearer realm="nanofaktura"`)
			http.Error(w, msg, http.StatusUnauthorized)
		}
		ip := clientIP(r.Context())
		if err := s.rateBlocked(s.limits.mcpAuth, ip); err != nil {
			w.Header().Set("Retry-After", retryAfter(err))
			http.Error(w, "too many failed attempts", http.StatusTooManyRequests)
			return
		}
		token, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
		if !ok {
			unauthorized("API token required (Authorization: Bearer nf_…)")
			return
		}
		if _, err := s.auth.UserByAPIToken(r.Context(), token); err != nil {
			if !errors.Is(err, auth.ErrInvalidCredentials) {
				slog.ErrorContext(r.Context(), "mcp token check", "err", err)
				http.Error(w, "authentication failed", http.StatusInternalServerError)
				return
			}
			s.rateFail(s.limits.mcpAuth, ip)
			unauthorized("invalid or expired API token")
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, mcpMaxBody)
		ctx := context.WithValue(r.Context(), mcpReqKey{}, mcpReq{remoteAddr: r.RemoteAddr, header: r.Header.Clone()})
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// retryAfter extracts the Retry-After header of a rate limit error ("60" fallback).
func retryAfter(err error) string {
	var he huma.HeadersError
	if errors.As(err, &he) {
		if v := he.GetHeaders().Get("Retry-After"); v != "" {
			return v
		}
	}
	return "60"
}

var mcpPathParam = regexp.MustCompile(`\{([a-z_]+)\}`)

// tool builds the MCP tool definition from the OpenAPI operation.
func (t mcpTool) tool(oapi *huma.OpenAPI) *mcp.Tool {
	op := mcpOperation(oapi, t.Method, t.openAPIPath())
	props := map[string]any{}
	var required []string
	if t.account() {
		props["account"] = map[string]any{"type": "string", "description": "Account slug (see list_accounts)"}
		required = append(required, "account")
	}
	defs := map[string]any{}
	for _, p := range op.Parameters {
		if p.Name == "slug" || (p.In != "path" && p.In != "query") {
			continue
		}
		sch := mcpSchemaJSON(p.Schema, oapi, defs)
		if p.Description != "" {
			sch["description"] = p.Description
		}
		props[p.Name] = sch
		if p.Required || p.In == "path" {
			required = append(required, p.Name)
		}
	}
	if rb := op.RequestBody; rb != nil {
		if c := rb.Content["application/json"]; c != nil && c.Schema != nil {
			props["body"] = mcpSchemaJSON(c.Schema, oapi, defs)
			required = append(required, "body")
		}
	}
	in := map[string]any{"type": "object", "properties": props, "additionalProperties": false}
	if len(required) > 0 {
		in["required"] = required
	}
	if len(defs) > 0 {
		in["$defs"] = defs
	}
	desc := t.Description
	if roles := auth.AllowedRoles(op); len(roles) > 0 {
		desc += " Roles: " + strings.Join(roles, ", ") + "."
	}
	return &mcp.Tool{
		Name: t.Name, Title: t.Title, Description: desc, InputSchema: in,
		Annotations: &mcp.ToolAnnotations{
			Title: t.Title, ReadOnlyHint: !t.Write, IdempotentHint: !t.Write,
			DestructiveHint: new(false), OpenWorldHint: new(t.Name == "lookup_ares"),
		},
	}
}

// mcpOperation finds a registered operation; the catalogue must match the API.
func mcpOperation(oapi *huma.OpenAPI, method, path string) *huma.Operation {
	if pi := oapi.Paths[path]; pi != nil {
		var op *huma.Operation
		switch method {
		case http.MethodGet:
			op = pi.Get
		case http.MethodPost:
			op = pi.Post
		case http.MethodPatch:
			op = pi.Patch
		}
		if op != nil {
			return op
		}
	}
	panic(fmt.Sprintf("mcp: no operation %s %s", method, path))
}

// mcpSchemaJSON converts a huma schema to plain JSON Schema, moving the
// referenced components (transitively) into defs as "#/$defs/Name".
func mcpSchemaJSON(sch *huma.Schema, oapi *huma.OpenAPI, defs map[string]any) map[string]any {
	const prefix = "#/components/schemas/"
	b, err := json.Marshal(sch)
	if err != nil {
		panic(err)
	}
	var out map[string]any
	if err := json.Unmarshal(bytes.ReplaceAll(b, []byte(prefix), []byte("#/$defs/")), &out); err != nil {
		panic(err)
	}
	for _, ref := range mcpRefs(out) {
		if _, done := defs[ref]; done {
			continue
		}
		comp := oapi.Components.Schemas.Map()[ref]
		if comp == nil {
			panic("mcp: unknown schema " + ref)
		}
		defs[ref] = nil // cycle guard
		defs[ref] = mcpSchemaJSON(comp, oapi, defs)
	}
	return out
}

// mcpRefs lists the $defs names referenced anywhere in v.
func mcpRefs(v any) []string {
	var refs []string
	switch x := v.(type) {
	case map[string]any:
		if r, ok := x["$ref"].(string); ok {
			refs = append(refs, strings.TrimPrefix(r, "#/$defs/"))
		}
		for _, k := range slices.Sorted(maps.Keys(x)) {
			refs = append(refs, mcpRefs(x[k])...)
		}
	case []any:
		for _, e := range x {
			refs = append(refs, mcpRefs(e)...)
		}
	}
	return refs
}

// mcpCall dispatches a tool call as a REST request through router.
func (s *server) mcpCall(router http.Handler, t mcpTool) mcp.ToolHandler {
	return func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := map[string]any{}
		if len(req.Params.Arguments) > 0 {
			if err := json.Unmarshal(req.Params.Arguments, &args); err != nil {
				return mcpError("arguments must be a JSON object"), nil
			}
		}
		path := t.Path
		if t.account() {
			slug, _ := args["account"].(string)
			if slug == "" {
				return mcpError(`missing "account" (account slug, see list_accounts)`), nil
			}
			path = "/api/accounts/" + url.PathEscape(slug) + path
			delete(args, "account")
		}
		var missing []string
		path = mcpPathParam.ReplaceAllStringFunc(path, func(m string) string {
			name := m[1 : len(m)-1]
			v, ok := args[name]
			delete(args, name)
			if !ok {
				missing = append(missing, name)
				return m
			}
			return url.PathEscape(mcpScalar(v))
		})
		if len(missing) > 0 {
			return mcpError("missing " + strings.Join(missing, ", ")), nil
		}
		var body io.Reader = http.NoBody
		if b, ok := args["body"]; ok {
			delete(args, "body")
			if obj, ok := b.(map[string]any); ok {
				maps.Copy(obj, t.Force)
			}
			raw, err := json.Marshal(b)
			if err != nil {
				return mcpError("invalid body"), nil
			}
			body = bytes.NewReader(raw)
		}
		q := url.Values{}
		for _, k := range slices.Sorted(maps.Keys(args)) {
			if args[k] != nil {
				q.Set(k, mcpScalar(args[k]))
			}
		}
		if len(q) > 0 {
			path += "?" + q.Encode()
		}

		// a fresh chi routing context: the outer one belongs to /api/mcp
		r, err := http.NewRequestWithContext(context.WithValue(ctx, chi.RouteCtxKey, nil), t.Method, path, body)
		if err != nil {
			return mcpError("invalid request: " + err.Error()), nil
		}
		if outer, ok := ctx.Value(mcpReqKey{}).(mcpReq); ok {
			r.RemoteAddr = outer.remoteAddr
			for k, v := range outer.header { // forwarded headers (client IP), Authorization
				r.Header[k] = v
			}
		} else if req.Extra != nil {
			r.Header = req.Extra.Header.Clone()
		}
		r.Header.Del("Cookie") // the token is the only credential
		r.Header.Del("Content-Length")
		r.Header.Set("Accept", "application/json")
		r.Header.Set("Content-Type", "application/json")
		rec := &mcpRecorder{header: http.Header{}, status: http.StatusOK}
		router.ServeHTTP(rec, r)
		return mcpResult(rec), nil
	}
}

func mcpScalar(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case float64:
		return strconv.FormatFloat(x, 'f', -1, 64)
	case bool:
		return strconv.FormatBool(x)
	default:
		b, _ := json.Marshal(x)
		return string(b)
	}
}

func mcpError(msg string) *mcp.CallToolResult {
	return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: msg}}}
}

// mcpResult turns the REST response into a tool result: the JSON body as
// text (and structured content for objects); errors keep the problem+json.
func mcpResult(rec *mcpRecorder) *mcp.CallToolResult {
	text := strings.TrimSpace(rec.body.String())
	if rec.status >= 400 {
		return mcpError(fmt.Sprintf("HTTP %d: %s", rec.status, text))
	}
	if text == "" {
		text = http.StatusText(rec.status)
	}
	res := &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: text}}}
	var obj map[string]any
	if json.Unmarshal(rec.body.Bytes(), &obj) == nil {
		res.StructuredContent = obj
	}
	return res
}

// mcpRecorder buffers the in-process REST response.
type mcpRecorder struct {
	header http.Header
	status int
	body   bytes.Buffer
	wrote  bool
}

func (r *mcpRecorder) Header() http.Header { return r.header }

func (r *mcpRecorder) WriteHeader(code int) {
	if !r.wrote {
		r.status, r.wrote = code, true
	}
}

func (r *mcpRecorder) Write(b []byte) (int, error) {
	r.wrote = true
	return r.body.Write(b)
}
