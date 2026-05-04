// Command dedup-shows merges show records that share
// (artist_id, venue_id, event_date+time) and re-points all FKs to a
// single canonical row. Time-of-day is part of the dedup key so
// matinee+evening sets at the same venue on the same day are NOT
// collapsed (PSY-559).
//
// Usage:
//
//	go run ./cmd/dedup-shows                # dry-run (default)
//	go run ./cmd/dedup-shows --confirm      # apply changes
//	go run ./cmd/dedup-shows --verbose      # per-cluster output
//	go run ./cmd/dedup-shows --no-slug-fix  # skip slug recanonicalisation
//
// The dry-run path prints a per-cluster plan plus an aggregate count.
// The confirm path runs the same plan inside a single transaction
// per cluster (each cluster is independent — a partial run leaves
// partial progress, never half-merged shows).
package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/joho/godotenv"
	"gorm.io/gorm"

	"psychic-homily-backend/db"
	"psychic-homily-backend/internal/config"
	catalogm "psychic-homily-backend/internal/models/catalog"
	"psychic-homily-backend/internal/services/catalog"
)

var (
	confirm   bool
	verbose   bool
	noSlugFix bool
	envFile   string
)

func main() {
	flag.BoolVar(&confirm, "confirm", false, "Apply changes (default: dry-run only)")
	flag.BoolVar(&verbose, "verbose", false, "Print per-cluster details")
	flag.BoolVar(&noSlugFix, "no-slug-fix", false, "Skip slug recanonicalisation pass on winners")
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
	fmt.Printf("=== Show Dedup (%s) — PSY-559 ===\n\n", mode)

	clusters, err := catalog.FindShowDedupClusters(database)
	if err != nil {
		log.Fatalf("find clusters: %v", err)
	}

	fmt.Printf("Found %d duplicate cluster(s).\n\n", len(clusters))
	if len(clusters) == 0 {
		fmt.Println("No duplicates to merge. Done.")
		return
	}

	summary := &catalog.ShowDedupSummary{ClustersFound: len(clusters)}

	for _, cluster := range clusters {
		printCluster(database, cluster)

		if !confirm {
			continue
		}

		// Each cluster runs in its own transaction. A failed merge on
		// one cluster doesn't roll back the others — keeps the cmd
		// safe to re-run on partially-completed state.
		err := database.Transaction(func(tx *gorm.DB) error {
			for _, loserID := range cluster.LoserIDs {
				if err := catalog.MergeDuplicateShow(tx, cluster.WinnerID, loserID, summary); err != nil {
					return fmt.Errorf("merge loser %d into winner %d: %w", loserID, cluster.WinnerID, err)
				}
			}
			if !noSlugFix {
				rewritten, err := catalog.RecanonicaliseShowSlug(tx, cluster.WinnerID)
				if err != nil {
					return fmt.Errorf("slug recanonicalise winner %d: %w", cluster.WinnerID, err)
				}
				if rewritten {
					summary.SlugsRewritten++
				}
			}
			return nil
		})
		if err != nil {
			fmt.Printf("  [ERROR] %v\n", err)
			continue
		}
		fmt.Printf("  [MERGED] winner=%d, losers=%v\n", cluster.WinnerID, cluster.LoserIDs)
	}

	// Slug recanonicalise on shows that were already in canonical
	// form is a separate pass for non-cluster shows? Out of scope —
	// the work plan only requires winners be backfilled to canonical
	// form, and the cluster loop handles that above.

	printSummary(summary)
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

func printCluster(database *gorm.DB, cluster catalog.ShowDedupCluster) {
	fmt.Printf("Cluster: artist=%d, venue=%d, event_date=%s\n",
		cluster.Key.ArtistID, cluster.Key.VenueID, cluster.Key.EventDate.UTC().Format("2006-01-02T15:04:05Z"))
	fmt.Printf("  Winner: show %d (created %s)\n", cluster.WinnerID, cluster.CreatedAt[0].UTC().Format("2006-01-02"))
	for i, loserID := range cluster.LoserIDs {
		// CreatedAt index for loser i is i+1 (winner is index 0).
		fmt.Printf("  Loser:  show %d (created %s)\n", loserID, cluster.CreatedAt[i+1].UTC().Format("2006-01-02"))
	}

	if !verbose {
		return
	}

	// Pull a one-line description of each show for reviewer context.
	for _, id := range cluster.ShowIDs {
		var s catalogm.Show
		if err := database.First(&s, id).Error; err != nil {
			fmt.Printf("    show %d: <load failed: %v>\n", id, err)
			continue
		}
		slug := "<no slug>"
		if s.Slug != nil {
			slug = *s.Slug
		}
		fmt.Printf("    show %d: title=%q slug=%s status=%s event_date=%s\n",
			s.ID, s.Title, slug, s.Status, s.EventDate.UTC().Format("2006-01-02T15:04:05Z"))
	}
}

func printSummary(s *catalog.ShowDedupSummary) {
	fmt.Println()
	fmt.Println("=== Summary ===")
	fmt.Printf("Clusters found:           %d\n", s.ClustersFound)
	fmt.Printf("Losers merged:            %d\n", s.LosersMerged)
	fmt.Println()
	fmt.Println("FK repointing:")
	fmt.Printf("  show_venues:            moved=%d skipped=%d\n", s.ShowVenuesMoved, s.ShowVenuesSkipped)
	fmt.Printf("  show_artists:           moved=%d skipped=%d\n", s.ShowArtistsMoved, s.ShowArtistsSkipped)
	fmt.Printf("  show_reports:           moved=%d\n", s.ShowReportsMoved)
	fmt.Printf("  enrichment_queue:       moved=%d\n", s.EnrichmentMoved)
	fmt.Printf("  duplicate_of_show_id:   repointed=%d\n", s.DuplicateOfRepoint)
	fmt.Println("Polymorphic refs:")
	fmt.Printf("  comments:               moved=%d\n", s.CommentsRepointed)
	fmt.Printf("  comment_subscriptions:  moved=%d skipped=%d\n", s.SubsRepointed, s.SubsSkipped)
	fmt.Printf("  entity_tags:            moved=%d skipped=%d\n", s.EntityTagsMoved, s.EntityTagsSkipped)
	fmt.Printf("  entity_reports:         moved=%d\n", s.EntityReportsMoved)
	fmt.Printf("  pending_entity_edits:   moved=%d\n", s.PendingEditsMoved)
	fmt.Printf("  revisions:              moved=%d\n", s.RevisionsMoved)
	fmt.Printf("  requests:               moved=%d\n", s.RequestsMoved)
	fmt.Printf("  audit_logs:             moved=%d\n", s.AuditLogsMoved)
	fmt.Printf("  collection_items:       moved=%d skipped=%d\n", s.CollectionsMoved, s.CollectionsSkipped)
	fmt.Printf("  user_bookmarks:         moved=%d skipped=%d\n", s.BookmarksMoved, s.BookmarksSkipped)
	fmt.Println()
	fmt.Printf("Slugs recanonicalised:    %d\n", s.SlugsRewritten)
	fmt.Println()
	if !confirm {
		fmt.Println("DRY RUN — no DB writes. Re-run with --confirm to apply.")
	} else {
		fmt.Println("LIVE — changes committed.")
	}

	if s.ClustersFound > 0 && !confirm {
		fmt.Println()
		fmt.Println("NOTE: dry-run prints the cluster plan, NOT per-FK movement counts;")
		fmt.Println("those numbers are populated only on --confirm because conflict-aware")
		fmt.Println("counts depend on actual UPDATE/DELETE row counts in a transaction.")
	}

	// Exit non-zero if we found duplicates and didn't confirm — useful
	// for CI/cron wrappers that want to alert on regression.
	if s.ClustersFound > 0 && !confirm {
		os.Exit(2)
	}
}
