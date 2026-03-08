package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/joho/godotenv"
	"gorm.io/gorm"

	"psychic-homily-backend/db"
	"psychic-homily-backend/internal/config"
	"psychic-homily-backend/internal/models"
	"psychic-homily-backend/internal/utils"
)

const (
	mbBaseURL   = "https://musicbrainz.org/ws/2"
	mbUserAgent = "PsychicHomily/1.0 (https://psychichomily.com)"
	// MusicBrainz rate limit: 1 request per second
	mbRateLimit = 1100 * time.Millisecond // slightly over 1s to be safe
	// Minimum score to accept a MusicBrainz match
	mbMinScore = 90
)

// --- MusicBrainz API response types ---

// MBArtistSearchResponse is the response from the MusicBrainz artist search endpoint
type MBArtistSearchResponse struct {
	Artists []MBArtist `json:"artists"`
}

// MBArtist represents an artist from MusicBrainz
type MBArtist struct {
	ID               string  `json:"id"`
	Name             string  `json:"name"`
	SortName         string  `json:"sort-name"`
	Score            int     `json:"score"`
	Disambiguation   string  `json:"disambiguation"`
	Type             string  `json:"type"`
	Country          string  `json:"country"`
	Area             *MBArea `json:"area"`
	BeginArea        *MBArea `json:"begin-area"`
	LifeSpan         *MBLifeSpan `json:"life-span"`
}

// MBArea represents a geographic area from MusicBrainz
type MBArea struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Type string `json:"type"`
}

// MBLifeSpan represents the life span of an artist
type MBLifeSpan struct {
	Begin string `json:"begin"`
	End   string `json:"end"`
	Ended bool   `json:"ended"`
}

// MBReleaseSearchResponse is the response from the MusicBrainz release endpoint
type MBReleaseSearchResponse struct {
	Releases     []MBRelease `json:"releases"`
	ReleaseCount int         `json:"release-count"`
	Offset       int         `json:"offset"`
}

// MBRelease represents a release from MusicBrainz
type MBRelease struct {
	ID               string             `json:"id"`
	Title            string             `json:"title"`
	Date             string             `json:"date"`
	Country          string             `json:"country"`
	Status           string             `json:"status"`
	ReleaseGroup     *MBReleaseGroup    `json:"release-group"`
	LabelInfo        []MBLabelInfo      `json:"label-info"`
	ArtistCredit     []MBArtistCredit   `json:"artist-credit"`
}

// MBReleaseGroup represents a release group from MusicBrainz
type MBReleaseGroup struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	PrimaryType string `json:"primary-type"`
	SecondaryTypes []string `json:"secondary-types"`
}

// MBLabelInfo represents label info on a release
type MBLabelInfo struct {
	CatalogNumber string   `json:"catalog-number"`
	Label         *MBLabel `json:"label"`
}

// MBLabel represents a label from MusicBrainz
type MBLabel struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// MBArtistCredit represents an artist credit on a release
type MBArtistCredit struct {
	Artist   MBArtist `json:"artist"`
	JoinPhrase string `json:"joinphrase"`
}

// MBArtistRelResponse is the response for artist label relations
type MBArtistRelResponse struct {
	Relations []MBRelation `json:"relations"`
}

// MBRelation represents a relation from MusicBrainz
type MBRelation struct {
	Type       string   `json:"type"`
	TargetType string   `json:"target-type"`
	Label      *MBLabel `json:"label"`
}

// --- CLI flags ---

var (
	dryRun    bool
	artist    string
	confirm   bool
	limitFlag int
	verbose   bool
)

func init() {
	flag.BoolVar(&dryRun, "dry-run", true, "Show what would be imported without writing to DB (default: true)")
	flag.StringVar(&artist, "artist", "", "Process a single artist by name")
	flag.BoolVar(&confirm, "confirm", false, "Prompt before each import")
	flag.IntVar(&limitFlag, "limit", 0, "Process only N artists (0 = all)")
	flag.BoolVar(&verbose, "verbose", false, "Show verbose output")
}

