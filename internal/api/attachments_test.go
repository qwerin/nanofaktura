package api_test

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/qwerin/nanofaktura/internal/api"
	"github.com/qwerin/nanofaktura/internal/auth"
	"github.com/qwerin/nanofaktura/internal/model"
)

var (
	pdfData  = []byte("%PDF-1.4\n1 0 obj << >> endobj\ntrailer << >>\n%%EOF\n")
	pngData  = encodeImage("png", 4, 2)
	jpegData = encodeImage("jpeg", 4, 2)
	webpData = append([]byte("RIFF\x24\x00\x00\x00WEBPVP8 "), bytes.Repeat([]byte{2}, 40)...)
	heicData = append([]byte("\x00\x00\x00\x18ftypheic\x00\x00\x00\x00mif1heic"), bytes.Repeat([]byte{3}, 40)...)
	xmlData  = []byte(`<?xml version="1.0" encoding="UTF-8"?><Invoice xmlns="http://isdoc.cz/namespace/2013"></Invoice>`)
	xml2Data = []byte("\n  <Invoice><ID>1</ID></Invoice>")
)

// encodeImage returns a real w×h PNG or JPEG (logos/stamps are decoded).
func encodeImage(format string, w, h int) []byte {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for x := range w {
		img.Set(x, x*h/w, color.RGBA{R: 200, A: 255})
	}
	var buf bytes.Buffer
	var err error
	if format == "jpeg" {
		err = jpeg.Encode(&buf, img, nil)
	} else {
		err = png.Encode(&buf, img)
	}
	if err != nil {
		panic(err)
	}
	return buf.Bytes()
}

