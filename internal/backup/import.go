package backup

import (
	"archive/zip"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/qwerin/nanofaktura/internal/billing"
	"github.com/qwerin/nanofaktura/internal/events"
	"github.com/qwerin/nanofaktura/internal/model"
	"github.com/qwerin/nanofaktura/internal/slug"
	"github.com/qwerin/nanofaktura/internal/storage"
)

// ImportOptions configure Import.
type ImportOptions struct {
	OwnerUserID uint      // becomes the owner of the new account
	Name        string    // name of the new account; "" = name from the backup
	Now         time.Time // clock of the account.imported event; zero = time.Now
	MaxBytes    int64     // limit of the total uncompressed size; 0 = DefaultMaxBytes
}

// Warning describes something Import changed on purpose (Code is stable,
// Message English; Count = number of affected records when relevant).
type Warning struct {
	Code    string `json:"code" enum:"recurring_deactivated,reminders_disabled,paid_thanks_disabled,webhooks_inactive,bank_tokens_removed,public_links_regenerated,members_not_imported,attachments_missing,orphans_skipped"`
	Message string `json:"message"`
	Count   int    `json:"count,omitempty"`
}

// Warning codes.
const (
	WarnRecurringDeactivated   = "recurring_deactivated"
	WarnRemindersDisabled      = "reminders_disabled"
	WarnPaidThanksDisabled     = "paid_thanks_disabled"
	WarnWebhooksInactive       = "webhooks_inactive"
	WarnBankTokensRemoved      = "bank_tokens_removed"
	WarnPublicLinksRegenerated = "public_links_regenerated"
	WarnMembersNotImported     = "members_not_imported"
	WarnAttachmentsMissing     = "attachments_missing"
	WarnOrphansSkipped         = "orphans_skipped"
	// WarnTotalsRecomputed: stored totals or paid amounts of documents did not
	// match their lines / payments (an edited or older backup) and were recomputed.
	WarnTotalsRecomputed = "totals_recomputed"
)

// archive is a validated backup: parsed JSON files + attachment entries.
type archive struct {
	manifest Manifest
	entries  map[string]*zip.File

	account          Account
	bankAccounts     []BankAccount
	numberFormats    []NumberFormat
	subjects         []Subject
	priceItems       []PriceItem
	stockMoves       []StockMove
	invoices         []Invoice
	expenses         []Expense
	templates        []Template
	recurring        []Recurring
	bankTransactions []BankTransaction
	todos            []Todo
	events           []Event
	emailLogs        []EmailLog
	webhooks         []Webhook
	members          []Member
	attachments      []Attachment
}

func corrupt(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrCorrupt, fmt.Sprintf(format, args...))
}

// safeName reports whether a ZIP entry name is a clean relative path (no
// absolute paths, "..", backslashes or drive letters — zip-slip). Import
// never writes entry names to disk, but rejects such archives anyway.
func safeName(name string) bool {
	if name == "" || len(name) > 1024 || strings.ContainsAny(name, "\\\x00:") || strings.HasPrefix(name, "/") {
		return false
	}
	if path.Clean(name) != name {
		return false
	}
	for _, seg := range strings.Split(name, "/") {
		if seg == ".." || seg == "." || seg == "" {
			return false
		}
	}
	return true
}

