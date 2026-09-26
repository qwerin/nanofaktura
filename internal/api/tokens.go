package api

import (
	"context"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"github.com/qwerin/nanofaktura/internal/auth"
	"github.com/qwerin/nanofaktura/internal/model"
)

type APIToken struct {
	ID         uint       `json:"id"`
	Name       string     `json:"name"`
	Prefix     string     `json:"prefix" doc:"First 8 characters of the token, for identification"`
	LastUsedAt *time.Time `json:"last_used_at,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
}

type CreatedAPIToken struct {
	APIToken
	Token string `json:"token" doc:"Plaintext token, returned only once"`
}

type APITokenCreate struct {
	Name string `json:"name" minLength:"1" maxLength:"100"`
}

func toAPIToken(t *model.APIToken) APIToken {
	return APIToken{ID: t.ID, Name: t.Name, Prefix: t.Prefix, LastUsedAt: t.LastUsedAt, CreatedAt: t.CreatedAt}
}

func (s *server) registerTokens(authed huma.API) {
	huma.Get(authed, "/api/auth/tokens", s.listTokens)
	huma.Post(authed, "/api/auth/tokens", s.createToken, status(http.StatusCreated))
	huma.Delete(authed, "/api/auth/tokens/{id}", s.deleteToken, status(http.StatusNoContent))
}

func (s *server) listTokens(ctx context.Context, in *struct{ PageParams }) (*Out[ListResponse[APIToken]], error) {
	q := s.db.WithContext(ctx).Where("user_id = ?", auth.UserFrom(ctx).ID).Order("id DESC")
	return paginate(q, in.PageParams, toAPIToken)
}

func (s *server) createToken(ctx context.Context, in *struct{ Body APITokenCreate }) (*Out[CreatedAPIToken], error) {
	plain, tok, err := s.auth.CreateAPIToken(ctx, auth.UserFrom(ctx).ID, in.Body.Name)
	if err != nil {
		return nil, dbErr(err, "token")
	}
	return &Out[CreatedAPIToken]{Body: CreatedAPIToken{APIToken: toAPIToken(tok), Token: plain}}, nil
}

func (s *server) deleteToken(ctx context.Context, in *struct {
	ID uint `path:"id"`
}) (*NoContent, error) {
	res := s.db.WithContext(ctx).Where("id = ? AND user_id = ?", in.ID, auth.UserFrom(ctx).ID).Delete(&model.APIToken{})
	if res.Error != nil {
		return nil, dbErr(res.Error, "token")
	}
	if res.RowsAffected == 0 {
		return nil, notFound("token")
	}
	return &NoContent{}, nil
}
