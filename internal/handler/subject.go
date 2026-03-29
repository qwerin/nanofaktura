package handler

import (
	"context"
	"fmt"

	"github.com/danielgtaylor/huma/v2"
	"gorm.io/gorm"

	"github.com/qwerin/nanofaktura/internal/auth"
	"github.com/qwerin/nanofaktura/internal/models"
)

// RegisterSubject registruje CRUD endpointy pro kontakty (subjects).
func RegisterSubject(api huma.API, db *gorm.DB) {

	type GetOutput struct {
		Body models.Subject
	}

	// --- LIST -------------------------------------------------------------
	type ListOutput struct {
		Body []models.Subject
	}

	huma.Register(api, huma.Operation{
		OperationID: "list-subjects",
		Method:      "GET",
		Path:        "/subjects",
		Summary:     "Seznam kontaktů",
		Tags:        []string{"subjects"},
	}, func(ctx context.Context, _ *struct{}) (*ListOutput, error) {
		var subjects []models.Subject
		userID := auth.UserIDFromCtx(ctx)
		if err := db.WithContext(ctx).Where("user_id = ?", userID).Find(&subjects).Error; err != nil {
			return nil, huma.Error500InternalServerError("chyba databáze", err)
		}
		return &ListOutput{Body: subjects}, nil
	})

	// --- CREATE -----------------------------------------------------------
	type CreateInput struct {
		Body SubjectInput // DTO bez gorm.Model
	}

	huma.Register(api, huma.Operation{
		OperationID:   "create-subject",
		Method:        "POST",
		Path:          "/subjects",
		Summary:       "Vytvoř nový kontakt",
		Tags:          []string{"subjects"},
		DefaultStatus: 201,
	}, func(ctx context.Context, in *CreateInput) (*GetOutput, error) {
		subject := in.Body.toModel()
		subject.UserID = auth.UserIDFromCtx(ctx)
		if err := db.WithContext(ctx).Create(&subject).Error; err != nil {
			return nil, huma.Error422UnprocessableEntity("uložení kontaktu selhalo", err)
		}
		return &GetOutput{Body: subject}, nil
	})

	// --- GET BY ID --------------------------------------------------------
	type GetInput struct {
		ID uint `path:"id" doc:"ID kontaktu"`
	}

	huma.Register(api, huma.Operation{
		OperationID: "get-subject",
		Method:      "GET",
		Path:        "/subjects/{id}",
		Summary:     "Detail kontaktu",
		Tags:        []string{"subjects"},
	}, func(ctx context.Context, in *GetInput) (*GetOutput, error) {
		var subject models.Subject
		userID := auth.UserIDFromCtx(ctx)
		if err := db.WithContext(ctx).Where("user_id = ?", userID).First(&subject, in.ID).Error; err != nil {
			return nil, huma.Error404NotFound(fmt.Sprintf("kontakt %d nenalezen", in.ID), err)
		}
		return &GetOutput{Body: subject}, nil
	})

	// --- UPDATE -----------------------------------------------------------
	type UpdateInput struct {
		ID   uint         `path:"id" doc:"ID kontaktu"`
		Body SubjectInput // DTO bez gorm.Model
	}

	huma.Register(api, huma.Operation{
		OperationID: "update-subject",
		Method:      "PUT",
		Path:        "/subjects/{id}",
		Summary:     "Aktualizuj kontakt",
		Tags:        []string{"subjects"},
	}, func(ctx context.Context, in *UpdateInput) (*GetOutput, error) {
		userID := auth.UserIDFromCtx(ctx)
		var existing models.Subject
		if err := db.WithContext(ctx).Where("user_id = ?", userID).First(&existing, in.ID).Error; err != nil {
			return nil, huma.Error404NotFound(fmt.Sprintf("kontakt %d nenalezen", in.ID), err)
		}
		updated := in.Body.toModel()
		updated.Base = existing.Base // zachováme ID a timestamps
		updated.UserID = existing.UserID
		if err := db.WithContext(ctx).Save(&updated).Error; err != nil {
			return nil, huma.Error422UnprocessableEntity("aktualizace kontaktu selhala", err)
		}
		return &GetOutput{Body: updated}, nil
	})

	// --- DELETE -----------------------------------------------------------
	type DeleteInput struct {
		ID uint `path:"id" doc:"ID kontaktu"`
	}

	huma.Register(api, huma.Operation{
		OperationID:   "delete-subject",
		Method:        "DELETE",
		Path:          "/subjects/{id}",
		Summary:       "Smaž kontakt",
		Tags:          []string{"subjects"},
		DefaultStatus: 204,
	}, func(ctx context.Context, in *DeleteInput) (*struct{}, error) {
		userID := auth.UserIDFromCtx(ctx)
		if err := db.WithContext(ctx).Where("user_id = ?", userID).Delete(&models.Subject{}, in.ID).Error; err != nil {
			return nil, huma.Error500InternalServerError("smazání kontaktu selhalo", err)
		}
		return nil, nil
	})
}