// readArchive validates the ZIP (paths, sizes, manifest, checksums) and parses its JSON files.
func readArchive(r io.ReaderAt, size, maxBytes int64) (*archive, error) {
	zr, err := zip.NewReader(r, size)
	if err != nil {
		return nil, corrupt("not a ZIP archive")
	}
	if len(zr.File) > maxEntries {
		return nil, fmt.Errorf("%w: more than %d files", ErrTooLarge, maxEntries)
	}
	a := &archive{entries: map[string]*zip.File{}}
	var total uint64
	for _, f := range zr.File {
		if strings.HasSuffix(f.Name, "/") && f.UncompressedSize64 == 0 {
			continue // directory
		}
		if !safeName(f.Name) {
			return nil, corrupt("unsafe path %q", f.Name)
		}
		if _, dup := a.entries[f.Name]; dup {
			return nil, corrupt("duplicate file %q", f.Name)
		}
		a.entries[f.Name] = f
		total += f.UncompressedSize64
		if total > uint64(maxBytes) {
			return nil, fmt.Errorf("%w: more than %d MB uncompressed", ErrTooLarge, maxBytes>>20)
		}
	}

	mf, ok := a.entries[FileManifest]
	if !ok {
		return nil, corrupt("manifest.json is missing (not a NanoFaktura backup)")
	}
	mb, err := readEntry(mf, maxManifest)
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(mb, &a.manifest); err != nil {
		return nil, corrupt("manifest.json: %v", err)
	}
	if a.manifest.Format != Format {
		return nil, corrupt("manifest.json: format %q is not %q", a.manifest.Format, Format)
	}
	if a.manifest.Version > Version {
		return nil, fmt.Errorf("%w: backup version %d is newer than supported version %d; upgrade NanoFaktura",
			ErrUnsupportedVersion, a.manifest.Version, Version)
	}
	if a.manifest.Version < 1 {
		return nil, corrupt("manifest.json: invalid version %d", a.manifest.Version)
	}
	// every file listed in the manifest must be present and intact
	for name, fi := range a.manifest.Files {
		f, ok := a.entries[name]
		if !ok {
			return nil, corrupt("file %q listed in the manifest is missing", name)
		}
		limit := maxBytes
		if strings.HasPrefix(name, attachmentsDir) {
			limit = maxAttachment
		}
		sum, n, err := hashEntry(f, limit)
		if err != nil {
			return nil, err
		}
		if sum != strings.ToLower(fi.SHA256) || n != fi.Size {
			return nil, corrupt("checksum of %q does not match the manifest", name)
		}
	}

	parse := func(name string, v any, required bool) error {
		if _, listed := a.manifest.Files[name]; !listed {
			if required {
				return corrupt("%s is missing", name)
			}
			return nil // absent (or not covered by the manifest) = empty
		}
		b, err := readEntry(a.entries[name], maxBytes)
		if err != nil {
			return err
		}
		if err := json.Unmarshal(b, v); err != nil {
			return corrupt("%s: %v", name, err)
		}
		return nil
	}
	for _, p := range []struct {
		name     string
		v        any
		required bool
	}{
		{FileAccount, &a.account, true},
		{FileBankAccounts, &a.bankAccounts, false},
		{FileNumberFormats, &a.numberFormats, false},
		{FileSubjects, &a.subjects, false},
		{FilePriceItems, &a.priceItems, false},
		{FileStockMoves, &a.stockMoves, false},
		{FileInvoices, &a.invoices, false},
		{FileExpenses, &a.expenses, false},
		{FileTemplates, &a.templates, false},
		{FileRecurring, &a.recurring, false},
		{FileBankTransactions, &a.bankTransactions, false},
		{FileTodos, &a.todos, false},
		{FileEvents, &a.events, false},
		{FileEmailLogs, &a.emailLogs, false},
		{FileWebhooks, &a.webhooks, false},
		{FileMembers, &a.members, false},
		{FileAttachments, &a.attachments, false},
	} {
		if err := parse(p.name, p.v, p.required); err != nil {
			return nil, err
		}
	}
	for i := range a.attachments {
		at := &a.attachments[i]
		if at.Path == "" {
			continue
		}
		if !strings.HasPrefix(at.Path, attachmentsDir) {
			return nil, corrupt("attachment %d: invalid path %q", at.ID, at.Path)
		}
		fi, listed := a.manifest.Files[at.Path]
		if !listed {
			return nil, corrupt("attachment %d: %q is not listed in the manifest", at.ID, at.Path)
		}
		at.Size = fi.Size
	}
	return a, nil
}

// readEntry reads a whole entry of at most limit bytes.
func readEntry(f *zip.File, limit int64) ([]byte, error) {
	if f.UncompressedSize64 > uint64(limit) {
		return nil, fmt.Errorf("%w: %s is larger than %d MB", ErrTooLarge, f.Name, limit>>20)
	}
	rc, err := f.Open()
	if err != nil {
		return nil, corrupt("%s: %v", f.Name, err)
	}
	defer rc.Close()
	b, err := io.ReadAll(io.LimitReader(rc, limit+1))
	if err != nil {
		return nil, corrupt("%s: %v", f.Name, err)
	}
	if int64(len(b)) > limit {
		return nil, fmt.Errorf("%w: %s is larger than %d MB", ErrTooLarge, f.Name, limit>>20)
	}
	return b, nil
}

