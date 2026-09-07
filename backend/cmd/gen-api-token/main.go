// gen-api-token generates a phk_ API token for a user directly against the
// database, bypassing the session-gated POST /admin/tokens endpoint.
//
// This is the bootstrap path when no working credential exists for an
// environment: a database restore drops every token, and re-minting one through
// the API would otherwise require a browser login as an admin who may be
// unreachable.
//
// Local targets stay a one-liner. Any NON-local database is dry-run by default
// and requires --confirm, because the same command that seeds a dev box will
// happily mint a year-long admin credential against production if DATABASE_URL
// points there.
//
// Usage:
//
//	go run ./cmd/gen-api-token                                  # user ID 1, local
//	go run ./cmd/gen-api-token --email admin@test --make-admin  # by email, grant admin
//	go run ./cmd/gen-api-token --env /tmp/x.env --email a@b.com # non-local: dry run
//	go run ./cmd/gen-api-token --env /tmp/x.env --email a@b.com --confirm
//
// The plaintext token is printed once, on creation — only a SHA-256 hash is
// stored, so a lost token cannot be recovered. Issue a new one instead.
package main

import (
	"flag"
	"fmt"
	"net/url"
	"os"
	"strings"

	"github.com/joho/godotenv"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	"psychic-homily-backend/internal/config"
	authm "psychic-homily-backend/internal/models/auth"
	adminsvc "psychic-homily-backend/internal/services/admin"
)

