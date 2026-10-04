package api

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"log/slog"
	"mime"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"time"
	"unicode"

	"github.com/danielgtaylor/huma/v2"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/qwerin/nanofaktura/internal/auth"
	"github.com/qwerin/nanofaktura/internal/httpsec"
	"github.com/qwerin/nanofaktura/internal/model"
)

// OAuth 2.1 authorization server for MCP clients (SPEC §7.17, MCP
// authorization): metadata (RFC 9728, RFC 8414), dynamic client registration
// (RFC 7591), authorization code + PKCE S256 with consent in the SPA
// (/oauth/authorize), rotating refresh tokens. Access tokens are API tokens
// (nf_…) bound to an OAuthGrant, so the MCP endpoint and the API accept them
// like any API token with the user's rights.
//
// The protocol endpoints (metadata, register, token) are plain handlers with
// RFC-defined bodies ({error, error_description}, form-encoded token
// requests) and bypass the CSRF middleware (no cookies are read there). The
// consent endpoints for the SPA are ordinary huma operations.

const (
	oauthScope         = "nanofaktura"
	oauthCodeTTL       = 10 * time.Minute
	oauthAccessTTL     = time.Hour
	oauthRefreshTTL    = 90 * 24 * time.Hour
	oauthUnusedClient  = 7 * 24 * time.Hour // registered clients never used are removed after this
	oauthMaxBody       = 64 << 10
	oauthAuthorizePage = "/oauth/authorize" // SPA route showing the consent
)

// oauthIssuer is the issuer and base URL of every OAuth endpoint.
func (s *server) oauthIssuer() string { return strings.TrimRight(s.publicURL(), "/") }

// mcpResource is the canonical URL of the MCP endpoint (RFC 8707 resource).
func (s *server) mcpResource() string { return s.oauthIssuer() + mcpPath }

// oauthHandler serves the protocol endpoints that bypass CSRF; nil = not ours.
func (s *server) oauthHandler(path string) http.HandlerFunc {
	switch path {
	case "/.well-known/oauth-protected-resource", "/.well-known/oauth-protected-resource" + mcpPath:
		return s.oauthProtectedResource
	case "/.well-known/oauth-authorization-server", "/.well-known/oauth-authorization-server" + mcpPath:
		return s.oauthServerMetadata
	case "/api/oauth/register":
		return s.oauthRegister
	case "/api/oauth/token":
		return s.oauthToken
	}
	return nil
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// oauthErr writes an OAuth error response (RFC 6749 §5.2).
func oauthErr(w http.ResponseWriter, status int, code, desc string) {
	if status == http.StatusUnauthorized {
		w.Header().Set("WWW-Authenticate", `Basic realm="nanofaktura"`)
	}
	writeJSON(w, status, map[string]string{"error": code, "error_description": desc})
}

func onlyGet(w http.ResponseWriter, r *http.Request) bool {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return false
	}
	return true
}

func onlyPost(w http.ResponseWriter, r *http.Request) bool {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", "POST")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return false
	}
	return true
}

// ---- metadata ----

func (s *server) oauthProtectedResource(w http.ResponseWriter, r *http.Request) {
	if !onlyGet(w, r) {
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"resource":                 s.mcpResource(),
		"resource_name":            "NanoFaktura",
		"authorization_servers":    []string{s.oauthIssuer()},
		"bearer_methods_supported": []string{"header"},
		"scopes_supported":         []string{oauthScope},
	})
}

func (s *server) oauthServerMetadata(w http.ResponseWriter, r *http.Request) {
	if !onlyGet(w, r) {
		return
	}
	iss := s.oauthIssuer()
	writeJSON(w, http.StatusOK, map[string]any{
		"issuer":                                         iss,
		"authorization_endpoint":                         iss + oauthAuthorizePage,
		"token_endpoint":                                 iss + "/api/oauth/token",
		"registration_endpoint":                          iss + "/api/oauth/register",
		"response_types_supported":                       []string{"code"},
		"grant_types_supported":                          []string{"authorization_code", "refresh_token"},
		"code_challenge_methods_supported":               []string{"S256"},
		"token_endpoint_auth_methods_supported":          []string{"none", "client_secret_post", "client_secret_basic"},
		"scopes_supported":                               []string{oauthScope},
		"authorization_response_iss_parameter_supported": true,
	})
}

