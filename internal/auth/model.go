// Package auth obsahuje modely, service logiku a middleware pro autentizaci uživatelů.
package auth

import (
	"time"

	"github.com/qwerin/nanofaktura/internal/models"
)

// UserRole definuje úroveň oprávnění.
type UserRole string

const (
	RoleSuperAdmin UserRole = "superadmin"
	RoleAdmin      UserRole = "admin"
	RoleUser       UserRole = "user"
)

// User je GORM model uživatelského účtu.
type User struct {
	models.Base
	Username     string   `gorm:"uniqueIndex;not null;size:100"  json:"username"`
	Email        string   `gorm:"uniqueIndex;size:200"           json:"email,omitempty"`
	PasswordHash string   `gorm:"not null"                       json:"-"` // nikdy serializován
	Role         UserRole `gorm:"not null;default:'user';size:20" json:"role"`
	IsActive     bool     `gorm:"not null;default:true"          json:"is_active"`
}

// Session ukládá auth tokeny. Nemá soft-delete — fyzicky se maže při odhlášení/expiraci.
type Session struct {
	ID        uint      `gorm:"primarykey"                    json:"id"`
	TokenHash string    `gorm:"uniqueIndex;not null;size:64"  json:"-"`
	UserID    uint      `gorm:"not null;index"                json:"user_id"`
	User      User      `gorm:"foreignKey:UserID"             json:"-"`
	ExpiresAt time.Time `gorm:"not null;index"                json:"expires_at"`
	CreatedAt time.Time `                                      json:"created_at"`
}
