package main

import (
	"flag"
	"fmt"
	"log"
	"strings"

	"psychic-homily-backend/db"
	"psychic-homily-backend/internal/config"
	"psychic-homily-backend/internal/models"

	"github.com/joho/godotenv"
	"gorm.io/gorm"
)

// CLI flags
var (
	dryRun  bool
	verbose bool
)

func init() {
	flag.BoolVar(&dryRun, "dry-run", true, "Show what would be linked without writing to DB (default: true)")
	flag.BoolVar(&verbose, "verbose", false, "Show detailed output including skipped items")
}

// unlinkdRelease holds a release that is not yet linked to any label
type unlinkedRelease struct {
	ID    uint
	Title string
}

// labelInfo holds label ID and name for matching
type labelInfo struct {
	ID   uint
	Name string
}

// artistLabelPair holds an artist's label association
type artistLabelPair struct {
	ArtistID uint
	LabelID  uint
}

func main() {
	flag.Parse()

	fmt.Println("=== Release-Label Linker ===")
	if dryRun {
		fmt.Println("Mode: DRY RUN (no database writes)")
	} else {
		fmt.Println("Mode: LIVE (will write to database)")
	}
	fmt.Println()

	database := connectToDatabase()

	// Track stats
	var stats struct {
		titleMatches  int
		artistMatches int
		alreadyLinked int
		errors        int
	}

	// 1. Get all releases that have no label link
	unlinkedReleases := getUnlinkedReleases(database)
	fmt.Printf("Found %d releases with no label link\n", len(unlinkedReleases))

	// 2. Get all labels
	labels := getAllLabels(database)
	fmt.Printf("Found %d labels in database\n", len(labels))

	if len(unlinkedReleases) == 0 {
		fmt.Println("\nAll releases are already linked to labels. Nothing to do.")
		return
	}

	if len(labels) == 0 {
		fmt.Println("\nNo labels in database. Nothing to link.")
		return
	}

	fmt.Println()

	// 3. Strategy 1: Match by label name appearing in release title
	fmt.Println("--- Strategy 1: Title-based matching ---")
	// Sort labels longest name first so "Habibi Funk Records" matches before "Habibi Funk"
	// (more specific match wins)
	sortedLabels := sortLabelsByNameLength(labels)

	for i := range unlinkedReleases {
		rel := &unlinkedReleases[i]
		titleLower := strings.ToLower(rel.Title)

		for _, label := range sortedLabels {
			labelLower := strings.ToLower(label.Name)

			// Skip very short label names (3 chars or less) to avoid false positives
			if len(labelLower) <= 3 {
				continue
			}

			if strings.Contains(titleLower, labelLower) {
				// Extract potential catalog number from the part after the label name
				catalogNumber := extractCatalogNumber(rel.Title, label.Name)

				fmt.Printf("  [TITLE MATCH] Release: %q -> Label: %q", rel.Title, label.Name)
				if catalogNumber != nil {
					fmt.Printf(" (catalog: %s)", *catalogNumber)
				}
				fmt.Println()

				if !dryRun {
					if err := linkReleaseToLabel(database, rel.ID, label.ID, catalogNumber); err != nil {
						fmt.Printf("    [ERROR] %v\n", err)
						stats.errors++
					} else {
						stats.titleMatches++
					}
				} else {
					stats.titleMatches++
				}
				// Mark as linked (remove from unlinked set for strategy 2)
				rel.ID = 0
				break
			}
		}
	}

	// 4. Strategy 2: Match via artist overlap
	// If an artist is on a label (artist_labels) and has a release, link that release to the label
	fmt.Println("\n--- Strategy 2: Artist-label inference ---")

	// Get all artist-label pairs
	artistLabels := getArtistLabelPairs(database)
	fmt.Printf("  Found %d artist-label associations\n", len(artistLabels))

	// Get artist-release pairs for unlinked releases
	for _, rel := range unlinkedReleases {
		if rel.ID == 0 {
			continue // Already linked by strategy 1
		}

		// Get artists for this release
		artistIDs := getArtistIDsForRelease(database, rel.ID)
		if len(artistIDs) == 0 {
			if verbose {
				fmt.Printf("  [SKIP] Release %q (ID: %d) has no artists\n", rel.Title, rel.ID)
			}
			continue
		}

		// Check if any of these artists are on a label
		for _, artistID := range artistIDs {
			for _, al := range artistLabels {
				if al.ArtistID == artistID {
					// Found a match: artist is on a label, link the release
					labelName := getLabelName(labels, al.LabelID)
					fmt.Printf("  [ARTIST MATCH] Release: %q -> Label: %q (via artist ID %d)\n",
						rel.Title, labelName, artistID)

					if !dryRun {
						if err := linkReleaseToLabel(database, rel.ID, al.LabelID, nil); err != nil {
							fmt.Printf("    [ERROR] %v\n", err)
							stats.errors++
						} else {
							stats.artistMatches++
						}
					} else {
						stats.artistMatches++
					}
				}
			}
		}
	}

	// Summary
	fmt.Println("\n=== Summary ===")
	fmt.Printf("  Title-based matches:  %d\n", stats.titleMatches)
	fmt.Printf("  Artist-based matches: %d\n", stats.artistMatches)
	fmt.Printf("  Errors:               %d\n", stats.errors)
	total := stats.titleMatches + stats.artistMatches
	fmt.Printf("  Total links created:  %d\n", total)
	if dryRun {
		fmt.Println("\n  (DRY RUN - no changes were made)")
		fmt.Println("  Run with --dry-run=false to apply changes")
	}
}

