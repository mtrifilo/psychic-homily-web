package db

import (
	"fmt"
	"log"
	"os"
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

	// ErrRecordNotFound is application-level branching (lookup-or-create, radio
	// matching against unmatched plays), not an error worth surfacing.
	gormLogger := logger.New(log.New(os.Stdout, "", log.LstdFlags), logger.Config{
		SlowThreshold:             200 * time.Millisecond,
		LogLevel:                  logger.Warn,
		IgnoreRecordNotFoundError: true,
		Colorful:                  cfg.Server.LogLevel == "debug",
	})
	if cfg.Server.LogLevel == "debug" {
		gormLogger = gormLogger.LogMode(logger.Info)
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
