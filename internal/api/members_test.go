package api_test

import (
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/qwerin/nanofaktura/internal/api"
	"github.com/qwerin/nanofaktura/internal/config"
	"github.com/qwerin/nanofaktura/internal/model"
)

func userID(ts *testServer, email string) uint {
	ts.t.Helper()
	var u model.User
	if err := ts.db.Where("email = ?", email).First(&u).Error; err != nil {
		ts.t.Fatal(err)
	}
	return u.ID
}

func memberURL(c *client, id uint) string { return fmt.Sprintf("%s/%d", c.acct("/members"), id) }

func roles(l api.ListResponse[api.Member]) map[string]string {
	out := map[string]string{}
	for _, m := range l.Items {
		out[m.Email] = m.Role
	}
	return out
}

func TestMembers(t *testing.T) {
	ts := newTestServer(t)
	owner := ts.signup("owner@example.cz", "Firma")
	admin := ts.memberOf(owner, "admin@example.cz", "admin")
	member := ts.memberOf(owner, "member@example.cz", "member")
	acct := ts.memberOf(owner, "acc@example.cz", "accountant")
	other := ts.signup("other@example.cz", "Jiná")

	list := doJSON[api.ListResponse[api.Member]](member, http.StatusOK, "GET", member.acct("/members"), nil)
	if list.Total != 4 || fmt.Sprint(roles(list)) != "map[acc@example.cz:accountant admin@example.cz:admin member@example.cz:member owner@example.cz:owner]" {
		t.Fatalf("members: %+v", list)
	}
	ownerID, adminID, memberID, accID := userID(ts, "owner@example.cz"), userID(ts, "admin@example.cz"), userID(ts, "member@example.cz"), userID(ts, "acc@example.cz")

	// admin manages non-owners
	m := doJSON[api.Member](admin, http.StatusOK, "PATCH", memberURL(admin, memberID), api.MemberPatch{Role: "accountant"})
	if m.Role != "accountant" || m.Email != "member@example.cz" {
		t.Fatalf("patched: %+v", m)
	}
	doJSON[api.Member](admin, http.StatusOK, "PATCH", memberURL(admin, memberID), api.MemberPatch{Role: "member"})
	// … but not owners
	res, body := admin.do("PATCH", memberURL(admin, ownerID), api.MemberPatch{Role: "member"})
	assertError(t, res, body, http.StatusForbidden, "only an owner can manage owners")
	res, body = admin.do("PATCH", memberURL(admin, memberID), api.MemberPatch{Role: "owner"})
	assertError(t, res, body, http.StatusForbidden, "only an owner can manage owners")
	res, body = admin.do("DELETE", memberURL(admin, ownerID), nil)
	assertError(t, res, body, http.StatusForbidden, "only an owner can manage owners")

	// the last owner cannot be demoted or leave
	res, body = owner.do("PATCH", memberURL(owner, ownerID), api.MemberPatch{Role: "admin"})
	assertError(t, res, body, http.StatusConflict, "at least one owner")
	res, body = owner.do("DELETE", memberURL(owner, ownerID), nil)
	assertError(t, res, body, http.StatusConflict, "at least one owner")
	// with a second owner it can
	doJSON[api.Member](owner, http.StatusOK, "PATCH", memberURL(owner, adminID), api.MemberPatch{Role: "owner"})
	doJSON[api.Member](owner, http.StatusOK, "PATCH", memberURL(owner, ownerID), api.MemberPatch{Role: "admin"})

	// invalid role, unknown / foreign user
	res, body = admin.do("PATCH", memberURL(admin, memberID), map[string]any{"role": "king"})
	assertError(t, res, body, http.StatusUnprocessableEntity, "role")
	res, body = admin.do("PATCH", memberURL(admin, userID(ts, "other@example.cz")), api.MemberPatch{Role: "member"})
	assertError(t, res, body, http.StatusNotFound, "member not found")

	// a non-manager cannot remove others but can leave
	res, body = member.do("DELETE", memberURL(member, accID), nil)
	assertError(t, res, body, http.StatusForbidden, "your role (member)")
	member.mustDo(http.StatusNoContent, "DELETE", memberURL(member, memberID), nil)
	res, body = member.do("GET", member.acct(""), nil)
	assertError(t, res, body, http.StatusNotFound, "account not found")
	// a manager removes others
	owner.mustDo(http.StatusNoContent, "DELETE", memberURL(owner, accID), nil)
	res, body = acct.do("GET", acct.acct(""), nil)
	assertError(t, res, body, http.StatusNotFound, "account not found")

	list = doJSON[api.ListResponse[api.Member]](owner, http.StatusOK, "GET", owner.acct("/members"), nil)
	if fmt.Sprint(roles(list)) != "map[admin@example.cz:owner owner@example.cz:admin]" {
		t.Fatalf("members after: %+v", list)
	}
	// other account sees only itself
	list = doJSON[api.ListResponse[api.Member]](other, http.StatusOK, "GET", other.acct("/members"), nil)
	if list.Total != 1 || list.Items[0].Email != "other@example.cz" {
		t.Fatalf("other members: %+v", list)
	}
}

