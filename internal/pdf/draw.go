package pdf

// Low-level drawing on top of Maroto: a generic component ("box") that measures
// and draws itself directly through the provider, plus vector primitives.
// Maroto's grid only offers rows/cols; boxes give us stacked text, key/value
// lists, small tables, filled rectangles and a vector QR code while Maroto
// still handles page flow.

import (
	"image"

	"github.com/boombuler/barcode/qr"
	"github.com/johnfercher/go-tree/node"
	"github.com/johnfercher/maroto/v2/pkg/consts/align"
	"github.com/johnfercher/maroto/v2/pkg/consts/breakline"
	"github.com/johnfercher/maroto/v2/pkg/consts/fontstyle"
	"github.com/johnfercher/maroto/v2/pkg/consts/linestyle"
	"github.com/johnfercher/maroto/v2/pkg/consts/orientation"
	"github.com/johnfercher/maroto/v2/pkg/core"
	"github.com/johnfercher/maroto/v2/pkg/core/entity"
	"github.com/johnfercher/maroto/v2/pkg/props"
)

// box is a custom Maroto component. measure returns its height for a given
// width; draw paints it into the rectangle (x, y, w, h), coordinates relative
// to the page margins like Maroto cells.
type box struct {
	measure func(p core.Provider, w float64) float64
	draw    func(p core.Provider, x, y, w, h float64)
}

func (b *box) SetConfig(*entity.Config) {}

func (b *box) GetStructure() *node.Node[core.Structure] {
	return node.New(core.Structure{Type: "nanofaktura-box"})
}

func (b *box) GetHeight(p core.Provider, cell *entity.Cell) float64 {
	if b.measure == nil {
		return 0
	}
	return b.measure(p, cell.Width)
}

func (b *box) Render(p core.Provider, cell *entity.Cell) {
	if b.draw != nil {
		b.draw(p, cell.X, cell.Y, cell.Width, cell.Height)
	}
}

// fixed returns a box of a constant height.
func fixed(h float64, draw func(p core.Provider, x, y, w, h float64)) *box {
	return &box{measure: func(core.Provider, float64) float64 { return h }, draw: draw}
}

// fillRect paints a solid rectangle (a butt-capped line as thick as the rect).
func fillRect(p core.Provider, x, y, w, h float64, c props.Color) {
	if w <= 0 || h <= 0 {
		return
	}
	p.AddLine(&entity.Cell{X: x, Y: y, Width: w, Height: h}, &props.Line{
		Color: &c, Style: linestyle.Solid, Thickness: h,
		Orientation: orientation.Horizontal, OffsetPercent: 50, SizePercent: 100,
	})
}

// hline draws a horizontal rule whose top edge is at y.
func hline(p core.Provider, x, y, w, thickness float64, c props.Color) {
	fillRect(p, x, y, w, thickness, c)
}

// strokeRect draws a rectangle outline of the given thickness (inside the bounds).
func strokeRect(p core.Provider, x, y, w, h, t float64, c props.Color) {
	fillRect(p, x, y, w, t, c)
	fillRect(p, x, y+h-t, w, t, c)
	fillRect(p, x, y, t, h, c)
	fillRect(p, x+w-t, y, t, h, c)
}

// txt describes one piece of text.
type txt struct {
	s     string
	size  float64
	bold  bool
	ital  bool
	color props.Color
	align align.Type
	lead  float64 // extra space between wrapped lines (mm)
}

func (t txt) props() props.Text {
	st := fontstyle.Normal
	switch {
	case t.bold && t.ital:
		st = fontstyle.BoldItalic
	case t.bold:
		st = fontstyle.Bold
	case t.ital:
		st = fontstyle.Italic
	}
	al := t.align
	if al == "" {
		al = align.Left
	}
	c := t.color
	return props.Text{
		Family: fontFamily, Style: st, Size: t.size, Color: &c, Align: al,
		BreakLineStrategy: breakline.EmptySpaceStrategy, VerticalPadding: t.lead,
	}
}

// height of t wrapped into width w.
func (t txt) height(p core.Provider, w float64) float64 {
	if t.s == "" {
		return 0
	}
	pr := t.props()
	n := p.GetLinesQuantity(t.s, &pr, w)
	if n < 1 {
		n = 1
	}
	fh := p.GetFontHeight(&props.Font{Family: fontFamily, Style: pr.Style, Size: t.size})
	return float64(n)*fh + float64(n-1)*t.lead
}

// render draws t with its top edge at y.
func (t txt) render(p core.Provider, x, y, w float64) {
	if t.s == "" {
		return
	}
	pr := t.props()
	p.AddText(t.s, &entity.Cell{X: x, Y: y, Width: w, Height: 10000}, &pr)
}

// stackItem is one element of a vertical stack: text or a spacer.
type stackItem struct {
	t   txt
	gap float64 // space above the item
}

// stack is a vertical list of texts with padding, optionally on a background.
type stack struct {
	items                  []stackItem
	padT, padB, padL, padR float64
	bg                     *props.Color
	topRule                *props.Color // rule along the top edge
	topRuleW               float64
}

func (s *stack) add(t txt, gap float64) *stack {
	s.items = append(s.items, stackItem{t: t, gap: gap})
	return s
}

func (s *stack) measure(p core.Provider, w float64) float64 {
	h := s.padT + s.padB
	for _, it := range s.items {
		if it.t.s == "" {
			continue
		}
		h += it.gap + it.t.height(p, w-s.padL-s.padR)
	}
	return h
}

