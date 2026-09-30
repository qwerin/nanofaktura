package api_test

import (
	"bytes"
	"encoding/binary"
	"hash/crc32"
	"net/http"
	"testing"

	"github.com/qwerin/nanofaktura/internal/model"
)

// hugePNG is a PNG header declaring w×h pixels (IHDR only): tiny on disk.
func hugePNG(w, h uint32) []byte {
	ihdr := make([]byte, 13)
	binary.BigEndian.PutUint32(ihdr[0:], w)
	binary.BigEndian.PutUint32(ihdr[4:], h)
	ihdr[8], ihdr[9] = 8, 6
	var b bytes.Buffer
	b.WriteString("\x89PNG\r\n\x1a\n")
	_ = binary.Write(&b, binary.BigEndian, uint32(len(ihdr)))
	crc := crc32.NewIEEE()
	crc.Write([]byte("IHDR"))
	crc.Write(ihdr)
	b.WriteString("IHDR")
	b.Write(ihdr)
	_ = binary.Write(&b, binary.BigEndian, crc.Sum32())
	b.Write(bytes.Repeat([]byte{0}, 64))
	return b.Bytes()
}

// Logos/stamps are limited in size and dimensions (decompression bombs), and
// an oversized image already stored is left out of the PDF instead of being
// inflated.
func TestAccountImageLimits(t *testing.T) {
	ts := newTestServer(t)
	a := ts.signup("a@example.cz", "Firma A")

	bomb := uploadOK(a, "account", 0, "bomb.png", hugePNG(8000, 8000))
	res, body := a.do("PATCH", a.acct(""), map[string]any{"logo_attachment_id": bomb.ID})
	assertError(t, res, body, http.StatusUnprocessableEntity, "4000×4000 px")
	res, body = a.do("PATCH", a.acct(""), map[string]any{"stamp_attachment_id": bomb.ID})
	assertError(t, res, body, http.StatusUnprocessableEntity, "4000×4000 px")

	big := uploadOK(a, "account", 0, "big.png", append(encodeImage("png", 4, 4), make([]byte, 3<<20)...))
	res, body = a.do("PATCH", a.acct(""), map[string]any{"logo_attachment_id": big.ID})
	assertError(t, res, body, http.StatusUnprocessableEntity, "at most 2 MB")

	// stored before the limits existed: the PDF renders without it
	if err := ts.db.Model(&model.Account{}).Where("slug = ?", a.slug).
		Updates(map[string]any{"logo_attachment_id": bomb.ID, "stamp_attachment_id": bomb.ID}).Error; err != nil {
		t.Fatal(err)
	}
	res, body = a.do("GET", a.acct("/pdf-preview"), nil)
	if res.StatusCode != http.StatusOK || !bytes.HasPrefix(body, []byte("%PDF")) {
		t.Fatalf("preview: %d %.200s", res.StatusCode, body)
	}
}