var inviteLink = regexp.MustCompile(`http://localhost:8080/invite/([A-Za-z0-9_-]+)`)

// invite sends an invitation as c and returns it with the token from the e-mail.
func invite(c *client, email, role string) (api.Invitation, string) {
	c.ts.t.Helper()
	inv := doJSON[api.Invitation](c, http.StatusCreated, "POST", c.acct("/members/invite"), api.InvitationCreate{Email: email, Role: role})
	msg, ok := c.ts.mail.Last()
	if !ok {
		c.ts.t.Fatal("no invitation e-mail")
	}
	m := inviteLink.FindStringSubmatch(msg.Text)
	if m == nil || len(msg.To) != 1 || msg.To[0] != strings.ToLower(email) {
		c.ts.t.Fatalf("invitation e-mail: %+v", msg)
	}
	return inv, m[1]
}

func TestInvitations(t *testing.T) {
	ts := newTestServer(t, func(c *config.Config) { c.AllowSignup = false })
	// first user may register even with signup disabled
	owner := ts.signup("owner@example.cz", "Firma")
	anon := ts.anon()

	inv, token := invite(owner, "Nova@Example.cz", "accountant")
	if inv.Email != "nova@example.cz" || inv.Role != "accountant" || !inv.ExpiresAt.Equal(ts.now.Add(7*24*time.Hour)) {
		t.Fatalf("invitation: %+v", inv)
	}
	msg, _ := ts.mail.Last()
	if msg.Subject != "Pozvánka do účtu Firma" || !strings.Contains(msg.Text, "role: účetní") || !strings.Contains(msg.Text, "Test owner@example.cz vás zve") {
		t.Fatalf("mail: %+v", msg)
	}
	pending := doJSON[api.ListResponse[api.Invitation]](owner, http.StatusOK, "GET", owner.acct("/invitations"), nil)
	if pending.Total != 1 || pending.Items[0].ID != inv.ID {
		t.Fatalf("pending: %+v", pending)
	}

	info := doJSON[api.InvitationInfo](anon, http.StatusOK, "GET", "/api/invitations/"+token, nil)
	if info.AccountName != "Firma" || info.Email != "nova@example.cz" || info.Role != "accountant" || info.UserExists {
		t.Fatalf("info: %+v", info)
	}
	res, body := anon.do("GET", "/api/invitations/nonexistent", nil)
	assertError(t, res, body, http.StatusNotFound, "invitation not found")

	// registration without a token is disabled; account_name is required without a token
	res, body = anon.do("POST", "/api/auth/register", api.RegisterRequest{Email: "x@example.cz", Name: "X", Password: testPassword, AccountName: "X"})
	assertError(t, res, body, http.StatusForbidden, "signup is disabled")
	res, body = anon.do("POST", "/api/auth/register", api.RegisterRequest{Email: "x@example.cz", Name: "X", Password: testPassword})
	assertError(t, res, body, http.StatusUnprocessableEntity, "account_name")
	// the token only works for the invited address
	res, body = anon.do("POST", "/api/auth/register", api.RegisterRequest{Email: "x@example.cz", Name: "X", Password: testPassword, InvitationToken: token})
	assertError(t, res, body, http.StatusUnprocessableEntity, "does not match the invitation")

	nova := ts.anon()
	me := doJSON[api.Me](nova, http.StatusCreated, "POST", "/api/auth/register", api.RegisterRequest{
		Email: "NOVA@example.cz", Name: "Nová", Password: testPassword, InvitationToken: token,
	})
	if len(me.Accounts) != 1 || me.Accounts[0].Slug != owner.slug || me.Accounts[0].Role != "accountant" {
		t.Fatalf("me: %+v", me)
	}
	// used token
	res, body = anon.do("GET", "/api/invitations/"+token, nil)
	assertError(t, res, body, http.StatusGone, "expired or was already used")
	res, body = nova.do("POST", "/api/invitations/"+token+"/accept", nil)
	assertError(t, res, body, http.StatusGone, "expired or was already used")
	if p := doJSON[api.ListResponse[api.Invitation]](owner, http.StatusOK, "GET", owner.acct("/invitations"), nil); p.Total != 0 {
		t.Fatalf("pending after accept: %+v", p)
	}
	// already a member → 409 on invite
	res, body = owner.do("POST", owner.acct("/members/invite"), api.InvitationCreate{Email: "nova@example.cz", Role: "member"})
	assertError(t, res, body, http.StatusConflict, "already a member")

}

