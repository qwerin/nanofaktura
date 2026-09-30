package api_test

import (
	"net/http"
	"testing"

	"github.com/qwerin/nanofaktura/internal/auth"
	"github.com/qwerin/nanofaktura/internal/model"
)

// TestAcceptInvitationAlreadyMember: a pending invitation of someone who has
// meanwhile become a member (e.g. two invitations sent) answers 409
// already_joined and stays pending.
func TestAcceptInvitationAlreadyMember(t *testing.T) {
	ts := newTestServer(t)
	owner := ts.signup("owner@example.cz", "Firma")
	m := ts.memberOf(owner, "clen@example.cz", model.RoleMember)
	var acc model.Account
	ts.db.Where("slug = ?", owner.slug).First(&acc)
	var ou model.User
	ts.db.Where("email = ?", "owner@example.cz").First(&ou)
	plain, hash := auth.NewSecret()
	inv := model.Invitation{AccountID: acc.ID, Email: "clen@example.cz", Role: model.RoleAdmin, TokenHash: hash,
		InvitedBy: ou.ID, ExpiresAt: ts.now.AddDate(0, 0, 7)}
	if err := ts.db.Create(&inv).Error; err != nil {
		t.Fatal(err)
	}
	res, body := m.do("POST", "/api/invitations/"+plain+"/accept", nil)
	assertCode(t, res, body, http.StatusConflict, "already_joined")
	var got model.Invitation
	ts.db.First(&got, inv.ID)
	if got.AcceptedAt != nil {
		t.Fatal("invitation marked accepted despite the conflict")
	}
	var role string
	ts.db.Model(&model.Membership{}).Select("role").Where("account_id = ? AND user_id = (SELECT id FROM users WHERE email = ?)", acc.ID, "clen@example.cz").Scan(&role)
	if role != model.RoleMember {
		t.Fatalf("role changed to %q", role)
	}
}
