package handler

import (
	"context"
	"fmt"

	"github.com/danielgtaylor/huma/v2"
	"gorm.io/gorm"

	"github.com/qwerin/nanofaktura/internal/auth"
	"github.com/qwerin/nanofaktura/internal/database"
	"github.com/qwerin/nanofaktura/internal/models"
)

// SettingsInput is the DTO for updating Settings (no gorm.Model).
type SettingsInput struct {
	CompanyName          string `json:"company_name,omitempty"`
	CompanyStreet        string `json:"company_street,omitempty"`
	CompanyCity          string `json:"company_city,omitempty"`
	CompanyZip           string `json:"company_zip,omitempty"`
	CompanyCountry       string `json:"company_country,omitempty"`
	RegistrationNo       string `json:"registration_no,omitempty"`
	VatNo                string `json:"vat_no,omitempty"`
	VatExempt            bool   `json:"vat_exempt,omitempty"`
	BankAccount          string `json:"bank_account,omitempty"`
	IBAN                 string `json:"iban,omitempty"`
	SwiftBIC             string `json:"swift_bic,omitempty"`
	DefaultDue           int    `json:"default_due,omitempty"`
	DefaultCurrency      string `json:"default_currency,omitempty"`
	DefaultPaymentMethod string `json:"default_payment_method,omitempty"`
	DefaultNote          string `json:"default_note,omitempty"`
	InvoiceTemplate      string `json:"invoice_template,omitempty"`
}

// NumberFormatInput is the DTO for creating/updating NumberFormat.
type NumberFormatInput struct {
	DocumentType string `json:"document_type,omitempty"`
	Label        string `json:"label,omitempty"`
	Pattern      string `json:"pattern,omitempty"`
	NextNumber   int    `json:"next_number,omitempty"`
	PaddingWidth int    `json:"padding_width,omitempty"`
}

