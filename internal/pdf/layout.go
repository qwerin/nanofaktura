package pdf

import (
	"strings"

	"github.com/johnfercher/maroto/v2/pkg/components/col"
	"github.com/johnfercher/maroto/v2/pkg/components/page"
	"github.com/johnfercher/maroto/v2/pkg/components/row"
	"github.com/johnfercher/maroto/v2/pkg/consts/align"
	"github.com/johnfercher/maroto/v2/pkg/consts/extension"
	"github.com/johnfercher/maroto/v2/pkg/core"
	"github.com/johnfercher/maroto/v2/pkg/core/entity"
	"github.com/johnfercher/maroto/v2/pkg/props"

	"github.com/qwerin/nanofaktura/internal/model"
)

type renderer struct {
	m    core.Maroto
	d    *doc
	th   theme
	opt  Options
	prov core.Provider // captured from Maroto for measuring rows before adding them
}

func (r *renderer) build() error {
	if err := r.m.RegisterFooter(r.footer()); err != nil {
		return err
	}
	// Zero-height row whose only job is to hand us Maroto's provider.
	r.m.AddRows(row.New().Add(col.New(gridSize).Add(&box{
		measure: func(p core.Provider, _ float64) float64 { r.prov = p; return 0 },
	})))

	r.m.AddRows(r.header()...)
	r.m.AddRows(r.parties())
	r.m.AddRows(r.info()...)
	if note := strings.TrimSpace(r.d.inv.Note); note != "" {
		r.m.AddRows(spacer(5), full(r.paragraph(note, 8.5, r.th.ink)))
	}
	r.m.AddRows(spacer(6))
	r.lines()
	r.keep(6, r.summary()...)
	if b := r.bottom(); b != nil {
		r.keep(6, full(b))
	}
	if fn := strings.TrimSpace(r.d.inv.FooterNote); fn != "" {
		r.keep(7, full(r.paragraph(fn, 8, r.th.muted)))
	}
	return nil
}

// ---------------------------------------------------------------- helpers

func full(b *box) core.Row { return row.New().Add(col.New(gridSize).Add(b)) }

func spacer(h float64) core.Row { return row.New(h).Add(col.New(gridSize)) }

// cols builds a row from (size, box) pairs; a nil box is an empty column.
func cols(parts ...any) core.Row {
	rw := row.New()
	for i := 0; i+1 < len(parts); i += 2 {
		c := col.New(parts[i].(int))
		if b, ok := parts[i+1].(*box); ok && b != nil {
			c.Add(b)
		}
		rw.Add(c)
	}
	return rw
}

func (r *renderer) rowsHeight(rows ...core.Row) float64 {
	cfg := r.m.GetCurrentConfig()
	h := 0.0
	for _, rw := range rows {
		rw.SetConfig(cfg)
		cell := entity.Cell{Width: contentW, Height: pageH - marginT - marginB}
		h += rw.GetHeight(r.prov, &cell)
	}
	return h
}

// keep adds rows so that they end up on one page (breaking before them if
// needed); lead is the space above them, dropped at the top of a new page.
func (r *renderer) keep(lead float64, rows ...core.Row) {
	if lead > 0 && r.m.FitlnCurrentPage(r.rowsHeight(append([]core.Row{spacer(lead)}, rows...)...)) {
		r.m.AddRows(append([]core.Row{spacer(lead)}, rows...)...)
		return
	}
	if lead == 0 && r.m.FitlnCurrentPage(r.rowsHeight(rows...)) {
		r.m.AddRows(rows...)
		return
	}
	r.m.AddPages(page.New().Add(rows...))
}

func (r *renderer) t(key string, args ...any) string { return r.d.l.t(key, args...) }

func (r *renderer) money(v int64) string { return r.d.l.money(v, r.d.currency) }

func (r *renderer) paragraph(s string, size float64, c props.Color) *box {
	st := &stack{}
	for i, line := range strings.Split(s, "\n") {
		gap := 0.0
		if i > 0 {
			gap = 1
		}
		if strings.TrimSpace(line) == "" {
			line = " "
		}
		st.add(txt{s: line, size: size, color: c, lead: 1}, gap)
	}
	return st.box()
}

