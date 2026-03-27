package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"sort"
	"strings"

	"psychic-homily-backend/db"
	"psychic-homily-backend/internal/config"
	"psychic-homily-backend/internal/models"

	"github.com/joho/godotenv"
	"gorm.io/gorm"
)

// CLI flags
var (
	dryRun      bool
	verbose     bool
	mappingFile string
	interactive bool
)

func init() {
	flag.BoolVar(&dryRun, "dry-run", true, "Show what would be linked without writing to DB (default: true)")
	flag.BoolVar(&verbose, "verbose", false, "Show detailed output including skipped items")
	flag.StringVar(&mappingFile, "mapping-file", "", "Path to JSON file with explicit release-label mappings")
	flag.BoolVar(&interactive, "interactive", false, "Interactively assign labels to unlinked releases")
}

// unlinkedRelease holds a release that is not yet linked to any label
type unlinkedRelease struct {
	ID    uint
	Title string
}

// unlinkedReleaseWithArtist holds a release + its artist names for display
type unlinkedReleaseWithArtist struct {
	ID          uint
	Title       string
	ArtistNames []string
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

// mappingEntry represents a single entry from the mapping JSON file
type mappingEntry struct {
	ReleaseTitle  string  `json:"release_title"`
	LabelName     string  `json:"label_name"`
	CatalogNumber *string `json:"catalog_number,omitempty"`
}

// linkStats tracks statistics across all strategies
type linkStats struct {
	mappingMatches    int
	titleMatches      int
	artistMatches     int
	releaseInference  int
	interactiveLinks  int
	errors            int
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

	var stats linkStats

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

	if len(labels) == 0 && mappingFile == "" {
		fmt.Println("\nNo labels in database. Nothing to link.")
		return
	}

	fmt.Println()

	// Strategy 0: Manual mapping file (highest priority)
	if mappingFile != "" {
		fmt.Println("--- Strategy 0: Manual mapping file ---")
		runMappingFileStrategy(database, &unlinkedReleases, labels, &stats)
		fmt.Println()
	}

	// Strategy 1: Match by label name appearing in release title
	if len(labels) > 0 {
		fmt.Println("--- Strategy 1: Title-based matching ---")
		runTitleMatchStrategy(database, &unlinkedReleases, labels, &stats)
		fmt.Println()
	}

	// Strategy 2: Match via artist-label junction table
	fmt.Println("--- Strategy 2: Artist-label inference ---")
	runArtistLabelStrategy(database, &unlinkedReleases, labels, &stats)

	// Strategy 3: Match via existing release-label patterns
	fmt.Println("\n--- Strategy 3: Release-label inference ---")
	runReleaseLabelInferenceStrategy(database, &unlinkedReleases, labels, &stats)

	// Strategy 4: Interactive mode
	if interactive {
		fmt.Println("\n--- Strategy 4: Interactive assignment ---")
		runInteractiveStrategy(database, &unlinkedReleases, labels, &stats)
	}

	// Summary
	printSummary(&stats)
}

// runMappingFileStrategy loads a JSON mapping file and links releases to labels
func runMappingFileStrategy(database *gorm.DB, unlinkedReleases *[]unlinkedRelease, labels []labelInfo, stats *linkStats) {
	data, err := os.ReadFile(mappingFile)
	if err != nil {
		fmt.Printf("  [ERROR] Failed to read mapping file %s: %v\n", mappingFile, err)
		stats.errors++
		return
	}

	var mappings []mappingEntry
	if err := json.Unmarshal(data, &mappings); err != nil {
		fmt.Printf("  [ERROR] Failed to parse mapping file: %v\n", err)
		stats.errors++
		return
	}

	fmt.Printf("  Loaded %d mappings from %s\n", len(mappings), mappingFile)

	// Build a case-insensitive label lookup
	labelByName := buildLabelLookup(labels)

	for _, m := range mappings {
		if m.ReleaseTitle == "" || m.LabelName == "" {
			fmt.Printf("  [SKIP] Incomplete mapping entry: release=%q label=%q\n", m.ReleaseTitle, m.LabelName)
			continue
		}

		// Find the label
		label, ok := labelByName[strings.ToLower(m.LabelName)]
		if !ok {
			fmt.Printf("  [SKIP] Label not found in database: %q\n", m.LabelName)
			stats.errors++
			continue
		}

		// Find matching releases (case-insensitive exact match)
		matched := false
		for i := range *unlinkedReleases {
			rel := &(*unlinkedReleases)[i]
			if rel.ID == 0 {
				continue // Already linked
			}
			if strings.EqualFold(rel.Title, m.ReleaseTitle) {
				catNum := m.CatalogNumber
				fmt.Printf("  [MAPPING] Release: %q -> Label: %q", rel.Title, label.Name)
				if catNum != nil {
					fmt.Printf(" (catalog: %s)", *catNum)
				}
				fmt.Println()

				if !dryRun {
					if err := linkReleaseToLabel(database, rel.ID, label.ID, catNum); err != nil {
						fmt.Printf("    [ERROR] %v\n", err)
						stats.errors++
					} else {
						stats.mappingMatches++
					}
				} else {
					stats.mappingMatches++
				}
				rel.ID = 0 // Mark as linked
				matched = true
				break
			}
		}
		if !matched {
			fmt.Printf("  [SKIP] No unlinked release matches title: %q\n", m.ReleaseTitle)
		}
	}
}

// runTitleMatchStrategy matches releases to labels when the label name appears in the release title
func runTitleMatchStrategy(database *gorm.DB, unlinkedReleases *[]unlinkedRelease, labels []labelInfo, stats *linkStats) {
	// Sort labels longest name first so "Habibi Funk Records" matches before "Habibi Funk"
	sortedLabels := sortLabelsByNameLength(labels)

	for i := range *unlinkedReleases {
		rel := &(*unlinkedReleases)[i]
		if rel.ID == 0 {
			continue // Already linked by earlier strategy
		}
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
				// Mark as linked
				rel.ID = 0
				break
			}
		}
	}
}

