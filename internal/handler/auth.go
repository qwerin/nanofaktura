package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/go-chi/chi/v5"
	"gorm.io/gorm"

	"github.com/qwerin/nanofaktura/internal/auth"
	"github.com/qwerin/nanofaktura/internal/config"
	"github.com/qwerin/nanofaktura/internal/database"
	"github.com/qwerin/nanofaktura/internal/models"
)

// RegisterAuth registruje auth a setup endpointy.
//
//	POST /auth/login    — přihlášení (raw chi, Set-Cookie)
//	POST /auth/logout   — odhlášení  (raw chi, smazat cookie)
//	GET  /auth/me       — info o přihlášeném uživateli (Huma)
//	POST /setup/init    — first-run wizard (dostupný dokud není Initialized=true)
func RegisterAuth(
	api huma.API,
	r chi.Router,
	db *gorm.DB,
	cfg config.AppConfig,
	multiUser *atomic.Bool,
	initialized *atomic.Bool,
) {
	ttl := time.Duration(cfg.SessionTTL) * time.Hour

	// --- POST /api/auth/login -------------------------------------------------
	r.Post("/auth/login", func(w http.ResponseWriter, req *http.Request) {
		if !multiUser.Load() {
			writeJSON(w, http.StatusForbidden, map[string]string{"title": "Single-user mode"})
			return
		}
		var body struct {
			Username string `json:"username"`
			Password string `json:"password"`
		}
		if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"title": "Bad Request"})
			return
		}
		var user auth.User
		if err := db.Where("username = ? AND is_active = true", body.Username).First(&user).Error; err != nil {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"title": "Neplatné přihlašovací údaje"})
			return
		}
		if !auth.CheckPassword(user.PasswordHash, body.Password) {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"title": "Neplatné přihlašovací údaje"})
			return
		}
		token, err := auth.NewSession(db, user.ID, ttl)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"title": "Interní chyba"})
			return
		}
		auth.SetSessionCookie(w, token, ttl)
		writeJSON(w, http.StatusOK, userResponse(user))
	})

	// --- POST /api/auth/logout ------------------------------------------------
	r.Post("/auth/logout", func(w http.ResponseWriter, req *http.Request) {
		if cookie, err := req.Cookie(auth.CookieName); err == nil {
			db.Where("token_hash = ?", auth.HashToken(cookie.Value)).Delete(&auth.Session{})
		}
		auth.ClearSessionCookie(w)
		w.WriteHeader(http.StatusNoContent)
	})

	// --- GET /api/auth/me (Huma) ----------------------------------------------
	type MeOutput struct {
		Body struct {
			ID       uint          `json:"id"`
			Username string        `json:"username"`
			Email    string        `json:"email,omitempty"`
			Role     auth.UserRole `json:"role"`
		}
	}

	huma.Register(api, huma.Operation{
		OperationID: "get-me",
		Method:      "GET",
		Path:        "/auth/me",
		Summary:     "Přihlášený uživatel",
		Tags:        []string{"auth"},
	}, func(ctx context.Context, _ *struct{}) (*MeOutput, error) {
		u := auth.UserFromCtx(ctx)
		if u == nil {
			return nil, huma.NewError(http.StatusUnauthorized, "not authenticated")
		}
		out := &MeOutput{}
		out.Body.ID = u.ID
		out.Body.Username = u.Username
		out.Body.Email = u.Email
		out.Body.Role = u.Role
		return out, nil
	})

	// --- POST /api/setup/init — first-run wizard ------------------------------
	// Přijme firemní údaje, volbu multi-user a (pokud multi-user) přihlašovací údaje admina.
	// Po úspěšné inicializaci vrací 410 Gone.
	r.Post("/setup/init", func(w http.ResponseWriter, req *http.Request) {
		if initialized.Load() {
			writeJSON(w, http.StatusGone, map[string]string{"title": "Setup already completed"})
			return
		}

		var body struct {
			// Firemní údaje
			CompanyName    string `json:"company_name"`
			CompanyStreet  string `json:"company_street"`
			CompanyCity    string `json:"company_city"`
			CompanyZip     string `json:"company_zip"`
			RegistrationNo string `json:"registration_no"`
			VatNo          string `json:"vat_no"`
			VatExempt      bool   `json:"vat_exempt"`
			BankAccount    string `json:"bank_account"`
			IBAN           string `json:"iban"`
			SwiftBIC       string `json:"swift_bic"`

			// Režim aplikace
			MultiUser bool `json:"multi_user"`

			// Přihlašovací údaje admina (povinné pokud multi_user=true)
			Username string `json:"username"`
			Email    string `json:"email"`
			Password string `json:"password"`
		}
		if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"title": "Neplatný požadavek"})
			return
		}
		if body.CompanyName == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"title": "Název firmy je povinný"})
			return
		}
		if body.MultiUser && (body.Username == "" || body.Password == "") {
			writeJSON(w, http.StatusBadRequest, map[string]string{"title": "Uživatelské jméno a heslo jsou povinné pro multi-user mód"})
			return
		}

		var adminUserID uint

		if body.MultiUser {
			// Vytvoř superadmina
			hash, err := auth.HashPassword(body.Password)
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"title": "Interní chyba"})
				return
			}
			admin := auth.User{
				Username:     body.Username,
				Email:        body.Email,
				PasswordHash: hash,
				Role:         auth.RoleSuperAdmin,
				IsActive:     true,
			}
			if err := db.Create(&admin).Error; err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"title": "Vytvoření uživatele selhalo"})
				return
			}
			adminUserID = admin.ID
		}

		// Vytvoř nastavení firmy
		s := models.DefaultSettings()
		s.UserID = adminUserID // 0 = single-user, >0 = patří superadminovi
		s.CompanyName = body.CompanyName
		s.CompanyStreet = body.CompanyStreet
		s.CompanyCity = body.CompanyCity
		s.CompanyZip = body.CompanyZip
		s.RegistrationNo = body.RegistrationNo
		s.VatNo = body.VatNo
		s.VatExempt = body.VatExempt
		s.BankAccount = body.BankAccount
		s.IBAN = body.IBAN
		s.SwiftBIC = body.SwiftBIC
		if err := db.Create(&s).Error; err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"title": "Uložení nastavení selhalo"})
			return
		}

		// Ulož SystemConfig a aktualizuj runtime stav
		syscfg := models.SystemConfig{
			MultiUser:   body.MultiUser,
			Initialized: true,
		}
		if err := database.SaveSystemConfig(db, syscfg); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"title": "Uložení konfigurace selhalo"})
			return
		}
		multiUser.Store(body.MultiUser)
		initialized.Store(true)

		if body.MultiUser {
			// Nastav session cookie pro přihlášeného admina
			var admin auth.User
			db.First(&admin, adminUserID)
			token, err := auth.NewSession(db, adminUserID, ttl)
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"title": "Interní chyba"})
				return
			}
			auth.SetSessionCookie(w, token, ttl)
			writeJSON(w, http.StatusCreated, userResponse(admin))
		} else {
			writeJSON(w, http.StatusCreated, map[string]string{"status": "ok"})
		}
	})
}

// userResponse vrátí mapu s veřejnými poli uživatele (bez hash hesla).
func userResponse(u auth.User) map[string]interface{} {
	return map[string]interface{}{
		"id":        u.ID,
		"username":  u.Username,
		"email":     u.Email,
		"role":      u.Role,
		"is_active": u.IsActive,
	}
}

// writeJSON zapíše JSON odpověď s daným status kódem.
func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