// image draws img fitted into (w, h) at x, aligned left or right.
func drawImage(p core.Provider, img []byte, ext string, x, y, w, h float64, right bool) {
	if ext == "" {
		return
	}
	dim, err := p.GetDimensionsByImageByte(img, extension.Type(ext))
	if err != nil || dim.Width <= 0 || dim.Height <= 0 {
		return
	}
	s := min(w/dim.Width, h/dim.Height)
	iw, ih := dim.Width*s, dim.Height*s
	if right {
		x += w - iw
	}
	p.AddImageFromBytes(img, &entity.Cell{X: x, Y: y, Width: iw, Height: ih}, &props.Rect{Percent: 100}, extension.Type(ext))
}

// ---------------------------------------------------------------- header

func (r *renderer) header() []core.Row {
	d, th := r.d, r.th
	num := d.inv.Number
	switch th.name {
	case TemplateModern:
		const band = 26.0
		rows := []core.Row{row.New(band).Add(col.New(gridSize).Add(fixed(band, func(p core.Provider, x, y, w, h float64) {
			fillRect(p, -marginL, -marginT, pageW, marginT+band, th.accent)
			// thin lighter strip at the bottom edge of the band
			fillRect(p, -marginL, band-1.2, pageW, 1.2, mix(th.accent, colWhite, 0.25))
			tx := x
			if d.logoExt != "" {
				drawImage(p, r.opt.Logo, d.logoExt, x, y+1, 34, 14, false)
				tx += 40
			}
			tw := w - 58 - (tx - x)
			title := txt{s: d.title, size: 18, bold: true, color: th.onAcc, lead: 1}
			fitOneLine(p, &title, tw, 13)
			th1 := title.height(p, tw)
			sub := txt{s: d.subtitle, size: 8.5, color: mix(th.onAcc, th.accent, 0.25)}
			ty := y + 3
			if d.subtitle == "" && th1 < 8 {
				ty += 2
			}
			title.render(p, tx, ty, tw)
			sub.render(p, tx, ty+th1+1.8, tw)
			lbl := txt{s: strings.ToUpper(r.t(lNumber)), size: 7, bold: true, color: mix(th.onAcc, th.accent, 0.3), align: align.Right}
			lbl.render(p, x+w*0.5, y+3.5, w*0.5)
			txt{s: num, size: 17, bold: true, color: th.onAcc, align: align.Right}.render(p, x+w*0.5, y+7.5, w*0.5)
		}))), spacer(8)}
		return rows

	case TemplateMinimal:
		left := (&stack{}).
			add(txt{s: strings.ToUpper(d.title), size: 8, bold: true, color: th.muted}, 0).
			add(txt{s: num, size: 22, color: th.strong}, 2).
			add(txt{s: d.subtitle, size: 8.5, color: th.muted}, 1.5)
		var logo *box
		if d.logoExt != "" {
			logo = fixed(16, func(p core.Provider, x, y, w, h float64) {
				drawImage(p, r.opt.Logo, d.logoExt, x, y, w, h, true)
			})
		}
		return []core.Row{
			cols(15, left.box(), 9, logo),
			spacer(5),
			full(fixed(0.3, func(p core.Provider, x, y, w, h float64) { hline(p, x, y, w, 0.3, th.strong) })),
			spacer(7),
		}

	default: // classic
		right := (&stack{}).
			add(txt{s: r.t(lNumber), size: 7.5, color: th.muted, align: align.Right}, 0).
			add(txt{s: num, size: 17, bold: true, color: th.accent, align: align.Right}, 1)
		titleSt := (&stack{}).
			add(txt{s: d.title, size: 17, bold: true, color: th.strong}, 0).
			add(txt{s: d.subtitle, size: 8.5, color: th.muted}, 1.5)
		var top core.Row
		if d.logoExt != "" {
			logo := fixed(18, func(p core.Provider, x, y, w, h float64) {
				drawImage(p, r.opt.Logo, d.logoExt, x, y, w, h, false)
			})
			top = cols(9, logo, 15, vstack(1.5, titleSt.alignRight().box(), right.box()))
		} else {
			top = cols(15, titleSt.box(), 9, right.box())
		}
		return []core.Row{
			top,
			spacer(3.5),
			full(fixed(0.8, func(p core.Provider, x, y, w, h float64) { hline(p, x, y, w, 0.8, th.accent) })),
			spacer(7),
		}
	}
}