// hashEntry returns the hex SHA-256 and size of an entry of at most limit bytes.
func hashEntry(f *zip.File, limit int64) (string, int64, error) {
	if f.UncompressedSize64 > uint64(limit) {
		return "", 0, fmt.Errorf("%w: %s is larger than %d MB", ErrTooLarge, f.Name, limit>>20)
	}
	rc, err := f.Open()
	if err != nil {
		return "", 0, corrupt("%s: %v", f.Name, err)
	}
	defer rc.Close()
	h := sha256.New()
	n, err := io.Copy(h, io.LimitReader(rc, limit+1))
	if err != nil {
		return "", 0, corrupt("%s: %v", f.Name, err)
	}
	if n > limit {
		return "", 0, fmt.Errorf("%w: %s is larger than %d MB", ErrTooLarge, f.Name, limit>>20)
	}
	return hex.EncodeToString(h.Sum(nil)), n, nil
}

// idMap maps IDs of the backup to IDs of the new account.
type idMap map[uint]uint

// opt maps an optional reference; unknown (deleted or foreign) → nil.
func (m idMap) opt(id *uint) *uint {
	if id == nil {
		return nil
	}
	if n, ok := m[*id]; ok {
		return &n
	}
	return nil
}

// recompute sets the document totals (and the line amounts) from the lines
// with the billing rules and paid from the payments; it reports whether a
// stored value differed. Lines that cannot be computed are left as stored.
func recompute(lines []Line, payments []Payment, opt billing.Options, subtotal, vat, rounding, total, paid *int64) bool {
	bl := make([]billing.Line, len(lines))
	for i, l := range lines {
		bl[i] = billing.Line{QuantityMilli: l.QuantityMilli, UnitPrice: l.UnitPrice, VatRateBps: billing.EffectiveRate(l.VatRateBps, opt)}
	}
	changed := false
	if t, err := billing.Calculate(bl, opt); err == nil {
		for i, la := range t.Lines {
			l := &lines[i]
			if l.Base != la.Base || l.Vat != la.Vat || l.Total != la.Total {
				l.Base, l.Vat, l.Total, changed = la.Base, la.Vat, la.Total, true
			}
		}
		if *subtotal != t.Subtotal || *vat != t.VatTotal || *rounding != t.Rounding || *total != t.Total {
			*subtotal, *vat, *rounding, *total, changed = t.Subtotal, t.VatTotal, t.Rounding, t.Total, true
		}
	}
	var sum int64
	for _, p := range payments {
		sum += p.Amount
	}
	if *paid != sum {
		*paid, changed = sum, true
	}
	return changed
}

// defaultFormats are added for document types the backup has no series for.
var defaultFormats = map[string]string{
	model.DocInvoice: "{YYYY}-{NNNN}", model.DocProforma: "Z{YYYY}-{NNNN}",
	model.DocCorrection: "D{YYYY}-{NNNN}", model.DocExpense: "N{YYYY}-{NNNN}", model.DocTaxDocument: "ZD{YYYY}-{NNNN}",
}

// Import validates the backup ZIP r and creates a NEW account from it in one
// transaction (all or nothing): the owner is opts.OwnerUserID, the slug is
// new, every ID is remapped (references to records missing in the backup
// are never resolved against the database, so a backup cannot reach other
// accounts), invoices get new public tokens, secrets are absent (bank
// accounts without Fio token, webhooks inactive without secret), recurring
// invoices are inactive and reminders/paid-thanks e-mails are disabled; the
// returned warnings describe these changes. Attachment contents are written
// to st at the end of the transaction and removed again if it fails.
//
// Errors: ErrCorrupt, ErrUnsupportedVersion, ErrTooLarge (wrapped), others = internal.
func Import(ctx context.Context, db *gorm.DB, st storage.Storage, r io.ReaderAt, size int64, opts ImportOptions) (*model.Account, []Warning, error) {
	if opts.MaxBytes <= 0 {
		opts.MaxBytes = DefaultMaxBytes
	}
	if opts.Now.IsZero() {
		opts.Now = time.Now()
	}
	a, err := readArchive(r, size, opts.MaxBytes)
	if err != nil {
		return nil, nil, err
	}
	im := &importer{a: a, opts: opts, st: st, ctx: ctx,
		bankAccounts: idMap{}, subjects: idMap{}, priceItems: idMap{}, invoices: idMap{}, expenses: idMap{},
		payments: idMap{}, expensePayments: idMap{}, templates: idMap{}, recurring: idMap{},
		bankTransactions: idMap{}, webhooks: idMap{}, attachments: idMap{}}
	err = db.WithContext(ctx).Transaction(im.run)
	if err != nil {
		im.cleanup()
		return nil, nil, err
	}
	return im.acc, im.warnings, nil
}

