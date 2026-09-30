package maildiag

import (
	"strconv"
	"strings"
	"time"
	_ "time/tzdata" // Europe/Prague without system tzdata

	"github.com/qwerin/nanofaktura/internal/mail"
)

// TestSubject is the subject of the test e-mail.
const TestSubject = "Testovací e-mail z NanoFaktury"

const testText = `Dobrý den,

tento e-mail poslala NanoFaktura ({url}) jako test nastavení odesílání pošty.
Pokud ho čtete, server zprávu úspěšně doručil.

Odesláno: {time}
Odesílatel: {from}
SMTP server: {smtp}
Podpis DKIM v aplikaci: {dkim}

Jak ověřit, že e-maily neskončí ve spamu (Gmail):
1. Otevřete tuto zprávu, klikněte na tři tečky vpravo nahoře a zvolte „Zobrazit originál“.
2. V tabulce nahoře by mělo být u SPF, DKIM i DMARC „PASS“.
   V hlavičce Authentication-Results hledejte dkim=pass, spf=pass a dmarc=pass.
3. Pokud některá kontrola neprojde, porovnejte DNS záznamy domény s výsledkem testu
   v NanoFaktuře (Správa instance → Test e-mailu).

Na tuto zprávu není třeba odpovídat.
`

var prague, _ = time.LoadLocation("Europe/Prague")

// TestMessage is the Czech test e-mail (From empty = the configured sender).
func TestMessage(publicURL string, cfg mail.SMTPConfig, to string, now time.Time) mail.Message {
	dkim := "vypnutý"
	if cfg.DKIM != nil {
		dkim = "zapnutý (doména " + cfg.DKIM.Domain + ", selektor " + cfg.DKIM.Selector + ")"
	}
	loc := prague
	if loc == nil {
		loc = time.UTC
	}
	text := mail.Render(testText, map[string]string{
		"url": publicURL, "time": now.In(loc).Format("2. 1. 2006 15:04:05 MST"), "from": cfg.From,
		"smtp": cfg.Host + ":" + strconv.Itoa(cfg.Port) + " (" + defaultStr(cfg.TLS, mail.TLSStartTLS) + ")", "dkim": dkim,
	})
	return mail.Message{To: []string{to}, Subject: TestSubject, Text: strings.TrimSpace(text) + "\n"}
}
