// Command upgrade-scene-scopes migrates fallback scene-registry rows
// (metro IS NULL) whose city now resolves to a Census CBSA into the metro's
// canonical row (PSY-1339, spike doc Q3).
//
// This is the realistic scene-identity drift: a geocoder improvement (e.g. the
// PSY-1276 "St."→"Saint" alias) makes a previously unresolvable city resolve
// to a metro. New follows already land on the metro row (get-or-create tries
// metro resolution first); this command moves the OLD fallback row's follows
// over and deletes it, so a scene never splits across two identities.
//
// Follows are moved by updating user_bookmarks.entity_id; a user who follows
// both rows (possible only across the drift window) keeps one follow — the
// duplicate is deleted rather than violating the unique index.
//
// Usage:
//
//	go run ./cmd/upgrade-scene-scopes                  # dry-run (default)
//	go run ./cmd/upgrade-scene-scopes --confirm        # apply
//	go run ./cmd/upgrade-scene-scopes --env .env.stage # target a specific env
package main

import (
	"flag"
	"fmt"
	"log"
	"strings"

	"github.com/joho/godotenv"
	"gorm.io/gorm"

	"psychic-homily-backend/db"
	"psychic-homily-backend/internal/config"
	catalogm "psychic-homily-backend/internal/models/catalog"
	"psychic-homily-backend/internal/services/geo"
)

var (
	confirm bool
	envFile string
)

const usCountry = "US"

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
	g := geo.Default()

	mode := "DRY RUN"
	if confirm {
		mode = "LIVE"
	}
	fmt.Printf("=== Scene Scope Upgrade (%s) — PSY-1339 ===\n", mode)

	var fallbacks []catalogm.Scene
	if err := database.Where("metro IS NULL").Order("id").Find(&fallbacks).Error; err != nil {
		log.Fatalf("list fallback scenes: %v", err)
	}
	fmt.Printf("fallback scene rows: %d\n", len(fallbacks))

	upgraded, skipped := 0, 0
	for _, row := range fallbacks {
		m, ok := g.ResolveMetro(row.City, row.State, usCountry)
		if !ok {
			skipped++
			continue
		}
		mp, ok := geo.MetroPrincipalByCBSA(m.CBSACode)
		if !ok {
			skipped++
			continue
		}
		fmt.Printf("UPGRADE scene %d (%s, %s / %s) → metro %s (%s, %s)\n",
			row.ID, row.City, row.State, row.Slug, m.CBSACode, mp.City, mp.State)
		upgraded++
		if !confirm {
			continue
		}
		if err := upgradeRow(database, row, m.CBSACode, mp.City, mp.State); err != nil {
			log.Fatalf("upgrade scene %d: %v", row.ID, err)
		}
	}

	fmt.Printf("\nupgradable: %d, still-fallback: %d\n", upgraded, skipped)

	// Orphan sweep: a follow can land on a row id in the instant between this
	// command's merge transaction and a concurrent request that resolved the
	// old id (user_bookmarks is polymorphic — no FK). Report always; delete
	// only with --confirm.
	var orphans int64
	if err := database.Raw(`
		SELECT COUNT(*) FROM user_bookmarks b
		WHERE b.entity_type = 'scene'
		  AND NOT EXISTS (SELECT 1 FROM scenes sc WHERE sc.id = b.entity_id)`,
	).Scan(&orphans).Error; err != nil {
		log.Fatalf("count orphaned scene follows: %v", err)
	}
	if orphans > 0 {
		fmt.Printf("orphaned scene follows (no scenes row): %d\n", orphans)
		if confirm {
			if err := database.Exec(`
				DELETE FROM user_bookmarks b
				WHERE b.entity_type = 'scene'
				  AND NOT EXISTS (SELECT 1 FROM scenes sc WHERE sc.id = b.entity_id)`,
			).Error; err != nil {
				log.Fatalf("sweep orphaned scene follows: %v", err)
			}
			fmt.Println("orphaned scene follows swept")
		}
	}

	if !confirm && upgraded > 0 {
		fmt.Println("dry-run only — re-run with --confirm to apply")
	}
}

