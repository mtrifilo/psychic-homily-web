package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/joho/godotenv"

	"psychic-homily-backend/db"
	"psychic-homily-backend/internal/config"
	"psychic-homily-backend/internal/services"
)

func main() {
	// Parse command line flags
	inputPattern := flag.String("input", "", "Input JSON file(s) - supports glob patterns (e.g., './output/scraped-*.json')")
	envFile := flag.String("env", "", "Path to .env file (optional, defaults to .env.development)")
	dryRun := flag.Bool("dry-run", false, "Simulate import without making database changes")
	verbose := flag.Bool("verbose", false, "Show detailed output for each event")
	flag.Parse()

	if *inputPattern == "" {
		fmt.Println("Discovery Importer - Import discovered venue events into the database")
		fmt.Println()
		fmt.Println("Usage:")
		fmt.Println("  discovery-import -input <file.json> [-env <envfile>] [-dry-run] [-verbose]")
		fmt.Println()
		fmt.Println("Options:")
		fmt.Println("  -input     Input JSON file(s) - supports glob patterns")
		fmt.Println("  -env       Path to .env file (defaults to .env.development)")
		fmt.Println("  -dry-run   Simulate import without making database changes")
		fmt.Println("  -verbose   Show detailed output for each event")
		fmt.Println()
		fmt.Println("Examples:")
		fmt.Println("  discovery-import -input ./output/scraped-events-2026-01-21.json")
		fmt.Println("  discovery-import -input './output/scraped-*.json' -dry-run")
		fmt.Println("  discovery-import -input ./output/events.json -env .env.production")
		os.Exit(1)
	}

	// Load environment file
	if *envFile != "" {
		if err := godotenv.Load(*envFile); err != nil {
			log.Fatalf("Failed to load env file %s: %v", *envFile, err)
		}
		log.Printf("Loaded environment from %s", *envFile)
	} else {
		// Try default env files
		envFiles := []string{".env.development", ".env"}
		loaded := false
		for _, ef := range envFiles {
			if err := godotenv.Load(ef); err == nil {
				log.Printf("Loaded environment from %s", ef)
				loaded = true
				break
			}
		}
		if !loaded {
			log.Println("Warning: No .env file loaded, using environment variables")
		}
	}

	// Load configuration and connect to database
	cfg := config.Load()
	if err := db.Connect(cfg); err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}

	// Find matching files
	files, err := filepath.Glob(*inputPattern)
	if err != nil {
		log.Fatalf("Invalid glob pattern: %v", err)
	}
	if len(files) == 0 {
		log.Fatalf("No files found matching pattern: %s", *inputPattern)
	}

	log.Printf("Found %d file(s) to process", len(files))
	if *dryRun {
		log.Println("DRY RUN - No database changes will be made")
	}

	// Create discovery service
	discoveryService := services.NewDiscoveryService()

	// Process each file
	totalResult := &services.ImportResult{
		Messages: make([]string, 0),
	}

	for _, file := range files {
		log.Printf("\nProcessing: %s", file)

		result, err := discoveryService.ImportFromJSON(file, *dryRun)
		if err != nil {
			log.Printf("ERROR processing %s: %v", file, err)
			continue
		}

		// Aggregate results
		totalResult.Total += result.Total
		totalResult.Imported += result.Imported
		totalResult.Duplicates += result.Duplicates
		totalResult.Rejected += result.Rejected
		totalResult.Errors += result.Errors

		if *verbose {
			for _, msg := range result.Messages {
				fmt.Printf("  %s\n", msg)
			}
		}

		// Print file summary
		log.Printf("  File results: %d total, %d imported, %d duplicates, %d rejected, %d errors",
			result.Total, result.Imported, result.Duplicates, result.Rejected, result.Errors)
	}

	// Print final summary
	fmt.Println()
	fmt.Println(strings.Repeat("=", 60))
	fmt.Println("IMPORT SUMMARY")
	fmt.Println(strings.Repeat("=", 60))
	fmt.Printf("Files processed:  %d\n", len(files))
	fmt.Printf("Total events:     %d\n", totalResult.Total)
	fmt.Printf("Imported:         %d\n", totalResult.Imported)
	fmt.Printf("Duplicates:       %d (already in database)\n", totalResult.Duplicates)
	fmt.Printf("Rejected:         %d (matched rejected shows)\n", totalResult.Rejected)
	fmt.Printf("Errors:           %d\n", totalResult.Errors)
	fmt.Println(strings.Repeat("=", 60))

	if *dryRun {
		fmt.Println("\nThis was a DRY RUN - no changes were made to the database.")
		fmt.Println("Run without -dry-run to actually import the events.")
	} else if totalResult.Imported > 0 {
		fmt.Printf("\nSuccessfully imported %d new shows to the pending queue.\n", totalResult.Imported)
		fmt.Println("Review them in the admin panel at /admin/pending")
	}

	// Exit with error code if there were errors
	if totalResult.Errors > 0 {
		os.Exit(1)
	}
}
