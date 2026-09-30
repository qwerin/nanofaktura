package api

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"gorm.io/gorm"

	"github.com/qwerin/nanofaktura/internal/auth"
	"github.com/qwerin/nanofaktura/internal/mail"
	"github.com/qwerin/nanofaktura/internal/model"
)

// InvitationTTL is how long an invitation link stays valid.
const InvitationTTL = 7 * 24 * time.Hour

type Member struct {
	UserID   uint      `json:"user_id"`
	Email    string    `json:"email"`
	Name     string    `json:"name"`
	Role     string    `json:"role" enum:"owner,admin,accountant,member"`
	JoinedAt time.Time `json:"joined_at"`
}

type MemberPatch struct {
	Role string `json:"role" enum:"owner,admin,accountant,member"`
}

type InvitationCreate struct {
	Email string `json:"email" format:"email" maxLength:"254"`
	Role  string `json:"role" enum:"owner,admin,accountant,member"`
}

// Invitation is a pending invitation as seen by account managers.
type Invitation struct {
	ID            uint      `json:"id"`
	Email         string    `json:"email"`
	Role          string    `json:"role" enum:"owner,admin,accountant,member"`
	InvitedBy     uint      `json:"invited_by" doc:"User ID of the inviter"`
	InvitedByName string    `json:"invited_by_name" doc:"Name of the inviter (empty when the user no longer exists)"`
	InviteURL     string    `json:"invite_url" doc:"The invitation link (for copying); empty for invitations created before links were stored and for roles the caller cannot grant (owner invitations for admins)"`
	ExpiresAt     time.Time `json:"expires_at"`
	CreatedAt     time.Time `json:"created_at"`
}

// InvitationInfo is the public view of an invitation (by its link token).
type InvitationInfo struct {
	AccountName string    `json:"account_name"`
	Email       string    `json:"email"`
	Role        string    `json:"role" enum:"owner,admin,accountant,member"`
	ExpiresAt   time.Time `json:"expires_at"`
	UserExists  bool      `json:"user_exists" doc:"The invited e-mail already has a user: log in and accept instead of registering"`
}

func toInvitation(i *model.Invitation) Invitation {
	return Invitation{ID: i.ID, Email: i.Email, Role: i.Role, InvitedBy: i.InvitedBy, ExpiresAt: i.ExpiresAt, CreatedAt: i.CreatedAt}
}

// invitationOut adds the inviter's name and the link (decrypted token). The
// link is a credential granting the invitation's role, so it is only shown to
// callers who may grant that role themselves (admins never see owner links).
func (s *server) invitationOut(ctx context.Context, rows []model.Invitation) ([]Invitation, error) {
	ids := make([]uint, 0, len(rows))
	for _, r := range rows {
		ids = append(ids, r.InvitedBy)
	}
	names := map[uint]string{}
	if len(ids) > 0 {
		var users []model.User
		if err := s.db.WithContext(ctx).Select("id", "name").Where("id IN ?", ids).Find(&users).Error; err != nil {
			return nil, dbErr(err, "users")
		}
		for _, u := range users {
			names[u.ID] = u.Name
		}
	}
	out := make([]Invitation, len(rows))
	for i := range rows {
		out[i] = toInvitation(&rows[i])
		out[i].InvitedByName = names[rows[i].InvitedBy]
		if canManageRole(ctx, rows[i].Role) != nil {
			continue
		}
		if plain, err := s.invitationToken(&rows[i]); err == nil && plain != "" {
			out[i].InviteURL = s.inviteLink(plain)
		}
	}
	return out, nil
}

func (s *server) invitationToken(inv *model.Invitation) (string, error) {
	if inv.TokenEnc == "" {
		return "", nil
	}
	return s.deps.Secrets.Decrypt(inv.TokenEnc)
}

func (s *server) inviteLink(plain string) string { return s.publicURL() + "/invite/" + plain }