// runArtistLabelStrategy links releases to labels based on artist_labels junction table
func runArtistLabelStrategy(database *gorm.DB, unlinkedReleases *[]unlinkedRelease, labels []labelInfo, stats *linkStats) {
	artistLabels := getArtistLabelPairs(database)
	fmt.Printf("  Found %d artist-label associations\n", len(artistLabels))

	for i := range *unlinkedReleases {
		rel := &(*unlinkedReleases)[i]
		if rel.ID == 0 {
			continue // Already linked
		}

		artistIDs := getArtistIDsForRelease(database, rel.ID)
		if len(artistIDs) == 0 {
			if verbose {
				fmt.Printf("  [SKIP] Release %q (ID: %d) has no artists\n", rel.Title, rel.ID)
			}
			continue
		}

		linked := false
		for _, artistID := range artistIDs {
			for _, al := range artistLabels {
				if al.ArtistID == artistID {
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
					linked = true
				}
			}
		}
		if linked {
			rel.ID = 0
		}
	}
}

// runReleaseLabelInferenceStrategy infers labels from existing release-label links for the same artist.
// If artist A has release R1 linked to label L, and artist A has unlinked release R2,
// infer R2 should also be on label L.
func runReleaseLabelInferenceStrategy(database *gorm.DB, unlinkedReleases *[]unlinkedRelease, labels []labelInfo, stats *linkStats) {
	// Build a map: artistID -> set of labelIDs (from existing release_labels)
	artistToLabels := getArtistLabelsFromReleases(database)
	fmt.Printf("  Found %d artists with existing release-label links\n", len(artistToLabels))

	if len(artistToLabels) == 0 {
		fmt.Println("  No existing release-label patterns to infer from.")
		return
	}

	for i := range *unlinkedReleases {
		rel := &(*unlinkedReleases)[i]
		if rel.ID == 0 {
			continue // Already linked
		}

		artistIDs := getArtistIDsForRelease(database, rel.ID)
		if len(artistIDs) == 0 {
			continue
		}

		linked := false
		for _, artistID := range artistIDs {
			labelIDs, ok := artistToLabels[artistID]
			if !ok {
				continue
			}
			for labelID := range labelIDs {
				labelName := getLabelName(labels, labelID)
				fmt.Printf("  [RELEASE INFER] Release: %q -> Label: %q (artist ID %d has other releases on this label)\n",
					rel.Title, labelName, artistID)

				if !dryRun {
					if err := linkReleaseToLabel(database, rel.ID, labelID, nil); err != nil {
						fmt.Printf("    [ERROR] %v\n", err)
						stats.errors++
					} else {
						stats.releaseInference++
					}
				} else {
					stats.releaseInference++
				}
				linked = true
			}
		}
		if linked {
			rel.ID = 0
		}
	}
}

// runInteractiveStrategy prompts the user to manually assign labels to each remaining unlinked release
func runInteractiveStrategy(database *gorm.DB, unlinkedReleases *[]unlinkedRelease, labels []labelInfo, stats *linkStats) {
	// Count remaining unlinked releases
	remaining := 0
	for _, rel := range *unlinkedReleases {
		if rel.ID != 0 {
			remaining++
		}
	}

	if remaining == 0 {
		fmt.Println("  No unlinked releases remaining. Nothing to do interactively.")
		return
	}

	fmt.Printf("  %d unlinked releases remaining.\n", remaining)
	fmt.Println("  For each release, type a label name (or partial name to search), or press Enter to skip.")
	fmt.Println("  Type 'q' to quit interactive mode.")
	fmt.Println()

	reader := bufio.NewReader(os.Stdin)
	count := 0

	for i := range *unlinkedReleases {
		rel := &(*unlinkedReleases)[i]
		if rel.ID == 0 {
			continue // Already linked
		}

		count++

		// Get artist names for context
		artistNames := getArtistNamesForRelease(database, rel.ID)
		artistStr := "(no artists)"
		if len(artistNames) > 0 {
			artistStr = strings.Join(artistNames, ", ")
		}

		fmt.Printf("  [%d/%d] Release: %q\n", count, remaining, rel.Title)
		fmt.Printf("         Artist(s): %s\n", artistStr)
		fmt.Printf("         Label name (or search term, Enter to skip, 'q' to quit): ")

		line, _ := reader.ReadString('\n')
		input := strings.TrimSpace(line)

		if input == "" {
			continue // Skip
		}
		if strings.EqualFold(input, "q") {
			fmt.Println("  Exiting interactive mode.")
			break
		}

		// Search for matching labels
		matchedLabel := findLabelInteractive(reader, labels, input)
		if matchedLabel == nil {
			fmt.Println("         Skipped (no label selected).")
			continue
		}

		fmt.Printf("         -> Linking to label: %q\n", matchedLabel.Name)

		if !dryRun {
			if err := linkReleaseToLabel(database, rel.ID, matchedLabel.ID, nil); err != nil {
				fmt.Printf("         [ERROR] %v\n", err)
				stats.errors++
			} else {
				stats.interactiveLinks++
				rel.ID = 0
			}
		} else {
			stats.interactiveLinks++
			rel.ID = 0
		}
		fmt.Println()
	}
}