// fitOneLine shrinks t's font (down to minSize) until it fits one line of width w.
func fitOneLine(p core.Provider, t *txt, w, minSize float64) {
	for t.size > minSize {
		pr := t.props()
		if p.GetLinesQuantity(t.s, &pr, w) <= 1 {
			return
		}
		t.size -= 0.5
	}
}

// alignRight returns a copy of s with all texts right-aligned.
func (s *stack) alignRight() *stack {
	c := *s
	c.items = make([]stackItem, len(s.items))
	for i, it := range s.items {
		it.t.align = align.Right
		c.items[i] = it
	}
	return &c
}

// ---------------------------------------------------------------- parties

func (r *renderer) partyBox(heading string, p party) *box {
	th := r.th
	hc := th.accent
	if th.name == TemplateMinimal {
		hc = th.muted
	}
	st := &stack{}
	st.add(txt{s: strings.ToUpper(heading), size: 7, bold: true, color: hc}, 0).
		add(txt{s: p.name, size: 11, bold: true, color: th.strong, lead: 0.6}, 2.4).
		add(txt{s: p.fullName, size: 8.5, color: th.ink}, 1).
		add(txt{s: p.street, size: 8.5, color: th.ink}, 1.4).
		add(txt{s: p.cityLine, size: 8.5, color: th.ink}, 1).
		add(txt{s: p.country, size: 8.5, color: th.ink}, 1)
	gap := 2.6
	if p.regNo != "" {
		st.add(txt{s: r.t(lRegNo) + ": " + p.regNo, size: 8.5, color: th.ink}, gap)
		gap = 1
	}
	if p.vatNo != "" {
		st.add(txt{s: r.t(lVatNo) + ": " + p.vatNo, size: 8.5, color: th.ink}, gap)
		gap = 1
	}
	st.add(txt{s: p.vatNote, size: 8, ital: true, color: th.muted}, gap)

	switch th.name {
	case TemplateModern:
		bg := th.tint
		st.bg, st.padT, st.padB, st.padL, st.padR = &bg, 5, 5, 5, 5
	case TemplateClassic:
		rc := th.rule
		st.topRule, st.topRuleW, st.padT = &rc, 0.35, 3
	}
	return st.box()
}

func (r *renderer) parties() core.Row {
	sup, cli := r.d.supplier, r.d.customer
	if r.th.name == TemplateModern {
		return cols(12, r.partyBox(r.t(lSupplier), sup), 12, pad(r.partyBox(r.t(lCustomer), cli), 3, 0))
	}
	return cols(11, r.partyBox(r.t(lSupplier), sup), 2, nil, 11, r.partyBox(r.t(lCustomer), cli))
}

// pad insets b horizontally.
func pad(b *box, left, right float64) *box {
	return &box{
		measure: func(p core.Provider, w float64) float64 { return b.measure(p, w-left-right) },
		draw:    func(p core.Provider, x, y, w, h float64) { b.draw(p, x+left, y, w-left-right, h) },
	}
}

// ---------------------------------------------------------------- dates + payment

type kv struct{ k, v string }

func (r *renderer) paymentKVs() []kv {
	inv := r.d.inv
	out := []kv{{r.t(lMethod), r.d.l.paymentMethod(inv.PaymentMethod, inv.CustomPaymentMethod)}}
	if inv.PaymentMethod == "" || inv.PaymentMethod == "bank" {
		if inv.BankAccount != "" {
			out = append(out, kv{r.t(lAccountNo), inv.BankAccount})
		}
		if inv.IBAN != "" {
			out = append(out, kv{r.t(lIBAN), formatIBAN(inv.IBAN)})
		}
		if inv.SwiftBIC != "" {
			out = append(out, kv{r.t(lSWIFT), inv.SwiftBIC})
		}
	}
	if inv.VariableSymbol != "" {
		out = append(out, kv{r.t(lVS), inv.VariableSymbol})
	}
	if r.th.name == TemplateModern && inv.OrderNumber != "" {
		out = append(out, kv{r.t(lOrderNo), inv.OrderNumber})
	}
	return out
}