// sendInvitation e-mails the invitation link.
func (s *server) sendInvitation(ctx context.Context, inv *model.Invitation, plain, inviter string) error {
	acc := auth.AccountFrom(ctx)
	vars := map[string]string{
		"inviter": inviter, "account_name": acc.Name, "role": roleLabels[inv.Role],
		"link": s.inviteLink(plain), "expires_on": inv.ExpiresAt.Format("2. 1. 2006"),
	}
	msg := mail.Message{To: []string{inv.Email}, Subject: mail.Render(invitationSubject, vars), Text: mail.Render(invitationText, vars)}
	if err := s.deps.Mailer.Send(ctx, msg); err != nil {
		return huma.Error502BadGateway("failed to send the invitation e-mail", err)
	}
	return nil
}

// resendInvitation e-mails the same link again (the link stays valid) and
// extends its validity by InvitationTTL. Invitations without a stored link
// get a new one (the old link stops working).
func (s *server) resendInvitation(ctx context.Context, in *struct {
	ID uint `path:"id"`
}) (*Out[Invitation], error) {
	var inv model.Invitation
	if err := s.scoped(ctx).Where("accepted_at IS NULL").First(&inv, in.ID).Error; err != nil {
		return nil, dbErr(err, "invitation")
	}
	if err := canManageRole(ctx, inv.Role); err != nil {
		return nil, err
	}
	if err := s.rateLimit(s.limits.mail, accountKey(ctx)); err != nil {
		return nil, err
	}
	plain, err := s.invitationToken(&inv)
	if err != nil {
		return nil, huma.Error500InternalServerError("cannot read the invitation link", err)
	}
	if plain == "" {
		var hash string
		plain, hash = auth.NewSecret()
		if inv.TokenEnc, err = s.deps.Secrets.Encrypt(plain); err != nil {
			return nil, huma.Error500InternalServerError("cannot store the invitation link", err)
		}
		inv.TokenHash = hash
	}
	inv.ExpiresAt = s.deps.Now().Add(InvitationTTL)
	if err := s.db.WithContext(ctx).Save(&inv).Error; err != nil {
		return nil, dbErr(err, "invitation")
	}
	if err := s.sendInvitation(ctx, &inv, plain, auth.UserFrom(ctx).Name); err != nil {
		return nil, err
	}
	out, err := s.invitationOut(ctx, []model.Invitation{inv})
	if err != nil {
		return nil, err
	}
	return &Out[Invitation]{Body: out[0]}, nil
}

// roleLabels are the Czech role names used in e-mails.
var roleLabels = map[string]string{
	model.RoleOwner: "vlastník", model.RoleAdmin: "správce", model.RoleAccountant: "účetní", model.RoleMember: "člen",
}

const invitationSubject = "Pozvánka do účtu {account_name}"

const invitationText = `Dobrý den,

{inviter} vás zve do účtu {account_name} v aplikaci NanoFaktura (role: {role}).

Pozvánku přijmete na tomto odkazu:
{link}

Odkaz platí do {expires_on}. Pokud pozvánku nečekáte, e-mail ignorujte.
`

func (s *server) registerMembers(public, authed, account huma.API) {
	huma.Get(account, "/members", s.listMembers)
	huma.Patch(account, "/members/{user_id}", s.patchMember, auth.ForManagers)
	// any member may leave; removing others is checked in the handler
	huma.Delete(account, "/members/{user_id}", s.deleteMember, status(http.StatusNoContent))
	huma.Post(account, "/members/invite", s.inviteMember, status(http.StatusCreated), auth.ForManagers)
	huma.Get(account, "/invitations", s.listInvitations, auth.ForManagers)
	huma.Delete(account, "/invitations/{id}", s.deleteInvitation, status(http.StatusNoContent), auth.ForManagers)
	huma.Post(account, "/invitations/{id}/resend", s.resendInvitation, auth.ForManagers)

	huma.Get(public, "/api/invitations/{token}", s.getInvitationInfo)
	huma.Post(authed, "/api/invitations/{token}/accept", s.acceptInvitation)
}

