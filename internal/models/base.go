package models

import (
	"time"

	"gorm.io/gorm"
)

// Base nahrazuje gorm.Model se správnými json tagy (lowercase).
// gorm.Model nemá json tagy, takže serializuje jako "ID", "CreatedAt" atd.,
// ale frontend očekává "id", "created_at".
type Base struct {
	ID        uint           `gorm:"primarykey"  json:"id"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index"       json:"deleted_at,omitempty"`
}
