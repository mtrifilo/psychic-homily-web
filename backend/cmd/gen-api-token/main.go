// gen-api-token generates a phk_ API token for a user directly against the database.
// Intended for local development only — bypasses API auth entirely.
//
// Usage:
//
//	go run ./cmd/gen-api-token                    # token for user ID 1
//	go run ./cmd/gen-api-token --user-id 3        # token for user ID 3
//	go run ./cmd/gen-api-token --email admin@test  # token for user by email
package main

import (
	"flag"
	"fmt"
	"os"

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
	makeAdmin := flag.Bool("make-admin", false, "Make the user an admin if not already")
	flag.Parse()

	cfg, err := config.Load()
	if err != nil {
		// In dev, validation errors are expected (placeholder secrets)
		// We only need the database URL which has a usable default
		cfg = &config.Config{
			Database: config.DatabaseConfig{
				URL: config.GetEnv("DATABASE_URL", "postgres://psychicadmin:secretpassword@localhost:5432/psychicdb?sslmode=disable"),
			},
		}
	}
	_ = err

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
		if err := db.Where("email = ?", *email).First(&user).Error; err != nil {
			fmt.Fprintf(os.Stderr, "User with email %q not found: %v\n", *email, err)
			os.Exit(1)
		}
	} else {
		if err := db.First(&user, *userID).Error; err != nil {
			fmt.Fprintf(os.Stderr, "User with ID %d not found: %v\n", *userID, err)
			os.Exit(1)
		}
	}

	// Optionally make admin
	if *makeAdmin && !user.IsAdmin {
		db.Model(&user).Update("is_admin", true)
		fmt.Fprintf(os.Stderr, "Made user %d (%s) an admin\n", user.ID, *user.Email)
	}

	if !user.IsAdmin {
		fmt.Fprintf(os.Stderr, "Warning: user %d is not an admin. Most ph commands require admin access.\n", user.ID)
		fmt.Fprintf(os.Stderr, "Run with --make-admin to fix.\n")
	}

	// Generate token
	svc := adminsvc.NewAPITokenService(db)
	resp, err := svc.CreateToken(user.ID, desc, *days)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create token: %v\n", err)
		os.Exit(1)
	}

	fmt.Fprintf(os.Stderr, "Token created for user %d (%s)\n", user.ID, *user.Email)
	fmt.Fprintf(os.Stderr, "Expires: %s\n", resp.ExpiresAt.Format("2006-01-02"))
	fmt.Fprintf(os.Stderr, "\nRun this to configure the CLI:\n\n")
	fmt.Fprintf(os.Stderr, "  cd cli && bun run src/entry.ts init --url http://localhost:8080 --token %s --name local\n\n", resp.Token)

	// Print just the token to stdout (for piping)
	fmt.Println(resp.Token)
}