type importer struct {
	a    *archive
	opts ImportOptions
	st   storage.Storage
	ctx  context.Context

	acc      *model.Account
	warnings []Warning
	pending  []pendingFile // attachment contents to write
	written  []string      // storage keys written (removed on failure)
	orphans  int

	bankAccounts, subjects, priceItems, invoices, expenses, payments, expensePayments,
	templates, recurring, bankTransactions, webhooks, attachments idMap
}

func (im *importer) warn(code, msg string, count int) {
	im.warnings = append(im.warnings, Warning{Code: code, Message: msg, Count: count})
}

func (im *importer) cleanup() {
	for _, k := range im.written {
		_ = im.st.Delete(context.WithoutCancel(im.ctx), k)
	}
	im.written = nil
}

// related maps a (subject/related type, id) reference of events and todos.
func (im *importer) related(typ string, id uint) (uint, bool) {
	var m idMap
	switch typ {
	case events.SubjectAccount:
		return im.acc.ID, true
	case events.SubjectInvoice:
		m = im.invoices
	case events.SubjectExpense:
		m = im.expenses
	case events.SubjectSubject:
		m = im.subjects
	case events.SubjectPriceItem:
		m = im.priceItems
	case events.SubjectBankTransaction:
		m = im.bankTransactions
	case events.SubjectBankAccount:
		m = im.bankAccounts
	case events.SubjectRecurring:
		m = im.recurring
	case events.SubjectWebhook:
		m = im.webhooks
	}
	n, ok := m[id]
	return n, ok
}