func main() {
	flag.Parse()

	fmt.Println("=== MusicBrainz Data Seeding Tool ===")
	fmt.Println()

	if dryRun {
		fmt.Println("[DRY RUN] No database writes will be performed.")
	} else {
		fmt.Println("[LIVE] Database writes ENABLED.")
	}
	fmt.Println()

	// Connect to database
	database := connectToDatabase()

	// Get artists to process
	artists := getArtistsToProcess(database)

	if len(artists) == 0 {
		fmt.Println("No artists found to process.")
		return
	}

	fmt.Printf("Found %d artist(s) to process.\n\n", len(artists))

	// Track statistics
	var stats struct {
		processed     int
		matched       int
		skipped       int
		releasesFound int
		releasesNew   int
		labelsFound   int
		labelsNew     int
		errors        int
	}

	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	for i, a := range artists {
		stats.processed++
		fmt.Printf("--- [%d/%d] Processing: %s (ID: %d) ---\n", i+1, len(artists), a.Name, a.ID)

		// Search MusicBrainz for the artist
		mbArtist, err := searchMBArtist(client, a.Name)
		if err != nil {
			fmt.Printf("  [ERROR] MusicBrainz search failed: %v\n", err)
			stats.errors++
			continue
		}

		if mbArtist == nil {
			fmt.Printf("  [SKIP] No match found (score >= %d)\n", mbMinScore)
			stats.skipped++
			continue
		}

		// Display match details
		printMatchDetails(mbArtist)

		// In confirm mode, prompt the user
		if confirm {
			if !promptYesNo("  Accept this match?") {
				fmt.Println("  [SKIP] Match rejected by user.")
				stats.skipped++
				continue
			}
		}

		stats.matched++
		fmt.Printf("  [MATCH] Accepted: %s (MBID: %s)\n", mbArtist.Name, mbArtist.ID)

		// Fetch releases for the matched artist
		time.Sleep(mbRateLimit)
		releases, err := fetchMBReleases(client, mbArtist.ID)
		if err != nil {
			fmt.Printf("  [ERROR] Failed to fetch releases: %v\n", err)
			stats.errors++
			continue
		}

		// Deduplicate releases by release group to avoid duplicate editions
		releases = deduplicateReleases(releases)

		fmt.Printf("  Found %d unique release(s)\n", len(releases))
		stats.releasesFound += len(releases)

		// Process each release
		for _, rel := range releases {
			releaseType := mapMBReleaseType(rel)
			yearStr := ""
			year := extractYear(rel.Date)
			if year > 0 {
				yearStr = fmt.Sprintf(" (%d)", year)
			}

			// Check if release already exists for this artist
			exists := releaseExistsForArtist(database, a.ID, rel.Title)
			if exists {
				if verbose {
					fmt.Printf("    [EXISTS] %s%s [%s]\n", rel.Title, yearStr, releaseType)
				}
				continue
			}

			fmt.Printf("    [NEW] %s%s [%s]\n", rel.Title, yearStr, releaseType)
			stats.releasesNew++

			if !dryRun {
				if confirm {
					if !promptYesNo(fmt.Sprintf("    Import release '%s'?", rel.Title)) {
						fmt.Println("    [SKIP] Release skipped by user.")
						stats.releasesNew--
						continue
					}
				}
				if err := createRelease(database, a.ID, rel); err != nil {
					fmt.Printf("    [ERROR] Failed to create release: %v\n", err)
					stats.errors++
					stats.releasesNew--
				}
			}
		}

		// Fetch label relations for the matched artist
		time.Sleep(mbRateLimit)
		labels, err := fetchMBArtistLabels(client, mbArtist.ID)
		if err != nil {
			fmt.Printf("  [ERROR] Failed to fetch label relations: %v\n", err)
			stats.errors++
			continue
		}

		// Also collect labels from release label-info
		releaseLabels := collectReleaseLabels(releases)
		labels = mergeLabels(labels, releaseLabels)

		fmt.Printf("  Found %d label(s)\n", len(labels))
		stats.labelsFound += len(labels)

		// Process each label
		for _, lbl := range labels {
			// Check if label already exists
			existingLabel := findLabelByName(database, lbl.Name)

			if existingLabel != nil {
				// Check if artist-label association exists
				if !artistLabelExists(database, a.ID, existingLabel.ID) {
					fmt.Printf("    [LINK] %s (existing label ID: %d)\n", lbl.Name, existingLabel.ID)
					if !dryRun {
						if confirm {
							if !promptYesNo(fmt.Sprintf("    Link artist to label '%s'?", lbl.Name)) {
								fmt.Println("    [SKIP] Link skipped by user.")
								continue
							}
						}
						if err := createArtistLabelLink(database, a.ID, existingLabel.ID); err != nil {
							fmt.Printf("    [ERROR] Failed to link artist to label: %v\n", err)
							stats.errors++
						}
					}
				} else if verbose {
					fmt.Printf("    [EXISTS] %s (already linked)\n", lbl.Name)
				}
			} else {
				fmt.Printf("    [NEW] %s\n", lbl.Name)
				stats.labelsNew++

				if !dryRun {
					if confirm {
						if !promptYesNo(fmt.Sprintf("    Create label '%s' and link to artist?", lbl.Name)) {
							fmt.Println("    [SKIP] Label skipped by user.")
							stats.labelsNew--
							continue
						}
					}
					labelID, err := createLabel(database, lbl.Name)
					if err != nil {
						fmt.Printf("    [ERROR] Failed to create label: %v\n", err)
						stats.errors++
						stats.labelsNew--
						continue
					}
					if err := createArtistLabelLink(database, a.ID, labelID); err != nil {
						fmt.Printf("    [ERROR] Failed to link artist to label: %v\n", err)
						stats.errors++
					}
				}
			}
		}

		fmt.Println()
	}

	// Print summary
	fmt.Println("=== Summary ===")
	fmt.Printf("Artists processed:   %d\n", stats.processed)
	fmt.Printf("Artists matched:     %d\n", stats.matched)
	fmt.Printf("Artists skipped:     %d\n", stats.skipped)
	fmt.Printf("Releases found:      %d\n", stats.releasesFound)
	fmt.Printf("New releases:        %d\n", stats.releasesNew)
	fmt.Printf("Labels found:        %d\n", stats.labelsFound)
	fmt.Printf("New labels:          %d\n", stats.labelsNew)
	fmt.Printf("Errors:              %d\n", stats.errors)
	if dryRun {
		fmt.Println("\n[DRY RUN] No changes were made. Run with --dry-run=false to import.")
	}
}

