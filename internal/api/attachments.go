package api

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"path"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humachi"
	"gorm.io/gorm"

	"github.com/qwerin/nanofaktura/internal/auth"
	"github.com/qwerin/nanofaktura/internal/model"
	"github.com/qwerin/nanofaktura/internal/storage"
)

// MaxAttachmentSize is the upload limit of one attachment (SPEC §7.12).
const MaxAttachmentSize = 20 << 20

// multipart overhead allowed on top of MaxAttachmentSize
const uploadOverhead = 64 << 10

type Attachment struct {
	ID          uint      `json:"id"`
	OwnerType   string    `json:"owner_type" enum:"invoice,expense,subject,account"`
	OwnerID     uint      `json:"owner_id"`
	Filename    string    `json:"filename"`
	ContentType string    `json:"content_type" doc:"Detected from the content"`
	Size        int64     `json:"size"`
	CreatedAt   time.Time `json:"created_at"`
}

// AttachmentUpload is the multipart/form-data body of POST /attachments.
type AttachmentUpload struct {
	File      huma.FormFile `form:"file" required:"true" doc:"PDF, PNG, JPEG, WebP, HEIC or XML, max 20 MB"`
	OwnerType string        `form:"owner_type" required:"true" enum:"invoice,expense,subject,account"`
	OwnerID   uint          `form:"owner_id" required:"true" doc:"ID of the owner record; for owner_type=account ignored (current account)"`
}

func toAttachment(a *model.Attachment) Attachment {
	return Attachment{ID: a.ID, OwnerType: a.OwnerType, OwnerID: a.OwnerID, Filename: a.Filename,
		ContentType: a.ContentType, Size: a.Size, CreatedAt: a.CreatedAt}
}

// ownerTables maps owner types to the table holding the owner record.
var ownerTables = map[string]string{
	model.OwnerInvoice: "invoices", model.OwnerExpense: "expenses", model.OwnerSubject: "subjects",
}

func (s *server) registerAttachments(g huma.API) {
	huma.Post(g, "/attachments", s.uploadAttachment, status(http.StatusCreated), auth.ForEditors, func(o *huma.Operation) {
		o.MaxBodyBytes = MaxAttachmentSize + uploadOverhead
		o.BodyReadTimeout = 5 * time.Minute
		o.Middlewares = append(o.Middlewares, limitBody(g, MaxAttachmentSize+uploadOverhead))
	})
	huma.Get(g, "/attachments", s.listAttachments)
	huma.Get(g, "/attachments/{id}", s.getAttachment)
	huma.Get(g, "/attachments/{id}/download", s.downloadAttachment, func(o *huma.Operation) {
		o.Responses = map[string]*huma.Response{"200": {
			Description: "File content",
			Content:     map[string]*huma.MediaType{"application/octet-stream": {Schema: &huma.Schema{Type: "string", Format: "binary"}}},
		}}
	})
	huma.Delete(g, "/attachments/{id}", s.deleteAttachment, status(http.StatusNoContent), auth.ForEditors)
}

// limitBody rejects request bodies over max bytes with 413 before they are parsed.
func limitBody(api huma.API, max int64) func(huma.Context, func(huma.Context)) {
	return func(ctx huma.Context, next func(huma.Context)) {
		r, w := humachi.Unwrap(ctx)
		if r.ContentLength > max {
			_ = huma.WriteErr(api, ctx, http.StatusRequestEntityTooLarge, "file is larger than 20 MB")
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, max)
		next(ctx)
	}
}

// sniffContentType returns the canonical content type of an allowed file
// from its first bytes, or "" when the type is not allowed.
func sniffContentType(head []byte) string {
	if len(head) >= 12 && string(head[4:8]) == "ftyp" {
		switch string(head[8:12]) {
		case "heic", "heix", "hevc", "hevx", "heim", "heis", "mif1", "msf1":
			return "image/heic"
		}
	}
	ct := http.DetectContentType(head)
	switch {
	case ct == "application/pdf", ct == "image/png", ct == "image/jpeg", ct == "image/webp":
		return ct
	case strings.HasPrefix(ct, "text/xml"):
		return "application/xml"
	case strings.HasPrefix(ct, "text/plain"):
		// XML without the <?xml …?> declaration
		t := bytes.TrimSpace(bytes.TrimPrefix(head, []byte("\xef\xbb\xbf")))
		if bytes.HasPrefix(t, []byte("<")) {
			tok, err := xml.NewDecoder(bytes.NewReader(t)).Token()
			if _, ok := tok.(xml.StartElement); ok && err == nil {
				return "application/xml"
			}
		}
	}
	return ""
}