func (r *renderer) dateKVs() []kv {
	inv, l := r.d.inv, r.d.l
	out := []kv{{r.t(lIssuedOn), l.date(inv.IssuedOn)}}
	if r.showTaxable() {
		out = append(out, kv{r.t(lTaxableOn), l.date(inv.TaxableFulfillmentDue)})
	}
	if inv.DueOn != "" {
		out = append(out, kv{r.t(lDueOn), l.date(inv.DueOn)})
	}
	if inv.OrderNumber != "" {
		out = append(out, kv{r.t(lOrderNo), inv.OrderNumber})
	}
	return out
}

func (r *renderer) showTaxable() bool {
	return r.d.payer && r.d.inv.DocumentType != model.DocProforma && r.d.inv.TaxableFulfillmentDue != ""
}

func (r *renderer) kvBlock(heading string, items []kv, boldKey string, keyPct float64) *box {
	th := r.th
	hc := th.accent
	if th.name == TemplateMinimal {
		hc = th.muted
	}
	head := (&stack{}).add(txt{s: strings.ToUpper(heading), size: 7, bold: true, color: hc}, 0)
	l := &kvList{keyPct: keyPct}
	for i, it := range items {
		v := txt{s: it.v, size: 8.5, color: th.strong, align: align.Right}
		if it.k == boldKey {
			v.bold = true
		}
		row := kvRow{k: txt{s: it.k, size: 8.5, color: th.muted}, v: v, gap: 1.2}
		if i == 0 {
			row.gap = 0
		}
		l.rows = append(l.rows, row)
	}
	b := vstack(2.4, head.box(), l.box())
	if th.name == TemplateClassic {
		rc := th.rule
		inner := b
		b = &box{
			measure: func(p core.Provider, w float64) float64 { return inner.measure(p, w) + 3 },
			draw: func(p core.Provider, x, y, w, h float64) {
				hline(p, x, y, w, 0.35, rc)
				inner.draw(p, x, y+3, w, h-3)
			},
		}
	}
	return b
}

func (r *renderer) info() []core.Row {
	if r.th.name == TemplateModern {
		return []core.Row{spacer(6), full(r.factsStrip())}
	}
	return []core.Row{
		spacer(6),
		cols(11, r.kvBlock(r.t(lPaymentDetails), r.paymentKVs(), r.t(lVS), 0.36), 2, nil,
			11, r.kvBlock(r.t(lDates), r.dateKVs(), r.t(lDueOn), 0.5)),
	}
}

// factsStrip is the modern template's row of key facts ending with the amount due.
func (r *renderer) factsStrip() *box {
	d, th, l := r.d, r.th, r.d.l
	facts := []kv{{r.t(lIssuedOn), l.date(d.inv.IssuedOn)}}
	if r.showTaxable() {
		facts = append(facts, kv{r.t(lTaxableOn), l.date(d.inv.TaxableFulfillmentDue)})
	}
	if d.inv.DueOn != "" {
		facts = append(facts, kv{r.t(lDueOn), l.date(d.inv.DueOn)})
	}
	if d.inv.VariableSymbol != "" {
		facts = append(facts, kv{r.t(lVS), d.inv.VariableSymbol})
	}
	dueLabel := r.t(lToPay)
	if d.paid != 0 {
		dueLabel = r.t(lRemaining)
	}
	return fixed(16, func(p core.Provider, x, y, w, h float64) {
		amountW := w * 0.28
		fw := (w - amountW) / float64(len(facts))
		hline(p, x, y, w, 0.3, th.rule)
		hline(p, x, y+h-0.3, w, 0.3, th.rule)
		for i, f := range facts {
			fx := x + float64(i)*fw
			if i > 0 {
				fillRect(p, fx, y+3, 0.3, h-6, th.rule)
				fx += 4
			}
			txt{s: f.k, size: 7, color: th.muted}.render(p, fx, y+3.4, fw-5)
			txt{s: f.v, size: 9.5, bold: true, color: th.strong}.render(p, fx, y+8, fw-5)
		}
		ax := x + w - amountW
		fillRect(p, ax, y, amountW, h, th.accent)
		txt{s: dueLabel, size: 7, color: mix(th.onAcc, th.accent, 0.25), align: align.Right}.render(p, ax, y+3.4, amountW-4)
		txt{s: r.money(d.due), size: 12, bold: true, color: th.onAcc, align: align.Right}.render(p, ax, y+7.6, amountW-4)
	})
}