// ---- dynamic client registration (RFC 7591) ----

type oauthRegistration struct {
	RedirectURIs            []string `json:"redirect_uris"`
	ClientName              string   `json:"client_name"`
	TokenEndpointAuthMethod string   `json:"token_endpoint_auth_method"`
	GrantTypes              []string `json:"grant_types"`
	ResponseTypes           []string `json:"response_types"`
}

func (s *server) oauthRegister(w http.ResponseWriter, r *http.Request) {
	if !onlyPost(w, r) {
		return
	}
	if err := s.rateLimit(s.limits.oauthRegister, clientIP(r.Context())); err != nil {
		w.Header().Set("Retry-After", retryAfter(err))
		oauthErr(w, http.StatusTooManyRequests, "invalid_request", "too many registrations; try again later")
		return
	}
	var in oauthRegistration
	r.Body = http.MaxBytesReader(w, r.Body, oauthMaxBody)
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		oauthErr(w, http.StatusBadRequest, "invalid_client_metadata", "the body must be a JSON object")
		return
	}
	if len(in.RedirectURIs) == 0 || len(in.RedirectURIs) > 10 {
		oauthErr(w, http.StatusBadRequest, "invalid_redirect_uri", "1 to 10 redirect_uris are required")
		return
	}
	for _, u := range in.RedirectURIs {
		if !validRedirectURI(u) {
			oauthErr(w, http.StatusBadRequest, "invalid_redirect_uri",
				"redirect URIs must be https, http on a loopback address, or a private app scheme: "+u)
			return
		}
	}
	for _, g := range in.GrantTypes {
		if g != "authorization_code" && g != "refresh_token" {
			oauthErr(w, http.StatusBadRequest, "invalid_client_metadata", "unsupported grant type "+g)
			return
		}
	}
	for _, t := range in.ResponseTypes {
		if t != "code" {
			oauthErr(w, http.StatusBadRequest, "invalid_client_metadata", "unsupported response type "+t)
			return
		}
	}
	method := defaultStr(in.TokenEndpointAuthMethod, "none")
	if !slices.Contains([]string{"none", "client_secret_post", "client_secret_basic"}, method) {
		oauthErr(w, http.StatusBadRequest, "invalid_client_metadata", "unsupported token_endpoint_auth_method "+method)
		return
	}
	name := cleanClientName(in.ClientName)
	if name == "" {
		name = "Aplikace bez názvu"
	}
	clientID, _ := auth.NewSecret()
	c := model.OAuthClient{ClientID: "nfc_" + clientID[:32], Name: name, RedirectURIs: in.RedirectURIs, CreatedAt: s.deps.Now()}
	out := map[string]any{
		"client_id": c.ClientID, "client_id_issued_at": s.deps.Now().Unix(), "client_name": name,
		"redirect_uris": in.RedirectURIs, "grant_types": []string{"authorization_code", "refresh_token"},
		"response_types": []string{"code"}, "token_endpoint_auth_method": method,
	}
	if method != "none" {
		secretPlain, hash := auth.NewSecret()
		c.SecretHash = hash
		out["client_secret"], out["client_secret_expires_at"] = secretPlain, 0
	}
	if err := s.db.WithContext(r.Context()).Create(&c).Error; err != nil {
		slog.ErrorContext(r.Context(), "oauth register", "err", err)
		oauthErr(w, http.StatusInternalServerError, "server_error", "registration failed")
		return
	}
	slog.InfoContext(r.Context(), "oauth client registered", "client_id", c.ClientID, "name", name)
	writeJSON(w, http.StatusCreated, out)
}

