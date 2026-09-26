package api

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"gorm.io/gorm"

	"github.com/qwerin/nanofaktura/internal/auth"
	"github.com/qwerin/nanofaktura/internal/model"
	"github.com/qwerin/nanofaktura/internal/numbering"
)

// NumberFormat is a document numbering series (SPEC §4.3).
type NumberFormat struct {
	ID           uint      `json:"id"`
	DocumentType string    `json:"document_type" enum:"invoice,proforma,correction,expense"`
	Format       string    `json:"format" example:"{YYYY}-{NNNN}"`
	IsDefault    bool      `json:"is_default"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type NumberFormatCreate struct {
	DocumentType string `json:"document_type" enum:"invoice,proforma,correction,expense"`
	Format       string `json:"format" minLength:"1" maxLength:"50" example:"{YYYY}-{NNNN}" doc:"Placeholders {YYYY}, {YY}, {MM}, {N}…{NNNNNN}; at least one {N…}"`
	IsDefault    bool   `json:"is_default,omitempty" doc:"The first format of a document type is always default"`
}

// NumberFormatPatch: nil fields are left unchanged. The document type cannot change.
type NumberFormatPatch struct {
	Format    *string `json:"format,omitempty" minLength:"1" maxLength:"50"`
	IsDefault *bool   `json:"is_default,omitempty" doc:"Only true is accepted for the current default; set another format as default instead"`
}

// NumberPreview is the number a format would assign next.
type NumberPreview struct {
	Number string `json:"number" example:"2026-0001"`
}

func toNumberFormat(m *model.NumberFormat) NumberFormat {
	return NumberFormat{ID: m.ID, DocumentType: m.DocumentType, Format: m.Format, IsDefault: m.IsDefault,
		CreatedAt: m.CreatedAt, UpdatedAt: m.UpdatedAt}
}

func (s *server) registerNumberFormats(g huma.API) {
	huma.Get(g, "/number-formats", s.listNumberFormats)
	huma.Post(g, "/number-formats", s.createNumberFormat, status(http.StatusCreated), auth.ForManagers)
	huma.Get(g, "/number-formats/{id}", s.getNumberFormat)
	huma.Patch(g, "/number-formats/{id}", s.patchNumberFormat, auth.ForManagers)
	huma.Delete(g, "/number-formats/{id}", s.deleteNumberFormat, status(http.StatusNoContent), auth.ForManagers)
	huma.Get(g, "/number-formats/{id}/preview", s.previewNumberFormat)
}

func (s *server) listNumberFormats(ctx context.Context, in *struct {
	PageParams
	DocumentType string `query:"document_type" enum:"invoice,proforma,correction,expense"`
}) (*Out[ListResponse[NumberFormat]], error) {
	q := s.scoped(ctx).Model(&model.NumberFormat{}).Order("document_type, id")
	if in.DocumentType != "" {
		q = q.Where("document_type = ?", in.DocumentType)
	}
	return paginate(q, in.PageParams, toNumberFormat)
}

func (s *server) getNumberFormat(ctx context.Context, in *struct {
	ID uint `path:"id"`
}) (*Out[NumberFormat], error) {
	var m model.NumberFormat
	if err := s.scoped(ctx).First(&m, in.ID).Error; err != nil {
		return nil, dbErr(err, "number format")
	}
	return &Out[NumberFormat]{Body: toNumberFormat(&m)}, nil
}

// checkFormat validates a format and that the document type does not already
// have the same format (two series with one format would issue duplicates).
func checkFormat(ctx context.Context, tx *gorm.DB, docType, format string, exceptID uint) error {
	if err := numbering.Validate(format); err != nil {
		return invalid("format", err.Error())
	}
	var n int64
	if err := tx.Model(&model.NumberFormat{}).Scopes(inAccount(ctx)).
		Where("document_type = ? AND format = ? AND id <> ?", docType, format, exceptID).
		Count(&n).Error; err != nil {
		return dbErr(err, "number format")
	}
	if n > 0 {
		return conflict("this format already exists for the document type")
	}
	return nil
}

// makeDefault unsets is_default on the other formats of the same document type.
func makeDefault(ctx context.Context, tx *gorm.DB, m *model.NumberFormat) error {
	return tx.Model(&model.NumberFormat{}).Scopes(inAccount(ctx)).
		Where("document_type = ? AND id <> ? AND is_default = ?", m.DocumentType, m.ID, true).
		Update("is_default", false).Error
}

func (s *server) createNumberFormat(ctx context.Context, in *struct{ Body NumberFormatCreate }) (*Out[NumberFormat], error) {
	m := model.NumberFormat{
		AccountID:    auth.AccountFrom(ctx).ID,
		DocumentType: in.Body.DocumentType,
		Format:       strings.TrimSpace(in.Body.Format),
		IsDefault:    in.Body.IsDefault,
	}
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := checkFormat(ctx, tx, m.DocumentType, m.Format, 0); err != nil {
			return err
		}
		if !m.IsDefault {
			var n int64
			if err := tx.Model(&model.NumberFormat{}).Scopes(inAccount(ctx)).
				Where("document_type = ? AND is_default = ?", m.DocumentType, true).Count(&n).Error; err != nil {
				return dbErr(err, "number format")
			}
			m.IsDefault = n == 0
		}
		if err := tx.Create(&m).Error; err != nil {
			return dbErr(err, "number format")
		}
		if m.IsDefault {
			if err := makeDefault(ctx, tx, &m); err != nil {
				return dbErr(err, "number format")
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &Out[NumberFormat]{Body: toNumberFormat(&m)}, nil
}

func (s *server) patchNumberFormat(ctx context.Context, in *struct {
	ID   uint `path:"id"`
	Body NumberFormatPatch
}) (*Out[NumberFormat], error) {
	var m model.NumberFormat
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Scopes(inAccount(ctx)).First(&m, in.ID).Error; err != nil {
			return dbErr(err, "number format")
		}
		if p := in.Body.Format; p != nil {
			m.Format = strings.TrimSpace(*p)
			if err := checkFormat(ctx, tx, m.DocumentType, m.Format, m.ID); err != nil {
				return err
			}
		}
		if p := in.Body.IsDefault; p != nil {
			if !*p && m.IsDefault {
				return conflict("a document type must keep a default format; set another format as default instead")
			}
			m.IsDefault = *p
		}
		if err := tx.Save(&m).Error; err != nil {
			return dbErr(err, "number format")
		}
		if m.IsDefault {
			if err := makeDefault(ctx, tx, &m); err != nil {
				return dbErr(err, "number format")
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &Out[NumberFormat]{Body: toNumberFormat(&m)}, nil
}

func (s *server) deleteNumberFormat(ctx context.Context, in *struct {
	ID uint `path:"id"`
}) (*NoContent, error) {
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var m model.NumberFormat
		if err := tx.Scopes(inAccount(ctx)).First(&m, in.ID).Error; err != nil {
			return dbErr(err, "number format")
		}
		if m.IsDefault {
			return conflict("cannot delete the default number format; set another format as default first")
		}
		var n int64
		if err := tx.Model(&model.NumberFormat{}).Scopes(inAccount(ctx)).
			Where("document_type = ?", m.DocumentType).Count(&n).Error; err != nil {
			return dbErr(err, "number format")
		}
		if n <= 1 {
			return conflict("cannot delete the last number format of a document type")
		}
		if err := tx.Where("number_format_id = ?", m.ID).Delete(&model.NumberCounter{}).Error; err != nil {
			return dbErr(err, "number counter")
		}
		if err := tx.Delete(&m).Error; err != nil {
			return dbErr(err, "number format")
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &NoContent{}, nil
}

func (s *server) previewNumberFormat(ctx context.Context, in *struct {
	ID   uint   `path:"id"`
	Date string `query:"date" format:"date" doc:"Issue date YYYY-MM-DD (default today)"`
}) (*Out[NumberPreview], error) {
	var m model.NumberFormat
	if err := s.scoped(ctx).First(&m, in.ID).Error; err != nil {
		return nil, dbErr(err, "number format")
	}
	date := in.Date
	if date == "" {
		date = s.deps.Now().Format(numbering.DateLayout)
	}
	n, err := numbering.Preview(s.db.WithContext(ctx), &m, date)
	if err != nil {
		if _, perr := numbering.ParseDate(date); perr != nil {
			return nil, huma.NewError(http.StatusUnprocessableEntity, "validation failed",
				&huma.ErrorDetail{Location: "query.date", Message: perr.Error(), Value: in.Date})
		}
		return nil, huma.Error500InternalServerError("preview failed", err)
	}
	return &Out[NumberPreview]{Body: NumberPreview{Number: n}}, nil
}