func (s *stack) box() *box {
	return &box{
		measure: s.measure,
		draw: func(p core.Provider, x, y, w, h float64) {
			if s.bg != nil {
				fillRect(p, x, y, w, h, *s.bg)
			}
			if s.topRule != nil {
				hline(p, x, y, w, s.topRuleW, *s.topRule)
			}
			cy := y + s.padT
			iw := w - s.padL - s.padR
			for _, it := range s.items {
				if it.t.s == "" {
					continue
				}
				cy += it.gap
				it.t.render(p, x+s.padL, cy, iw)
				cy += it.t.height(p, iw)
			}
		},
	}
}

// kvRow is one line of a key/value list.
type kvRow struct {
	k, v   txt
	gap    float64 // space above
	bg     *props.Color
	padV   float64 // vertical padding inside bg
	rule   *props.Color
	ruleW  float64
	keyPct float64 // share of width for the key (0 = kvList default)
}

// kvList renders label/value rows; values right- or left-aligned per txt.align.
type kvList struct {
	rows   []kvRow
	keyPct float64
}

func (l *kvList) rowHeight(p core.Provider, r kvRow, w float64) float64 {
	kw, vw := l.split(r, w)
	h := max(r.k.height(p, kw-1), r.v.height(p, vw))
	return h + 2*r.padV
}

func (l *kvList) split(r kvRow, w float64) (float64, float64) {
	pct := l.keyPct
	if r.keyPct > 0 {
		pct = r.keyPct
	}
	kw := w * pct
	return kw, w - kw
}

func (l *kvList) box() *box {
	return &box{
		measure: func(p core.Provider, w float64) float64 {
			h := 0.0
			for _, r := range l.rows {
				h += r.gap + l.rowHeight(p, r, w)
			}
			return h
		},
		draw: func(p core.Provider, x, y, w, _ float64) {
			cy := y
			for _, r := range l.rows {
				cy += r.gap
				rh := l.rowHeight(p, r, w)
				if r.bg != nil {
					fillRect(p, x, cy, w, rh, *r.bg)
				}
				if r.rule != nil {
					hline(p, x, cy, w, r.ruleW, *r.rule)
				}
				kw, vw := l.split(r, w)
				pad := 0.0
				if r.bg != nil {
					pad = 3
				}
				// Vertically center both texts in the row.
				kh, vh := r.k.height(p, kw-1), r.v.height(p, vw)
				r.k.render(p, x+pad, cy+(rh-kh)/2, kw-1-pad)
				r.v.render(p, x+kw, cy+(rh-vh)/2, vw-pad)
				cy += rh
			}
		},
	}
}

// grid is a small table (VAT recap) with fixed column shares.
type grid struct {
	cols   []float64 // column shares, sum 1
	header []txt
	rows   [][]txt
	hdrBG  *props.Color
	rule   props.Color
	padV   float64
}

func (g *grid) rowH(p core.Provider, cells []txt, w float64) float64 {
	h := 0.0
	for i, c := range cells {
		h = max(h, c.height(p, w*g.cols[i]-2))
	}
	return h + 2*g.padV
}

func (g *grid) box() *box {
	all := append([][]txt{g.header}, g.rows...)
	return &box{
		measure: func(p core.Provider, w float64) float64 {
			h := 0.0
			for _, r := range all {
				h += g.rowH(p, r, w)
			}
			return h
		},
		draw: func(p core.Provider, x, y, w, _ float64) {
			cy := y
			for ri, r := range all {
				rh := g.rowH(p, r, w)
				if ri == 0 && g.hdrBG != nil {
					fillRect(p, x, cy, w, rh, *g.hdrBG)
				}
				cx := x
				for i, c := range r {
					cw := w * g.cols[i]
					c.render(p, cx+1.5, cy+g.padV, cw-3)
					cx += cw
				}
				cy += rh
				hline(p, x, cy-0.15, w, 0.15, g.rule)
			}
		},
	}
}

// vstack stacks boxes vertically with gaps.
func vstack(gap float64, boxes ...*box) *box {
	var bs []*box
	for _, b := range boxes {
		if b != nil {
			bs = append(bs, b)
		}
	}
	return &box{
		measure: func(p core.Provider, w float64) float64 {
			h := 0.0
			for i, b := range bs {
				if i > 0 {
					h += gap
				}
				h += b.measure(p, w)
			}
			return h
		},
		draw: func(p core.Provider, x, y, w, _ float64) {
			cy := y
			for i, b := range bs {
				if i > 0 {
					cy += gap
				}
				bh := b.measure(p, w)
				b.draw(p, x, cy, w, bh)
				cy += bh
			}
		},
	}
}

// qrBox draws a vector QR code of the given side (mm) with a caption under it.
// Modules are painted as horizontal runs, slightly overlapping to avoid
// hairline seams in anti-aliasing viewers.
func qrBox(payload string, side float64, caption txt) *box {
	code, err := qr.Encode(payload, qr.M, qr.Auto)
	if err != nil {
		return nil
	}
	n := code.Bounds().Dx()
	capH := func(p core.Provider) float64 { return 1.5 + caption.height(p, side+20) }
	return &box{
		measure: func(p core.Provider, _ float64) float64 { return side + capH(p) },
		draw: func(p core.Provider, x, y, _, _ float64) {
			m := side / float64(n)
			ink := props.Color{Red: 17, Green: 24, Blue: 39}
			for row := range n {
				for col := 0; col < n; {
					if !isDark(code, col, row) {
						col++
						continue
					}
					start := col
					for col < n && isDark(code, col, row) {
						col++
					}
					fillRect(p, x+float64(start)*m, y+float64(row)*m, float64(col-start)*m+0.02, m+0.04, ink)
				}
			}
			caption.render(p, x-10, y+side+1.5, side+20)
		},
	}
}

func isDark(img image.Image, x, y int) bool {
	r, _, _, _ := img.At(img.Bounds().Min.X+x, img.Bounds().Min.Y+y).RGBA()
	return r < 0x8000
}
