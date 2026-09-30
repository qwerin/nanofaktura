package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/qwerin/nanofaktura/internal/api"
	"github.com/qwerin/nanofaktura/internal/config"
	"github.com/qwerin/nanofaktura/internal/maildiag"
)

const mailUsage = `usage:
  nanofaktura mail test --to <email> [--dkim-selector <selector>]

test diagnoses the SMTP settings step by step (DNS, connection, TLS, login,
sending), checks the MX/SPF/DKIM/DMARC records of the sender domain and sends
a test e-mail to --to. --dkim-selector checks the DKIM record of that selector
when the application does not sign itself (e.g. your SMTP provider's).
The settings come from the NANOFAKTURA_* environment. Exit status 1 when a
check failed.`

// runMail runs "nanofaktura mail …" (args after "mail").
func runMail(ctx context.Context, args []string, stdout io.Writer) error {
	if len(args) == 0 {
		return errors.New(mailUsage)
	}
	switch args[0] {
	case "test":
		return mailTest(ctx, args[1:], stdout, nil)
	case "-h", "--help", "help":
		fmt.Fprintln(stdout, mailUsage)
		return nil
	}
	return fmt.Errorf("unknown mail command %q\n%s", args[0], mailUsage)
}

// mailTest runs the diagnostics; resolver nil = system DNS.
func mailTest(ctx context.Context, args []string, stdout io.Writer, resolver maildiag.Resolver) error {
	fs := flag.NewFlagSet("mail test", flag.ContinueOnError)
	to := fs.String("to", "", "recipient of the test e-mail")
	selector := fs.String("dkim-selector", "", "DKIM selector to check (optional)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if !strings.Contains(*to, "@") {
		return errors.New("--to <email> is required\n" + mailUsage)
	}
	if *selector != "" && !maildiag.ValidSelector(*selector) {
		return fmt.Errorf("invalid --dkim-selector %q", *selector)
	}
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	r := maildiag.Run(ctx, api.EmailTestOptions(cfg, strings.ToLower(strings.TrimSpace(*to)), *selector, resolver, time.Now()))
	printReport(stdout, r)
	if r.Status == maildiag.Error {
		return errors.New("e-mail test failed")
	}
	return nil
}

var statusMark = map[maildiag.Status]string{maildiag.OK: "[ OK ]", maildiag.Info: "[INFO]", maildiag.Warning: "[WARN]", maildiag.Error: "[FAIL]"}

func printReport(w io.Writer, r *maildiag.DiagReport) {
	fmt.Fprintf(w, "Test e-mailu: %s → %s přes %s:%d (%s)\n", r.From, r.To, r.SMTPHost, r.SMTPPort, r.TLSMode)
	section := func(title string, checks []maildiag.DiagCheck) {
		fmt.Fprintf(w, "\n%s\n", title)
		for _, c := range checks {
			fmt.Fprintf(w, "%s %s: %s (%d ms)\n", statusMark[c.Status], c.Title, c.Message, c.DurationMS)
			if c.Hint != "" {
				fmt.Fprintf(w, "       → %s\n", c.Hint)
			}
			for _, d := range c.Details {
				fmt.Fprintf(w, "         %s\n", d)
			}
		}
	}
	section("SMTP", r.SMTP)
	section("DNS", r.DNS)
	fmt.Fprintln(w)
	if r.Sent {
		fmt.Fprintf(w, "Zpráva odeslána (%s). Celkem %d ms.\n", r.ServerReply, r.DurationMS)
	} else {
		fmt.Fprintf(w, "Zpráva NEBYLA odeslána. Celkem %d ms.\n", r.DurationMS)
	}
}