// formatIBAN groups an IBAN by 4 characters.
func formatIBAN(s string) string {
	s = strings.ToUpper(strings.ReplaceAll(s, " ", ""))
	var b strings.Builder
	for i, c := range s {
		if i > 0 && i%4 == 0 {
			b.WriteString(nbsp)
		}
		b.WriteRune(c)
	}
	return b.String()
}

// ---------------------------------------------------------------- lines table

type column struct {
	label string
	share float64
	right bool
}

func (r *renderer) columns() []column {
	unit := r.t(lUnitPrice)
	if r.d.inv.PricesIncludeVat {
		unit = r.t(lUnitPriceVat)
	}
	if r.d.payer {
		return []column{
			{r.t(lItem), 0.40, false}, {r.t(lQty), 0.11, true}, {unit, 0.13, true},
			{r.t(lVatRate), 0.08, true}, {r.t(lBase), 0.14, true}, {r.t(lLineTotal), 0.14, true},
		}
	}
	return []column{{r.t(lItem), 0.50, false}, {r.t(lQty), 0.14, true}, {unit, 0.17, true}, {r.t(lLineTotal), 0.19, true}}
}

func (r *renderer) cells(ln model.InvoiceLine) []string {
	l := r.d.l
	qty := l.quantity(ln.QuantityMilli)
	if ln.UnitName != "" {
		qty += nbsp + ln.UnitName
	}
	if r.d.payer {
		return []string{ln.Name, qty, l.amount(ln.UnitPrice), l.vatRate(ln.VatRateBps), l.amount(ln.Base), l.amount(ln.Total)}
	}
	total := ln.Total
	if total == 0 && ln.Base != 0 {
		total = ln.Base
	}
	return []string{ln.Name, qty, l.amount(ln.UnitPrice), l.amount(total)}
}

const cellPadX = 2.2

func (r *renderer) tableHeader() core.Row {
	th, cs := r.th, r.columns()
	fg := th.onAcc
	switch th.name {
	case TemplateModern:
		fg = th.accent
	case TemplateMinimal:
		fg = th.muted
	}
	hdrs := make([]txt, len(cs))
	for i, c := range cs {
		a := align.Left
		if c.right {
			a = align.Right
		}
		hdrs[i] = txt{s: c.label, size: 7.5, bold: true, color: fg, align: a}
	}
	measure := func(p core.Provider, w float64) float64 {
		h := 0.0
		for i, c := range cs {
			h = max(h, hdrs[i].height(p, w*c.share-2*cellPadX))
		}
		return h + 5
	}
	return full(&box{measure: measure, draw: func(p core.Provider, x, y, w, h float64) {
		switch th.name {
		case TemplateClassic:
			fillRect(p, x, y, w, h, th.accent)
		case TemplateModern:
			fillRect(p, x, y, w, h, th.tint)
		case TemplateMinimal:
			hline(p, x, y+h-0.35, w, 0.35, th.strong)
		}
		cx := x
		for i, c := range cs {
			cw := w * c.share
			hh := hdrs[i].height(p, cw-2*cellPadX)
			hdrs[i].render(p, cx+cellPadX, y+h-2.5-hh, cw-2*cellPadX)
			cx += cw
		}
	}})
}