func TestInvitationAcceptExistingUser(t *testing.T) {
	ts := newTestServer(t)
	owner := ts.signup("owner@example.cz", "Firma")
	b := ts.signup("b@example.cz", "Firma B")
	c := ts.signup("c@example.cz", "Firma C")

	_, token := invite(owner, "b@example.cz", "member")
	info := doJSON[api.InvitationInfo](ts.anon(), http.StatusOK, "GET", "/api/invitations/"+token, nil)
	if !info.UserExists {
		t.Fatalf("info: %+v", info)
	}
	res, body := ts.anon().do("POST", "/api/invitations/"+token+"/accept", nil)
	assertError(t, res, body, http.StatusUnauthorized, "authentication required")
	res, body = c.do("POST", "/api/invitations/"+token+"/accept", nil)
	assertError(t, res, body, http.StatusForbidden, "different e-mail address")
	// registering an existing address with the token → 409
	res, body = ts.anon().do("POST", "/api/auth/register", api.RegisterRequest{Email: "b@example.cz", Name: "B", Password: testPassword, InvitationToken: token})
	assertError(t, res, body, http.StatusConflict, "already registered")

	acc := doJSON[api.MeAccount](b, http.StatusOK, "POST", "/api/invitations/"+token+"/accept", nil)
	if acc.Slug != owner.slug || acc.Role != "member" || acc.Name != "Firma" {
		t.Fatalf("accepted: %+v", acc)
	}
	b.slug = owner.slug
	b.mustDo(http.StatusOK, "GET", b.acct("/subjects"), nil)
	if me := doJSON[api.Me](b, http.StatusOK, "GET", "/api/auth/me", nil); len(me.Accounts) != 2 {
		t.Fatalf("me: %+v", me)
	}
}

func TestInvitationLifecycle(t *testing.T) {
	ts := newTestServer(t)
	owner := ts.signup("owner@example.cz", "Firma")
	admin := ts.memberOf(owner, "admin@example.cz", "admin")
	other := ts.signup("other@example.cz", "Jiná")

	// admin cannot invite owners, owner can
	res, body := admin.do("POST", admin.acct("/members/invite"), api.InvitationCreate{Email: "x@example.cz", Role: "owner"})
	assertError(t, res, body, http.StatusForbidden, "only an owner can manage owners")
	ownerInv, _ := invite(owner, "co-owner@example.cz", "owner")
	res, body = admin.do("DELETE", fmt.Sprintf("%s/%d", admin.acct("/invitations"), ownerInv.ID), nil)
	assertError(t, res, body, http.StatusForbidden, "only an owner can manage owners")

	res, body = admin.do("POST", admin.acct("/members/invite"), map[string]any{"email": "not-an-email", "role": "member"})
	assertError(t, res, body, http.StatusUnprocessableEntity, "email")

	// a new invitation replaces the pending one for the same address
	first, firstToken := invite(admin, "x@example.cz", "member")
	second, secondToken := invite(admin, "X@example.cz", "admin")
	pending := doJSON[api.ListResponse[api.Invitation]](admin, http.StatusOK, "GET", admin.acct("/invitations"), nil)
	if pending.Total != 2 || pending.Items[0].ID != second.ID {
		t.Fatalf("pending: %+v", pending)
	}
	res, body = ts.anon().do("GET", "/api/invitations/"+firstToken, nil)
	assertError(t, res, body, http.StatusNotFound, "invitation not found")
	res, body = admin.do("DELETE", fmt.Sprintf("%s/%d", admin.acct("/invitations"), first.ID), nil)
	assertError(t, res, body, http.StatusNotFound, "invitation not found")

	// isolation: another account cannot see or revoke it
	res, body = other.do("DELETE", fmt.Sprintf("%s/%d", other.acct("/invitations"), second.ID), nil)
	assertError(t, res, body, http.StatusNotFound, "invitation not found")
	if p := doJSON[api.ListResponse[api.Invitation]](other, http.StatusOK, "GET", other.acct("/invitations"), nil); p.Total != 0 {
		t.Fatalf("other pending: %+v", p)
	}

	// revoke
	admin.mustDo(http.StatusNoContent, "DELETE", fmt.Sprintf("%s/%d", admin.acct("/invitations"), second.ID), nil)
	res, body = ts.anon().do("GET", "/api/invitations/"+secondToken, nil)
	assertError(t, res, body, http.StatusNotFound, "invitation not found")

	// expiry after 7 days
	_, token := invite(owner, "late@example.cz", "member")
	ts.now = ts.now.Add(7*24*time.Hour + time.Second)
	res, body = ts.anon().do("GET", "/api/invitations/"+token, nil)
	assertError(t, res, body, http.StatusGone, "expired")
	res, body = ts.anon().do("POST", "/api/auth/register", api.RegisterRequest{Email: "late@example.cz", Name: "L", Password: testPassword, InvitationToken: token})
	assertError(t, res, body, http.StatusGone, "expired")
	if p := doJSON[api.ListResponse[api.Invitation]](owner, http.StatusOK, "GET", owner.acct("/invitations"), nil); p.Total != 0 {
		t.Fatalf("expired invitations must not be pending: %+v", p)
	}

	// mailer failure → 502 and no invitation is kept
	ts.mail.Err = errors.New("smtp down")
	res, body = owner.do("POST", owner.acct("/members/invite"), api.InvitationCreate{Email: "y@example.cz", Role: "member"})
	assertError(t, res, body, http.StatusBadGateway, "invitation e-mail")
	var n int64
	ts.db.Model(&model.Invitation{}).Where("email = ?", "y@example.cz").Count(&n)
	if n != 0 {
		t.Fatalf("invitation kept after mail failure")
	}
}