// canManageRole: only owners may manage owners (change/remove an owner, grant
// the owner role, invite an owner).
func canManageRole(ctx context.Context, targetRole string) error {
	if targetRole == model.RoleOwner && auth.RoleFrom(ctx) != model.RoleOwner {
		return apiError(http.StatusForbidden, CodeOwnerOnly, "only an owner can manage owners")
	}
	return nil
}

type memberRow struct {
	UserID    uint
	Email     string
	Name      string
	Role      string
	CreatedAt time.Time
}

func (s *server) listMembers(ctx context.Context, in *struct{ PageParams }) (*Out[ListResponse[Member]], error) {
	q := s.db.WithContext(ctx).Table("memberships").
		Select("memberships.user_id, users.email, users.name, memberships.role, memberships.created_at").
		Joins("JOIN users ON users.id = memberships.user_id").
		Where("memberships.account_id = ?", auth.AccountFrom(ctx).ID).
		Order("users.email")
	return paginate(q, in.PageParams, func(r *memberRow) Member {
		return Member{UserID: r.UserID, Email: r.Email, Name: r.Name, Role: r.Role, JoinedAt: r.CreatedAt}
	})
}

func (s *server) member(tx *gorm.DB, ctx context.Context, userID uint) (*model.Membership, error) {
	var m model.Membership
	if err := tx.Scopes(inAccount(ctx)).Where("user_id = ?", userID).First(&m).Error; err != nil {
		return nil, dbErr(err, "member")
	}
	return &m, nil
}

// ensureAnotherOwner returns 409 when m is the account's last owner.
func ensureAnotherOwner(tx *gorm.DB, ctx context.Context, m *model.Membership) error {
	if m.Role != model.RoleOwner {
		return nil
	}
	var n int64
	if err := tx.Model(&model.Membership{}).Scopes(inAccount(ctx)).
		Where("role = ? AND id <> ?", model.RoleOwner, m.ID).Count(&n).Error; err != nil {
		return dbErr(err, "member")
	}
	if n == 0 {
		return conflict(CodeLastOwner, "the account must keep at least one owner")
	}
	return nil
}

func (s *server) memberOut(tx *gorm.DB, m *model.Membership) (Member, error) {
	var u model.User
	if err := tx.First(&u, m.UserID).Error; err != nil {
		return Member{}, dbErr(err, "user")
	}
	return Member{UserID: u.ID, Email: u.Email, Name: u.Name, Role: m.Role, JoinedAt: m.CreatedAt}, nil
}

func (s *server) patchMember(ctx context.Context, in *struct {
	UserID uint `path:"user_id"`
	Body   MemberPatch
}) (*Out[Member], error) {
	var out Member
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		m, err := s.member(tx, ctx, in.UserID)
		if err != nil {
			return err
		}
		if err := canManageRole(ctx, m.Role); err != nil {
			return err
		}
		if err := canManageRole(ctx, in.Body.Role); err != nil {
			return err
		}
		if in.Body.Role != model.RoleOwner {
			if err := ensureAnotherOwner(tx, ctx, m); err != nil {
				return err
			}
		}
		m.Role = in.Body.Role
		if err := tx.Model(m).Update("role", m.Role).Error; err != nil {
			return dbErr(err, "member")
		}
		out, err = s.memberOut(tx, m)
		return err
	})
	if err != nil {
		return nil, err
	}
	return &Out[Member]{Body: out}, nil
}

func (s *server) deleteMember(ctx context.Context, in *struct {
	UserID uint `path:"user_id"`
}) (*NoContent, error) {
	self := in.UserID == auth.UserFrom(ctx).ID
	if !self {
		if err := auth.RequireRole(ctx, auth.RolesManagers...); err != nil {
			return nil, err
		}
	}
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		m, err := s.member(tx, ctx, in.UserID)
		if err != nil {
			return err
		}
		if !self {
			if err := canManageRole(ctx, m.Role); err != nil {
				return err
			}
		}
		if err := ensureAnotherOwner(tx, ctx, m); err != nil {
			return err
		}
		return dbErrOrNil(tx.Delete(m).Error, "member")
	})
	if err != nil {
		return nil, err
	}
	return &NoContent{}, nil
}

