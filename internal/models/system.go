package models

import "time"

// SystemConfig je globální konfigurace aplikace (vždy ID=1).
// Uchovává příznak inicializace a zvoleného režimu.
type SystemConfig struct {
	ID          uint      `gorm:"primarykey"             json:"id"`
	MultiUser   bool      `gorm:"not null;default:false" json:"multi_user"`
	Initialized bool      `gorm:"not null;default:false" json:"initialized"`
	CreatedAt   time.Time `                              json:"created_at"`
	UpdatedAt   time.Time `                              json:"updated_at"`
}