func (r *renderer) lineRow(ln model.InvoiceLine, idx int, last bool) core.Row {
	th, cs := r.th, r.columns()
	vals := r.cells(ln)
	ts := make([]txt, len(cs))
	for i, c := range cs {
		t := txt{s: vals[i], size: 8.5, color: th.ink, lead: 0.8}
		if c.right {
			t.align = align.Right
		}
		if i == 0 {
			t.color = th.strong
		}
		if i == len(cs)-1 {
			t.bold = true
			t.color = th.strong
		}
		ts[i] = t
	}
	const padY = 2.1
	measure := func(p core.Provider, w float64) float64 {
		h := 0.0
		for i, c := range cs {
			h = max(h, ts[i].height(p, w*c.share-2*cellPadX))
		}
		return h + 2*padY
	}
	return full(&box{measure: measure, draw: func(p core.Provider, x, y, w, h float64) {
		switch th.name {
		case TemplateClassic:
			if idx%2 == 1 {
				fillRect(p, x, y, w, h, th.zebra)
			}
			if last {
				hline(p, x, y+h-0.4, w, 0.4, th.accent)
			}
		default:
			c, t := th.rule, 0.25
			if last && th.name == TemplateMinimal {
				c, t = th.strong, 0.35
			}
			hline(p, x, y+h-t, w, t, c)
		}
		cx := x
		for i, c := range cs {
			cw := w * c.share
			ts[i].render(p, cx+cellPadX, y+padY, cw-2*cellPadX)
			cx += cw
		}
	}})
}

// lines adds the table; on page breaks it repeats the header with a
// "continued" caption and keeps the header together with the first row.
func (r *renderer) lines() {
	lines := r.d.inv.Lines
	head := r.tableHeader()
	if len(lines) == 0 {
		r.m.AddRows(head)
		return
	}
	first := r.lineRow(lines[0], 0, len(lines) == 1)
	r.keep(0, head, first)
	for i := 1; i < len(lines); i++ {
		rw := r.lineRow(lines[i], i, i == len(lines)-1)
		if r.m.FitlnCurrentPage(r.rowsHeight(rw)) {
			r.m.AddRows(rw)
			continue
		}
		cont := full((&stack{}).add(txt{
			s:    r.d.title + " " + r.d.inv.Number + " – " + r.t(lContinued),
			size: 7.5, color: r.th.muted,
		}, 0).box())
		r.m.AddPages(page.New().Add(cont, spacer(3), r.tableHeader(), rw))
	}
}

// ---------------------------------------------------------------- summary

func (r *renderer) summary() []core.Row {
	var left *box
	if r.d.payer && len(r.d.recap) > 0 {
		left = r.recapBox()
	} else if r.qrInSummary() {
		left = r.qr()
	}
	return []core.Row{cols(13, left, 1, nil, 10, r.totalsBox())}
}

// qrInSummary: without a VAT recap the QR code sits next to the totals
// (except in modern, where it goes with the payment details).
func (r *renderer) qrInSummary() bool {
	return r.d.qr != "" && !(r.d.payer && len(r.d.recap) > 0) && r.th.name != TemplateModern
}

func (r *renderer) qr() *box {
	return qrBox(r.d.qr, 30, txt{s: r.t(lQR), size: 7.5, bold: true, color: r.th.muted, align: align.Center})
}