// findLabelInteractive searches labels by the user's input and returns the selected one
func findLabelInteractive(reader *bufio.Reader, labels []labelInfo, query string) *labelInfo {
	queryLower := strings.ToLower(query)

	// First try exact match (case-insensitive)
	for _, l := range labels {
		if strings.EqualFold(l.Name, query) {
			return &l
		}
	}

	// Then try substring match
	var matches []labelInfo
	for _, l := range labels {
		if strings.Contains(strings.ToLower(l.Name), queryLower) {
			matches = append(matches, l)
		}
	}

	if len(matches) == 0 {
		fmt.Printf("         No labels matching %q found.\n", query)
		return nil
	}

	if len(matches) == 1 {
		return &matches[0]
	}

	// Multiple matches: let user pick
	fmt.Printf("         Found %d matching labels:\n", len(matches))
	for i, m := range matches {
		if i >= 20 {
			fmt.Printf("         ... and %d more (try a more specific search)\n", len(matches)-20)
			break
		}
		fmt.Printf("           %d) %s\n", i+1, m.Name)
	}

	fmt.Printf("         Pick a number (or Enter to skip): ")
	line, _ := reader.ReadString('\n')
	choice := strings.TrimSpace(line)

	if choice == "" {
		return nil
	}

	var idx int
	if _, err := fmt.Sscanf(choice, "%d", &idx); err != nil || idx < 1 || idx > len(matches) {
		fmt.Println("         Invalid selection.")
		return nil
	}

	return &matches[idx-1]
}

// getArtistLabelsFromReleases builds a map of artistID -> set of labelIDs
// by looking at which labels already-linked releases belong to.
func getArtistLabelsFromReleases(database *gorm.DB) map[uint]map[uint]bool {
	type artistLabelRow struct {
		ArtistID uint
		LabelID  uint
	}
	var rows []artistLabelRow
	database.Raw(`
		SELECT DISTINCT ar.artist_id, rl.label_id
		FROM artist_releases ar
		JOIN release_labels rl ON ar.release_id = rl.release_id
	`).Scan(&rows)

	result := make(map[uint]map[uint]bool)
	for _, r := range rows {
		if result[r.ArtistID] == nil {
			result[r.ArtistID] = make(map[uint]bool)
		}
		result[r.ArtistID][r.LabelID] = true
	}
	return result
}

// getArtistNamesForRelease returns artist names for a release (for display in interactive mode)
func getArtistNamesForRelease(database *gorm.DB, releaseID uint) []string {
	var names []string
	database.Raw(`
		SELECT a.name
		FROM artists a
		JOIN artist_releases ar ON a.id = ar.artist_id
		WHERE ar.release_id = ?
		ORDER BY ar.position, a.name
	`, releaseID).Pluck("name", &names)
	return names
}

// buildLabelLookup creates a case-insensitive name -> labelInfo map
func buildLabelLookup(labels []labelInfo) map[string]labelInfo {
	m := make(map[string]labelInfo, len(labels))
	for _, l := range labels {
		m[strings.ToLower(l.Name)] = l
	}
	return m
}

// printSummary outputs the final stats
func printSummary(stats *linkStats) {
	fmt.Println("\n=== Summary ===")
	if stats.mappingMatches > 0 {
		fmt.Printf("  Mapping file matches:    %d\n", stats.mappingMatches)
	}
	fmt.Printf("  Title-based matches:     %d\n", stats.titleMatches)
	fmt.Printf("  Artist-label matches:    %d\n", stats.artistMatches)
	fmt.Printf("  Release-label inference: %d\n", stats.releaseInference)
	if stats.interactiveLinks > 0 {
		fmt.Printf("  Interactive links:       %d\n", stats.interactiveLinks)
	}
	fmt.Printf("  Errors:                  %d\n", stats.errors)
	total := stats.mappingMatches + stats.titleMatches + stats.artistMatches + stats.releaseInference + stats.interactiveLinks
	fmt.Printf("  Total links created:     %d\n", total)
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
	sort.Slice(sorted, func(i, j int) bool {
		return len(sorted[i].Name) > len(sorted[j].Name)
	})
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
