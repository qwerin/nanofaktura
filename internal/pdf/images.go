package pdf

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"sync"

	xdraw "golang.org/x/image/draw"
)

// Limits of images embedded into documents (logo, stamp). An image is decoded
// only after its header passed CheckImage, so a small file declaring huge
// dimensions (decompression bomb) never allocates its pixel buffer.
const (
	MaxImageSide   = 4000       // px, either side
	MaxImagePixels = 16_000_000 // width × height
	// MaxImageBytes is the file size limit of a logo/stamp.
	MaxImageBytes = 2 << 20

	// renderMaxSide: images are downscaled to at most this many pixels on the
	// longer side before embedding (plenty for a 60 mm wide logo at 300 dpi).
	renderMaxSide = 1200
)

// ErrImage is returned by CheckImage for unsupported or oversized images.
var ErrImage = errors.New("unsupported or oversized image")

// CheckImage validates an image for embedding without decoding its pixels:
// PNG or JPEG, at most MaxImageBytes, MaxImageSide per side and
// MaxImagePixels in total. It returns the format ("png" | "jpeg").
func CheckImage(b []byte) (string, error) {
	if len(b) > MaxImageBytes {
		return "", fmt.Errorf("%w: file is larger than %d MB", ErrImage, MaxImageBytes>>20)
	}
	return checkImageHeader(b)
}

// checkImageHeader is CheckImage without the file size limit (render time:
// images stored before the limit existed are still embedded when their
// dimensions are fine).
func checkImageHeader(b []byte) (string, error) {
	var (
		cfg  image.Config
		err  error
		kind string
	)
	switch imageExt(b) {
	case "png":
		cfg, err = png.DecodeConfig(bytes.NewReader(b))
		kind = "png"
	case "jpg":
		cfg, err = jpeg.DecodeConfig(bytes.NewReader(b))
		kind = "jpeg"
	default:
		return "", fmt.Errorf("%w: not a PNG or JPEG", ErrImage)
	}
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrImage, err)
	}
	if cfg.Width <= 0 || cfg.Height <= 0 || cfg.Width > MaxImageSide || cfg.Height > MaxImageSide ||
		int64(cfg.Width)*int64(cfg.Height) > MaxImagePixels {
		return "", fmt.Errorf("%w: %d×%d px exceeds %d×%d px", ErrImage, cfg.Width, cfg.Height, MaxImageSide, MaxImageSide)
	}
	return kind, nil
}

// imageCache keeps normalized images (by content hash) so repeated renders
// (public links, ZIP exports) decode each logo only once.
var imageCache = struct {
	sync.Mutex
	m map[[32]byte][]byte
}{m: map[[32]byte][]byte{}}

const imageCacheSize = 64

// decodeSem serializes image decoding (bounds peak memory).
var decodeSem = make(chan struct{}, 1)

// normalizeImage returns img re-encoded by Go's decoders (PNG always, JPEG
// when downscaled) at most renderMaxSide px on the longer side, or nil when
// the image is invalid or exceeds the limits (the document is then rendered
// without it). The PDF library never sees the original bytes of a PNG: its
// own decoder inflates the whole compressed stream regardless of dimensions.
func normalizeImage(b []byte) []byte {
	if len(b) == 0 {
		return nil
	}
	key := sha256.Sum256(b)
	imageCache.Lock()
	out, ok := imageCache.m[key]
	imageCache.Unlock()
	if ok {
		return out
	}
	decodeSem <- struct{}{}
	out = normalizeUncached(b)
	<-decodeSem
	imageCache.Lock()
	if len(imageCache.m) >= imageCacheSize {
		clear(imageCache.m)
	}
	imageCache.m[key] = out
	imageCache.Unlock()
	return out
}

func normalizeUncached(b []byte) []byte {
	format, err := checkImageHeader(b)
	if err != nil {
		return nil
	}
	var img image.Image
	if format == "png" {
		img, err = png.Decode(bytes.NewReader(b))
	} else {
		img, err = jpeg.Decode(bytes.NewReader(b))
	}
	if err != nil {
		return nil
	}
	w, h := img.Bounds().Dx(), img.Bounds().Dy()
	scaled := false
	if w > renderMaxSide || h > renderMaxSide {
		nw, nh := renderMaxSide, max(1, h*renderMaxSide/w)
		if h > w {
			nw, nh = max(1, w*renderMaxSide/h), renderMaxSide
		}
		dst := image.NewNRGBA(image.Rect(0, 0, nw, nh))
		xdraw.CatmullRom.Scale(dst, dst.Bounds(), img, img.Bounds(), xdraw.Src, nil)
		img, scaled = dst, true
	}
	var buf bytes.Buffer
	switch {
	case format == "jpeg" && !scaled:
		return b // fully decoded fine; JPEG data is embedded as is
	case format == "jpeg":
		err = jpeg.Encode(&buf, img, &jpeg.Options{Quality: 90})
	default:
		err = (&png.Encoder{CompressionLevel: png.BestCompression}).Encode(&buf, img)
	}
	if err != nil {
		return nil
	}
	return buf.Bytes()
}