// cleanClientName keeps a printable, bounded client name (it is shown on the
// consent page, always next to the redirect host).
func cleanClientName(s string) string {
	s = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || unicode.In(r, unicode.Cf) {
			return -1
		}
		return r
	}, strings.TrimSpace(s))
	if r := []rune(s); len(r) > 100 {
		s = string(r[:100])
	}
	return s
}

var badSchemes = []string{"javascript", "data", "file", "vbscript", "about", "blob", "ftp", "ws", "wss", "http", "https"}

// validRedirectURI: https, http on a loopback host (native apps, RFC 8252
// §7.3), or a private-use app scheme; never fragments or credentials.
func validRedirectURI(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || u.Fragment != "" || u.User != nil || u.Scheme == "" || len(raw) > 2000 {
		return false
	}
	switch u.Scheme {
	case "https":
		return u.Host != ""
	case "http":
		return isLoopback(u.Hostname())
	}
	return !slices.Contains(badSchemes, strings.ToLower(u.Scheme))
}

func isLoopback(host string) bool {
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// redirectAllowed: an exact match of a registered URI, or for loopback URIs
// the same URI on any port (RFC 8252 §7.3).
func redirectAllowed(registered []string, got string) bool {
	if slices.Contains(registered, got) {
		return true
	}
	g, err := url.Parse(got)
	if err != nil || g.Scheme != "http" || !isLoopback(g.Hostname()) {
		return false
	}
	for _, r := range registered {
		u, err := url.Parse(r)
		if err == nil && u.Scheme == "http" && u.Hostname() == g.Hostname() && u.Path == g.Path && u.RawQuery == g.RawQuery {
			return true
		}
	}
	return false
}

// ---- consent (SPA, session only) ----

// OAuthAuthorizeParams are the authorization request parameters the SPA
// received on /oauth/authorize and passes on.
type OAuthAuthorizeParams struct {
	ClientID            string `json:"client_id" query:"client_id" maxLength:"200"`
	RedirectURI         string `json:"redirect_uri" query:"redirect_uri" maxLength:"2000"`
	ResponseType        string `json:"response_type" query:"response_type" maxLength:"50"`
	State               string `json:"state,omitempty" query:"state" maxLength:"2000"`
	CodeChallenge       string `json:"code_challenge" query:"code_challenge" maxLength:"200"`
	CodeChallengeMethod string `json:"code_challenge_method" query:"code_challenge_method" maxLength:"20"`
	Scope               string `json:"scope,omitempty" query:"scope" maxLength:"500"`
	Resource            string `json:"resource,omitempty" query:"resource" maxLength:"2000"`
}

// OAuthConsent describes the application asking for access.
type OAuthConsent struct {
	ClientName   string `json:"client_name" doc:"Name the client registered itself with (not verified)"`
	RedirectHost string `json:"redirect_host" doc:"Where the user returns: host of the redirect URI, or its app scheme"`
	Resource     string `json:"resource"`
}

// OAuthDecision is the user's answer on the consent page.
type OAuthDecision struct {
	OAuthAuthorizeParams
	Approve bool `json:"approve"`
}

// OAuthRedirect is where the SPA sends the browser next.
type OAuthRedirect struct {
	RedirectTo string `json:"redirect_to"`
}

// OAuthGrantOut is a connected application of the current user.
type OAuthGrantOut struct {
	ID         uint       `json:"id"`
	ClientName string     `json:"client_name"`
	CreatedAt  time.Time  `json:"created_at"`
	LastUsedAt *time.Time `json:"last_used_at,omitempty"`
}

func (s *server) registerOAuth(authed huma.API) {
	huma.Get(authed, "/api/oauth/authorize", s.oauthConsentInfo)
	huma.Post(authed, "/api/oauth/authorize", s.oauthDecide)
	huma.Get(authed, "/api/auth/oauth-grants", s.listOAuthGrants)
	huma.Delete(authed, "/api/auth/oauth-grants/{id}", s.deleteOAuthGrant, status(http.StatusNoContent))
}

var pkceChallenge = regexp.MustCompile(`^[A-Za-z0-9._~-]{43,128}$`)

// checkAuthorize validates an authorization request against the client.
func (s *server) checkAuthorize(ctx context.Context, p *OAuthAuthorizeParams) (*model.OAuthClient, error) {
	var c model.OAuthClient
	if err := s.db.WithContext(ctx).Where("client_id = ?", p.ClientID).First(&c).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apiError(http.StatusUnprocessableEntity, CodeOAuthInvalidClient, "unknown client_id; the application must register again")
		}
		return nil, dbErr(err, "client")
	}
	if !redirectAllowed(c.RedirectURIs, p.RedirectURI) {
		return nil, apiError(http.StatusUnprocessableEntity, CodeOAuthInvalidRedirect, "redirect_uri is not registered for this client")
	}
	switch {
	case p.ResponseType != "code":
		return nil, invalid("response_type", "only response_type=code is supported")
	case p.CodeChallengeMethod != "S256":
		return nil, invalid("code_challenge_method", "PKCE with code_challenge_method=S256 is required")
	case !pkceChallenge.MatchString(p.CodeChallenge):
		return nil, invalid("code_challenge", "invalid PKCE code_challenge")
	case p.Resource != "" && !s.isMCPResource(p.Resource):
		return nil, invalid("resource", "unknown resource; use "+s.mcpResource())
	}
	return &c, nil
}

