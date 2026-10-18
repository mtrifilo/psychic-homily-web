package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"math"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"psychic-homily-backend/internal/config"
)

var (
	DB *gorm.DB
)

// connectTimeout bounds opening the database. A healthy server answers a
// connection in milliseconds, so a connection still waiting at the bound is
// one that is not being answered (a server that accepted TCP and never writes,
// a dial or lookup that never completes), and boot reports it instead of
// waiting with it. It is the same bound as the boot reads that follow the
// connect.
const connectTimeout = bootSchemaReadTimeout

// Connect opens the database, pings it, and sets DB. The first ping is bounded
// by connectTimeout. Every pool connection's dial and startup is bounded, per
// address tried, by connect_timeout on the DSN, which is connectTimeout unless
// the DSN or PGCONNECT_TIMEOUT sets its own. A server that accepts TCP and
// never answers therefore refuses boot instead of stalling it.
func Connect(cfg *config.Config) error {
	return connectWithin(cfg, connectTimeout)
}

func connectWithin(cfg *config.Config, bound time.Duration) error {
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
	//
	// GORM's automatic ping is unbounded, so it is off and pingWithin runs a
	// bounded one instead.
	dsn := withDefaultConnectTimeout(cfg.Database.URL, bound)
	gormDB, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger:               gormLogger,
		TranslateError:       true,
		DisableAutomaticPing: true,
		NowFunc: func() time.Time {
			return time.Now().UTC()
		},
	})
	if err != nil {
		return fmt.Errorf("failed to connect to database: %w", err)
	}

	if err := applyPoolBounds(gormDB, cfg.Database); err != nil {
		return err
	}

	sqlDB, err := gormDB.DB()
	if err != nil {
		return fmt.Errorf("failed to reach the underlying sql.DB: %w", err)
	}
	dialBound := effectiveConnectTimeout(dsn)
	if err := pingWithin(sqlDB, bound, dialBound); err != nil {
		_ = sqlDB.Close()
		return err
	}
	DB = gormDB

	// The pool shape and connect bound are logged, not just configured. Both are
	// the kind of assumption that fails silently: a wrong number keeps serving
	// and simply does the wrong thing. So they have to be readable from a
	// deploy's logs rather than inferred from the code.
	log.Printf("✅ Database connected successfully (pool: max_open=%d max_idle=%d conn_max_lifetime=%s; connect_timeout=%s)",
		cfg.Database.MaxOpenConns, cfg.Database.MaxIdleConns, cfg.Database.ConnMaxLifetime, describeBound(dialBound))
	return nil
}

// withDefaultConnectTimeout returns dsn with connect_timeout set to bound,
// unless dsn or PGCONNECT_TIMEOUT already sets one, which is kept. It accepts
// both DSN forms pgx does: a postgres:// or postgresql:// URL, and
// keyword/value. A blank dsn is returned as it is, so pgx reads it as it would
// without the bound. The setting is whole seconds, rounded up, since
// connect_timeout=0 is no bound at all.
func withDefaultConnectTimeout(dsn string, bound time.Duration) string {
	if os.Getenv("PGCONNECT_TIMEOUT") != "" || strings.TrimSpace(dsn) == "" {
		return dsn
	}
	seconds := max(int(math.Ceil(bound.Seconds())), 1)
	setting := "connect_timeout=" + strconv.Itoa(seconds)

	if !strings.HasPrefix(dsn, "postgres://") && !strings.HasPrefix(dsn, "postgresql://") {
		// pgx keeps the last value of a repeated keyword, so a leading setting
		// is the default an explicit one later in dsn replaces. Leading also
		// keeps it out of a trailing empty value ("dbname=" would read a
		// following setting as its value).
		return setting + " " + dsn
	}

	beforeFragment, fragment, hasFragment := strings.Cut(dsn, "#")
	_, query, hasQuery := strings.Cut(beforeFragment, "?")
	if values, _ := url.ParseQuery(query); values.Has("connect_timeout") {
		return dsn
	}
	separator := "?"
	if hasQuery {
		separator = "&"
		if query == "" || strings.HasSuffix(query, "&") {
			separator = ""
		}
	}
	withTimeout := beforeFragment + separator + setting
	if hasFragment {
		withTimeout += "#" + fragment
	}
	return withTimeout
}

// effectiveConnectTimeout is the connect timeout pgx applies to dsn, with
// PGCONNECT_TIMEOUT folded in. Zero means none.
func effectiveConnectTimeout(dsn string) time.Duration {
	parsed, err := pgconn.ParseConfig(dsn)
	if err != nil {
		return 0
	}
	return parsed.ConnectTimeout
}

// pingWithin runs the first ping, which also opens the first connection, with
// a context that ends after bound. A ping that ran out of time says so and
// names the bound that ended it. That is dialBound when it is set and shorter,
// since the dial then gives up first; it applies per address, so a DSN with
// several addresses can wait a multiple of it before bound ends the ping.
func pingWithin(sqlDB *sql.DB, bound, dialBound time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), bound)
	defer cancel()
	err := sqlDB.PingContext(ctx)
	if err == nil {
		return nil
	}
	if errors.Is(err, context.DeadlineExceeded) {
		if dialBound > 0 && dialBound < bound {
			return fmt.Errorf("database did not answer within %s per address (connect_timeout): %w", dialBound, err)
		}
		return fmt.Errorf("database did not answer within %s: %w", bound, err)
	}
	return fmt.Errorf("failed to connect to database: %w", err)
}

func describeBound(bound time.Duration) string {
	if bound == 0 {
		return "none"
	}
	return bound.String()
}

// applyPoolBounds sizes the database/sql pool behind GORM.
//
// database/sql leaves max-open UNLIMITED by default, so any path that fans out
// (a batch's per-item background writes, alert fan-out, discovery import) can
// open a connection per unit of work until the SERVER refuses, which surfaces
// as "too many connections" on unrelated requests rather than as slowness here.
// A bounded pool queues instead. The numbers come from internal/config.
func applyPoolBounds(gormDB *gorm.DB, cfg config.DatabaseConfig) error {
	sqlDB, err := gormDB.DB()
	if err != nil {
		return fmt.Errorf("failed to reach the underlying sql.DB: %w", err)
	}

	sqlDB.SetMaxOpenConns(cfg.MaxOpenConns)
	sqlDB.SetMaxIdleConns(cfg.MaxIdleConns)
	sqlDB.SetConnMaxLifetime(cfg.ConnMaxLifetime)
	return nil
}

// GetDB returns the database connection
func GetDB() *gorm.DB {
	return DB
}