// upgradeRow merges one fallback row into its metro identity inside a
// transaction. When no metro row exists, the fallback row itself is upgraded
// IN PLACE (set metro + principal city/state/slug) — this is both the common
// case and the only correct handling when the fallback row already holds the
// canonical slug (an insert would no-op on the slug index and strand the row).
// When a metro row already exists, the fallback row's follows (deduped) and
// description (if the target has none) move over and the fallback row is
// deleted.
func upgradeRow(database *gorm.DB, row catalogm.Scene, cbsa, principalCity, principalState string) error {
	slug := buildSceneSlug(principalCity, principalState)
	return database.Transaction(func(tx *gorm.DB) error {
		var target catalogm.Scene
		res := tx.Where("metro = ?", cbsa).Limit(1).Find(&target)
		if res.Error != nil {
			return fmt.Errorf("load metro row: %w", res.Error)
		}
		if res.RowsAffected == 0 {
			// No metro row — upgrade the fallback row in place. Another
			// fallback row may hold the canonical slug (e.g. the principal
			// city's own fallback row while we're upgrading a member city's):
			// upgrade THAT one instead and fall through to merge ours into it.
			holder := row
			if row.Slug != slug {
				var bySlug catalogm.Scene
				r := tx.Where("slug = ?", slug).Limit(1).Find(&bySlug)
				if r.Error != nil {
					return fmt.Errorf("load slug row: %w", r.Error)
				}
				if r.RowsAffected > 0 {
					holder = bySlug
				}
			}
			if err := tx.Model(&catalogm.Scene{}).Where("id = ?", holder.ID).
				Updates(map[string]any{
					"metro": cbsa, "city": principalCity, "state": principalState, "slug": slug,
				}).Error; err != nil {
				return fmt.Errorf("upgrade row in place: %w", err)
			}
			if holder.ID == row.ID {
				return nil
			}
			target = holder
			target.Metro = &cbsa
		}
		if target.ID == row.ID {
			return nil
		}
		// Carry a curated description over if the target has none.
		if row.Description != nil {
			if err := tx.Exec(
				`UPDATE scenes SET description = COALESCE(description, ?) WHERE id = ?`,
				*row.Description, target.ID,
			).Error; err != nil {
				return fmt.Errorf("carry description: %w", err)
			}
		}
		// A user following BOTH rows would collide on the unique index — drop
		// the fallback-row duplicate first, then move the rest.
		if err := tx.Exec(`
			DELETE FROM user_bookmarks b
			WHERE b.entity_type = 'scene' AND b.entity_id = ?
			  AND EXISTS (
			    SELECT 1 FROM user_bookmarks t
			    WHERE t.entity_type = 'scene' AND t.entity_id = ?
			      AND t.user_id = b.user_id AND t.action = b.action
			  )`, row.ID, target.ID).Error; err != nil {
			return fmt.Errorf("dedupe follows: %w", err)
		}
		if err := tx.Exec(
			`UPDATE user_bookmarks SET entity_id = ? WHERE entity_type = 'scene' AND entity_id = ?`,
			target.ID, row.ID,
		).Error; err != nil {
			return fmt.Errorf("move follows: %w", err)
		}
		if err := tx.Delete(&catalogm.Scene{}, row.ID).Error; err != nil {
			return fmt.Errorf("delete fallback row: %w", err)
		}
		return nil
	})
}

// buildSceneSlug mirrors catalog.buildSceneSlug (unexported there — keep in
// sync; it's a 2-line format pinned by TestBuildSceneSlug).
func buildSceneSlug(city, state string) string {
	return strings.ToLower(strings.ReplaceAll(city, " ", "-")) + "-" + strings.ToLower(state)
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