func (im *importer) run(tx *gorm.DB) error {
	a := im.a
	if err := im.importAccount(tx); err != nil {
		return err
	}
	accID := im.acc.ID

	// bank accounts (never with a token)
	tokens := 0
	for i := range a.bankAccounts {
		d := &a.bankAccounts[i]
		m := model.BankAccount{}
		convert(&m, d)
		m.ID, m.AccountID, m.FioToken = 0, accID, ""
		if d.HadFioToken {
			tokens++
		}
		if err := tx.Create(&m).Error; err != nil {
			return fmt.Errorf("import bank account %d: %w", d.ID, err)
		}
		im.bankAccounts[d.ID] = m.ID
	}
	if tokens > 0 {
		im.warn(WarnBankTokensRemoved, "Fio API tokens are not part of a backup; enter them again in the bank account settings", tokens)
	}

	// number formats + counters (the next number continues)
	have := map[string]bool{}
	for i := range a.numberFormats {
		d := &a.numberFormats[i]
		m := model.NumberFormat{}
		convert(&m, d)
		m.ID, m.AccountID = 0, accID
		if err := tx.Create(&m).Error; err != nil {
			return fmt.Errorf("import number format %d: %w", d.ID, err)
		}
		have[m.DocumentType] = true
		for _, c := range d.Counters {
			if err := tx.Create(&model.NumberCounter{NumberFormatID: m.ID, Period: c.Period, LastNumber: c.LastNumber}).Error; err != nil {
				return fmt.Errorf("import number counter: %w", err)
			}
		}
	}
	for _, typ := range []string{model.DocInvoice, model.DocProforma, model.DocCorrection, model.DocExpense, model.DocTaxDocument} {
		if !have[typ] {
			if err := tx.Create(&model.NumberFormat{AccountID: accID, DocumentType: typ, Format: defaultFormats[typ], IsDefault: true}).Error; err != nil {
				return fmt.Errorf("import number format: %w", err)
			}
		}
	}

	for i := range a.subjects {
		d := &a.subjects[i]
		m := model.Subject{}
		convert(&m, d)
		m.ID, m.AccountID = 0, accID
		if err := tx.Create(&m).Error; err != nil {
			return fmt.Errorf("import subject %d: %w", d.ID, err)
		}
		im.subjects[d.ID] = m.ID
	}
	for i := range a.priceItems {
		d := &a.priceItems[i]
		m := model.PriceItem{}
		convert(&m, d)
		m.ID, m.AccountID = 0, accID
		if err := tx.Create(&m).Error; err != nil {
			return fmt.Errorf("import price item %d: %w", d.ID, err)
		}
		im.priceItems[d.ID] = m.ID
	}

	recomputed := 0
	// invoices: related_id / recurring_id are set in a second pass
	for i := range a.invoices {
		d := &a.invoices[i]
		m := model.Invoice{}
		convert(&m, d)
		if d.SubjectID != nil {
			sid, ok := im.subjects[*d.SubjectID]
			if !ok {
				return corrupt("invoice %d: subject %d is not in the backup", d.ID, *d.SubjectID)
			}
			m.SubjectID = &sid
		}
		m.ID, m.AccountID, m.RelatedID, m.RecurringID = 0, accID, nil, nil
		m.BankAccountID = im.bankAccounts.opt(d.BankAccountID)
		m.PublicToken = newPublicToken()
		m.Lines, m.Payments = nil, nil
		if recompute(d.Lines, d.Payments, billing.Options{PricesIncludeVAT: m.PricesIncludeVat, ReverseCharge: m.ReverseCharge,
			RoundTotal: m.RoundTotal, NonVATPayer: billing.ChargesNoVAT(m.YourVatMode, m.ReverseCharge)},
			&m.Subtotal, &m.VatTotal, &m.Rounding, &m.Total, &m.PaidAmount) {
			recomputed++
		}
		if err := tx.Omit(clause.Associations).Create(&m).Error; err != nil {
			return fmt.Errorf("import invoice %d: %w", d.ID, err)
		}
		im.invoices[d.ID] = m.ID
		for _, l := range d.Lines {
			line := model.InvoiceLine{}
			convert(&line, &l)
			line.ID, line.InvoiceID, line.PriceItemID = 0, m.ID, im.priceItems.opt(l.PriceItemID)
			if err := tx.Create(&line).Error; err != nil {
				return fmt.Errorf("import invoice %d line: %w", d.ID, err)
			}
		}
		for _, p := range d.Payments {
			pm := model.Payment{}
			convert(&pm, &p)
			pm.ID, pm.AccountID, pm.InvoiceID = 0, accID, m.ID
			pm.TaxDocumentID, pm.SourcePaymentID = nil, nil // second pass
			if err := tx.Create(&pm).Error; err != nil {
				return fmt.Errorf("import invoice %d payment: %w", d.ID, err)
			}
			im.payments[p.ID] = pm.ID
		}
	}
	if len(a.invoices) > 0 {
		im.warn(WarnPublicLinksRegenerated, "public links of invoices got new tokens; links sent to clients before no longer work", len(a.invoices))
	}

	for i := range a.expenses {
		d := &a.expenses[i]
		m := model.Expense{}
		convert(&m, d)
		if d.VatDeductible == nil { // backups from before the VAT / income-tax split
			m.VatDeductible = d.TaxDeductible
		}
		m.ID, m.AccountID, m.SubjectID = 0, accID, im.subjects.opt(d.SubjectID)
		m.Lines, m.Payments = nil, nil
		if recompute(d.Lines, d.Payments, billing.Options{PricesIncludeVAT: m.PricesIncludeVat, ReverseCharge: m.ReverseCharge,
			RoundTotal: m.RoundTotal}, &m.Subtotal, &m.VatTotal, &m.Rounding, &m.Total, &m.PaidAmount) {
			recomputed++
		}
		if err := tx.Omit(clause.Associations).Create(&m).Error; err != nil {
			return fmt.Errorf("import expense %d: %w", d.ID, err)
		}
		im.expenses[d.ID] = m.ID
		for _, l := range d.Lines {
			line := model.ExpenseLine{}
			convert(&line, &l)
			line.ID, line.ExpenseID, line.PriceItemID = 0, m.ID, im.priceItems.opt(l.PriceItemID)
			if err := tx.Create(&line).Error; err != nil {
				return fmt.Errorf("import expense %d line: %w", d.ID, err)
			}
		}
		for _, p := range d.Payments {
			pm := model.ExpensePayment{}
			convert(&pm, &p)
			pm.ID, pm.AccountID, pm.ExpenseID = 0, accID, m.ID
			if err := tx.Create(&pm).Error; err != nil {
				return fmt.Errorf("import expense %d payment: %w", d.ID, err)
			}
			im.expensePayments[p.ID] = pm.ID
		}
	}

	if recomputed > 0 {
		im.warn(WarnTotalsRecomputed, "totals or paid amounts of documents did not match their lines and payments; they were recomputed", recomputed)
	}

	for i := range a.templates {
		d := &a.templates[i]
		m := model.InvoiceTemplate{}
		convert(&m, d)
		sid, ok := im.subjects[d.SubjectID]
		if !ok {
			return corrupt("template %d: subject %d is not in the backup", d.ID, d.SubjectID)
		}
		m.ID, m.AccountID, m.SubjectID = 0, accID, sid
		m.BankAccountID = im.bankAccounts.opt(d.BankAccountID)
		for j := range m.Lines {
			m.Lines[j].PriceItemID = im.priceItems.opt(m.Lines[j].PriceItemID)
		}
		if err := tx.Create(&m).Error; err != nil {
			return fmt.Errorf("import template %d: %w", d.ID, err)
		}
		im.templates[d.ID] = m.ID
	}

	deactivated := 0
	for i := range a.recurring {
		d := &a.recurring[i]
		m := model.Recurring{}
		convert(&m, d)
		tid, ok := im.templates[d.TemplateID]
		if !ok {
			return corrupt("recurring %d: template %d is not in the backup", d.ID, d.TemplateID)
		}
		m.ID, m.AccountID, m.TemplateID = 0, accID, tid
		m.LastInvoiceID = im.invoices.opt(d.LastInvoiceID)
		if m.Active {
			deactivated++
		}
		m.Active = false
		if err := tx.Create(&m).Error; err != nil {
			return fmt.Errorf("import recurring %d: %w", d.ID, err)
		}
		im.recurring[d.ID] = m.ID
	}
	if deactivated > 0 {
		im.warn(WarnRecurringDeactivated, "recurring invoices were imported inactive (so two instances do not issue them twice); activate them when ready", deactivated)
	}

	// second pass: invoice references to other invoices and recurring
	for i := range a.invoices {
		d := &a.invoices[i]
		upd := map[string]any{}
		if id := im.invoices.opt(d.RelatedID); id != nil {
			upd["related_id"] = *id
		}
		if id := im.recurring.opt(d.RecurringID); id != nil {
			upd["recurring_id"] = *id
		}
		for _, p := range d.Payments {
			pu := map[string]any{}
			if id := im.invoices.opt(p.TaxDocumentID); id != nil {
				pu["tax_document_id"] = *id
			}
			if id := im.payments.opt(p.SourcePaymentID); id != nil {
				pu["source_payment_id"] = *id
			}
			if len(pu) > 0 {
				if err := tx.Model(&model.Payment{}).Where("id = ?", im.payments[p.ID]).UpdateColumns(pu).Error; err != nil {
					return fmt.Errorf("import invoice %d payment references: %w", d.ID, err)
				}
			}
		}
		if len(upd) > 0 {
			if err := tx.Model(&model.Invoice{}).Where("id = ?", im.invoices[d.ID]).UpdateColumns(upd).Error; err != nil {
				return fmt.Errorf("import invoice %d references: %w", d.ID, err)
			}
		}
	}

	for i := range a.stockMoves {
		d := &a.stockMoves[i]
		pid, ok := im.priceItems[d.PriceItemID]
		if !ok {
			im.orphans++
			continue
		}
		m := model.StockMove{}
		convert(&m, d)
		m.ID, m.AccountID, m.PriceItemID = 0, accID, pid
		m.InvoiceID, m.ExpenseID = im.invoices.opt(d.InvoiceID), im.expenses.opt(d.ExpenseID)
		if err := tx.Create(&m).Error; err != nil {
			return fmt.Errorf("import stock move %d: %w", d.ID, err)
		}
	}

	for i := range a.bankTransactions {
		d := &a.bankTransactions[i]
		bid, ok := im.bankAccounts[d.BankAccountID]
		if !ok {
			im.orphans++
			continue
		}
		m := model.BankTransaction{}
		convert(&m, d)
		m.ID, m.AccountID, m.BankAccountID = 0, accID, bid
		m.MatchedInvoiceID, m.MatchedExpenseID, m.PaymentID = nil, nil, nil
		switch {
		case d.MatchedInvoiceID != nil:
			inv, pay := im.invoices.opt(d.MatchedInvoiceID), im.payments.opt(d.PaymentID)
			if inv != nil && pay != nil {
				m.MatchedInvoiceID, m.PaymentID = inv, pay
			}
		case d.MatchedExpenseID != nil:
			exp, pay := im.expenses.opt(d.MatchedExpenseID), im.expensePayments.opt(d.PaymentID)
			if exp != nil && pay != nil {
				m.MatchedExpenseID, m.PaymentID = exp, pay
			}
		}
		if m.PaymentID == nil {
			m.AutoMatched = false
		}
		if m.Suggestions != nil {
			sugg := make([]model.MatchSuggestion, 0, len(m.Suggestions))
			for _, s := range m.Suggestions {
				s.InvoiceID, s.ExpenseID = im.invoices.opt(s.InvoiceID), im.expenses.opt(s.ExpenseID)
				if s.InvoiceID != nil || s.ExpenseID != nil {
					sugg = append(sugg, s)
				}
			}
			m.Suggestions, m.SuggestionCount = sugg, len(sugg)
		}
		if err := tx.Create(&m).Error; err != nil {
			return fmt.Errorf("import bank transaction %d: %w", d.ID, err)
		}
		im.bankTransactions[d.ID] = m.ID
	}

	for i := range a.emailLogs {
		d := &a.emailLogs[i]
		iid, ok := im.invoices[d.InvoiceID]
		if !ok {
			im.orphans++
			continue
		}
		m := model.EmailLog{}
		convert(&m, d)
		m.ID, m.AccountID, m.InvoiceID = 0, accID, iid
		if err := tx.Create(&m).Error; err != nil {
			return fmt.Errorf("import e-mail log %d: %w", d.ID, err)
		}
	}

	for i := range a.webhooks {
		d := &a.webhooks[i]
		m := model.Webhook{}
		convert(&m, d)
		m.ID, m.AccountID, m.Secret, m.Active = 0, accID, "", false
		if err := tx.Create(&m).Error; err != nil {
			return fmt.Errorf("import webhook %d: %w", d.ID, err)
		}
		im.webhooks[d.ID] = m.ID
	}
	if len(a.webhooks) > 0 {
		im.warn(WarnWebhooksInactive, "webhooks were imported inactive and without secret; set a new secret and activate them", len(a.webhooks))
	}

	if err := im.importTodosEvents(tx); err != nil {
		return err
	}
	if err := im.importAttachments(tx); err != nil {
		return err
	}
	if im.orphans > 0 {
		im.warn(WarnOrphansSkipped, "records referring to data missing in the backup were skipped", im.orphans)
	}
	if n := len(a.members) - 1; n > 0 {
		im.warn(WarnMembersNotImported, "other members are not transferred; invite them again", n)
	}

	_, err := events.Record(tx, events.Meta{AccountID: accID, AccountSlug: im.acc.Slug, UserID: &im.opts.OwnerUserID, At: im.opts.Now},
		events.Event{Name: events.AccountImported, SubjectType: events.SubjectAccount, SubjectID: accID,
			Text: fmt.Sprintf("Účet byl obnoven ze zálohy účtu %s", a.manifest.Account.Slug),
			Data: map[string]any{"source_slug": a.manifest.Account.Slug, "exported_at": a.manifest.ExportedAt,
				"app_version": a.manifest.AppVersion, "counts": a.manifest.Counts}})
	if err != nil {
		return err
	}
	return im.writeFiles()
}