func (s *server) inviteMember(ctx context.Context, in *struct{ Body InvitationCreate }) (*Out[Invitation], error) {
	if err := canManageRole(ctx, in.Body.Role); err != nil {
		return nil, err
	}
	if err := s.rateLimit(s.limits.mail, accountKey(ctx)); err != nil {
		return nil, err
	}
	acc, user := auth.AccountFrom(ctx), auth.UserFrom(ctx)
	email := normalizeEmail(in.Body.Email)
	plain, hash := auth.NewSecret()
	enc, err := s.deps.Secrets.Encrypt(plain)
	if err != nil {
		return nil, huma.Error500InternalServerError("cannot store the invitation link", err)
	}
	inv := model.Invitation{
		AccountID: acc.ID, Email: email, Role: in.Body.Role, TokenHash: hash, TokenEnc: enc,
		InvitedBy: user.ID, ExpiresAt: s.deps.Now().Add(InvitationTTL),
	}
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var n int64
		err := tx.Model(&model.Membership{}).Scopes(inAccount(ctx)).
			Joins("JOIN users ON users.id = memberships.user_id").
			Where("users.email = ?", email).Count(&n).Error
		if err != nil {
			return dbErr(err, "member")
		}
		if n > 0 {
			return conflict(CodeAlreadyMember, "this user is already a member of the account")
		}
		// a new invitation replaces any unaccepted one for the same address
		if err := tx.Scopes(inAccount(ctx)).Where("email = ? AND accepted_at IS NULL", email).
			Delete(&model.Invitation{}).Error; err != nil {
			return dbErr(err, "invitation")
		}
		return dbErrOrNil(tx.Create(&inv).Error, "invitation")
	})
	if err != nil {
		return nil, err
	}

	if err := s.sendInvitation(ctx, &inv, plain, user.Name); err != nil {
		_ = s.db.WithContext(ctx).Delete(&inv).Error
		return nil, err
	}
	out, err := s.invitationOut(ctx, []model.Invitation{inv})
	if err != nil {
		return nil, err
	}
	return &Out[Invitation]{Body: out[0]}, nil
}

func (s *server) listInvitations(ctx context.Context, in *struct{ PageParams }) (*Out[ListResponse[Invitation]], error) {
	q := s.scoped(ctx).Where("accepted_at IS NULL AND expires_at > ?", s.deps.Now()).Order("id DESC")
	page, err := paginate(q, in.PageParams, func(m *model.Invitation) model.Invitation { return *m })
	if err != nil {
		return nil, err
	}
	items, err := s.invitationOut(ctx, page.Body.Items)
	if err != nil {
		return nil, err
	}
	b := page.Body
	return &Out[ListResponse[Invitation]]{Body: ListResponse[Invitation]{Items: items, Page: b.Page, PerPage: b.PerPage, Total: b.Total}}, nil
}

func (s *server) deleteInvitation(ctx context.Context, in *struct {
	ID uint `path:"id"`
}) (*NoContent, error) {
	var inv model.Invitation
	if err := s.scoped(ctx).Where("accepted_at IS NULL").First(&inv, in.ID).Error; err != nil {
		return nil, dbErr(err, "invitation")
	}
	if err := canManageRole(ctx, inv.Role); err != nil {
		return nil, err
	}
	if err := s.db.WithContext(ctx).Delete(&inv).Error; err != nil {
		return nil, dbErr(err, "invitation")
	}
	return &NoContent{}, nil
}

// pendingInvitation finds the invitation of a link token: unknown → 404,
// accepted or expired → 410, inviter no longer allowed to grant the role →
// 410 invitation_revoked.
func (s *server) pendingInvitation(tx *gorm.DB, token string) (*model.Invitation, error) {
	var inv model.Invitation
	if err := tx.Where("token_hash = ?", auth.HashSecret(token)).First(&inv).Error; err != nil {
		return nil, dbErr(err, "invitation")
	}
	if inv.AcceptedAt != nil || !inv.ExpiresAt.After(s.deps.Now()) {
		return nil, apiError(http.StatusGone, CodeInvitationExpired, "invitation has expired or was already used")
	}
	if err := inviterMayGrant(tx, &inv); err != nil {
		return nil, err
	}
	return &inv, nil
}