func (r *renderer) recapBox() *box {
	th, l := r.th, r.d.l
	hc := th.accent
	if th.name == TemplateMinimal {
		hc = th.muted
	}
	right := func(s string, bold bool) txt {
		return txt{s: s, size: 8, color: th.ink, align: align.Right, bold: bold}
	}
	g := &grid{cols: []float64{0.19, 0.27, 0.27, 0.27}, rule: th.rule, padV: 1.6}
	hdr := func(s string, a align.Type) txt { return txt{s: s, size: 7, bold: true, color: th.muted, align: a} }
	g.header = []txt{hdr(r.t(lRate), align.Left), hdr(r.t(lBase), align.Right), hdr(r.t(lVat), align.Right), hdr(r.t(lLineTotal), align.Right)}
	for _, rr := range r.d.recap {
		g.rows = append(g.rows, []txt{
			{s: l.vatRate(rr.rate), size: 8, color: th.ink},
			right(l.amount(rr.base), false), right(l.amount(rr.vat), false), right(l.amount(rr.total), false),
		})
	}
	head := (&stack{}).add(txt{s: strings.ToUpper(r.t(lVatRecap)), size: 7, bold: true, color: hc}, 0).box()
	parts := []*box{head, g.box()}
	if r.d.inv.ReverseCharge {
		parts = append(parts, (&stack{}).add(txt{s: r.t(lReverseCharge), size: 8, bold: true, color: th.strong}, 0).box())
	}
	return vstack(2, parts...)
}

func (r *renderer) totalsBox() *box {
	d, th := r.d, r.th
	l := &kvList{keyPct: 0.5}
	line := func(k, v string, strong bool) {
		kc := th.muted
		if strong {
			kc = th.strong
		}
		l.rows = append(l.rows, kvRow{
			k: txt{s: k, size: 8.5, color: kc, bold: strong}, v: txt{s: v, size: 8.5, color: th.strong, bold: strong, align: align.Right},
			gap: 1.4,
		})
	}
	if d.payer {
		line(r.t(lSubtotal), r.money(d.inv.Subtotal), false)
		line(r.t(lVatTotal), r.money(d.inv.VatTotal), false)
	}
	if d.inv.Rounding != 0 {
		line(r.t(lRounding), r.money(d.inv.Rounding), false)
	}
	fullyPaid := d.paid != 0 && d.due == 0
	if d.paid != 0 && !fullyPaid {
		line(r.t(lTotal), r.money(d.inv.Total), true)
		line(r.t(lPaid), r.money(-d.paid), false)
	}
	label, amount := r.t(lToPay), d.due
	switch {
	case fullyPaid:
		label, amount = r.t(lTotal), d.inv.Total
	case d.paid != 0:
		label = r.t(lRemaining)
	}
	parts := []*box{}
	if len(l.rows) > 0 {
		l.rows[0].gap = 0
		parts = append(parts, l.box())
	}
	parts = append(parts, r.amountBox(label, r.money(amount)))
	if fullyPaid {
		after := &kvList{keyPct: 0.5, rows: []kvRow{{
			k: txt{s: r.t(lPaid), size: 8.5, color: th.muted},
			v: txt{s: r.money(-d.paid), size: 8.5, color: th.strong, align: align.Right},
		}}}
		parts = append(parts, after.box())
	}
	return vstack(2.6, parts...)
}

// amountBox is the emphasized "total due" block: label above a large amount.
func (r *renderer) amountBox(label, amount string) *box {
	th := r.th
	lt := txt{s: label, size: 8, bold: true}
	at := txt{s: amount, size: 15, bold: true, align: align.Right}
	const padX, padY = 3.5, 3.2
	minimal := th.name == TemplateMinimal
	if minimal {
		lt.color, at.color = th.strong, th.accent
	} else {
		lt.color, at.color = mix(th.onAcc, th.accent, 0.2), th.onAcc
	}
	return &box{
		measure: func(p core.Provider, w float64) float64 {
			return 2*padY + lt.height(p, w-2*padX) + 1.6 + at.height(p, w-2*padX)
		},
		draw: func(p core.Provider, x, y, w, h float64) {
			if minimal {
				hline(p, x, y, w, 0.5, th.strong)
			} else {
				fillRect(p, x, y, w, h, th.accent)
			}
			px := padX
			if minimal {
				px = 0
			}
			lt.render(p, x+px, y+padY, w-2*px)
			at.render(p, x+px, y+padY+lt.height(p, w-2*px)+1.6, w-2*px)
		},
	}
}

// ---------------------------------------------------------------- bottom (QR, payment, stamps)