func (im *importer) importAccount(tx *gorm.DB) error {
	d := &im.a.account
	name := strings.TrimSpace(im.opts.Name)
	if name == "" {
		name = strings.TrimSpace(d.Name)
	}
	if name == "" {
		name = "Obnovený účet"
	}
	s, err := slug.Unique(tx, name)
	if err != nil {
		return err
	}
	acc := model.Account{}
	convert(&acc, d)
	acc.ID, acc.Slug, acc.Name = 0, s, name
	acc.LogoAttachmentID, acc.StampAttachmentID = nil, nil
	if acc.RemindersEnabled {
		im.warn(WarnRemindersDisabled, "automatic payment reminders were turned off; turn them on in the e-mail settings", 0)
	}
	if acc.PaidThanksEnabled {
		im.warn(WarnPaidThanksDisabled, "thank-you e-mails for payments were turned off; turn them on in the e-mail settings", 0)
	}
	acc.RemindersEnabled, acc.PaidThanksEnabled = false, false
	if err := tx.Create(&acc).Error; err != nil {
		return fmt.Errorf("import account: %w", err)
	}
	if err := tx.Create(&model.Membership{UserID: im.opts.OwnerUserID, AccountID: acc.ID, Role: model.RoleOwner}).Error; err != nil {
		return fmt.Errorf("import membership: %w", err)
	}
	im.acc = &acc
	return nil
}

