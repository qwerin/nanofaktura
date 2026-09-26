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
	ID        uint      `json:"id"`
	Email     string    `json:"email"`
	Role      string    `json:"role" enum:"owner,admin,accountant,member"`
	InvitedBy uint      `json:"invited_by" doc:"User ID of the inviter"`
	ExpiresAt time.Time `json:"expires_at"`
	CreatedAt time.Time `json:"created_at"`
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

	huma.Get(public, "/api/invitations/{token}", s.getInvitationInfo)
	huma.Post(authed, "/api/invitations/{token}/accept", s.acceptInvitation)
}

// canManageRole: only owners may manage owners (change/remove an owner, grant
// the owner role, invite an owner).
func canManageRole(ctx context.Context, targetRole string) error {
	if targetRole == model.RoleOwner && auth.RoleFrom(ctx) != model.RoleOwner {
		return huma.Error403Forbidden("only an owner can manage owners")
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
		return conflict("the account must keep at least one owner")
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
	acc, user := auth.AccountFrom(ctx), auth.UserFrom(ctx)
	email := normalizeEmail(in.Body.Email)
	plain, hash := auth.NewSecret()
	inv := model.Invitation{
		AccountID: acc.ID, Email: email, Role: in.Body.Role, TokenHash: hash,
		InvitedBy: user.ID, ExpiresAt: s.deps.Now().Add(InvitationTTL),
	}
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var n int64
		err := tx.Model(&model.Membership{}).Scopes(inAccount(ctx)).
			Joins("JOIN users ON users.id = memberships.user_id").
			Where("users.email = ?", email).Count(&n).Error
		if err != nil {
			return dbErr(err, "member")
		}
		if n > 0 {
			return conflict("this user is already a member of the account")
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

	vars := map[string]string{
		"inviter": user.Name, "account_name": acc.Name, "role": roleLabels[inv.Role],
		"link": s.publicURL() + "/invite/" + plain, "expires_on": inv.ExpiresAt.Format("2. 1. 2006"),
	}
	msg := mail.Message{To: []string{email}, Subject: mail.Render(invitationSubject, vars), Text: mail.Render(invitationText, vars)}
	if err := s.deps.Mailer.Send(ctx, msg); err != nil {
		_ = s.db.WithContext(ctx).Delete(&inv).Error
		return nil, huma.Error502BadGateway("failed to send the invitation e-mail", err)
	}
	return &Out[Invitation]{Body: toInvitation(&inv)}, nil
}

func (s *server) listInvitations(ctx context.Context, in *struct{ PageParams }) (*Out[ListResponse[Invitation]], error) {
	q := s.scoped(ctx).Where("accepted_at IS NULL AND expires_at > ?", s.deps.Now()).Order("id DESC")
	return paginate(q, in.PageParams, toInvitation)
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
// accepted or expired → 410.
func (s *server) pendingInvitation(tx *gorm.DB, token string) (*model.Invitation, error) {
	var inv model.Invitation
	if err := tx.Where("token_hash = ?", auth.HashSecret(token)).First(&inv).Error; err != nil {
		return nil, dbErr(err, "invitation")
	}
	if inv.AcceptedAt != nil || !inv.ExpiresAt.After(s.deps.Now()) {
		return nil, huma.NewError(http.StatusGone, "invitation has expired or was already used")
	}
	return &inv, nil
}

// joinByInvitation creates the membership and marks inv accepted (inside tx).
func (s *server) joinByInvitation(tx *gorm.DB, inv *model.Invitation, userID uint) error {
	if err := tx.Create(&model.Membership{UserID: userID, AccountID: inv.AccountID, Role: inv.Role}).Error; err != nil {
		if errors.Is(err, gorm.ErrDuplicatedKey) {
			return conflict("you are already a member of this account")
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
	user := auth.UserFrom(ctx)
	var out MeAccount
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		inv, err := s.pendingInvitation(tx, in.Token)
		if err != nil {
			return err
		}
		if inv.Email != normalizeEmail(user.Email) {
			return huma.Error403Forbidden("this invitation was sent to a different e-mail address")
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
