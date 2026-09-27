package pdf

// FormatMoney formats minor units with the currency the way the PDF does
// ("12 345,60 Kč", "€1,234.50") in lang (cs, en, sk, de; other → cs).
// Used for e-mail texts so they match the document.
func FormatMoney(minor int64, currency, lang string) string {
	return newLocale(lang).money(minor, currency)
}

// FormatDate formats "YYYY-MM-DD" the way the PDF does ("15. 3. 2026",
// "15 Mar 2026", "15.03.2026"); unparsable input is returned as is.
func FormatDate(iso, lang string) string {
	return newLocale(lang).date(iso)
}
