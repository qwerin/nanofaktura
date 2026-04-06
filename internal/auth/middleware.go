package auth

import (
	"context"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	"gorm.io/gorm"
)

type contextKey string

const userCtxKey contextKey = "auth_user"

// CookieName je název HTTP cookie pro session token.
const CookieName = "nf_session"

// publicPaths jsou endpointy dostupné bez přihlášení vždy.
// Vite proxy stripuje /api prefix, backend vidí cesty bez něj.
var publicPaths = map[string]bool{
	"/health":      true,
	"/auth/login":  true,
	"/auth/logout": true,
	"/setup/init":  true,
}

// UserFromCtx vrátí přihlášeného uživatele z kontextu (nil v single-user módu nebo bez přihlášení).
func UserFromCtx(ctx context.Context) *User {
	u, _ := ctx.Value(userCtxKey).(*User)
	return u
}

// UserIDFromCtx vrátí ID přihlášeného uživatele, nebo 0 v single-user módu.
func UserIDFromCtx(ctx context.Context) uint {
	if u := UserFromCtx(ctx); u != nil {
		return u.ID
	}
	return 0
}

// ConditionalAuth je chi middleware který:
//   - vždy propustí publicPaths bez ověření
//   - pokud multiUser=false: propustí vše (single-user mód)
//   - pokud multiUser=true: vyžaduje platnou session cookie
func ConditionalAuth(db *gorm.DB, multiUser *atomic.Bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// chi nemění r.URL.Path pro middleware v mountovaném subrouteru;
			// stripujeme /api prefix (mount point) před porovnáním s publicPaths.
			path := r.URL.Path
			if after, ok := strings.CutPrefix(path, "/api"); ok && after != "" {
				path = after
			}
			if !multiUser.Load() || publicPaths[path] {
				next.ServeHTTP(w, r)
				return
			}
			cookie, err := r.Cookie(CookieName)
			if err != nil {
				http.Error(w, `{"title":"Unauthorized","status":401}`, http.StatusUnauthorized)
				return
			}
			user, err := ValidateSession(db, cookie.Value)
			if err != nil {
				ClearSessionCookie(w)
				http.Error(w, `{"title":"Unauthorized","status":401}`, http.StatusUnauthorized)
				return
			}
			ctx := context.WithValue(r.Context(), userCtxKey, user)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// IsSuperAdmin vrátí true pokud je v kontextu přihlášen superadmin (nebo je single-user mód).
func IsSuperAdmin(ctx context.Context, multiUser *atomic.Bool) bool {
	if !multiUser.Load() {
		return true
	}
	u := UserFromCtx(ctx)
	return u != nil && u.Role == RoleSuperAdmin
}

// SetSessionCookie zapíše HttpOnly session cookie do response.
func SetSessionCookie(w http.ResponseWriter, token string, ttl time.Duration) {
	http.SetCookie(w, &http.Cookie{
		Name:     CookieName,
		Value:    token,
		Path:     "/",
		MaxAge:   int(ttl.Seconds()),
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
}

// ClearSessionCookie vymaže session cookie.
func ClearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     CookieName,
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
}
