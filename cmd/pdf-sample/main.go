// Command pdf-sample renders demo invoices with every PDF template into a
// directory (default tmp/pdf-samples, gitignored) for visual review:
//
//	go run ./cmd/pdf-sample [-out dir] [-accent "#0f766e"] [-logo logo.png] [-png]
//
// With -png every page is also rasterized via pdftoppm (poppler) if installed.
package main

import (
	"bytes"
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"log"
	"math"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/qwerin/nanofaktura/internal/model"
	"github.com/qwerin/nanofaktura/internal/pdf"
)

type sample struct {
	name string
	spec pdf.SampleSpec
	logo bool
}

var samples = []sample{
	{"invoice-payer", pdf.SampleSpec{VatPayer: true}, false},
	{"invoice-nonpayer", pdf.SampleSpec{Lines: 3}, false},
	{"invoice-logo-partial", pdf.SampleSpec{VatPayer: true, Lines: 4, PaidPartially: true}, true},
	{"invoice-paid", pdf.SampleSpec{VatPayer: true, Lines: 3, Status: model.StatusPaid}, false},
	{"invoice-reverse-charge-en", pdf.SampleSpec{VatPayer: true, Lines: 3, ReverseCharge: true, Language: "en"}, false},
	{"proforma-payer", pdf.SampleSpec{DocumentType: model.DocProforma, VatPayer: true, Lines: 3}, false},
	{"correction-payer-de", pdf.SampleSpec{DocumentType: model.DocCorrection, VatPayer: true, Language: "de"}, false},
	{"invoice-sk-cancelled", pdf.SampleSpec{Lines: 2, Status: model.StatusCancelled, Language: "sk"}, false},
	{"invoice-60-lines", pdf.SampleSpec{VatPayer: true, Lines: 60}, false},
}

func main() {
	out := flag.String("out", "tmp/pdf-samples", "output directory")
	accent := flag.String("accent", "", "accent color override (#RRGGBB)")
	logoPath := flag.String("logo", "", "logo PNG/JPEG (default: generated demo logo)")
	pngs := flag.Bool("png", false, "also render PNG pages with pdftoppm")
	flag.Parse()

	logo := demoLogo()
	if *logoPath != "" {
		b, err := os.ReadFile(*logoPath)
		if err != nil {
			log.Fatal(err)
		}
		logo = b
	}
	if err := os.MkdirAll(*out, 0o755); err != nil {
		log.Fatal(err)
	}
	for _, tpl := range pdf.Templates {
		for _, s := range samples {
			inv, acc := pdf.Sample(s.spec)
			opt := pdf.Options{Template: tpl, Accent: *accent, ShowQR: true}
			if s.spec.DocumentType == model.DocCorrection {
				opt.RelatedNumber = "2026-0041"
			}
			if s.logo {
				opt.Logo = logo
			}
			b, err := pdf.Render(inv, acc, opt)
			if err != nil {
				log.Fatalf("%s/%s: %v", tpl, s.name, err)
			}
			path := filepath.Join(*out, tpl+"-"+s.name+".pdf")
			if err := os.WriteFile(path, b, 0o644); err != nil {
				log.Fatal(err)
			}
			fmt.Println(path)
			if *pngs {
				cmd := exec.Command("pdftoppm", "-png", "-r", "110", path, path[:len(path)-4])
				if msg, err := cmd.CombinedOutput(); err != nil {
					log.Printf("pdftoppm: %v %s", err, msg)
				}
			}
		}
	}
}

// demoLogo draws a simple geometric mark (a ring and a bar) as a PNG.
func demoLogo() []byte {
	const w, h = 360, 120
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	teal := color.NRGBA{R: 15, G: 118, B: 110, A: 255}
	amber := color.NRGBA{R: 245, G: 158, B: 11, A: 255}
	for y := range h {
		for x := range w {
			dx, dy := float64(x-60), float64(y-60)
			d := math.Hypot(dx, dy)
			switch {
			case d > 34 && d < 54:
				img.Set(x, y, teal)
			case x > 130 && x < 350 && y > 40 && y < 58:
				img.Set(x, y, teal)
			case x > 130 && x < 260 && y > 68 && y < 80:
				img.Set(x, y, amber)
			}
		}
	}
	var buf bytes.Buffer
	_ = png.Encode(&buf, img)
	return buf.Bytes()
}