// isMCPResource compares a requested resource with ours (trailing slash and
// letter case of scheme/host do not matter).
func (s *server) isMCPResource(res string) bool {
	norm := func(v string) string {
		u, err := url.Parse(strings.TrimRight(v, "/"))
		if err != nil {
			return ""
		}
		return strings.ToLower(u.Scheme) + "://" + strings.ToLower(u.Host) + u.Path
	}
	return norm(res) != "" && norm(res) == norm(s.mcpResource())
}

func redirectHost(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	if u.Host == "" {
		return u.Scheme + ":"
	}
	return u.Host
}

func (s *server) oauthConsentInfo(ctx context.Context, in *struct{ OAuthAuthorizeParams }) (*Out[OAuthConsent], error) {
	c, err := s.checkAuthorize(ctx, &in.OAuthAuthorizeParams)
	if err != nil {
		return nil, err
	}
	return &Out[OAuthConsent]{Body: OAuthConsent{ClientName: c.Name, RedirectHost: redirectHost(in.RedirectURI), Resource: s.mcpResource()}}, nil
}

// oauthDecide records the consent: approve → a one-time code, otherwise
// access_denied; both as the redirect back to the client.
func (s *server) oauthDecide(ctx context.Context, in *struct {
	Authorization string `header:"Authorization"`
	Body          OAuthDecision
}) (*Out[OAuthRedirect], error) {
	if in.Authorization != "" {
		// a token must not mint further grants; consent needs a browser session
		return nil, apiError(http.StatusForbidden, CodeOAuthSessionRequired, "authorize applications in the web app (session login), not with an API token")
	}
	p := &in.Body.OAuthAuthorizeParams
	c, err := s.checkAuthorize(ctx, p)
	if err != nil {
		return nil, err
	}
	u, _ := url.Parse(p.RedirectURI) // validated at registration
	q := u.Query()
	if p.State != "" {
		q.Set("state", p.State)
	}
	q.Set("iss", s.oauthIssuer())
	user := auth.UserFrom(ctx)
	if !in.Body.Approve {
		q.Set("error", "access_denied")
		u.RawQuery = q.Encode()
		return &Out[OAuthRedirect]{Body: OAuthRedirect{RedirectTo: u.String()}}, nil
	}
	plain, hash := auth.NewSecret()
	code := model.OAuthCode{CodeHash: hash, ClientID: c.ClientID, UserID: user.ID, RedirectURI: p.RedirectURI,
		CodeChallenge: p.CodeChallenge, Resource: p.Resource, ExpiresAt: s.deps.Now().Add(oauthCodeTTL), CreatedAt: s.deps.Now()}
	if err := s.db.WithContext(ctx).Create(&code).Error; err != nil {
		return nil, dbErr(err, "code")
	}
	slog.InfoContext(ctx, "oauth consent", "user_id", user.ID, "client_id", c.ClientID, "client", c.Name)
	q.Set("code", plain)
	u.RawQuery = q.Encode()
	return &Out[OAuthRedirect]{Body: OAuthRedirect{RedirectTo: u.String()}}, nil
}

