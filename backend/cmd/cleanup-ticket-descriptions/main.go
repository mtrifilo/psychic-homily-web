// Command cleanup-ticket-descriptions moves vendor ticket URLs out of stored
// show descriptions and into shows.ticket_url.
//
// Usage:
//
//	go run ./cmd/cleanup-ticket-descriptions                  # dry-run (default)
//	go run ./cmd/cleanup-ticket-descriptions --confirm        # apply changes
//	go run ./cmd/cleanup-ticket-descriptions --verbose        # per-row detail
//	go run ./cmd/cleanup-ticket-descriptions --env .env.stage # target a specific env
//
// Dry-run prints exactly what a live run would change and writes nothing. The
// pass is idempotent: a second run reports zero rows, because it only rewrites
// descriptions that still carry the vendor line.
package main

import (
	"flag"
	"fmt"
	"log"
	"net/url"
	"os"

	"github.com/joho/godotenv"

	"psychic-homily-backend/db"
	"psychic-homily-backend/internal/config"
	"psychic-homily-backend/internal/services/pipeline"
)

var (
	confirm bool
	verbose bool
	envFile string
)

func main() {
	flag.BoolVar(&confirm, "confirm", false, "Apply changes (default: dry-run only)")
	flag.BoolVar(&verbose, "verbose", false, "Print per-show detail")
	flag.StringVar(&envFile, "env", "", "Path to .env file (defaults to .env.development / .env)")
	flag.Parse()

	loadEnv()

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("load config: %v", err)
	}
	if err := db.Connect(cfg); err != nil {
		log.Fatalf("connect db: %v", err)
	}

	mode := "DRY RUN"
	if confirm {
		mode = "LIVE"
	}
	fmt.Printf("=== Ticket URL Description Cleanup (%s) ===\n", mode)
	// Surface the resolved target so a mistargeted --confirm is caught before
	// any write. Credentials are redacted.
	fmt.Printf("Target: ENVIRONMENT=%q  db=%s\n\n",
		os.Getenv(config.EnvEnvironment), redactDBHost(cfg.Database.URL))

	report, err := pipeline.CleanupTicketDescriptions(db.GetDB(), pipeline.TicketDescriptionCleanupOptions{
		DryRun:  !confirm,
		Verbose: verbose,
	})
	if err != nil {
		log.Fatalf("cleanup: %v", err)
	}

	printReport(report)
}

func loadEnv() {
	if envFile != "" {
		if err := godotenv.Load(envFile); err != nil {
			log.Fatalf("load env file %s: %v", envFile, err)
		}
		log.Printf("loaded env from %s", envFile)
		return
	}
	for _, ef := range []string{".env.development", ".env"} {
		if err := godotenv.Load(ef); err == nil {
			log.Printf("loaded env from %s", ef)
			return
		}
	}
}

func printReport(report *pipeline.TicketDescriptionCleanupReport) {
	verb := "would strip"
	if confirm {
		verb = "stripped"
	}

	if verbose {
		for _, row := range report.Rows {
			desc := "(cleared)"
			if row.NewDescription != nil {
				desc = *row.NewDescription
			}
			fmt.Printf("show #%d [%s] ticket_url %s | description -> %q\n",
				row.ShowID, row.Source, movedLabel(row.MovedToColumn), desc)
		}
		if len(report.Rows) > 0 {
			fmt.Println()
		}
	}

	fmt.Printf("Rows scanned (description carries the line): %d\n", report.Scanned)
	fmt.Printf("Rows %s: %d\n", verb, report.Stripped)
	fmt.Printf("Rows also given a ticket_url: %d\n", report.MovedToColumn)
	fmt.Printf("Rows skipped (line is not an absolute http(s) URL): %d\n", report.SkippedNonURL)
	fmt.Printf("Rows skipped (URL wider than the column, nowhere to move it): %d\n", report.SkippedOversizeURL)
	fmt.Printf("Stripped rows by source: %s\n", report.SourceBreakdown())

	if !confirm && report.Stripped > 0 {
		fmt.Printf("\nNothing was written. Re-run with --confirm to apply.\n")
	}
}

func movedLabel(moved bool) string {
	if moved {
		return "SET"
	}
	return "kept (already populated)"
}

// redactDBHost renders the target host without credentials.
func redactDBHost(dbURL string) string {
	parsed, err := url.Parse(dbURL)
	if err != nil || parsed.Host == "" {
		return "(unparsed)"
	}
	return parsed.Host + parsed.Path
}
