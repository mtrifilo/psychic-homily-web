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

	// Connect to database.
	//
	// TranslateError maps driver errors to GORM sentinel errors (e.g. Postgres
	// 23505 unique violations → gorm.ErrDuplicatedKey, 23503 FK violations →
	// gorm.ErrForeignKeyViolated). This lets the service layer discriminate on
	// errors.Is(err, gorm.ErrDuplicatedKey) instead of fragile substring
	// matching on the raw driver message. See services/shared/db_errors.go for
	// the canonical helper.
	DB, err = gorm.Open(postgres.Open(cfg.Database.URL), &gorm.Config{
		Logger:         gormLogger,
		TranslateError: true,
		NowFunc: func() time.Time {
			return time.Now().UTC()
		},
	})
	if err != nil {
		return fmt.Errorf("failed to connect to database: %w", err)
	}

	if err := applyPoolBounds(DB, cfg.Database); err != nil {
		return err
	}

	return nil
}

// applyPoolBounds sizes the database/sql pool behind GORM.
//
// database/sql leaves max-open UNLIMITED by default, so any path that fans out
// (a batch's per-item background writes, alert fan-out, discovery import) can
// open a connection per unit of work until the SERVER refuses, which surfaces
// as "too many connections" on unrelated requests rather than as slowness here.
// A bounded pool queues instead.
//
// The shape is logged because a pool ceiling is exactly the kind of assumption
// that fails silently: a wrong number keeps serving and simply does the wrong
// thing, so it has to be readable from a deploy's logs rather than inferred
// from the code (see internal/config for the env vars that set it).
func applyPoolBounds(gormDB *gorm.DB, cfg config.DatabaseConfig) error {
	sqlDB, err := gormDB.DB()
	if err != nil {
		return fmt.Errorf("failed to reach the underlying sql.DB: %w", err)
	}

	sqlDB.SetMaxOpenConns(cfg.MaxOpenConns)
	sqlDB.SetMaxIdleConns(cfg.MaxIdleConns)
	sqlDB.SetConnMaxLifetime(cfg.ConnMaxLifetime)

	log.Printf("✅ Database connected successfully (pool: max_open=%d max_idle=%d conn_max_lifetime=%s)",
		cfg.MaxOpenConns, cfg.MaxIdleConns, cfg.ConnMaxLifetime)
	return nil
}

// GetDB returns the database connection
func GetDB() *gorm.DB {
	return DB
}
