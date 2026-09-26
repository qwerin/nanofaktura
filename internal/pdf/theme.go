package pdf

import "github.com/johnfercher/maroto/v2/pkg/props"

// theme holds the colors of a template. Layout differences are switched on name.
type theme struct {
	name   string
	accent props.Color // brand color
	onAcc  props.Color // text on accent fills
	tint   props.Color // very light accent (cards, table header in modern)
	ink    props.Color // body text
	strong props.Color // names, amounts
	muted  props.Color // labels
	rule   props.Color // hairlines
	zebra  props.Color // alternate table rows
}

var (
	colInk    = props.Color{Red: 31, Green: 41, Blue: 55}    // gray-800
	colStrong = props.Color{Red: 17, Green: 24, Blue: 39}    // gray-900
	colMuted  = props.Color{Red: 107, Green: 114, Blue: 128} // gray-500
	colRule   = props.Color{Red: 229, Green: 231, Blue: 235} // gray-200
	colZebra  = props.Color{Red: 247, Green: 248, Blue: 250}
	colWhite  = props.Color{Red: 255, Green: 255, Blue: 255}
	colPaid   = props.Color{Red: 21, Green: 128, Blue: 61}   // green-700
	colVoid   = props.Color{Red: 185, Green: 28, Blue: 28}   // red-700
	colOnAccS = props.Color{Red: 226, Green: 232, Blue: 240} // secondary text on dark fills
)

func themeFor(name string) (theme, bool) {
	base := theme{name: name, onAcc: colWhite, ink: colInk, strong: colStrong, muted: colMuted, rule: colRule, zebra: colZebra}
	switch name {
	case TemplateClassic:
		return base.withAccent(props.Color{Red: 30, Green: 58, Blue: 95}), true // navy
	case TemplateModern:
		return base.withAccent(props.Color{Red: 91, Green: 33, Blue: 182}), true // violet-800
	case TemplateMinimal:
		return base.withAccent(colStrong), true
	}
	return theme{}, false
}

func (t theme) withAccent(c props.Color) theme {
	t.accent = c
	t.tint = mix(c, colWhite, 0.93)
	// Dark text on very light accents so the "total due" box stays legible.
	if luminance(c) > 0.6 {
		t.onAcc = colStrong
	} else {
		t.onAcc = colWhite
	}
	return t
}

// mix returns a blended with b; w is the weight of b (0..1).
func mix(a, b props.Color, w float64) props.Color {
	f := func(x, y int) int { return int(float64(x)*(1-w) + float64(y)*w + 0.5) }
	return props.Color{Red: f(a.Red, b.Red), Green: f(a.Green, b.Green), Blue: f(a.Blue, b.Blue)}
}

func luminance(c props.Color) float64 {
	return (0.299*float64(c.Red) + 0.587*float64(c.Green) + 0.114*float64(c.Blue)) / 255
}