func (s *server) listOAuthGrants(ctx context.Context, _ *struct{}) (*Out[ListResponse[OAuthGrantOut]], error) {
	db := s.db.WithContext(ctx)
	uid := auth.UserFrom(ctx).ID
	var gs []model.OAuthGrant
	if err := db.Where("user_id = ?", uid).Order("id DESC").Find(&gs).Error; err != nil {
		return nil, dbErr(err, "grants")
	}
	var toks []model.APIToken // the live access tokens (expired ones are cleaned up)
	if err := db.Select("oauth_grant_id", "last_used_at").
		Where("user_id = ? AND oauth_grant_id IS NOT NULL AND last_used_at IS NOT NULL", uid).Find(&toks).Error; err != nil {
		return nil, dbErr(err, "grants")
	}
	last := map[uint]*time.Time{}
	for _, t := range toks {
		if prev := last[*t.OAuthGrantID]; prev == nil || t.LastUsedAt.After(*prev) {
			last[*t.OAuthGrantID] = t.LastUsedAt
		}
	}
	items := make([]OAuthGrantOut, len(gs))
	for i, g := range gs {
		items[i] = OAuthGrantOut{ID: g.ID, ClientName: g.ClientName, CreatedAt: g.CreatedAt, LastUsedAt: last[g.ID]}
	}
	return &Out[ListResponse[OAuthGrantOut]]{Body: ListResponse[OAuthGrantOut]{Items: items, Page: 1, PerPage: len(items), Total: int64(len(items))}}, nil
}

func (s *server) deleteOAuthGrant(ctx context.Context, in *struct {
	ID uint `path:"id"`
}) (*NoContent, error) {
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		res := tx.Where("id = ? AND user_id = ?", in.ID, auth.UserFrom(ctx).ID).Delete(&model.OAuthGrant{})
		if res.Error != nil {
			return dbErr(res.Error, "grant")
		}
		if res.RowsAffected == 0 {
			return notFound("grant")
		}
		return dbErrOrNil(tx.Where("oauth_grant_id = ?", in.ID).Delete(&model.APIToken{}).Error, "grant")
	})
	if err != nil {
		return nil, err
	}
	return &NoContent{}, nil
}

// deleteUserGrants disconnects every OAuth application of a user (password change).
func deleteUserGrants(tx *gorm.DB, userID uint) error {
	return tx.Where("user_id = ?", userID).Delete(&model.OAuthGrant{}).Error
}

// ---- token endpoint ----