func (r *renderer) bottom() *box {
	d, th := r.d, r.th
	var qr *box
	if d.qr != "" && !r.qrInSummary() {
		qr = r.qr()
	}
	var payment *box
	if th.name == TemplateModern {
		payment = r.kvBlock(r.t(lPaymentDetails), r.paymentKVs(), r.t(lVS), 0.36)
	}
	stampText, stampColor := "", colPaid
	switch d.inv.Status {
	case model.StatusPaid:
		stampText = r.t(lStampPaid)
	case model.StatusCancelled:
		stampText, stampColor = r.t(lStampCancelled), colVoid
	}
	hasImg := d.stampExt != ""
	if qr == nil && payment == nil && stampText == "" && !hasImg {
		return nil
	}
	const imgW, imgH = 52.0, 26.0
	measure := func(p core.Provider, w float64) float64 {
		h := 0.0
		if qr != nil {
			h = qr.measure(p, 30)
		}
		if payment != nil {
			h = max(h, payment.measure(p, w*0.5))
		}
		if stampText != "" {
			h = max(h, 22)
		}
		if hasImg {
			h = max(h, imgH)
		}
		return h
	}
	return &box{measure: measure, draw: func(p core.Provider, x, y, w, h float64) {
		cx := x
		if qr != nil {
			qr.draw(p, cx, y, 30, h)
			cx += 38
		}
		if payment != nil {
			pw := w*0.66 - (cx - x)
			payment.draw(p, cx, y, pw, h)
			cx += pw + 6
		}
		if stampText != "" {
			drawStamp(p, stampText, r.stampDate(), stampColor, cx, y+1)
		}
		if hasImg {
			drawImage(p, r.opt.Stamp, d.stampExt, x+w-imgW, y, imgW, imgH, true)
		}
	}}
}

func (r *renderer) stampDate() string {
	switch r.d.inv.Status {
	case model.StatusPaid:
		return r.d.l.date(r.d.inv.PaidOn)
	case model.StatusCancelled:
		if r.d.inv.CancelledAt != nil {
			return r.d.l.date(r.d.inv.CancelledAt.Format("2006-01-02"))
		}
	}
	return ""
}

// drawStamp paints a rubber-stamp style double frame with text and date.
func drawStamp(p core.Provider, s, date string, c props.Color, x, y float64) {
	w, h := 50.0, 19.0
	strokeRect(p, x, y, w, h, 0.7, c)
	strokeRect(p, x+1.4, y+1.4, w-2.8, h-2.8, 0.25, c)
	ts := txt{s: s, size: 15, bold: true, color: c, align: align.Center}
	dt := txt{s: date, size: 8, color: c, align: align.Center}
	if date == "" {
		ts.render(p, x, y+(h-ts.height(p, w))/2, w)
		return
	}
	ts.render(p, x, y+3.6, w)
	dt.render(p, x, y+h-3.4-dt.height(p, w), w)
}

// ---------------------------------------------------------------- footer

func (r *renderer) footer() core.Row {
	d, th := r.d, r.th
	var info []string
	info = append(info, d.supplier.name)
	if d.supplier.regNo != "" {
		info = append(info, r.t(lRegNo)+" "+d.supplier.regNo)
	}
	if d.acc != nil {
		info = append(info, d.acc.Email, d.acc.Phone, d.acc.Web)
	}
	contact := joinNonEmpty("  ·  ", info...)
	reg := strings.TrimSpace(d.inv.YourRegisteredBy)
	if reg == "" && d.acc != nil && d.inv.YourName == "" {
		reg = d.acc.RegisteredBy
	}
	regT := txt{s: reg, size: 6.5, color: th.muted, lead: 0.5}
	conT := txt{s: contact, size: 7, color: th.muted}
	const w = contentW * 0.8
	return full(&box{
		measure: func(p core.Provider, _ float64) float64 { return 3 + regT.height(p, w) + 1 },
		draw: func(p core.Provider, x, y, fw, h float64) {
			hline(p, x, y+1, fw, 0.25, th.rule)
			regT.render(p, x, y+3, w)
			// Last line shares the baseline of Maroto's page number just below.
			conT.render(p, x, y+h, w)
		},
	})
}
