package api

import (
	"context"
	"fmt"
	"strings"

	"github.com/danielgtaylor/huma/v2"

	"github.com/qwerin/nanofaktura/internal/auth"
	"github.com/qwerin/nanofaktura/internal/billing"
	"github.com/qwerin/nanofaktura/internal/model"
	"github.com/qwerin/nanofaktura/internal/search"
)

// SearchHit is one global search result.
type SearchHit struct {
	Type     string `json:"type" enum:"invoice,expense,subject,price_item"`
	ID       uint   `json:"id"`
	Title    string `json:"title" doc:"Number / name"`
	Subtitle string `json:"subtitle" doc:"Client, amount, IČO …"`
	Status   string `json:"status,omitempty"`
	URLHint  string `json:"url_hint" doc:"SPA path of the detail, e.g. /a/{slug}/invoices/12"`
}

// SearchResults are the hits grouped by type (each at most limit, newest /
// alphabetically first).
type SearchResults struct {
	Invoices   []SearchHit `json:"invoices" nullable:"false"`
	Expenses   []SearchHit `json:"expenses" nullable:"false"`
	Subjects   []SearchHit `json:"subjects" nullable:"false"`
	PriceItems []SearchHit `json:"price_items" nullable:"false"`
}

func (s *server) registerSearch(g huma.API) {
	huma.Get(g, "/search", s.globalSearch, func(o *huma.Operation) {
		o.Description = "Case- and diacritics-insensitive search in invoices (number, VS, client name, IČO, DIČ, e-mail), " +
			"expenses (numbers, VS, supplier, description), contacts (name, IČO, DIČ, e-mail, custom id) and price items (name, SKU)."
	})
}

func (s *server) globalSearch(ctx context.Context, in *struct {
	Q     string `query:"q" minLength:"1" maxLength:"100" required:"true"`
	Limit int    `query:"limit" minimum:"1" maximum:"20" default:"5"`
}) (*Out[SearchResults], error) {
	res := SearchResults{Invoices: []SearchHit{}, Expenses: []SearchHit{}, Subjects: []SearchHit{}, PriceItems: []SearchHit{}}
	if search.Fold(in.Q) == "" {
		return &Out[SearchResults]{Body: res}, nil
	}
	pattern := search.LikePattern(in.Q)
	base := "/a/" + auth.AccountFrom(ctx).Slug
	match := func(v any, order string, dst any) error {
		return s.scoped(ctx).Model(v).Where(`search_text LIKE ? ESCAPE '\'`, pattern).Order(order).Limit(in.Limit).Find(dst).Error
	}

	var invs []model.Invoice
	if err := match(&model.Invoice{}, "issued_on DESC, id DESC", &invs); err != nil {
		return nil, dbErr(err, "invoice")
	}
	for _, m := range invs {
		noun, _ := docNoun(m.DocumentType)
		res.Invoices = append(res.Invoices, SearchHit{Type: "invoice", ID: m.ID, Title: noun + " " + defaultStr(m.Number, "(koncept)"),
			Subtitle: joinNonEmpty(m.ClientName, money(m.Total, m.Currency), czDate(m.IssuedOn)),
			Status:   billing.EffectiveStatus(m.Status, m.DueOn, s.today()), URLHint: fmt.Sprintf("%s/invoices/%d", base, m.ID)})
	}
	var exps []model.Expense
	if err := match(&model.Expense{}, "issued_on DESC, id DESC", &exps); err != nil {
		return nil, dbErr(err, "expense")
	}
	for _, m := range exps {
		res.Expenses = append(res.Expenses, SearchHit{Type: "expense", ID: m.ID, Title: joinNonEmpty(m.Number, m.OriginalNumber),
			Subtitle: joinNonEmpty(m.SupplierName, money(m.Total, m.Currency), czDate(m.IssuedOn)),
			Status:   m.Status, URLHint: fmt.Sprintf("%s/expenses/%d", base, m.ID)})
	}
	var subs []model.Subject
	if err := match(&model.Subject{}, "name, id", &subs); err != nil {
		return nil, dbErr(err, "subject")
	}
	for _, m := range subs {
		ico := ""
		if m.RegistrationNo != "" {
			ico = "IČO " + m.RegistrationNo
		}
		res.Subjects = append(res.Subjects, SearchHit{Type: "subject", ID: m.ID, Title: m.Name,
			Subtitle: joinNonEmpty(ico, m.City, m.Email), URLHint: fmt.Sprintf("%s/subjects/%d", base, m.ID)})
	}
	var items []model.PriceItem
	if err := match(&model.PriceItem{}, "CASE WHEN archived_at IS NULL THEN 0 ELSE 1 END, name, id", &items); err != nil {
		return nil, dbErr(err, "price item")
	}
	for _, m := range items {
		st := "active"
		if m.ArchivedAt != nil {
			st = "archived"
		}
		res.PriceItems = append(res.PriceItems, SearchHit{Type: "price_item", ID: m.ID, Title: m.Name,
			Subtitle: joinNonEmpty(m.SKU, money(m.UnitPrice, m.Currency)), Status: st,
			URLHint: fmt.Sprintf("%s/price-items/%d", base, m.ID)})
	}
	return &Out[SearchResults]{Body: res}, nil
}

func joinNonEmpty(parts ...string) string {
	out := parts[:0:0]
	for _, p := range parts {
		if p != "" {
			out = append(out, p)
		}
	}
	return strings.Join(out, " · ")
}
