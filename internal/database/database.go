// Package database inicializuje databázové spojení a provádí auto-migraci GORM modelů.
// Podporuje SQLite (výchozí, pro lokální provoz) a PostgreSQL (pro produkci v kontejneru).
package database

import (
	"fmt"

	"github.com/qwerin/nanofaktura/internal/auth"
	"github.com/qwerin/nanofaktura/internal/models"
	"gorm.io/driver/postgres"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// Open otevře databázi podle zadaného driveru a DSN, a provede automatickou migraci.
// driver: "sqlite" (výchozí) nebo "postgres".
// Pro SQLite je dsn cesta k souboru (např. "nanofaktura.db" nebo ":memory:").
// Pro PostgreSQL je dsn connection string (např. "host=... user=... dbname=... sslmode=disable").
func Open(dsn string, driver ...string) (*gorm.DB, error) {
	d := "sqlite"
	if len(driver) > 0 && driver[0] != "" {
		d = driver[0]
	}

	var dialector gorm.Dialector
	switch d {
	case "postgres":
		dialector = postgres.Open(dsn)
	case "sqlite":
		dialector = sqlite.Open(dsn)
	default:
		return nil, fmt.Errorf("neznámý db driver: %q (použijte sqlite nebo postgres)", d)
	}

	db, err := gorm.Open(dialector, &gorm.Config{
		Logger: logger.Default.LogMode(logger.Warn),
	})
	if err != nil {
		return nil, err
	}

	if err := db.AutoMigrate(
		&models.Invoice{},
		&models.InvoiceLine{},
		&models.Subject{},
		&models.Settings{},
		&models.NumberFormat{},
		&models.SystemConfig{},
		&auth.User{},
		&auth.Session{},
	); err != nil {
		return nil, err
	}

	// SQLite-specifická nastavení
	if d == "sqlite" {
		db.Exec("PRAGMA journal_mode=WAL;")
		db.Exec("PRAGMA foreign_keys=ON;")
	}

	return db, nil
}

// EnsureSystemConfig načte SystemConfig (ID=1), nebo ho vytvoří.
// Pokud existují Settings ale žádný SystemConfig → upgrade ze staré verze → označíme jako initialized (single-user).
func EnsureSystemConfig(db *gorm.DB) models.SystemConfig {
	var cfg models.SystemConfig
	if err := db.First(&cfg, 1).Error; err == nil {
		return cfg
	}

	// SystemConfig neexistuje — zjistíme zda existují Settings (upgrade)
	var settingsCount int64
	db.Model(&models.Settings{}).Count(&settingsCount)

	cfg = models.SystemConfig{
		ID:          1,
		MultiUser:   false,
		Initialized: settingsCount > 0, // existující data = již inicializováno jako single-user
	}
	db.Create(&cfg)
	return cfg
}

// SaveSystemConfig uloží SystemConfig ID=1.
func SaveSystemConfig(db *gorm.DB, cfg models.SystemConfig) error {
	cfg.ID = 1
	return db.Save(&cfg).Error
}

// GetUserSettings vrátí Settings pro daného uživatele.
// userID=0 → single-user mód: hledá Settings ID=1.
// userID>0 → multi-user mód: hledá WHERE user_id = userID.
func GetUserSettings(db *gorm.DB, userID uint) (*models.Settings, error) {
	var s models.Settings
	var err error
	if userID == 0 {
		err = db.Preload("NumberFormats").First(&s, 1).Error
	} else {
		err = db.Preload("NumberFormats").Where("user_id = ?", userID).First(&s).Error
	}
	if err != nil {
		return nil, err
	}
	return &s, nil
}

// MigrateOwnership přiřadí všechna osiřelá data (user_id=0) danému uživateli.
func MigrateOwnership(db *gorm.DB, ownerUserID uint) {
	db.Exec("UPDATE invoices SET user_id = ? WHERE user_id = 0", ownerUserID)
	db.Exec("UPDATE subjects SET user_id = ? WHERE user_id = 0", ownerUserID)
	db.Exec("UPDATE settings SET user_id = ? WHERE user_id = 0", ownerUserID)
}
