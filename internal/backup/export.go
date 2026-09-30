package backup

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path"
	"strings"
	"time"
	"unicode"

	"gorm.io/gorm"

	"github.com/qwerin/nanofaktura/internal/events"
	"github.com/qwerin/nanofaktura/internal/model"
	"github.com/qwerin/nanofaktura/internal/storage"
)

// Filename is the download name of a backup of the account slug made at now.
func Filename(slug string, now time.Time) string {
	return "nanofaktura-" + slug + "-" + now.Format("2006-01-02") + ".zip"
}

// entityFile is one JSON file of the archive.
type entityFile struct {
	name  string
	count int
	data  []byte
}

// snapshot is everything of an account read in one transaction.
type snapshot struct {
	account     model.Account
	files       []entityFile
	attachments []Attachment
	storageKeys map[uint]string // attachment id → storage key
}

// Export writes the backup ZIP of the account to w and returns its manifest.
// All records are read in one transaction first (consistent snapshot, the DB
// is not held while the archive is streamed); attachment contents are then
// streamed from st. An attachment whose content is missing in the storage is
// exported without content (Path "").
func Export(ctx context.Context, db *gorm.DB, st storage.Storage, accountID uint, w io.Writer) (*Manifest, error) {
	snap, err := readSnapshot(ctx, db, accountID)
	if err != nil {
		return nil, err
	}
	man := &Manifest{
		Format: Format, Version: Version, ExportedAt: time.Now().UTC(), AppVersion: AppVersion(),
		Account: ManifestAccount{Slug: snap.account.Slug, Name: snap.account.Name},
		Counts:  map[string]int{}, Files: map[string]FileInfo{},
	}
	zw := zip.NewWriter(w)
	add := func(name string, r io.Reader) error {
		fw, err := zw.CreateHeader(&zip.FileHeader{Name: name, Method: zip.Deflate, Modified: man.ExportedAt})
		if err != nil {
			return err
		}
		h := sha256.New()
		n, err := io.Copy(io.MultiWriter(fw, h), r)
		if err != nil {
			return err
		}
		man.Files[name] = FileInfo{SHA256: hex.EncodeToString(h.Sum(nil)), Size: n}
		return nil
	}
	for _, f := range snap.files {
		if f.name != FileAccount {
			man.Counts[strings.TrimSuffix(f.name, ".json")] = f.count
		}
		if err := add(f.name, strings.NewReader(string(f.data))); err != nil {
			return nil, fmt.Errorf("backup: write %s: %w", f.name, err)
		}
	}
	for i := range snap.attachments {
		a := &snap.attachments[i]
		rc, err := st.Get(ctx, snap.storageKeys[a.ID])
		if errors.Is(err, storage.ErrNotFound) {
			a.Path = ""
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("backup: attachment %d: %w", a.ID, err)
		}
		err = add(a.Path, rc)
		rc.Close()
		if err != nil {
			return nil, fmt.Errorf("backup: attachment %d: %w", a.ID, err)
		}
	}
	b, err := json.MarshalIndent(snap.attachments, "", "  ")
	if err != nil {
		return nil, err
	}
	man.Counts["attachments"] = len(snap.attachments)
	if err := add(FileAttachments, strings.NewReader(string(b))); err != nil {
		return nil, err
	}
	mb, err := json.MarshalIndent(man, "", "  ")
	if err != nil {
		return nil, err
	}
	fw, err := zw.CreateHeader(&zip.FileHeader{Name: FileManifest, Method: zip.Deflate, Modified: man.ExportedAt})
	if err != nil {
		return nil, err
	}
	if _, err := fw.Write(mb); err != nil {
		return nil, err
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return man, nil
}

// RecordExported records the account.exported event (userID nil = CLI).
func RecordExported(db *gorm.DB, acc *model.Account, userID *uint, at time.Time, man *Manifest) error {
	return db.Transaction(func(tx *gorm.DB) error {
		_, err := events.Record(tx, events.Meta{AccountID: acc.ID, AccountSlug: acc.Slug, UserID: userID, At: at},
			events.Event{Name: events.AccountExported, SubjectType: events.SubjectAccount, SubjectID: acc.ID,
				Text: "Záloha účtu byla stažena", Data: map[string]any{"counts": man.Counts, "app_version": man.AppVersion}})
		return err
	})
}

func readSnapshot(ctx context.Context, db *gorm.DB, accountID uint) (*snapshot, error) {
	snap := &snapshot{storageKeys: map[uint]string{}}
	err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.First(&snap.account, accountID).Error; err != nil {
			return fmt.Errorf("backup: account %d: %w", accountID, err)
		}
		scoped := func() *gorm.DB { return tx.Where("account_id = ?", accountID).Order("id") }
		addFile := func(name string, v any, n int) error {
			b, err := json.MarshalIndent(v, "", "  ")
			if err != nil {
				return fmt.Errorf("backup: %s: %w", name, err)
			}
			snap.files = append(snap.files, entityFile{name: name, count: n, data: b})
			return nil
		}

		var acc Account
		convert(&acc, &snap.account)
		if err := addFile(FileAccount, acc, 1); err != nil {
			return err
		}

		var bas []model.BankAccount
		if err := scoped().Find(&bas).Error; err != nil {
			return err
		}
		bankAccounts := make([]BankAccount, len(bas))
		for i := range bas {
			convert(&bankAccounts[i], &bas[i])
			bankAccounts[i].HadFioToken = bas[i].FioToken != ""
		}
		if err := addFile(FileBankAccounts, bankAccounts, len(bankAccounts)); err != nil {
			return err
		}

		var nfs []model.NumberFormat
		if err := scoped().Find(&nfs).Error; err != nil {
			return err
		}
		formats := make([]NumberFormat, len(nfs))
		for i := range nfs {
			convert(&formats[i], &nfs[i])
			var cs []model.NumberCounter
			if err := tx.Where("number_format_id = ?", nfs[i].ID).Order("period").Find(&cs).Error; err != nil {
				return err
			}
			formats[i].Counters = make([]NumberCounter, len(cs))
			for j := range cs {
				convert(&formats[i].Counters[j], &cs[j])
			}
		}
		if err := addFile(FileNumberFormats, formats, len(formats)); err != nil {
			return err
		}

		if err := exportAll[model.Subject, Subject](scoped(), addFile, FileSubjects); err != nil {
			return err
		}
		if err := exportAll[model.PriceItem, PriceItem](scoped(), addFile, FilePriceItems); err != nil {
			return err
		}
		if err := exportAll[model.StockMove, StockMove](scoped(), addFile, FileStockMoves); err != nil {
			return err
		}
		if err := exportAll[model.Invoice, Invoice](scoped().
			Preload("Lines", func(db *gorm.DB) *gorm.DB { return db.Order("position, id") }).
			Preload("Payments", func(db *gorm.DB) *gorm.DB { return db.Order("id") }), addFile, FileInvoices); err != nil {
			return err
		}
		if err := exportAll[model.Expense, Expense](scoped().
			Preload("Lines", func(db *gorm.DB) *gorm.DB { return db.Order("position, id") }).
			Preload("Payments", func(db *gorm.DB) *gorm.DB { return db.Order("id") }), addFile, FileExpenses); err != nil {
			return err
		}
		if err := exportAll[model.InvoiceTemplate, Template](scoped(), addFile, FileTemplates); err != nil {
			return err
		}
		if err := exportAll[model.Recurring, Recurring](scoped(), addFile, FileRecurring); err != nil {
			return err
		}
		if err := exportAll[model.BankTransaction, BankTransaction](scoped(), addFile, FileBankTransactions); err != nil {
			return err
		}
		if err := exportAll[model.Todo, Todo](scoped(), addFile, FileTodos); err != nil {
			return err
		}
		if err := exportAll[model.Event, Event](scoped(), addFile, FileEvents); err != nil {
			return err
		}
		if err := exportAll[model.EmailLog, EmailLog](scoped(), addFile, FileEmailLogs); err != nil {
			return err
		}

		var hooks []model.Webhook
		if err := scoped().Find(&hooks).Error; err != nil {
			return err
		}
		webhooks := make([]Webhook, len(hooks))
		for i := range hooks {
			convert(&webhooks[i], &hooks[i])
			webhooks[i].HadSecret = hooks[i].Secret != ""
		}
		if err := addFile(FileWebhooks, webhooks, len(webhooks)); err != nil {
			return err
		}

		members := []Member{}
		if err := tx.Table("memberships").Select("users.email, users.name, memberships.role").
			Joins("JOIN users ON users.id = memberships.user_id").
			Where("memberships.account_id = ?", accountID).Order("memberships.id").Scan(&members).Error; err != nil {
			return err
		}
		if err := addFile(FileMembers, members, len(members)); err != nil {
			return err
		}

		var atts []model.Attachment
		if err := scoped().Find(&atts).Error; err != nil {
			return err
		}
		snap.attachments = make([]Attachment, len(atts))
		for i := range atts {
			convert(&snap.attachments[i], &atts[i])
			snap.attachments[i].Path = attachmentPath(atts[i].ID, atts[i].Filename)
			snap.storageKeys[atts[i].ID] = atts[i].StorageKey
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return snap, nil
}

// exportAll converts every row of q (model M) to DTO D and adds the file.
func exportAll[M, D any](q *gorm.DB, addFile func(string, any, int) error, name string) error {
	var rows []M
	if err := q.Find(&rows).Error; err != nil {
		return fmt.Errorf("backup: %s: %w", name, err)
	}
	out := make([]D, len(rows))
	for i := range rows {
		convert(&out[i], &rows[i])
	}
	return addFile(name, out, len(out))
}

// attachmentPath is the ZIP path of an attachment's content: the file name
// reduced to safe characters (no separators, no control characters).
func attachmentPath(id uint, filename string) string {
	name := strings.Map(func(r rune) rune {
		switch {
		case r == '/' || r == '\\' || r == ':' || unicode.IsControl(r):
			return '_'
		}
		return r
	}, path.Base(strings.ReplaceAll(filename, "\\", "/")))
	name = strings.Trim(name, ". ")
	if name == "" {
		name = "file"
	}
	if r := []rune(name); len(r) > 120 {
		name = string(r[len(r)-120:])
	}
	return fmt.Sprintf("%s%d/%s", attachmentsDir, id, name)
}