// upload posts a multipart form with the given fields and one file.
func (c *client) upload(fields map[string]string, filename string, data []byte) (*http.Response, []byte) {
	c.ts.t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	for k, v := range fields {
		_ = mw.WriteField(k, v)
	}
	if filename != "" {
		fw, _ := mw.CreateFormFile("file", filename)
		_, _ = fw.Write(data)
	}
	_ = mw.Close()
	req := httptest.NewRequest("POST", c.acct("/attachments"), &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.AddCookie(&http.Cookie{Name: auth.SessionCookie, Value: c.session})
	rec := httptest.NewRecorder()
	c.ts.handler.ServeHTTP(rec, req)
	return rec.Result(), rec.Body.Bytes()
}

func uploadOK(c *client, ownerType string, ownerID uint, filename string, data []byte) api.Attachment {
	c.ts.t.Helper()
	res, body := c.upload(map[string]string{"owner_type": ownerType, "owner_id": strconv.Itoa(int(ownerID))}, filename, data)
	if res.StatusCode != http.StatusCreated {
		c.ts.t.Fatalf("upload %s: %d %s", filename, res.StatusCode, body)
	}
	return decodeJSON[api.Attachment](c.ts.t, body)
}

func attURL(c *client, id uint, suffix string) string {
	return fmt.Sprintf("%s/%d%s", c.acct("/attachments"), id, suffix)
}

func TestAttachments(t *testing.T) {
	ts := newTestServer(t)
	a := ts.signup("a@example.cz", "Firma A")
	b := ts.signup("b@example.cz", "Firma B")
	sa := newSubject(a, api.SubjectCreate{Name: "ACME"})
	sb := newSubject(b, api.SubjectCreate{Name: "B klient"})

	att := uploadOK(a, "subject", sa.ID, "../../etc/smlouva č.1.pdf", pdfData)
	if att.OwnerType != "subject" || att.OwnerID != sa.ID || att.Filename != "smlouva č.1.pdf" ||
		att.ContentType != "application/pdf" || att.Size != int64(len(pdfData)) {
		t.Fatalf("uploaded: %+v", att)
	}

	// content sniffing: allowed types regardless of the declared name
	for name, tc := range map[string]struct {
		data []byte
		ct   string
	}{
		"a.png": {pngData, "image/png"}, "a.jpg": {jpegData, "image/jpeg"}, "a.webp": {webpData, "image/webp"},
		"a.heic": {heicData, "image/heic"}, "a.xml": {xmlData, "application/xml"}, "b.xml": {xml2Data, "application/xml"},
		"scan.bin": {pdfData, "application/pdf"},
	} {
		if got := uploadOK(a, "subject", sa.ID, name, tc.data); got.ContentType != tc.ct {
			t.Errorf("%s: content type %q, want %q", name, got.ContentType, tc.ct)
		}
	}
	for name, data := range map[string][]byte{"a.pdf": []byte("just text pretending to be a pdf"), "a.exe": []byte("MZ\x90\x00\x03\x00\x00\x00"), "a.html": []byte("<html><body>x</body></html>")} {
		res, body := a.upload(map[string]string{"owner_type": "subject", "owner_id": strconv.Itoa(int(sa.ID))}, name, data)
		assertError(t, res, body, http.StatusUnsupportedMediaType, "unsupported file type")
	}

	// owner validation
	for _, tc := range []struct {
		fields map[string]string
		status int
		msg    string
	}{
		{map[string]string{"owner_type": "subject", "owner_id": strconv.Itoa(int(sb.ID))}, http.StatusNotFound, "owner not found"},
		{map[string]string{"owner_type": "invoice", "owner_id": "999"}, http.StatusNotFound, "owner not found"},
		{map[string]string{"owner_type": "expense", "owner_id": "1"}, http.StatusNotFound, "owner not found"},
		{map[string]string{"owner_type": "account", "owner_id": "999"}, http.StatusNotFound, "owner not found"},
		{map[string]string{"owner_type": "robot", "owner_id": "1"}, http.StatusUnprocessableEntity, "owner_type"},
	} {
		res, body := a.upload(tc.fields, "a.pdf", pdfData)
		assertError(t, res, body, tc.status, tc.msg)
	}
	res, body := a.upload(map[string]string{"owner_type": "subject", "owner_id": strconv.Itoa(int(sa.ID))}, "", nil)
	assertError(t, res, body, http.StatusUnprocessableEntity, "file")
	res, body = a.upload(map[string]string{"owner_type": "subject", "owner_id": strconv.Itoa(int(sa.ID))}, "big.pdf",
		append(append([]byte{}, pdfData...), make([]byte, api.MaxAttachmentSize)...))
	assertError(t, res, body, http.StatusRequestEntityTooLarge, "20 MB")
	// an invoice can own attachments
	inv := createInv(a, api.InvoiceCreate{SubjectID: sa.ID, Lines: []api.InvoiceLineInput{line("Práce", "1", 1000, nil)}})
	uploadOK(a, "invoice", inv.ID, "podklad.pdf", pdfData)

	// list + filter
	list := doJSON[api.ListResponse[api.Attachment]](a, http.StatusOK, "GET", a.acct(fmt.Sprintf("/attachments?owner_type=subject&owner_id=%d", sa.ID)), nil)
	if list.Total != 8 || list.Items[0].ID != att.ID {
		t.Fatalf("list: total %d", list.Total)
	}
	if l := doJSON[api.ListResponse[api.Attachment]](a, http.StatusOK, "GET", a.acct("/attachments?owner_type=invoice"), nil); l.Total != 1 {
		t.Fatalf("invoice list: %+v", l)
	}
	got := doJSON[api.Attachment](a, http.StatusOK, "GET", attURL(a, att.ID, ""), nil)
	if got != att {
		t.Fatalf("get: %+v", got)
	}

	// download
	res, body = a.do("GET", attURL(a, att.ID, "/download"), nil)
	if res.StatusCode != http.StatusOK || !bytes.Equal(body, pdfData) || res.Header.Get("Content-Type") != "application/pdf" ||
		res.Header.Get("Content-Disposition") != `attachment; filename*=utf-8''smlouva%20%C4%8D.1.pdf` ||
		res.Header.Get("X-Content-Type-Options") != "nosniff" {
		t.Fatalf("download: %d %v %q", res.StatusCode, res.Header, body)
	}
	res, _ = a.do("GET", attURL(a, att.ID, "/download?inline=true"), nil)
	if cd := res.Header.Get("Content-Disposition"); cd[:6] != "inline" {
		t.Fatalf("inline disposition %q", cd)
	}

	// isolation: b sees nothing of a's attachments
	for _, r := range []struct{ method, suffix string }{{"GET", ""}, {"GET", "/download"}, {"DELETE", ""}} {
		res, body := b.do(r.method, attURL(b, att.ID, r.suffix), nil)
		assertError(t, res, body, http.StatusNotFound, "attachment not found")
	}
	if l := doJSON[api.ListResponse[api.Attachment]](b, http.StatusOK, "GET", b.acct("/attachments"), nil); l.Total != 0 {
		t.Fatalf("b list: %+v", l)
	}

	// delete removes the stored file
	var m model.Attachment
	ts.db.First(&m, att.ID)
	if _, err := os.Stat(filepath.Join(ts.dataDir, m.StorageKey)); err != nil {
		t.Fatalf("stored file: %v", err)
	}
	a.mustDo(http.StatusNoContent, "DELETE", attURL(a, att.ID, ""), nil)
	if _, err := os.Stat(filepath.Join(ts.dataDir, m.StorageKey)); !os.IsNotExist(err) {
		t.Fatalf("file not deleted: %v", err)
	}
	res, body = a.do("GET", attURL(a, att.ID, "/download"), nil)
	assertError(t, res, body, http.StatusNotFound, "attachment not found")
}

func TestAccountLogoAndStamp(t *testing.T) {
	ts := newTestServer(t)
	a := ts.signup("a@example.cz", "Firma A")
	b := ts.signup("b@example.cz", "Firma B")
	member := ts.memberOf(a, "m@example.cz", "member")

	logo := uploadOK(a, "account", 0, "logo.png", pngData) // owner_id 0 = current account
	stamp := uploadOK(a, "account", logo.OwnerID, "razitko.jpg", jpegData)
	// account files are settings: a member can neither upload nor delete them
	res, body := member.upload(map[string]string{"owner_type": "account", "owner_id": "0"}, "x.png", pngData)
	assertError(t, res, body, http.StatusForbidden, "your role (member)")
	res, body = member.do("DELETE", attURL(member, logo.ID, ""), nil)
	assertError(t, res, body, http.StatusForbidden, "your role (member)")
	pdf := uploadOK(a, "account", logo.OwnerID, "vypis.pdf", pdfData)
	foreign := uploadOK(b, "account", 0, "b.png", pngData)

	acc := doJSON[api.Account](a, http.StatusOK, "PATCH", a.acct(""), map[string]any{"logo_attachment_id": logo.ID, "stamp_attachment_id": stamp.ID})
	if acc.LogoAttachmentID == nil || *acc.LogoAttachmentID != logo.ID || acc.StampAttachmentID == nil || *acc.StampAttachmentID != stamp.ID {
		t.Fatalf("patched: %+v", acc)
	}
	res, body = a.do("PATCH", a.acct(""), map[string]any{"logo_attachment_id": pdf.ID})
	assertError(t, res, body, http.StatusUnprocessableEntity, "PNG or JPEG")
	res, body = a.do("PATCH", a.acct(""), map[string]any{"stamp_attachment_id": foreign.ID})
	assertError(t, res, body, http.StatusUnprocessableEntity, "attachment not found")
	// members cannot change settings
	res, body = member.do("PATCH", member.acct(""), map[string]any{"logo_attachment_id": 0})
	assertError(t, res, body, http.StatusForbidden, "your role (member)")

	// deleting the stamp attachment clears the account field; 0 clears the logo
	a.mustDo(http.StatusNoContent, "DELETE", attURL(a, stamp.ID, ""), nil)
	acc = doJSON[api.Account](a, http.StatusOK, "GET", a.acct(""), nil)
	if acc.StampAttachmentID != nil || acc.LogoAttachmentID == nil {
		t.Fatalf("after stamp delete: %+v", acc)
	}
	acc = doJSON[api.Account](a, http.StatusOK, "PATCH", a.acct(""), map[string]any{"logo_attachment_id": 0})
	if acc.LogoAttachmentID != nil {
		t.Fatalf("logo not cleared: %+v", acc)
	}
}

func TestInvoicePDFWithLogo(t *testing.T) {
	ts := newTestServer(t)
	a := ts.signup("a@example.cz", "Firma A")
	img := image.NewRGBA(image.Rect(0, 0, 40, 20))
	for x := 0; x < 40; x++ {
		img.Set(x, x/2, color.RGBA{R: 200, A: 255})
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	logo := uploadOK(a, "account", 0, "logo.png", buf.Bytes())
	a.mustDo(http.StatusOK, "PATCH", a.acct(""), map[string]any{"logo_attachment_id": logo.ID, "stamp_attachment_id": logo.ID})

	subj := newSubject(a, api.SubjectCreate{Name: "ACME"})
	inv := createInv(a, api.InvoiceCreate{SubjectID: subj.ID, Lines: []api.InvoiceLineInput{line("Práce", "1", 1000, nil)}})
	res, body := a.do("GET", invURL(a, inv.ID, "/pdf"), nil)
	if res.StatusCode != http.StatusOK || !bytes.HasPrefix(body, []byte("%PDF")) {
		t.Fatalf("pdf: %d %.200s", res.StatusCode, body)
	}
}
