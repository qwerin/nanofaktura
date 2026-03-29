package handler

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/go-chi/chi/v5"
	"gorm.io/gorm"

	"github.com/qwerin/nanofaktura/internal/auth"
	"github.com/qwerin/nanofaktura/internal/database"
	"github.com/qwerin/nanofaktura/internal/models"
	"github.com/qwerin/nanofaktura/internal/pdf"
)

func todayString() string {
	return time.Now().Format("2006-01-02")
}

// RegisterInvoice registruje CRUD endpointy pro faktury.
func RegisterInvoice(api huma.API, r chi.Router, db *gorm.DB) {

	// --- CREATE -----------------------------------------------------------
	type CreateInput struct {
		Body InvoiceInput // DTO bez GORM polí → Huma nebude vyžadovat computed/readonly pole
	}
	type CreateOutput struct {
		Body models.Invoice
	}

	huma.Register(api, huma.Operation{
		OperationID:   "create-invoice",
		Method:        "POST",
		Path:          "/invoices",
		Summary:       "Vytvoř novou fakturu",
		Tags:          []string{"invoices"},
		DefaultStatus: 201,
	}, func(ctx context.Context, in *CreateInput) (*CreateOutput, error) {
		inv := in.Body.toModel()
		inv.UserID = auth.UserIDFromCtx(ctx)
		inv.Recalculate()
		if err := db.WithContext(ctx).Create(&inv).Error; err != nil {
			return nil, huma.Error422UnprocessableEntity("uložení faktury selhalo", err)
		}
		// Načteme znovu včetně vygenerovaného ID a timestamps
		if err := db.WithContext(ctx).Preload("Lines").First(&inv, inv.ID).Error; err != nil {
			return nil, huma.Error500InternalServerError("chyba načtení", err)
		}
		return &CreateOutput{Body: inv}, nil
	})

	// --- LIST -------------------------------------------------------------
	type ListOutput struct {
		Body []models.Invoice
	}

	huma.Register(api, huma.Operation{
		OperationID: "list-invoices",
		Method:      "GET",
		Path:        "/invoices",
		Summary:     "Seznam faktur",
		Tags:        []string{"invoices"},
	}, func(ctx context.Context, _ *struct{}) (*ListOutput, error) {
		var invoices []models.Invoice
		userID := auth.UserIDFromCtx(ctx)
		if err := db.WithContext(ctx).Preload("Lines").Where("user_id = ?", userID).Find(&invoices).Error; err != nil {
			return nil, huma.Error500InternalServerError("chyba databáze", err)
		}
		return &ListOutput{Body: invoices}, nil
	})

	// --- GET BY ID --------------------------------------------------------
	type GetInput struct {
		ID uint `path:"id" doc:"ID faktury"`
	}
	type GetOutput struct {
		Body models.Invoice
	}

	huma.Register(api, huma.Operation{
		OperationID: "get-invoice",
		Method:      "GET",
		Path:        "/invoices/{id}",
		Summary:     "Detail faktury",
		Tags:        []string{"invoices"},
	}, func(ctx context.Context, in *GetInput) (*GetOutput, error) {
		var inv models.Invoice
		userID := auth.UserIDFromCtx(ctx)
		err := db.WithContext(ctx).Preload("Lines").Where("user_id = ?", userID).First(&inv, in.ID).Error
		if err != nil {
			return nil, huma.Error404NotFound(fmt.Sprintf("faktura %d nenalezena", in.ID), err)
		}
		return &GetOutput{Body: inv}, nil
	})

	// --- PDF --------------------------------------------------------------

	r.Get("/invoices/{id}/pdf", func(w http.ResponseWriter, req *http.Request) {
		id, err := strconv.ParseUint(chi.URLParam(req, "id"), 10, 64)
		if err != nil {
			http.Error(w, "invalid id", http.StatusBadRequest)
			return
		}
		userID := auth.UserIDFromCtx(req.Context())
		var inv models.Invoice
		if err := db.WithContext(req.Context()).Preload("Lines").Where("user_id = ?", userID).First(&inv, id).Error; err != nil {
			http.Error(w, fmt.Sprintf("faktura %d nenalezena", id), http.StatusNotFound)
			return
		}
		// Load template from user settings
		tmpl := "classic"
		if s, err := database.GetUserSettings(db.WithContext(req.Context()), userID); err == nil {
			tmpl = s.InvoiceTemplate
		}
		pdfBytes, err := pdf.Generate(pdf.InvoiceRequest{Invoice: &inv, Template: tmpl})
		if err != nil {
			http.Error(w, "generování PDF selhalo", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/pdf")
		w.Header().Set("Content-Disposition", fmt.Sprintf(`inline; filename="faktura-%s.pdf"`, inv.Number))
		w.WriteHeader(http.StatusOK)
		w.Write(pdfBytes) //nolint:errcheck
	})

	// --- DUPLICATE --------------------------------------------------------
	type DuplicateInput struct {
		ID uint `path:"id" doc:"ID faktury k duplikaci"`
	}

	huma.Register(api, huma.Operation{
		OperationID:   "duplicate-invoice",
		Method:        "POST",
		Path:          "/invoices/{id}/duplicate",
		Summary:       "Duplikuj fakturu (nové číslo a datum)",
		Tags:          []string{"invoices"},
		DefaultStatus: 201,
	}, func(ctx context.Context, in *DuplicateInput) (*CreateOutput, error) {
		userID := auth.UserIDFromCtx(ctx)
		var src models.Invoice
		if err := db.WithContext(ctx).Preload("Lines").Where("user_id = ?", userID).First(&src, in.ID).Error; err != nil {
			return nil, huma.Error404NotFound(fmt.Sprintf("faktura %d nenalezena", in.ID), err)
		}

		dup := models.Invoice{
			UserID: userID,
			DocumentType:          src.DocumentType,
			Number:                src.Number + "-kopie",
			IssuedOn:              todayString(),
			TaxableFulfillmentDue: todayString(),
			Due:                   src.Due,
			SubjectID:             src.SubjectID,

			YourName:           src.YourName,
			YourStreet:         src.YourStreet,
			YourCity:           src.YourCity,
			YourZip:            src.YourZip,
			YourCountry:        src.YourCountry,
			YourRegistrationNo: src.YourRegistrationNo,
			YourVatNo:          src.YourVatNo,

			ClientName:           src.ClientName,
			ClientStreet:         src.ClientStreet,
			ClientCity:           src.ClientCity,
			ClientZip:            src.ClientZip,
			ClientCountry:        src.ClientCountry,
			ClientRegistrationNo: src.ClientRegistrationNo,
			ClientVatNo:          src.ClientVatNo,

			PaymentMethod: src.PaymentMethod,
			BankAccount:   src.BankAccount,
			IBAN:          src.IBAN,
			SwiftBIC:      src.SwiftBIC,
			Currency:      src.Currency,
			ExchangeRate:  src.ExchangeRate,
			VatExempt:     src.VatExempt,
			Note:          src.Note,
			FooterNote:    src.FooterNote,
			Tags:          src.Tags,
		}
		for _, l := range src.Lines {
			dup.Lines = append(dup.Lines, models.InvoiceLine{
				Position:     l.Position,
				Name:         l.Name,
				Quantity:     l.Quantity,
				UnitName:     l.UnitName,
				UnitPriceHal: l.UnitPriceHal,
				VatRateBps:   l.VatRateBps,
			})
		}
		dup.Recalculate()

		if err := db.WithContext(ctx).Create(&dup).Error; err != nil {
			return nil, huma.Error422UnprocessableEntity("duplikace selhala", err)
		}
		if err := db.WithContext(ctx).Preload("Lines").First(&dup, dup.ID).Error; err != nil {
			return nil, huma.Error500InternalServerError("chyba načtení", err)
		}
		return &CreateOutput{Body: dup}, nil
	})

	// --- UPDATE -----------------------------------------------------------
	type UpdateInput struct {
		ID   uint         `path:"id" doc:"ID faktury"`
		Body InvoiceInput // DTO (stejný jako při vytváření)
	}

	huma.Register(api, huma.Operation{
		OperationID: "update-invoice",
		Method:      "PUT",
		Path:        "/invoices/{id}",
		Summary:     "Aktualizuj fakturu",
		Tags:        []string{"invoices"},
	}, func(ctx context.Context, in *UpdateInput) (*GetOutput, error) {
		userID := auth.UserIDFromCtx(ctx)
		var existing models.Invoice
		if err := db.WithContext(ctx).Where("user_id = ?", userID).First(&existing, in.ID).Error; err != nil {
			return nil, huma.Error404NotFound(fmt.Sprintf("faktura %d nenalezena", in.ID), err)
		}

		updated := in.Body.toModel()
		updated.ID = existing.ID
		updated.UserID = existing.UserID
		updated.Status = existing.Status
		updated.CreatedAt = existing.CreatedAt

		newLines := updated.Lines
		for i := range newLines {
			newLines[i].InvoiceID = existing.ID
		}
		updated.Recalculate()

		// Smaž stávající řádky
		if err := db.WithContext(ctx).Where("invoice_id = ?", existing.ID).Delete(&models.InvoiceLine{}).Error; err != nil {
			return nil, huma.Error500InternalServerError("smazání řádků selhalo", err)
		}

		// Ulož fakturu bez asociací
		updated.Lines = nil
		if err := db.WithContext(ctx).Save(&updated).Error; err != nil {
			return nil, huma.Error422UnprocessableEntity("uložení faktury selhalo", err)
		}

		// Vlož nové řádky
		if len(newLines) > 0 {
			if err := db.WithContext(ctx).Create(&newLines).Error; err != nil {
				return nil, huma.Error422UnprocessableEntity("uložení řádků selhalo", err)
			}
		}

		if err := db.WithContext(ctx).Preload("Lines").First(&updated, updated.ID).Error; err != nil {
			return nil, huma.Error500InternalServerError("chyba načtení", err)
		}
		return &GetOutput{Body: updated}, nil
	})

	// --- UPDATE STATUS ----------------------------------------------------
	type PatchStatusInput struct {
		ID   uint `path:"id" doc:"ID faktury"`
		Body struct {
			Status string `json:"status" enum:"open,sent,overdue,paid,cancelled"`
		}
	}

	huma.Register(api, huma.Operation{
		OperationID: "patch-invoice-status",
		Method:      "PATCH",
		Path:        "/invoices/{id}/status",
		Summary:     "Změň stav faktury",
		Tags:        []string{"invoices"},
	}, func(ctx context.Context, in *PatchStatusInput) (*GetOutput, error) {
		userID := auth.UserIDFromCtx(ctx)
		var inv models.Invoice
		if err := db.WithContext(ctx).Where("user_id = ?", userID).First(&inv, in.ID).Error; err != nil {
			return nil, huma.Error404NotFound(fmt.Sprintf("faktura %d nenalezena", in.ID), err)
		}
		inv.Status = models.InvoiceStatus(in.Body.Status)
		if err := db.WithContext(ctx).Save(&inv).Error; err != nil {
			return nil, huma.Error500InternalServerError("uložení selhalo", err)
		}
		if err := db.WithContext(ctx).Preload("Lines").First(&inv, inv.ID).Error; err != nil {
			return nil, huma.Error500InternalServerError("chyba načtení", err)
		}
		return &GetOutput{Body: inv}, nil
	})
}