func (im *importer) importTodosEvents(tx *gorm.DB) error {
	accID := im.acc.ID
	for i := range im.a.todos {
		d := &im.a.todos[i]
		m := model.Todo{}
		convert(&m, d)
		m.ID, m.AccountID, m.RelatedID = 0, accID, nil
		if d.RelatedID != nil && d.RelatedType != "" {
			if n, ok := im.related(d.RelatedType, *d.RelatedID); ok {
				m.RelatedID = &n
			}
		}
		if m.Key != nil { // automatic: the key names the related record
			if m.RelatedID == nil {
				im.orphans++
				continue
			}
			k := fmt.Sprintf("%s:%d", m.Name, *m.RelatedID)
			m.Key = &k
		}
		if err := tx.Create(&m).Error; err != nil {
			return fmt.Errorf("import todo %d: %w", d.ID, err)
		}
	}
	// events are copied as they are (no webhook deliveries, no todo sync);
	// records they name that are not in the backup (deleted) get subject_id 0
	for start := 0; start < len(im.a.events); start += 500 {
		batch := im.a.events[start:min(start+500, len(im.a.events))]
		rows := make([]model.Event, len(batch))
		for i := range batch {
			d := &batch[i]
			convert(&rows[i], d)
			rows[i].ID, rows[i].AccountID = 0, accID
			rows[i].SubjectID, _ = im.related(d.SubjectType, d.SubjectID)
			if rows[i].Data == nil {
				rows[i].Data = map[string]any{}
			}
		}
		if err := tx.Create(&rows).Error; err != nil {
			return fmt.Errorf("import events: %w", err)
		}
	}
	return nil
}

