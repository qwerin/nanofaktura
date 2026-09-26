package auth

import (
	"context"
	"slices"
	"strings"

	"github.com/danielgtaylor/huma/v2"

	"github.com/qwerin/nanofaktura/internal/model"
)

// Role checks are declarative: an account-scoped operation declares the roles
// allowed to call it with an operation option at registration, and
// RequireAccount enforces it (403 for other roles) before the handler runs:
//
//	huma.Get(g, "/subjects", s.listSubjects)                               // any member
//	huma.Post(g, "/subjects", s.createSubject, auth.ForEditors)            // owner, admin, member
//	huma.Patch(g, "/bank-accounts/{id}", s.patchBank, auth.ForManagers)    // owner, admin
//	huma.Get(g, "/exports/x.csv", s.export, auth.Allow(model.RoleOwner, model.RoleAccountant))
//
// An operation that declares nothing may be called by every member of the
// account, whatever the role. The declaration only has an effect on operations
// registered into the account group (the only place a role exists).

// metaRoles is the huma.Operation.Metadata key holding the allowed roles.
const metaRoles = "nanofaktura.roles"

// ExtRoles is the OpenAPI extension listing the allowed roles of an operation.
const ExtRoles = "x-roles"

// Role sets of the permission matrix (SPEC §7.13).
var (
	// RolesAll: every role (reads, and anything not declared otherwise).
	RolesAll = []string{model.RoleOwner, model.RoleAdmin, model.RoleAccountant, model.RoleMember}
	// RolesEditors may change documents, subjects, expenses and attachments.
	RolesEditors = []string{model.RoleOwner, model.RoleAdmin, model.RoleMember}
	// RolesManagers may change account settings and manage members.
	RolesManagers = []string{model.RoleOwner, model.RoleAdmin}
)

// Operation options for the common role sets.
var (
	ForEditors  = Allow(RolesEditors...)
	ForManagers = Allow(RolesManagers...)
)

// IsRole reports whether r is a known membership role.
func IsRole(r string) bool { return slices.Contains(RolesAll, r) }

// Allow is a huma operation option restricting the operation to the given
// roles. It also documents them (description + x-roles extension).
func Allow(roles ...string) func(*huma.Operation) {
	for _, r := range roles {
		if !IsRole(r) {
			panic("auth.Allow: unknown role " + r)
		}
	}
	roles = slices.Clone(roles)
	return func(o *huma.Operation) {
		if o.Metadata == nil {
			o.Metadata = map[string]any{}
		}
		o.Metadata[metaRoles] = roles
		if o.Extensions == nil {
			o.Extensions = map[string]any{}
		}
		o.Extensions[ExtRoles] = roles
		note := "Allowed roles: " + strings.Join(roles, ", ") + "."
		if o.Description == "" {
			o.Description = note
		} else {
			o.Description += "\n\n" + note
		}
	}
}

// AllowedRoles returns the roles declared on op, or nil when any role may call it.
func AllowedRoles(op *huma.Operation) []string {
	if op == nil || op.Metadata == nil {
		return nil
	}
	r, _ := op.Metadata[metaRoles].([]string)
	return r
}

// roleAllowed reports whether role may call op.
func roleAllowed(op *huma.Operation, role string) bool {
	allowed := AllowedRoles(op)
	return allowed == nil || slices.Contains(allowed, role)
}

// RequireRole is the imperative variant for checks that depend on the request
// (e.g. "admins may remove members, anyone may remove themselves"). It returns
// 403 unless the current role is one of roles.
func RequireRole(ctx context.Context, roles ...string) error {
	if r := RoleFrom(ctx); !slices.Contains(roles, r) {
		return forbiddenRole(r, roles)
	}
	return nil
}

func forbiddenRole(role string, allowed []string) error {
	return huma.Error403Forbidden(forbiddenMsg(role, allowed))
}

func forbiddenMsg(role string, allowed []string) string {
	return "your role (" + role + ") cannot do this; allowed roles: " + strings.Join(allowed, ", ")
}
