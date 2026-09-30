package api_test

import (
	"strings"
	"testing"

	"github.com/qwerin/nanofaktura/internal/api"
	"github.com/qwerin/nanofaktura/internal/config"
	"github.com/qwerin/nanofaktura/internal/mail"
)

func TestInstanceWarnings(t *testing.T) {
	secure := config.Config{PublicURL: "https://faktury.example.cz", SMTPHost: "smtp.example.cz", SMTPTLS: mail.TLSStartTLS, SetupToken: "x"}
	cases := []struct {
		name     string
		mod      func(*config.Config)
		hasUsers bool
		want     []string // substrings, in order; nil = only the DKIM hint
	}{
		{"production-like", func(*config.Config) {}, true, []string{"DKIM"}},
		{"no SMTP", func(c *config.Config) { c.SMTPHost = "" }, true, []string{"SMTP není nastavené"}},
		{"http public URL", func(c *config.Config) { c.PublicURL = "http://localhost:8080" }, true, []string{"není https", "DKIM"}},
		{"open registration", func(c *config.Config) { c.SetupToken = "" }, false, []string{"NANOFAKTURA_SETUP_TOKEN", "DKIM"}},
		{"open registration but users exist", func(c *config.Config) { c.SetupToken = "" }, true, []string{"DKIM"}},
		{"rate limit off", func(c *config.Config) { c.DisableRateLimit = true }, true, []string{"NANOFAKTURA_DISABLE_RATE_LIMIT", "DKIM"}},
		{"plain SMTP remote", func(c *config.Config) { c.SMTPTLS = mail.TLSNone }, true, []string{"není šifrované", "DKIM"}},
		{"plain SMTP localhost", func(c *config.Config) { c.SMTPTLS, c.SMTPHost = mail.TLSNone, "localhost" }, true, []string{"DKIM"}},
		{"plain SMTP .local", func(c *config.Config) { c.SMTPTLS, c.SMTPHost = mail.TLSNone, "mail.local" }, true, []string{"DKIM"}},
		{"plain SMTP ::1", func(c *config.Config) { c.SMTPTLS, c.SMTPHost = mail.TLSNone, "::1" }, true, []string{"DKIM"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := secure
			tc.mod(&cfg)
			got := api.InstanceWarnings(cfg, tc.hasUsers)
			if len(got) != len(tc.want) {
				t.Fatalf("warnings %q, want %d matching %q", got, len(tc.want), tc.want)
			}
			for i, w := range tc.want {
				if !strings.Contains(got[i], w) {
					t.Errorf("warning %d = %q, want it to contain %q", i, got[i], w)
				}
			}
		})
	}
}