func (im *importer) importAttachments(tx *gorm.DB) error {
	accID := im.acc.ID
	missing := 0
	for i := range im.a.attachments {
		d := &im.a.attachments[i]
		m := model.Attachment{}
		convert(&m, d)
		var owner uint
		ok := false
		switch d.OwnerType {
		case model.OwnerAccount:
			owner, ok = accID, true
		case model.OwnerInvoice:
			owner, ok = im.invoices[d.OwnerID]
		case model.OwnerExpense:
			owner, ok = im.expenses[d.OwnerID]
		case model.OwnerSubject:
			owner, ok = im.subjects[d.OwnerID]
		}
		if !ok {
			im.orphans++
			continue
		}
		if d.Path == "" {
			missing++
			continue
		}
		rnd := make([]byte, 16)
		_, _ = rand.Read(rnd)
		m.ID, m.AccountID, m.OwnerID = 0, accID, owner
		m.StorageKey = fmt.Sprintf("%d/%s", accID, hex.EncodeToString(rnd))
		if err := tx.Create(&m).Error; err != nil {
			return fmt.Errorf("import attachment %d: %w", d.ID, err)
		}
		im.attachments[d.ID] = m.ID
		im.pending = append(im.pending, pendingFile{key: m.StorageKey, path: d.Path, sha: im.a.manifest.Files[d.Path].SHA256})
	}
	if missing > 0 {
		im.warn(WarnAttachmentsMissing, "some attachments had no content in the backup and were skipped", missing)
	}
	upd := map[string]any{}
	if id := im.attachments.opt(im.a.account.LogoAttachmentID); id != nil {
		upd["logo_attachment_id"] = *id
	}
	if id := im.attachments.opt(im.a.account.StampAttachmentID); id != nil {
		upd["stamp_attachment_id"] = *id
	}
	if len(upd) > 0 {
		if err := tx.Model(&model.Account{}).Where("id = ?", accID).UpdateColumns(upd).Error; err != nil {
			return fmt.Errorf("import account logo: %w", err)
		}
		if v, ok := upd["logo_attachment_id"].(uint); ok {
			im.acc.LogoAttachmentID = &v
		}
		if v, ok := upd["stamp_attachment_id"].(uint); ok {
			im.acc.StampAttachmentID = &v
		}
	}
	return nil
}

type pendingFile struct {
	key, path, sha string
}

// writeFiles copies the attachment contents from the ZIP to the storage,
// verifying their checksums again (the ZIP is re-read).
func (im *importer) writeFiles() error {
	for _, p := range im.pending {
		f := im.a.entries[p.path]
		rc, err := f.Open()
		if err != nil {
			return corrupt("%s: %v", p.path, err)
		}
		h := sha256.New()
		im.written = append(im.written, p.key)
		err = im.st.Put(im.ctx, p.key, io.TeeReader(io.LimitReader(rc, maxAttachment+1), h))
		rc.Close()
		if err != nil {
			return fmt.Errorf("import attachment %s: %w", p.path, err)
		}
		if hex.EncodeToString(h.Sum(nil)) != strings.ToLower(p.sha) {
			return corrupt("checksum of %q does not match the manifest", p.path)
		}
	}
	return nil
}

// newPublicToken is a new random public link token (same shape as the API's).
func newPublicToken() string {
	b := make([]byte, 24)
	_, _ = rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}

// IsImportError reports whether err is a validation error of the archive
// (as opposed to an internal failure).
func IsImportError(err error) bool {
	return errors.Is(err, ErrCorrupt) || errors.Is(err, ErrUnsupportedVersion) || errors.Is(err, ErrTooLarge)
}