// getUnlinkedReleases returns releases that have no entry in release_labels
func getUnlinkedReleases(database *gorm.DB) []unlinkedRelease {
	var releases []unlinkedRelease
	database.Raw(`
		SELECT r.id, r.title
		FROM releases r
		LEFT JOIN release_labels rl ON r.id = rl.release_id
		WHERE rl.release_id IS NULL
		ORDER BY r.title
	`).Scan(&releases)
	return releases
}

// getAllLabels returns all labels in the database
func getAllLabels(database *gorm.DB) []labelInfo {
	var labels []labelInfo
	database.Model(&models.Label{}).Select("id, name").Order("name").Find(&labels)
	return labels
}

// getArtistLabelPairs returns all artist-label associations
func getArtistLabelPairs(database *gorm.DB) []artistLabelPair {
	var pairs []artistLabelPair
	database.Model(&models.ArtistLabel{}).Select("artist_id, label_id").Find(&pairs)
	return pairs
}

// getArtistIDsForRelease returns artist IDs associated with a release
func getArtistIDsForRelease(database *gorm.DB, releaseID uint) []uint {
	var ids []uint
	database.Model(&models.ArtistRelease{}).
		Where("release_id = ?", releaseID).
		Pluck("artist_id", &ids)
	return ids
}

// linkReleaseToLabel creates a release_labels entry
func linkReleaseToLabel(database *gorm.DB, releaseID, labelID uint, catalogNumber *string) error {
	rl := models.ReleaseLabel{
		ReleaseID:     releaseID,
		LabelID:       labelID,
		CatalogNumber: catalogNumber,
	}

	// Idempotent: skip if already exists
	var count int64
	database.Model(&models.ReleaseLabel{}).
		Where("release_id = ? AND label_id = ?", releaseID, labelID).
		Count(&count)
	if count > 0 {
		return nil
	}

	return database.Create(&rl).Error
}

// extractCatalogNumber tries to extract a catalog number from the release title
// given the label name. E.g., "Habibi Funk 031: ..." with label "Habibi Funk" -> "031"
func extractCatalogNumber(title, labelName string) *string {
	titleLower := strings.ToLower(title)
	labelLower := strings.ToLower(labelName)

	idx := strings.Index(titleLower, labelLower)
	if idx < 0 {
		return nil
	}

	// Get the part after the label name
	remainder := strings.TrimSpace(title[idx+len(labelName):])

	// Check if it starts with a number or catalog-like pattern (e.g., "031:", "031 -", "#031")
	remainder = strings.TrimLeft(remainder, " #")

	// Extract digits/alphanumeric catalog number
	var catNum strings.Builder
	for _, ch := range remainder {
		if (ch >= '0' && ch <= '9') || (ch >= 'A' && ch <= 'Z') || (ch >= 'a' && ch <= 'z') || ch == '-' {
			catNum.WriteRune(ch)
		} else {
			break
		}
	}

	result := catNum.String()
	if result == "" {
		return nil
	}

	// Only return if it looks like a catalog number (not too long, has digits)
	if len(result) > 20 {
		return nil
	}
	hasDigit := false
	for _, ch := range result {
		if ch >= '0' && ch <= '9' {
			hasDigit = true
			break
		}
	}
	if !hasDigit {
		return nil
	}

	return &result
}

// sortLabelsByNameLength returns labels sorted by name length (longest first)
func sortLabelsByNameLength(labels []labelInfo) []labelInfo {
	sorted := make([]labelInfo, len(labels))
	copy(sorted, labels)

	// Simple insertion sort (label count is small)
	for i := 1; i < len(sorted); i++ {
		key := sorted[i]
		j := i - 1
		for j >= 0 && len(sorted[j].Name) < len(key.Name) {
			sorted[j+1] = sorted[j]
			j--
		}
		sorted[j+1] = key
	}

	return sorted
}

// getLabelName returns the label name for a given ID
func getLabelName(labels []labelInfo, labelID uint) string {
	for _, l := range labels {
		if l.ID == labelID {
			return l.Name
		}
	}
	return fmt.Sprintf("(label %d)", labelID)
}

func connectToDatabase() *gorm.DB {
	envFile := fmt.Sprintf(".env.%s", config.GetEnv("NODE_ENV", "development"))

	if err := godotenv.Load(envFile); err != nil {
		log.Printf("Warning: %s file not found, trying .env: %v", envFile, err)
		if err := godotenv.Load(); err != nil {
			log.Printf("Warning: no .env file found: %v", err)
		}
	}

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	if err := db.Connect(cfg); err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}

	return db.GetDB()
}