func (s *server) oauthToken(w http.ResponseWriter, r *http.Request) {
	if !onlyPost(w, r) {
		return
	}
	ctx := r.Context()
	ip := clientIP(ctx)
	if err := s.rateBlocked(s.limits.oauthToken, ip); err != nil {
		w.Header().Set("Retry-After", retryAfter(err))
		oauthErr(w, http.StatusTooManyRequests, "invalid_request", "too many failed requests; try again later")
		return
	}
	fail := func(status int, code, desc string) {
		s.rateFail(s.limits.oauthToken, ip)
		oauthErr(w, status, code, desc)
	}
	if mt, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type")); mt != "application/x-www-form-urlencoded" {
		fail(http.StatusBadRequest, "invalid_request", "Content-Type must be application/x-www-form-urlencoded")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, oauthMaxBody)
	if err := r.ParseForm(); err != nil {
		fail(http.StatusBadRequest, "invalid_request", "invalid form body")
		return
	}
	f := r.PostForm
	clientID, secretPlain, basic := r.BasicAuth()
	if basic {
		clientID, _ = url.QueryUnescape(clientID)
		secretPlain, _ = url.QueryUnescape(secretPlain)
	} else {
		clientID, secretPlain = f.Get("client_id"), f.Get("client_secret")
	}
	var c model.OAuthClient
	if clientID == "" || s.db.WithContext(ctx).Where("client_id = ?", clientID).First(&c).Error != nil {
		fail(http.StatusUnauthorized, "invalid_client", "unknown client")
		return
	}
	if c.SecretHash != "" && subtle.ConstantTimeCompare([]byte(auth.HashSecret(secretPlain)), []byte(c.SecretHash)) != 1 {
		fail(http.StatusUnauthorized, "invalid_client", "invalid client secret")
		return
	}
	var (
		res   map[string]any
		gerr  *oauthGrantErr
		err   error
		now   = s.deps.Now()
		grant = f.Get("grant_type")
	)
	switch grant {
	case "authorization_code":
		res, gerr, err = s.exchangeCode(ctx, &c, f, now)
	case "refresh_token":
		res, gerr, err = s.refreshGrant(ctx, &c, f, now)
	default:
		fail(http.StatusBadRequest, "unsupported_grant_type", "grant_type must be authorization_code or refresh_token")
		return
	}
	switch {
	case err != nil:
		slog.ErrorContext(ctx, "oauth token", "err", err)
		oauthErr(w, http.StatusInternalServerError, "server_error", "token request failed")
	case gerr != nil:
		fail(http.StatusBadRequest, gerr.code, gerr.desc)
	default:
		writeJSON(w, http.StatusOK, res)
	}
}

type oauthGrantErr struct{ code, desc string }

func grantErr(code, desc string) *oauthGrantErr { return &oauthGrantErr{code, desc} }

// pkceS256 is BASE64URL(SHA256(verifier)) (RFC 7636).
func pkceS256(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

var pkceVerifier = regexp.MustCompile(`^[A-Za-z0-9._~-]{43,128}$`)

func (s *server) exchangeCode(ctx context.Context, c *model.OAuthClient, f url.Values, now time.Time) (map[string]any, *oauthGrantErr, error) {
	var out map[string]any
	var gerr *oauthGrantErr
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var code model.OAuthCode
		err := tx.Clauses(clause.Locking{Strength: clause.LockingStrengthUpdate}).
			Where("code_hash = ?", auth.HashSecret(f.Get("code"))).First(&code).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			gerr = grantErr("invalid_grant", "invalid or already used authorization code")
			return nil
		}
		if err != nil {
			return err
		}
		// single use: the code goes whatever the outcome
		if err := tx.Delete(&code).Error; err != nil {
			return err
		}
		verifier := f.Get("code_verifier")
		res := f.Get("resource")
		switch {
		case !code.ExpiresAt.After(now):
			gerr = grantErr("invalid_grant", "the authorization code expired")
		case code.ClientID != c.ClientID:
			gerr = grantErr("invalid_grant", "the code was issued to another client")
		case f.Get("redirect_uri") != code.RedirectURI:
			gerr = grantErr("invalid_grant", "redirect_uri does not match the authorization request")
		case !pkceVerifier.MatchString(verifier) ||
			subtle.ConstantTimeCompare([]byte(pkceS256(verifier)), []byte(code.CodeChallenge)) != 1:
			gerr = grantErr("invalid_grant", "PKCE verification failed")
		case res != "" && !s.isMCPResource(res):
			gerr = grantErr("invalid_target", "unknown resource")
		}
		if gerr != nil {
			return nil
		}
		refresh, refreshHash := auth.NewSecret()
		g := model.OAuthGrant{UserID: code.UserID, ClientID: c.ClientID, ClientName: c.Name, RefreshHash: refreshHash,
			Resource: defaultStr(res, code.Resource), ExpiresAt: now.Add(oauthRefreshTTL), CreatedAt: now, UpdatedAt: now}
		if err := tx.Create(&g).Error; err != nil {
			return err
		}
		access, err := issueAccessToken(tx, &g, now)
		if err != nil {
			return err
		}
		out = tokenResponse(access, refresh)
		return nil
	})
	return out, gerr, err
}