// --- Database helpers ---

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

func getArtistsToProcess(database *gorm.DB) []models.Artist {
	var artists []models.Artist

	query := database.Model(&models.Artist{}).Order("name ASC")

	if artist != "" {
		// Process a single artist by name (case-insensitive)
		query = query.Where("LOWER(name) = LOWER(?)", artist)
	}

	if limitFlag > 0 {
		query = query.Limit(limitFlag)
	}

	if err := query.Find(&artists).Error; err != nil {
		log.Fatalf("Failed to query artists: %v", err)
	}

	return artists
}

func releaseExistsForArtist(database *gorm.DB, artistID uint, title string) bool {
	var count int64
	database.Table("releases").
		Joins("JOIN artist_releases ON artist_releases.release_id = releases.id").
		Where("artist_releases.artist_id = ? AND LOWER(releases.title) = LOWER(?)", artistID, title).
		Count(&count)
	return count > 0
}

func createRelease(database *gorm.DB, artistID uint, mbRelease MBRelease) error {
	releaseType := mapMBReleaseType(mbRelease)
	year := extractYear(mbRelease.Date)

	// Generate unique slug
	baseSlug := utils.GenerateArtistSlug(mbRelease.Title)
	slug := utils.GenerateUniqueSlug(baseSlug, func(candidate string) bool {
		var count int64
		database.Model(&models.Release{}).Where("slug = ?", candidate).Count(&count)
		return count > 0
	})

	release := &models.Release{
		Title:       mbRelease.Title,
		Slug:        &slug,
		ReleaseType: models.ReleaseType(releaseType),
	}
	if year > 0 {
		release.ReleaseYear = &year
	}
	if mbRelease.Date != "" && len(mbRelease.Date) == 10 {
		release.ReleaseDate = &mbRelease.Date
	}

	return database.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(release).Error; err != nil {
			return fmt.Errorf("failed to create release: %w", err)
		}

		// Create artist-release link
		ar := &models.ArtistRelease{
			ArtistID:  artistID,
			ReleaseID: release.ID,
			Role:      models.ArtistReleaseRoleMain,
			Position:  0,
		}
		if err := tx.Create(ar).Error; err != nil {
			return fmt.Errorf("failed to create artist-release link: %w", err)
		}

		// Create release-label associations from the release's label-info
		for _, li := range mbRelease.LabelInfo {
			if li.Label == nil || li.Label.Name == "" {
				continue
			}

			// Find or create the label
			existingLabel := findLabelByName(tx, li.Label.Name)
			var labelID uint
			if existingLabel != nil {
				labelID = existingLabel.ID
			} else {
				newLabelID, err := createLabelTx(tx, li.Label.Name)
				if err != nil {
					log.Printf("Warning: Failed to create label '%s': %v", li.Label.Name, err)
					continue
				}
				labelID = newLabelID
			}

			// Create release-label link
			var catNum *string
			if li.CatalogNumber != "" {
				catNum = &li.CatalogNumber
			}
			rl := &models.ReleaseLabel{
				ReleaseID:     release.ID,
				LabelID:       labelID,
				CatalogNumber: catNum,
			}
			// Check if it exists first
			var rlCount int64
			tx.Model(&models.ReleaseLabel{}).
				Where("release_id = ? AND label_id = ?", release.ID, labelID).
				Count(&rlCount)
			if rlCount == 0 {
				if err := tx.Create(rl).Error; err != nil {
					log.Printf("Warning: Failed to create release-label link: %v", err)
				}
			}
		}

		return nil
	})
}

