package pdf

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"errors"
	"hash/crc32"
	"image"
	"image/color"
	"image/png"
	"runtime"
	"testing"
)

// pngChunk encodes one PNG chunk.
func pngChunk(typ string, data []byte) []byte {
	var b bytes.Buffer
	_ = binary.Write(&b, binary.BigEndian, uint32(len(data)))
	b.WriteString(typ)
	b.Write(data)
	crc := crc32.NewIEEE()
	crc.Write([]byte(typ))
	crc.Write(data)
	_ = binary.Write(&b, binary.BigEndian, crc.Sum32())
	return b.Bytes()
}

// bombPNG is a valid RGBA PNG of w×h fully transparent pixels: tiny on disk,
// w*h*4 bytes once inflated.
func bombPNG(t *testing.T, w, h int) []byte {
	t.Helper()
	ihdr := make([]byte, 13)
	binary.BigEndian.PutUint32(ihdr[0:], uint32(w))
	binary.BigEndian.PutUint32(ihdr[4:], uint32(h))
	ihdr[8], ihdr[9] = 8, 6 // 8-bit RGBA
	var idat bytes.Buffer
	zw, _ := zlib.NewWriterLevel(&idat, zlib.BestCompression)
	row := make([]byte, 1+w*4) // filter byte + pixels
	for range h {
		if _, err := zw.Write(row); err != nil {
			t.Fatal(err)
		}
	}
	_ = zw.Close()
	var b bytes.Buffer
	b.WriteString("\x89PNG\r\n\x1a\n")
	b.Write(pngChunk("IHDR", ihdr))
	b.Write(pngChunk("IDAT", idat.Bytes()))
	b.Write(pngChunk("IEND", nil))
	return b.Bytes()
}

func encodePNG(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			img.Set(x, y, color.NRGBA{R: uint8(x), G: uint8(y), B: 90, A: 200})
		}
	}
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func TestCheckImage(t *testing.T) {
	if f, err := CheckImage(encodePNG(t, 40, 20)); err != nil || f != "png" {
		t.Fatalf("small png: %q %v", f, err)
	}
	if _, err := CheckImage(bombPNG(t, 6000, 10)); !errors.Is(err, ErrImage) {
		t.Fatalf("too wide: %v", err)
	}
	if _, err := CheckImage([]byte("GIF89a....")); !errors.Is(err, ErrImage) {
		t.Fatalf("gif: %v", err)
	}
	big := append(encodePNG(t, 4, 4), make([]byte, MaxImageBytes)...)
	if _, err := CheckImage(big); !errors.Is(err, ErrImage) {
		t.Fatalf("over the size limit: %v", err)
	}
}

// A small PNG declaring huge dimensions must not be inflated when rendering
// (it is left out of the document instead).
func TestRenderSkipsDecompressionBomb(t *testing.T) {
	bomb := bombPNG(t, 6000, 6000) // ~144 MB of pixels in a few hundred KB
	if len(bomb) > 1<<20 {
		t.Fatalf("bomb is %d bytes", len(bomb))
	}
	inv, acc := Sample(SampleSpec{})
	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)
	b, err := Render(inv, acc, Options{Logo: bomb, Stamp: bomb})
	runtime.ReadMemStats(&after)
	if err != nil || !bytes.HasPrefix(b, []byte("%PDF")) {
		t.Fatalf("render: %v", err)
	}
	if alloc := after.TotalAlloc - before.TotalAlloc; alloc > 100<<20 {
		t.Fatalf("rendering allocated %d MB", alloc>>20)
	}
}

func TestNormalizeImageDownscales(t *testing.T) {
	src := encodePNG(t, 2400, 600)
	out := normalizeImage(src)
	cfg, err := png.DecodeConfig(bytes.NewReader(out))
	if err != nil || cfg.Width != renderMaxSide || cfg.Height != 300 {
		t.Fatalf("normalized: %+v %v", cfg, err)
	}
	if again := normalizeImage(src); !bytes.Equal(again, out) {
		t.Fatal("cache returned different bytes")
	}
	if normalizeImage([]byte("junk")) != nil {
		t.Fatal("junk accepted")
	}
}