// cleanFilename keeps the base name without control characters, max 200 runes.
func cleanFilename(name string) string {
	name = path.Base(strings.ReplaceAll(name, "\\", "/"))
	name = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, name)
	name = strings.TrimSpace(name)
	if r := []rune(name); len(r) > 200 {
		name = string(r[len(r)-200:])
	}
	if name == "" || name == "." || name == "/" {
		return "file"
	}
	return name
}

// checkOwner verifies the owner record exists in the current account (404 otherwise).
func (s *server) checkOwner(ctx context.Context, ownerType string, ownerID uint) error {
	if ownerType == model.OwnerAccount {
		if ownerID != auth.AccountFrom(ctx).ID {
			return notFound("owner")
		}
		return nil
	}
	table, ok := ownerTables[ownerType]
	if !ok {
		return invalid("owner_type", "unknown owner type")
	}
	db := s.db.WithContext(ctx)
	if !db.Migrator().HasTable(table) {
		return notFound("owner")
	}
	var n int64
	if err := db.Table(table).Where("id = ? AND account_id = ?", ownerID, auth.AccountFrom(ctx).ID).Count(&n).Error; err != nil {
		return dbErr(err, "owner")
	}
	if n == 0 {
		return notFound("owner")
	}
	return nil
}

func (s *server) uploadAttachment(ctx context.Context, in *struct {
	RawBody huma.MultipartFormFiles[AttachmentUpload]
}) (*Out[Attachment], error) {
	data := in.RawBody.Data()
	f := data.File
	if !f.IsSet || f.File == nil {
		return nil, invalid("file", "file is required")
	}
	defer f.Close()
	if f.Size > MaxAttachmentSize {
		return nil, huma.NewError(http.StatusRequestEntityTooLarge, "file is larger than 20 MB")
	}
	if f.Size == 0 {
		return nil, invalid("file", "file is empty")
	}
	acc := auth.AccountFrom(ctx)
	if data.OwnerType == model.OwnerAccount && data.OwnerID == 0 {
		data.OwnerID = acc.ID
	}
	if err := s.checkOwner(ctx, data.OwnerType, data.OwnerID); err != nil {
		return nil, err
	}

	head := make([]byte, 512)
	n, err := io.ReadFull(f, head)
	if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) && !errors.Is(err, io.EOF) {
		return nil, huma.Error400BadRequest("cannot read file", err)
	}
	ct := sniffContentType(head[:n])
	if ct == "" {
		return nil, huma.NewError(http.StatusUnsupportedMediaType, "unsupported file type (allowed: PDF, PNG, JPEG, WebP, HEIC, XML)")
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return nil, huma.Error500InternalServerError("cannot read file", err)
	}

	rnd := make([]byte, 16)
	_, _ = rand.Read(rnd)
	m := model.Attachment{
		AccountID: acc.ID, OwnerType: data.OwnerType, OwnerID: data.OwnerID,
		Filename: cleanFilename(f.Filename), ContentType: ct, Size: f.Size,
		StorageKey: fmt.Sprintf("%d/%s", acc.ID, hex.EncodeToString(rnd)),
	}
	if err := s.deps.Storage.Put(ctx, m.StorageKey, f); err != nil {
		return nil, huma.Error500InternalServerError("cannot store file", err)
	}
	if err := s.db.WithContext(ctx).Create(&m).Error; err != nil {
		_ = s.deps.Storage.Delete(context.WithoutCancel(ctx), m.StorageKey)
		return nil, dbErr(err, "attachment")
	}
	return &Out[Attachment]{Body: toAttachment(&m)}, nil
}