func (s *server) refreshGrant(ctx context.Context, c *model.OAuthClient, f url.Values, now time.Time) (map[string]any, *oauthGrantErr, error) {
	var out map[string]any
	var gerr *oauthGrantErr
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var g model.OAuthGrant
		err := tx.Clauses(clause.Locking{Strength: clause.LockingStrengthUpdate}).
			Where("refresh_hash = ?", auth.HashSecret(f.Get("refresh_token"))).First(&g).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			gerr = grantErr("invalid_grant", "invalid refresh token")
			return nil
		}
		if err != nil {
			return err
		}
		if g.ClientID != c.ClientID || !g.ExpiresAt.After(now) {
			gerr = grantErr("invalid_grant", "invalid or expired refresh token")
			return nil
		}
		// rotation: the presented refresh token is spent
		refresh, refreshHash := auth.NewSecret()
		if err := tx.Model(&g).Updates(map[string]any{"refresh_hash": refreshHash, "expires_at": now.Add(oauthRefreshTTL)}).Error; err != nil {
			return err
		}
		access, err := issueAccessToken(tx, &g, now)
		if err != nil {
			return err
		}
		out = tokenResponse(access, refresh)
		return nil
	})
	return out, gerr, err
}

// issueAccessToken stores a short-lived API token of grant g.
func issueAccessToken(tx *gorm.DB, g *model.OAuthGrant, now time.Time) (string, error) {
	exp := now.Add(oauthAccessTTL)
	plain, tok := auth.NewAPIToken(g.UserID, "OAuth: "+g.ClientName, &exp)
	tok.OAuthGrantID, tok.CreatedAt = &g.ID, now
	return plain, tx.Create(tok).Error
}

func tokenResponse(access, refresh string) map[string]any {
	return map[string]any{
		"access_token": access, "token_type": "Bearer", "expires_in": int(oauthAccessTTL.Seconds()),
		"refresh_token": refresh, "scope": oauthScope,
	}
}

// cleanupOAuth removes expired codes and grants (with their tokens) and
// registered clients that were never used (auth-cleanup job).
func cleanupOAuth(db *gorm.DB, now time.Time) error {
	if err := db.Where("expires_at <= ?", now).Delete(&model.OAuthCode{}).Error; err != nil {
		return err
	}
	var expired []uint
	if err := db.Model(&model.OAuthGrant{}).Where("expires_at <= ?", now).Pluck("id", &expired).Error; err != nil {
		return err
	}
	if len(expired) > 0 {
		if err := db.Where("oauth_grant_id IN ?", expired).Delete(&model.APIToken{}).Error; err != nil {
			return err
		}
		if err := db.Where("id IN ?", expired).Delete(&model.OAuthGrant{}).Error; err != nil {
			return err
		}
	}
	return db.Where("created_at <= ? AND client_id NOT IN (?) AND client_id NOT IN (?)", now.Add(-oauthUnusedClient),
		db.Model(&model.OAuthGrant{}).Select("client_id"), db.Model(&model.OAuthCode{}).Select("client_id")).
		Delete(&model.OAuthClient{}).Error
}

// withOAuth routes the OAuth protocol endpoints around the CSRF middleware
// (they read no cookies; the token endpoint is form-encoded by RFC).
func (s *server) withOAuth(next http.Handler, opts httpsec.Options) http.Handler {
	sec := httpsec.Middleware(opts)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if h := s.oauthHandler(r.URL.Path); h != nil {
			sec(h).ServeHTTP(w, r)
			return
		}
		next.ServeHTTP(w, r)
	})
}
