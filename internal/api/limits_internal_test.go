package api

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/qwerin/nanofaktura/internal/auth"
	"github.com/qwerin/nanofaktura/internal/config"
	"github.com/qwerin/nanofaktura/internal/model"
)

// Heavy exports: one at a time per account, two per instance.
func TestAcquireExport(t *testing.T) {
	s := newServer(nil, config.Config{DisableRateLimit: true}, Deps{})
	acct := func(id uint) context.Context {
		return auth.WithAccount(context.Background(), &model.Account{ID: id}, model.RoleOwner)
	}
	busy := func(err error) {
		t.Helper()
		var em *ErrorModel
		if !errors.As(err, &em) || em.Status != http.StatusTooManyRequests || em.Code != CodeExportBusy {
			t.Fatalf("want 429 %s, got %v", CodeExportBusy, err)
		}
	}
	rel1, err := s.acquireExport(acct(1))
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.acquireExport(acct(1))
	busy(err) // same account
	rel2, err := s.acquireExport(acct(2))
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.acquireExport(acct(3))
	busy(err) // instance slots taken
	rel1()
	rel1() // idempotent
	rel3, err := s.acquireExport(acct(3))
	if err != nil {
		t.Fatal(err)
	}
	rel2()
	rel3()
	if rel, err := s.acquireExport(acct(1)); err != nil {
		t.Fatal(err)
	} else {
		rel()
	}
}