func findLabelByName(database *gorm.DB, name string) *models.Label {
	var label models.Label
	err := database.Where("LOWER(name) = LOWER(?)", name).First(&label).Error
	if err != nil {
		return nil
	}
	return &label
}

func artistLabelExists(database *gorm.DB, artistID, labelID uint) bool {
	var count int64
	database.Model(&models.ArtistLabel{}).
		Where("artist_id = ? AND label_id = ?", artistID, labelID).
		Count(&count)
	return count > 0
}

func createLabel(database *gorm.DB, name string) (uint, error) {
	return createLabelTx(database, name)
}

func createLabelTx(tx *gorm.DB, name string) (uint, error) {
	baseSlug := utils.GenerateArtistSlug(name)
	slug := utils.GenerateUniqueSlug(baseSlug, func(candidate string) bool {
		var count int64
		tx.Model(&models.Label{}).Where("slug = ?", candidate).Count(&count)
		return count > 0
	})

	label := &models.Label{
		Name:   name,
		Slug:   &slug,
		Status: models.LabelStatusActive,
	}

	if err := tx.Create(label).Error; err != nil {
		return 0, fmt.Errorf("failed to create label: %w", err)
	}

	return label.ID, nil
}

func createArtistLabelLink(database *gorm.DB, artistID, labelID uint) error {
	al := &models.ArtistLabel{
		ArtistID: artistID,
		LabelID:  labelID,
	}
	return database.Create(al).Error
}

// --- MusicBrainz API helpers ---

func mbRequest(client *http.Client, urlStr string) ([]byte, error) {
	req, err := http.NewRequest("GET", urlStr, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("User-Agent", mbUserAgent)
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == 503 {
		return nil, fmt.Errorf("MusicBrainz rate limited (503) — slow down")
	}
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	return body, nil
}

func searchMBArtist(client *http.Client, name string) (*MBArtist, error) {
	// URL-encode the artist name for the search query
	encodedName := url.QueryEscape(name)
	searchURL := fmt.Sprintf("%s/artist/?query=artist:%s&fmt=json&limit=5", mbBaseURL, encodedName)

	body, err := mbRequest(client, searchURL)
	if err != nil {
		return nil, err
	}

	var result MBArtistSearchResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	if len(result.Artists) == 0 {
		return nil, nil
	}

	// Find best match with score >= mbMinScore
	var bestMatch *MBArtist
	for i, a := range result.Artists {
		if a.Score < mbMinScore {
			continue
		}

		// Require exact name match (case-insensitive)
		if !strings.EqualFold(a.Name, name) {
			continue
		}

		if bestMatch == nil {
			bestMatch = &result.Artists[i]
			continue
		}

		// Prefer matches from Arizona / United States
		if isRelevantArea(a) && !isRelevantArea(*bestMatch) {
			bestMatch = &result.Artists[i]
		}
	}

	return bestMatch, nil
}

func isRelevantArea(a MBArtist) bool {
	if a.Country == "US" {
		return true
	}
	if a.Area != nil {
		areaLower := strings.ToLower(a.Area.Name)
		if strings.Contains(areaLower, "arizona") || strings.Contains(areaLower, "phoenix") ||
			strings.Contains(areaLower, "united states") {
			return true
		}
	}
	if a.BeginArea != nil {
		areaLower := strings.ToLower(a.BeginArea.Name)
		if strings.Contains(areaLower, "arizona") || strings.Contains(areaLower, "phoenix") {
			return true
		}
	}
	return false
}

func fetchMBReleases(client *http.Client, mbArtistID string) ([]MBRelease, error) {
	var allReleases []MBRelease
	offset := 0
	limit := 100

	for {
		relURL := fmt.Sprintf("%s/release?artist=%s&fmt=json&limit=%d&offset=%d&inc=release-groups+labels+artist-credits",
			mbBaseURL, mbArtistID, limit, offset)

		body, err := mbRequest(client, relURL)
		if err != nil {
			return nil, err
		}

		var result MBReleaseSearchResponse
		if err := json.Unmarshal(body, &result); err != nil {
			return nil, fmt.Errorf("failed to parse releases: %w", err)
		}

		allReleases = append(allReleases, result.Releases...)

		// Check if we have all releases
		if offset+limit >= result.ReleaseCount || len(result.Releases) == 0 {
			break
		}

		offset += limit
		time.Sleep(mbRateLimit) // Rate limit between pages
	}

	return allReleases, nil
}

func fetchMBArtistLabels(client *http.Client, mbArtistID string) ([]MBLabel, error) {
	relURL := fmt.Sprintf("%s/artist/%s?inc=label-rels&fmt=json", mbBaseURL, mbArtistID)

	body, err := mbRequest(client, relURL)
	if err != nil {
		return nil, err
	}

	var result MBArtistRelResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to parse artist relations: %w", err)
	}

	var labels []MBLabel
	seen := make(map[string]bool)
	for _, rel := range result.Relations {
		if rel.Label != nil && rel.Label.Name != "" && !seen[rel.Label.ID] {
			labels = append(labels, *rel.Label)
			seen[rel.Label.ID] = true
		}
	}

	return labels, nil
}