// RegisterSettings registers all /settings endpoints.
func RegisterSettings(api huma.API, db *gorm.DB) {

	// --- GET /settings -------------------------------------------------------
	type GetSettingsOutput struct {
		Body models.Settings
	}

	huma.Register(api, huma.Operation{
		OperationID: "get-settings",
		Method:      "GET",
		Path:        "/settings",
		Summary:     "Načti nastavení účtu",
		Tags:        []string{"settings"},
	}, func(ctx context.Context, _ *struct{}) (*GetSettingsOutput, error) {
		userID := auth.UserIDFromCtx(ctx)
		s, err := database.GetUserSettings(db.WithContext(ctx), userID)
		if err != nil {
			return nil, huma.Error500InternalServerError("chyba načtení nastavení", err)
		}
		return &GetSettingsOutput{Body: *s}, nil
	})

	// --- PUT /settings -------------------------------------------------------
	type UpdateSettingsInput struct {
		Body SettingsInput
	}

	huma.Register(api, huma.Operation{
		OperationID: "update-settings",
		Method:      "PUT",
		Path:        "/settings",
		Summary:     "Aktualizuj nastavení účtu",
		Tags:        []string{"settings"},
	}, func(ctx context.Context, in *UpdateSettingsInput) (*GetSettingsOutput, error) {
		userID := auth.UserIDFromCtx(ctx)
		s, err := database.GetUserSettings(db.WithContext(ctx), userID)
		if err != nil {
			return nil, huma.Error500InternalServerError("chyba načtení nastavení", err)
		}

		updates := map[string]interface{}{}
		if in.Body.CompanyName != "" {
			updates["company_name"] = in.Body.CompanyName
		}
		if in.Body.CompanyStreet != "" {
			updates["company_street"] = in.Body.CompanyStreet
		}
		if in.Body.CompanyCity != "" {
			updates["company_city"] = in.Body.CompanyCity
		}
		if in.Body.CompanyZip != "" {
			updates["company_zip"] = in.Body.CompanyZip
		}
		if in.Body.CompanyCountry != "" {
			updates["company_country"] = in.Body.CompanyCountry
		}
		if in.Body.RegistrationNo != "" {
			updates["registration_no"] = in.Body.RegistrationNo
		}
		if in.Body.VatNo != "" {
			updates["vat_no"] = in.Body.VatNo
		}
		updates["vat_exempt"] = in.Body.VatExempt
		if in.Body.BankAccount != "" {
			updates["bank_account"] = in.Body.BankAccount
		}
		if in.Body.IBAN != "" {
			updates["iban"] = in.Body.IBAN
		}
		if in.Body.SwiftBIC != "" {
			updates["swift_bic"] = in.Body.SwiftBIC
		}
		if in.Body.DefaultDue != 0 {
			updates["default_due"] = in.Body.DefaultDue
		}
		if in.Body.DefaultCurrency != "" {
			updates["default_currency"] = in.Body.DefaultCurrency
		}
		if in.Body.DefaultPaymentMethod != "" {
			updates["default_payment_method"] = in.Body.DefaultPaymentMethod
		}
		if in.Body.DefaultNote != "" {
			updates["default_note"] = in.Body.DefaultNote
		}
		if in.Body.InvoiceTemplate != "" {
			updates["invoice_template"] = in.Body.InvoiceTemplate
		}

		if len(updates) > 0 {
			if err := db.WithContext(ctx).Model(&models.Settings{}).Where("id = ?", s.ID).Updates(updates).Error; err != nil {
				return nil, huma.Error500InternalServerError("uložení nastavení selhalo", err)
			}
		}

		s, err = database.GetUserSettings(db.WithContext(ctx), userID)
		if err != nil {
			return nil, huma.Error500InternalServerError("chyba načtení nastavení", err)
		}
		return &GetSettingsOutput{Body: *s}, nil
	})

	// --- GET /settings/number-formats ----------------------------------------
	type ListFormatsOutput struct {
		Body []models.NumberFormat
	}

	huma.Register(api, huma.Operation{
		OperationID: "list-number-formats",
		Method:      "GET",
		Path:        "/settings/number-formats",
		Summary:     "Seznam číselných řad",
		Tags:        []string{"settings"},
	}, func(ctx context.Context, _ *struct{}) (*ListFormatsOutput, error) {
		userID := auth.UserIDFromCtx(ctx)
		s, err := database.GetUserSettings(db.WithContext(ctx), userID)
		if err != nil {
			return nil, huma.Error500InternalServerError("chyba databáze", err)
		}
		var formats []models.NumberFormat
		if err := db.WithContext(ctx).Where("settings_id = ?", s.ID).Find(&formats).Error; err != nil {
			return nil, huma.Error500InternalServerError("chyba databáze", err)
		}
		return &ListFormatsOutput{Body: formats}, nil
	})

	// --- POST /settings/number-formats ---------------------------------------
	type CreateFormatInput struct {
		Body NumberFormatInput
	}
	type FormatOutput struct {
		Body models.NumberFormat
	}

	huma.Register(api, huma.Operation{
		OperationID:   "create-number-format",
		Method:        "POST",
		Path:          "/settings/number-formats",
		Summary:       "Vytvoř číselnou řadu",
		Tags:          []string{"settings"},
		DefaultStatus: 201,
	}, func(ctx context.Context, in *CreateFormatInput) (*FormatOutput, error) {
		userID := auth.UserIDFromCtx(ctx)
		s, err := database.GetUserSettings(db.WithContext(ctx), userID)
		if err != nil {
			return nil, huma.Error500InternalServerError("chyba načtení nastavení", err)
		}
		nf := models.NumberFormat{
			SettingsID:   s.ID,
			DocumentType: orDefault(in.Body.DocumentType, "invoice"),
			Label:        in.Body.Label,
			Pattern:      orDefault(in.Body.Pattern, "{YYYY}{NNN}"),
			NextNumber:   orDefaultInt(in.Body.NextNumber, 1),
			PaddingWidth: orDefaultInt(in.Body.PaddingWidth, 3),
		}
		if err := db.WithContext(ctx).Create(&nf).Error; err != nil {
			return nil, huma.Error422UnprocessableEntity("uložení číselné řady selhalo", err)
		}
		return &FormatOutput{Body: nf}, nil
	})

	// --- PUT /settings/number-formats/{id} -----------------------------------
	type UpdateFormatInput struct {
		ID   uint `path:"id" doc:"ID číselné řady"`
		Body NumberFormatInput
	}

	huma.Register(api, huma.Operation{
		OperationID: "update-number-format",
		Method:      "PUT",
		Path:        "/settings/number-formats/{id}",
		Summary:     "Aktualizuj číselnou řadu",
		Tags:        []string{"settings"},
	}, func(ctx context.Context, in *UpdateFormatInput) (*FormatOutput, error) {
		userID := auth.UserIDFromCtx(ctx)
		s, err := database.GetUserSettings(db.WithContext(ctx), userID)
		if err != nil {
			return nil, huma.Error500InternalServerError("chyba načtení nastavení", err)
		}
		var nf models.NumberFormat
		if err := db.WithContext(ctx).Where("id = ? AND settings_id = ?", in.ID, s.ID).First(&nf).Error; err != nil {
			return nil, huma.Error404NotFound(fmt.Sprintf("číselná řada %d nenalezena", in.ID), err)
		}
		updates := map[string]interface{}{}
		if in.Body.DocumentType != "" {
			updates["document_type"] = in.Body.DocumentType
		}
		if in.Body.Label != "" {
			updates["label"] = in.Body.Label
		}
		if in.Body.Pattern != "" {
			updates["pattern"] = in.Body.Pattern
		}
		if in.Body.NextNumber != 0 {
			updates["next_number"] = in.Body.NextNumber
		}
		if in.Body.PaddingWidth != 0 {
			updates["padding_width"] = in.Body.PaddingWidth
		}
		if len(updates) > 0 {
			if err := db.WithContext(ctx).Model(&nf).Updates(updates).Error; err != nil {
				return nil, huma.Error500InternalServerError("uložení selhalo", err)
			}
		}
		if err := db.WithContext(ctx).First(&nf, in.ID).Error; err != nil {
			return nil, huma.Error500InternalServerError("chyba načtení", err)
		}
		return &FormatOutput{Body: nf}, nil
	})

	// --- DELETE /settings/number-formats/{id} --------------------------------
	type DeleteFormatInput struct {
		ID uint `path:"id" doc:"ID číselné řady"`
	}

	huma.Register(api, huma.Operation{
		OperationID:   "delete-number-format",
		Method:        "DELETE",
		Path:          "/settings/number-formats/{id}",
		Summary:       "Smaž číselnou řadu",
		Tags:          []string{"settings"},
		DefaultStatus: 204,
	}, func(ctx context.Context, in *DeleteFormatInput) (*struct{}, error) {
		userID := auth.UserIDFromCtx(ctx)
		s, err := database.GetUserSettings(db.WithContext(ctx), userID)
		if err != nil {
			return nil, huma.Error500InternalServerError("chyba načtení nastavení", err)
		}
		var nf models.NumberFormat
		if err := db.WithContext(ctx).Where("id = ? AND settings_id = ?", in.ID, s.ID).First(&nf).Error; err != nil {
			return nil, huma.Error404NotFound(fmt.Sprintf("číselná řada %d nenalezena", in.ID), err)
		}
		if err := db.WithContext(ctx).Delete(&nf).Error; err != nil {
			return nil, huma.Error500InternalServerError("smazání selhalo", err)
		}
		return nil, nil
	})

	// --- POST /settings/number-formats/{id}/next -----------------------------
	type NextFormatInput struct {
		ID uint `path:"id" doc:"ID číselné řady"`
	}
	type NextFormatOutput struct {
		Body struct {
			Number string `json:"number"`
		}
	}

	huma.Register(api, huma.Operation{
		OperationID: "next-number-format",
		Method:      "POST",
		Path:        "/settings/number-formats/{id}/next",
		Summary:     "Vygeneruj další číslo dokladu",
		Tags:        []string{"settings"},
	}, func(ctx context.Context, in *NextFormatInput) (*NextFormatOutput, error) {
		userID := auth.UserIDFromCtx(ctx)
		s, err := database.GetUserSettings(db.WithContext(ctx), userID)
		if err != nil {
			return nil, huma.Error500InternalServerError("chyba načtení nastavení", err)
		}
		var generated string
		err = db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			var nf models.NumberFormat
			if err := tx.Where("id = ? AND settings_id = ?", in.ID, s.ID).First(&nf).Error; err != nil {
				return huma.Error404NotFound(fmt.Sprintf("číselná řada %d nenalezena", in.ID), err)
			}
			generated = nf.Generate()
			return tx.Model(&nf).Update("next_number", nf.NextNumber).Error
		})
		if err != nil {
			return nil, err
		}
		out := &NextFormatOutput{}
		out.Body.Number = generated
		return out, nil
	})
}
