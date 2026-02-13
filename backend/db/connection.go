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

const (
	DefaultMaxOpenConns    = 25
	DefaultMaxIdleConns    = 10
	DefaultConnMaxLifetime = 30 * time.Minute
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

	// Configure connection pool
	sqlDB, err := DB.DB()
	if err != nil {
		return fmt.Errorf("failed to get underlying sql.DB: %w", err)
	}

	// SetMaxOpenConns limits the total number of open connections to the database.
	// Prevents exhausting PostgreSQL's max_connections limit under load.
	sqlDB.SetMaxOpenConns(DefaultMaxOpenConns)

	// SetMaxIdleConns limits the number of idle connections kept open.
	// Reduces connection churn by reusing idle connections.
	sqlDB.SetMaxIdleConns(DefaultMaxIdleConns)

	// SetConnMaxLifetime sets the maximum duration a connection may be reused.
	// Connections older than this are closed and replaced.
	// Prevents stale connections from causing issues.
	sqlDB.SetConnMaxLifetime(DefaultConnMaxLifetime)

	log.Printf("✅ Database connected successfully (pool: max_open=%d, max_idle=%d, max_lifetime=%v)",
		DefaultMaxOpenConns, DefaultMaxIdleConns, DefaultConnMaxLifetime)

	return nil
}

// GetDB returns the database connection
func GetDB() *gorm.DB {
	return DB
}