func (s *server) listAttachments(ctx context.Context, in *struct {
	PageParams
	OwnerType string `query:"owner_type" enum:"invoice,expense,subject,account"`
	OwnerID   uint   `query:"owner_id"`
}) (*Out[ListResponse[Attachment]], error) {
	q := s.scoped(ctx).Order("id")
	if in.OwnerType != "" {
		q = q.Where("owner_type = ?", in.OwnerType)
	}
	if in.OwnerID != 0 {
		q = q.Where("owner_id = ?", in.OwnerID)
	}
	return paginate(q, in.PageParams, toAttachment)
}

func (s *server) attachment(ctx context.Context, id uint) (*model.Attachment, error) {
	var m model.Attachment
	if err := s.scoped(ctx).First(&m, id).Error; err != nil {
		return nil, dbErr(err, "attachment")
	}
	return &m, nil
}

// attachmentBytes loads a small attachment of the current account (logo,
// stamp) into memory; nil when id is nil or anything fails (best effort).
func (s *server) attachmentBytes(ctx context.Context, id *uint) []byte {
	if id == nil {
		return nil
	}
	m, err := s.attachment(ctx, *id)
	if err != nil {
		return nil
	}
	rc, err := s.deps.Storage.Get(ctx, m.StorageKey)
	if err != nil {
		return nil
	}
	defer rc.Close()
	b, err := io.ReadAll(io.LimitReader(rc, MaxAttachmentSize))
	if err != nil {
		return nil
	}
	return b
}

func (s *server) getAttachment(ctx context.Context, in *struct {
	ID uint `path:"id"`
}) (*Out[Attachment], error) {
	m, err := s.attachment(ctx, in.ID)
	if err != nil {
		return nil, err
	}
	return &Out[Attachment]{Body: toAttachment(m)}, nil
}

func (s *server) downloadAttachment(ctx context.Context, in *struct {
	ID     uint `path:"id"`
	Inline bool `query:"inline" doc:"Content-Disposition inline instead of attachment (open in the browser)"`
}) (*huma.StreamResponse, error) {
	m, err := s.attachment(ctx, in.ID)
	if err != nil {
		return nil, err
	}
	rc, err := s.deps.Storage.Get(ctx, m.StorageKey)
	if errors.Is(err, storage.ErrNotFound) {
		return nil, notFound("attachment content")
	} else if err != nil {
		return nil, huma.Error500InternalServerError("cannot read file", err)
	}
	disposition := "attachment"
	if in.Inline {
		disposition = "inline"
	}
	return &huma.StreamResponse{Body: func(hc huma.Context) {
		defer rc.Close()
		hc.SetHeader("Content-Type", m.ContentType)
		hc.SetHeader("Content-Length", strconv.FormatInt(m.Size, 10))
		hc.SetHeader("Content-Disposition", mime.FormatMediaType(disposition, map[string]string{"filename": m.Filename}))
		hc.SetHeader("X-Content-Type-Options", "nosniff")
		hc.SetHeader("Content-Security-Policy", "sandbox")
		hc.SetHeader("Cache-Control", "private, max-age=0")
		hc.SetStatus(http.StatusOK)
		_, _ = io.Copy(hc.BodyWriter(), rc)
	}}, nil
}

func (s *server) deleteAttachment(ctx context.Context, in *struct {
	ID uint `path:"id"`
}) (*NoContent, error) {
	m, err := s.attachment(ctx, in.ID)
	if err != nil {
		return nil, err
	}
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// the account's logo/stamp must not point to a deleted file
		for _, col := range []string{"logo_attachment_id", "stamp_attachment_id"} {
			if err := tx.Model(&model.Account{}).Where("id = ? AND "+col+" = ?", m.AccountID, m.ID).
				Update(col, nil).Error; err != nil {
				return dbErr(err, "account")
			}
		}
		return dbErrOrNil(tx.Delete(m).Error, "attachment")
	})
	if err != nil {
		return nil, err
	}
	if err := s.deps.Storage.Delete(ctx, m.StorageKey); err != nil && !errors.Is(err, storage.ErrNotFound) {
		return nil, huma.Error500InternalServerError("cannot delete file", err)
	}
	return &NoContent{}, nil
}