func main() {
	userID := flag.Uint("user-id", 1, "User ID to generate token for")
	email := flag.String("email", "", "User email (alternative to --user-id)")
	days := flag.Int("days", 365, "Token expiration in days")
	desc := flag.String("description", "CLI dev token", "Token description")
	makeAdmin := flag.Bool("make-admin", false, "Grant admin to the user if not already (tokens carry admin scope)")
	confirm := flag.Bool("confirm", false, "Required to write against a NON-local database")
	envFile := flag.String("env", "", "Path to .env file (defaults to process environment)")
	flag.Parse()

	if *envFile != "" {
		if err := godotenv.Load(*envFile); err != nil {
			fmt.Fprintf(os.Stderr, "load env file %s: %v\n", *envFile, err)
			os.Exit(1)
		}
	}

	cfg, err := config.Load()
	if err != nil {
		// In dev, validation errors are expected (placeholder secrets).
		// We only need the database URL, which has a usable default.
		cfg = &config.Config{
			Database: config.DatabaseConfig{
				URL: config.GetEnv("DATABASE_URL", "postgres://psychicadmin:secretpassword@localhost:5432/psychicdb?sslmode=disable"),
			},
		}
	}

	// Anything that isn't a loopback database is treated as deployed: dry-run
	// unless the operator explicitly confirms. This is what keeps a habitual
	// `go run ./cmd/gen-api-token --make-admin` from silently promoting a user
	// and minting a year-long admin key against production.
	local := isLocalDB(cfg.Database.URL)
	write := local || *confirm

	mode := "LIVE"
	if !write {
		mode = "DRY RUN"
	}
	fmt.Fprintf(os.Stderr, "=== Generate API Token (%s) ===\n", mode)
	fmt.Fprintf(os.Stderr, "Target: ENVIRONMENT=%q db=%s local=%t\n",
		os.Getenv(config.EnvEnvironment), redactDBHost(cfg.Database.URL), local)

	db, err := gorm.Open(postgres.Open(cfg.Database.URL), &gorm.Config{
		Logger: gormlogger.Default.LogMode(gormlogger.Silent),
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to connect to database: %v\n", err)
		os.Exit(1)
	}

	// Resolve user
	var user authm.User
	if *email != "" {
		if err := db.Where(authm.EmailIdentityWhere, *email).First(&user).Error; err != nil {
			fmt.Fprintf(os.Stderr, "User with email %q not found: %v\n", *email, err)
			os.Exit(1)
		}
	} else {
		if err := db.First(&user, *userID).Error; err != nil {
			fmt.Fprintf(os.Stderr, "User with ID %d not found: %v\n", *userID, err)
			os.Exit(1)
		}
	}

	fmt.Fprintf(os.Stderr, "User:   id=%d email=%s is_admin=%t is_active=%t\n",
		user.ID, deref(user.Email), user.IsAdmin, user.IsActive)

	// A deleted or deactivated account can be restored by its owner through the
	// self-service recovery flow, at which point any token minted here would go
	// live — unattributed and long after anyone remembers issuing it.
	if err := ensureAccountUsable(&user); err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}

	// CreateToken stamps every token with admin scope, and ValidateToken rejects
	// an admin-scope token whose owner is not an admin. Minting for a non-admin
	// therefore produces a credential that cannot work today but activates the
	// moment that user is promoted — refuse instead of warning.
	if !user.IsAdmin && !*makeAdmin {
		fmt.Fprintf(os.Stderr,
			"user %d (%s) is not an admin; API tokens carry admin scope and would not validate.\n"+
				"Re-run with --make-admin to grant admin, or target an admin account.\n",
			user.ID, deref(user.Email))
		os.Exit(1)
	}

	if !write {
		fmt.Fprintf(os.Stderr, "\nDry run against a non-local database — nothing written.\n")
		if !user.IsAdmin && *makeAdmin {
			fmt.Fprintf(os.Stderr, "Would GRANT ADMIN to user %d.\n", user.ID)
		}
		fmt.Fprintf(os.Stderr, "Would issue a %d-day token. Re-run with --confirm to apply.\n", *days)
		return
	}

	// Optionally make admin
	if *makeAdmin && !user.IsAdmin {
		if err := db.Model(&user).Update("is_admin", true).Error; err != nil {
			fmt.Fprintf(os.Stderr, "Failed to grant admin: %v\n", err)
			os.Exit(1)
		}
		fmt.Fprintf(os.Stderr, "Made user %d (%s) an admin\n", user.ID, deref(user.Email))
	}

	// Generate token
	svc := adminsvc.NewAPITokenService(db)
	resp, err := svc.CreateToken(user.ID, desc, *days)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create token: %v\n", err)
		os.Exit(1)
	}

	fmt.Fprintf(os.Stderr, "Token created for user %d (%s)\n", user.ID, deref(user.Email))
	// Render in UTC: expiry is stored and compared in UTC, and formatting in the
	// operator's local zone can print the PREVIOUS day for a UTC-evening
	// timestamp, which reads like the token expires sooner than it does.
	fmt.Fprintf(os.Stderr, "Expires: %s\n", resp.ExpiresAt.UTC().Format("2006-01-02 15:04 MST"))
	fmt.Fprintf(os.Stderr, "\nRun this to configure the CLI:\n\n")
	fmt.Fprintf(os.Stderr, "  cd cli && bun run src/entry.ts init --url http://localhost:8080 --token %s --name local\n\n", resp.Token)
	fmt.Fprintf(os.Stderr, "Store it now — only a hash is kept, so this value cannot be retrieved again.\n")

	// Print just the token to stdout (for piping)
	fmt.Println(resp.Token)
}

// ensureAccountUsable rejects accounts that are deactivated or soft-deleted.
// User.DeletedAt is a plain *time.Time rather than gorm.DeletedAt, so GORM does
// NOT filter soft-deleted rows automatically — the check has to be explicit.
func ensureAccountUsable(user *authm.User) error {
	if user.DeletedAt != nil {
		return fmt.Errorf("user %d (%s) is soft-deleted; refusing to mint a token that would go live if the account is restored",
			user.ID, deref(user.Email))
	}
	if !user.IsActive {
		return fmt.Errorf("user %d (%s) is not active; refusing to mint a token for a deactivated account",
			user.ID, deref(user.Email))
	}
	return nil
}

// isLocalDB reports whether the database URL points at a loopback host. Only
// loopback is treated as local: a Railway internal host is a deployed database.
func isLocalDB(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		// Unparseable (e.g. key/value DSN) — fail safe by treating it as remote
		// so the confirm gate applies.
		return false
	}
	host := u.Hostname()
	return host == "localhost" || host == "127.0.0.1" || host == "::1" ||
		strings.HasSuffix(host, ".localhost")
}

// redactDBHost extracts host[:port]/dbname from a database URL, dropping any
// embedded credentials, so the target can be logged without leaking secrets.
func redactDBHost(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return "<unparseable>"
	}
	return u.Host + u.Path
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
