package api_test

import (
	"bytes"
	"net/http"
	"testing"

	"github.com/qwerin/nanofaktura/internal/api"
)

func TestAccountAppearanceOnboardingCapabilities(t *testing.T) {
	ts := newTestServer(t)
	a := ts.signup("a@example.cz", "Firma A")

	acc := doJSON[api.Account](a, http.StatusOK, "GET", a.acct(""), nil)
	if acc.PdfTemplate != "classic" || !acc.PdfShowQR || acc.OnboardedAt != nil ||
		acc.Capabilities != (api.Capabilities{Edit: true, ManageSettings: true, ManageMembers: true, ManageOwners: true, ViewReports: true, Export: true}) {
		t.Fatalf("defaults %+v", acc)
	}
	acc = doJSON[api.Account](a, http.StatusOK, "PATCH", a.acct(""), map[string]any{
		"pdf_template": "modern", "pdf_accent": "#112233", "pdf_show_qr": false, "pdf_footer": "Děkujeme",
		"default_language": "de", "onboarded": true,
	})
	if acc.PdfTemplate != "modern" || acc.PdfAccent != "#112233" || acc.PdfShowQR || acc.PdfFooter != "Děkujeme" ||
		acc.DefaultLanguage != "de" || acc.OnboardedAt == nil || !acc.OnboardedAt.Equal(ts.now) {
		t.Fatalf("patched %+v", acc)
	}
	res, body := a.do("PATCH", a.acct(""), map[string]any{"pdf_accent": "red"})
	assertCode(t, res, body, http.StatusUnprocessableEntity, "validation_failed")
	if acc = doJSON[api.Account](a, http.StatusOK, "PATCH", a.acct(""), map[string]any{"onboarded": false}); acc.OnboardedAt != nil {
		t.Fatalf("onboarding reset %+v", acc.OnboardedAt)
	}

	// PDFs render with the saved settings; the preview accepts unsaved ones
	subj := newSubject(a, api.SubjectCreate{Name: "ACME"})
	inv := createInv(a, api.InvoiceCreate{SubjectID: subj.ID, Lines: []api.InvoiceLineInput{line("A", "1", 100, nil)}})
	if b := a.mustDo(http.StatusOK, "GET", invURL(a, inv.ID, "/pdf"), nil); !bytes.HasPrefix(b, []byte("%PDF")) {
		t.Fatalf("pdf %q", b[:10])
	}
	if b := a.mustDo(http.StatusOK, "GET", a.acct("/pdf-preview?accent=%23AA0000&show_qr=true&footer=x"), nil); !bytes.HasPrefix(b, []byte("%PDF")) {
		t.Fatalf("preview %q", b[:10])
	}

	acct := ts.memberOf(a, "u@example.cz", "accountant")
	if got := doJSON[api.Account](acct, http.StatusOK, "GET", acct.acct(""), nil).Capabilities; got !=
		(api.Capabilities{ViewReports: true, Export: true}) {
		t.Fatalf("accountant %+v", got)
	}
	mem := ts.memberOf(a, "m@example.cz", "member")
	if got := doJSON[api.Account](mem, http.StatusOK, "GET", mem.acct(""), nil).Capabilities; got !=
		(api.Capabilities{Edit: true, Export: true}) {
		t.Fatalf("member %+v", got)
	}
}