// inviterMayGrant re-checks at accept time that the inviter is still a member
// of the account allowed to grant the invited role (owner → only an owner;
// other roles → owner or admin). The invitation of a demoted or removed
// inviter is void (410 invitation_revoked).
func inviterMayGrant(tx *gorm.DB, inv *model.Invitation) error {
	var m model.Membership
	err := tx.Where("account_id = ? AND user_id = ?", inv.AccountID, inv.InvitedBy).First(&m).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return dbErr(err, "invitation")
	}
	ok := err == nil && (m.Role == model.RoleOwner || (m.Role == model.RoleAdmin && inv.Role != model.RoleOwner))
	if !ok {
		return apiError(http.StatusGone, CodeInvitationRevoked, "invitation is no longer valid (the inviter can no longer grant this role)")
	}
	return nil
}

// joinByInvitation creates the membership and marks inv accepted (inside tx).
func (s *server) joinByInvitation(tx *gorm.DB, inv *model.Invitation, userID uint) error {
	if err := tx.Create(&model.Membership{UserID: userID, AccountID: inv.AccountID, Role: inv.Role}).Error; err != nil {
		if errors.Is(err, gorm.ErrDuplicatedKey) {
			return conflict(CodeAlreadyJoined, "you are already a member of this account")
		}
		return dbErr(err, "membership")
	}
	now := s.deps.Now()
	inv.AcceptedAt = &now
	return dbErrOrNil(tx.Model(inv).Update("accepted_at", now).Error, "invitation")
}

func (s *server) getInvitationInfo(ctx context.Context, in *struct {
	Token string `path:"token"`
}) (*Out[InvitationInfo], error) {
	if err := s.rateLimit(s.limits.invitation, clientIP(ctx)); err != nil {
		return nil, err
	}
	db := s.db.WithContext(ctx)
	inv, err := s.pendingInvitation(db, in.Token)
	if err != nil {
		return nil, err
	}
	var acc model.Account
	if err := db.First(&acc, inv.AccountID).Error; err != nil {
		return nil, dbErr(err, "invitation")
	}
	var n int64
	if err := db.Model(&model.User{}).Where("email = ?", inv.Email).Count(&n).Error; err != nil {
		return nil, dbErr(err, "user")
	}
	return &Out[InvitationInfo]{Body: InvitationInfo{
		AccountName: acc.Name, Email: inv.Email, Role: inv.Role, ExpiresAt: inv.ExpiresAt, UserExists: n > 0,
	}}, nil
}

func (s *server) acceptInvitation(ctx context.Context, in *struct {
	Token string `path:"token"`
}) (*Out[MeAccount], error) {
	if err := s.rateLimit(s.limits.invitation, clientIP(ctx)); err != nil {
		return nil, err
	}
	user := auth.UserFrom(ctx)
	var out MeAccount
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		inv, err := s.pendingInvitation(tx, in.Token)
		if err != nil {
			return err
		}
		if inv.Email != normalizeEmail(user.Email) {
			return apiError(http.StatusForbidden, CodeInvitationEmail, "this invitation was sent to a different e-mail address")
		}
		if err := s.joinByInvitation(tx, inv, user.ID); err != nil {
			return err
		}
		var acc model.Account
		if err := tx.First(&acc, inv.AccountID).Error; err != nil {
			return dbErr(err, "account")
		}
		out = MeAccount{Slug: acc.Slug, Name: acc.Name, Role: inv.Role}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &Out[MeAccount]{Body: out}, nil
}

func (s *server) publicURL() string {
	if s.cfg.PublicURL == "" {
		return "http://localhost:8080"
	}
	return s.cfg.PublicURL
}