// --- Data processing helpers ---

func deduplicateReleases(releases []MBRelease) []MBRelease {
	// Deduplicate by release group ID to avoid multiple editions of the same release
	seen := make(map[string]bool)
	var deduped []MBRelease

	for _, rel := range releases {
		key := ""
		if rel.ReleaseGroup != nil && rel.ReleaseGroup.ID != "" {
			key = rel.ReleaseGroup.ID
		} else {
			// Fallback: use lowercase title
			key = strings.ToLower(rel.Title)
		}

		if seen[key] {
			continue
		}
		seen[key] = true
		deduped = append(deduped, rel)
	}

	return deduped
}

func mapMBReleaseType(rel MBRelease) string {
	if rel.ReleaseGroup == nil {
		return string(models.ReleaseTypeLP)
	}

	primaryType := strings.ToLower(rel.ReleaseGroup.PrimaryType)

	// Check secondary types first for more specific categorization
	for _, secondary := range rel.ReleaseGroup.SecondaryTypes {
		switch strings.ToLower(secondary) {
		case "compilation":
			return string(models.ReleaseTypeCompilation)
		case "live":
			return string(models.ReleaseTypeLive)
		case "remix":
			return string(models.ReleaseTypeRemix)
		case "demo":
			return string(models.ReleaseTypeDemo)
		}
	}

	switch primaryType {
	case "album":
		return string(models.ReleaseTypeLP)
	case "single":
		return string(models.ReleaseTypeSingle)
	case "ep":
		return string(models.ReleaseTypeEP)
	default:
		return string(models.ReleaseTypeLP)
	}
}

func extractYear(date string) int {
	if len(date) < 4 {
		return 0
	}
	year := 0
	fmt.Sscanf(date[:4], "%d", &year)
	return year
}

func collectReleaseLabels(releases []MBRelease) []MBLabel {
	seen := make(map[string]bool)
	var labels []MBLabel

	for _, rel := range releases {
		for _, li := range rel.LabelInfo {
			if li.Label != nil && li.Label.Name != "" && !seen[li.Label.ID] {
				labels = append(labels, *li.Label)
				seen[li.Label.ID] = true
			}
		}
	}

	return labels
}

func mergeLabels(a, b []MBLabel) []MBLabel {
	seen := make(map[string]bool)
	var merged []MBLabel

	for _, lbl := range a {
		if !seen[lbl.ID] {
			merged = append(merged, lbl)
			seen[lbl.ID] = true
		}
	}
	for _, lbl := range b {
		if !seen[lbl.ID] {
			merged = append(merged, lbl)
			seen[lbl.ID] = true
		}
	}

	return merged
}

// --- Display helpers ---

func printMatchDetails(a *MBArtist) {
	fmt.Printf("  MB Name:          %s\n", a.Name)
	fmt.Printf("  MB ID:            %s\n", a.ID)
	fmt.Printf("  Score:            %d\n", a.Score)
	if a.Disambiguation != "" {
		fmt.Printf("  Disambiguation:   %s\n", a.Disambiguation)
	}
	if a.Type != "" {
		fmt.Printf("  Type:             %s\n", a.Type)
	}
	if a.Country != "" {
		fmt.Printf("  Country:          %s\n", a.Country)
	}
	if a.Area != nil {
		fmt.Printf("  Area:             %s\n", a.Area.Name)
	}
	if a.BeginArea != nil {
		fmt.Printf("  Begin Area:       %s\n", a.BeginArea.Name)
	}
}

func promptYesNo(question string) bool {
	reader := bufio.NewReader(os.Stdin)
	fmt.Printf("%s [y/N]: ", question)
	answer, _ := reader.ReadString('\n')
	answer = strings.TrimSpace(strings.ToLower(answer))
	return answer == "y" || answer == "yes"
}
