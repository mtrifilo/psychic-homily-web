// Command dedup-radio-shows collapses the duplicated WFMU-family show
// catalog (PSY-1073). Before station-scoped discovery, every discover cycle
// assigned the entire WFMU DJ index to whichever family station triggered
// it, so all four stations (WFMU 91.1, Give the Drummer, Rock'n'Soul,
// Sheena's Jungle Room) carried a full duplicate copy of every show plus its
// episode/play history.
//
// The command fetches WFMU's schedule pages to compute the canonical owner
// of each show code (the station whose stream airs it; everything ambiguous
// or defunct defaults to the flagship), then merges each code's duplicate
// rows onto the owner: episodes follow their show, duplicate episode copies
// keep whichever side logged more plays, import jobs re-point to the
// survivor, and the duplicate rows are deleted. The whole run is one
// transaction.
//
// Usage:
//
//	go run ./cmd/dedup-radio-shows               # dry-run (default)
//	go run ./cmd/dedup-radio-shows --confirm     # apply changes
//
// Dry-run executes the full plan inside a transaction and rolls it back, so
// the printed counts are exactly what --confirm would commit. Idempotent:
// re-running after a live run reports zero duplicate groups.
package main

import (
	"flag"
	"fmt"
	"log"

	"github.com/joho/godotenv"

	"psychic-homily-backend/db"
	"psychic-homily-backend/internal/config"
	"psychic-homily-backend/internal/services/catalog"
)

var (
	confirm bool
	envFile string
)

func main() {
	flag.BoolVar(&confirm, "confirm", false, "Apply changes (default: dry-run only)")
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
	database := db.GetDB()

	mode := "DRY RUN"
	if confirm {
		mode = "LIVE"
	}
	fmt.Printf("=== WFMU Radio Show Dedup (%s) — PSY-1073 ===\n\n", mode)

	// Ownership comes from wfmu.org's live schedule pages — the same source
	// the station-scoped importer uses, so cleanup and future discovery
	// agree on every show's home. A fetch failure aborts: guessing
	// ownership would mis-home shows.
	provider := catalog.NewWFMUProvider()
	defer provider.Close()

	fmt.Println("Fetching WFMU schedule pages for show ownership...")
	ownership, err := provider.FetchShowOwnership()
	if err != nil {
		log.Fatalf("fetch show ownership from wfmu.org: %v", err)
	}
	fmt.Printf("Resolved ownership for %d show codes (codes not listed default to the flagship).\n\n", len(ownership))

	result, err := catalog.DedupWFMUFamilyShows(database, ownership, !confirm)
	if err != nil {
		log.Fatalf("dedup failed (transaction rolled back): %v", err)
	}

	printResult(result)
}

func printResult(r *catalog.WFMUDedupResult) {
	fmt.Println("=== Summary ===")
	fmt.Printf("Show-code groups found:      %d\n", r.GroupsTotal)
	fmt.Printf("Groups needing changes:      %d\n", r.GroupsWithDuplicates)
	fmt.Printf("Rows skipped (no ext. id):   %d\n", r.ShowsWithNoExternalID)
	fmt.Printf("Slugs recanonicalised:       %d\n", r.SlugsRecanonicalised)
	fmt.Println()
	fmt.Println("Per station:")
	for _, slug := range catalog.WFMUFamilySlugs {
		c := r.PerStation[slug]
		fmt.Printf("  %-22s shows kept=%d reassigned-in=%d deleted=%d | episodes moved-in=%d dup-deleted=%d | jobs reassigned=%d\n",
			slug, c.ShowsKept, c.ShowsReassignedIn, c.ShowsDeleted, c.EpisodesMovedIn, c.EpisodesDeleted, c.JobsReassigned)
	}
	fmt.Println()
	if r.DryRun {
		fmt.Println("DRY RUN — full plan executed in a transaction and rolled back.")
		fmt.Println("No DB writes. Re-run with --confirm to apply these exact changes.")
	} else {
		fmt.Println("LIVE — changes committed.")
	}
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
	log.Println("no .env loaded; using process environment")
}
