package handler

import (
	"context"
	"fmt"
	"net/http"
	"sync/atomic"

	"github.com/danielgtaylor/huma/v2"
	"gorm.io/gorm"

	"github.com/qwerin/nanofaktura/internal/auth"
	"github.com/qwerin/nanofaktura/internal/models"
)

// RegisterUsers registruje endpointy pro správu uživatelů.
// Každý handler interně ověřuje superadmin oprávnění.
func RegisterUsers(api huma.API, db *gorm.DB, multiUser *atomic.Bool) {
	requireSuperAdmin := func(ctx context.Context) error {
		if !auth.IsSuperAdmin(ctx, multiUser) {
			return huma.NewError(http.StatusForbidden, "superadmin required")
		}
		return nil
	}

	type UserOutput struct {
		Body auth.User
	}
	type ListOutput struct {
		Body []auth.User
	}

	// --- GET /users -----------------------------------------------------------
	huma.Register(api, huma.Operation{
		OperationID: "list-users",
		Method:      "GET",
		Path:        "/users",
		Summary:     "Seznam uživatelů (superadmin)",
		Tags:        []string{"users"},
	}, func(ctx context.Context, _ *struct{}) (*ListOutput, error) {
		if err := requireSuperAdmin(ctx); err != nil {
			return nil, err
		}
		var users []auth.User
		if err := db.WithContext(ctx).Find(&users).Error; err != nil {
			return nil, huma.Error500InternalServerError("chyba databáze", err)
		}
		return &ListOutput{Body: users}, nil
	})

	// --- GET /users/{id} ------------------------------------------------------
	type GetInput struct {
		ID uint `path:"id" doc:"ID uživatele"`
	}

	huma.Register(api, huma.Operation{
		OperationID: "get-user",
		Method:      "GET",
		Path:        "/users/{id}",
		Summary:     "Detail uživatele (superadmin)",
		Tags:        []string{"users"},
	}, func(ctx context.Context, in *GetInput) (*UserOutput, error) {
		if err := requireSuperAdmin(ctx); err != nil {
			return nil, err
		}
		var user auth.User
		if err := db.WithContext(ctx).First(&user, in.ID).Error; err != nil {
			return nil, huma.Error404NotFound(fmt.Sprintf("uživatel %d nenalezen", in.ID), err)
		}
		return &UserOutput{Body: user}, nil
	})

	// --- POST /users ----------------------------------------------------------
	type CreateUserInput struct {
		Body struct {
			Username string        `json:"username"`
			Password string        `json:"password"`
			Email    string        `json:"email,omitempty"`
			Role     auth.UserRole `json:"role,omitempty"`
		}
	}

	huma.Register(api, huma.Operation{
		OperationID:   "create-user",
		Method:        "POST",
		Path:          "/users",
		Summary:       "Vytvoř uživatele (superadmin)",
		Tags:          []string{"users"},
		DefaultStatus: 201,
	}, func(ctx context.Context, in *CreateUserInput) (*UserOutput, error) {
		if err := requireSuperAdmin(ctx); err != nil {
			return nil, err
		}
		if in.Body.Username == "" || in.Body.Password == "" {
			return nil, huma.NewError(http.StatusBadRequest, "username a password jsou povinné")
		}
		hash, err := auth.HashPassword(in.Body.Password)
		if err != nil {
			return nil, huma.Error500InternalServerError("hash hesla selhal", err)
		}
		role := in.Body.Role
		if role == "" {
			role = auth.RoleUser
		}
		user := auth.User{
			Username:     in.Body.Username,
			Email:        in.Body.Email,
			PasswordHash: hash,
			Role:         role,
			IsActive:     true,
		}
		if err := db.WithContext(ctx).Create(&user).Error; err != nil {
			return nil, huma.Error422UnprocessableEntity("vytvoření uživatele selhalo", err)
		}

		// Vytvoř výchozí nastavení pro nového uživatele
		s := models.DefaultSettings()
		s.UserID = user.ID
		db.WithContext(ctx).Create(&s)

		return &UserOutput{Body: user}, nil
	})

	// --- PUT /users/{id} -------------------------------------------------------
	type UpdateUserInput struct {
		ID   uint `path:"id" doc:"ID uživatele"`
		Body struct {
			Email    string        `json:"email,omitempty"`
			Role     auth.UserRole `json:"role,omitempty"`
			IsActive *bool         `json:"is_active,omitempty"`
		}
	}

	huma.Register(api, huma.Operation{
		OperationID: "update-user",
		Method:      "PUT",
		Path:        "/users/{id}",
		Summary:     "Aktualizuj uživatele (superadmin)",
		Tags:        []string{"users"},
	}, func(ctx context.Context, in *UpdateUserInput) (*UserOutput, error) {
		if err := requireSuperAdmin(ctx); err != nil {
			return nil, err
		}
		var user auth.User
		if err := db.WithContext(ctx).First(&user, in.ID).Error; err != nil {
			return nil, huma.Error404NotFound(fmt.Sprintf("uživatel %d nenalezen", in.ID), err)
		}
		updates := map[string]interface{}{}
		if in.Body.Email != "" {
			updates["email"] = in.Body.Email
		}
		if in.Body.Role != "" {
			updates["role"] = in.Body.Role
		}
		if in.Body.IsActive != nil {
			updates["is_active"] = *in.Body.IsActive
		}
		if len(updates) > 0 {
			if err := db.WithContext(ctx).Model(&user).Updates(updates).Error; err != nil {
				return nil, huma.Error500InternalServerError("aktualizace selhala", err)
			}
		}
		if err := db.WithContext(ctx).First(&user, in.ID).Error; err != nil {
			return nil, huma.Error500InternalServerError("chyba načtení", err)
		}
		return &UserOutput{Body: user}, nil
	})

	// --- POST /users/{id}/reset-password --------------------------------------
	type ResetPasswordInput struct {
		ID   uint `path:"id" doc:"ID uživatele"`
		Body struct {
			Password string `json:"password"`
		}
	}

	huma.Register(api, huma.Operation{
		OperationID: "reset-password",
		Method:      "POST",
		Path:        "/users/{id}/reset-password",
		Summary:     "Reset hesla (superadmin)",
		Tags:        []string{"users"},
	}, func(ctx context.Context, in *ResetPasswordInput) (*struct{}, error) {
		if err := requireSuperAdmin(ctx); err != nil {
			return nil, err
		}
		if in.Body.Password == "" {
			return nil, huma.NewError(http.StatusBadRequest, "password je povinný")
		}
		hash, err := auth.HashPassword(in.Body.Password)
		if err != nil {
			return nil, huma.Error500InternalServerError("hash hesla selhal", err)
		}
		if err := db.WithContext(ctx).Model(&auth.User{}).Where("id = ?", in.ID).
			Update("password_hash", hash).Error; err != nil {
			return nil, huma.Error500InternalServerError("reset hesla selhal", err)
		}
		// Zrušíme všechny aktivní session daného uživatele
		db.WithContext(ctx).Where("user_id = ?", in.ID).Delete(&auth.Session{})
		return nil, nil
	})

	// --- DELETE /users/{id} ---------------------------------------------------
	type DeleteUserInput struct {
		ID uint `path:"id" doc:"ID uživatele"`
	}

	huma.Register(api, huma.Operation{
		OperationID:   "delete-user",
		Method:        "DELETE",
		Path:          "/users/{id}",
		Summary:       "Smaž uživatele (superadmin)",
		Tags:          []string{"users"},
		DefaultStatus: 204,
	}, func(ctx context.Context, in *DeleteUserInput) (*struct{}, error) {
		if err := requireSuperAdmin(ctx); err != nil {
			return nil, err
		}
		// Nelze smazat sebe sama
		caller := auth.UserFromCtx(ctx)
		if caller != nil && caller.ID == in.ID {
			return nil, huma.NewError(http.StatusBadRequest, "nelze smazat vlastní účet")
		}
		if err := db.WithContext(ctx).Delete(&auth.User{}, in.ID).Error; err != nil {
			return nil, huma.Error500InternalServerError("smazání uživatele selhalo", err)
		}
		// Zrušíme session smazaného uživatele
		db.WithContext(ctx).Where("user_id = ?", in.ID).Delete(&auth.Session{})
		return nil, nil
	})
}
