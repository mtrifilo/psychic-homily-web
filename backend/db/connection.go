package db

import (
	"fmt"
	"log"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"psychic-homily-backend/internal/config"
)

var (
	DB *gorm.DB
)

func Connect(cfg *config.Config) error {
	var err error

	// Configure GORM logger
	gormLogger := logger.Default
	if cfg.Server.LogLevel == "debug" {
		gormLogger = logger.Default.LogMode(logger.Info)
	}

	// Connect to database
	DB, err = gorm.Open(postgres.Open(cfg.Database.URL), &gorm.Config{
		Logger: gormLogger,
		NowFunc: func() time.Time {
			return time.Now().UTC()
		},
	})
	if err != nil {
		return fmt.Errorf("failed to connect to database: %w", err)
	}

	log.Println("✅ Database connected successfully")
	return nil
}

// GetDB returns the database connection
func GetDB() *gorm.DB {
	return DB
}
