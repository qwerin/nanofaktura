package api

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"github.com/qwerin/nanofaktura/internal/auth"
	"github.com/qwerin/nanofaktura/internal/backup"
	"github.com/qwerin/nanofaktura/internal/model"
)

// Account backup, export and import (SPEC §7.16); the format is internal/backup.

// BackupUpload is the multipart body of POST /api/accounts/import.
type BackupUpload struct {
	File huma.FormFile `form:"file" required:"true" doc:"Backup ZIP (nanofaktura-<slug>-<date>.zip)"`
	Name string        `form:"name" required:"false" maxLength:"200" doc:"Name of the new account; empty = name from the backup"`
}

// ImportWarning is something the import changed on purpose.
type ImportWarning struct {
	Code    string `json:"code" enum:"recurring_deactivated,reminders_disabled,paid_thanks_disabled,webhooks_inactive,bank_tokens_removed,public_links_regenerated,members_not_imported,attachments_missing,orphans_skipped"`
	Message string `json:"message" doc:"English description; clients translate by code"`
	Count   int    `json:"count,omitempty" doc:"Number of affected records"`
}

type ImportResult struct {
	Account  Account         `json:"account" doc:"The new account (the caller is its owner)"`
	Warnings []ImportWarning `json:"warnings" nullable:"false"`
}

func (s *server) importMaxBytes() int64 {
	if s.cfg.ImportMaxMB > 0 {
		return int64(s.cfg.ImportMaxMB) << 20
	}
	return backup.DefaultMaxBytes
}

func (s *server) registerBackup(authed, account huma.API) {
	op := huma.Operation{
		OperationID: "get-account-backup",
		Method:      http.MethodGet,
		Path:        "/backup",
		Summary:     "Download a complete backup of the account (ZIP)",
		Description: "Everything of the account except secrets (Fio tokens, webhook secrets, invitation/session/API tokens). Records an account.exported event.",
		Tags:        []string{"Accounts"},
		Responses:   fileResponses("application/zip", "Backup ZIP"),
	}
	auth.Allow(model.RoleOwner, model.RoleAdmin)(&op)
	huma.Register(account, op, s.exportBackup)

	max := s.importMaxBytes() + uploadOverhead
	huma.Post(authed, "/api/accounts/import", s.importBackup, status(http.StatusCreated), slowUpload(30*time.Minute), func(o *huma.Operation) {
		o.Summary = "Create a new account from a backup ZIP"
		o.Tags = []string{"Accounts"}
		o.MaxBodyBytes = max
		o.Middlewares = append(o.Middlewares, limitBody(authed, max))
	})
}

func (s *server) exportBackup(ctx context.Context, _ *struct{}) (*huma.StreamResponse, error) {
	acc := auth.AccountFrom(ctx)
	user := auth.UserFrom(ctx)
	name := backup.Filename(acc.Slug, s.deps.Now())
	release, err := s.acquireExport(ctx)
	if err != nil {
		return nil, err
	}
	return &huma.StreamResponse{Body: func(hc huma.Context) {
		defer release()
		extendWriteDeadline(hc, time.Hour) // attachments may be large
		hc.SetHeader("Content-Type", "application/zip")
		hc.SetHeader("Content-Disposition", `attachment; filename="`+name+`"`)
		man, err := backup.Export(ctx, s.db, s.deps.Storage, acc.ID, hc.BodyWriter())
		if err != nil {
			// headers are sent already; the client gets a truncated (invalid) ZIP
			slog.Error("backup export failed", "account", acc.Slug, "err", err)
			return
		}
		uid := user.ID
		if err := backup.RecordExported(s.db.WithContext(context.WithoutCancel(ctx)), acc, &uid, nowFrom(ctx), man); err != nil {
			slog.Error("backup export: record event", "account", acc.Slug, "err", err)
		}
	}}, nil
}

func (s *server) importBackup(ctx context.Context, in *struct {
	RawBody huma.MultipartFormFiles[BackupUpload]
}) (*Out[ImportResult], error) {
	data := in.RawBody.Data()
	f := data.File
	if !f.IsSet || f.File == nil {
		return nil, invalid("file", "file is required")
	}
	defer f.Close()
	if f.Size > s.importMaxBytes() {
		return nil, apiError(http.StatusRequestEntityTooLarge, CodeBackupTooLarge, "backup is larger than the import limit (NANOFAKTURA_IMPORT_MAX_MB)")
	}
	ra, ok := f.File.(io.ReaderAt)
	if !ok {
		return nil, huma.Error500InternalServerError("uploaded file is not seekable")
	}
	acc, warnings, err := backup.Import(ctx, s.db, s.deps.Storage, ra, f.Size, backup.ImportOptions{
		OwnerUserID: auth.UserFrom(ctx).ID, Name: data.Name, Now: nowFrom(ctx), MaxBytes: s.importMaxBytes(),
	})
	if err != nil {
		return nil, backupErr(err)
	}
	out := ImportResult{Account: toAccount(acc, model.RoleOwner), Warnings: make([]ImportWarning, len(warnings))}
	for i, w := range warnings {
		out.Warnings[i] = ImportWarning(w)
	}
	return &Out[ImportResult]{Body: out}, nil
}

// backupErr maps backup.Import errors to problem responses.
func backupErr(err error) error {
	switch {
	case errors.Is(err, backup.ErrUnsupportedVersion):
		return apiError(http.StatusUnprocessableEntity, CodeUnsupportedBackupVersion, err.Error())
	case errors.Is(err, backup.ErrTooLarge):
		return apiError(http.StatusRequestEntityTooLarge, CodeBackupTooLarge, err.Error())
	case errors.Is(err, backup.ErrCorrupt):
		return apiError(http.StatusUnprocessableEntity, CodeCorruptBackup, err.Error())
	}
	return dbErr(err, "account")
}
