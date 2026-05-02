package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"

	"psychic-homily-backend/db"
	"psychic-homily-backend/internal/config"
	"psychic-homily-backend/internal/services/catalog"
	"psychic-homily-backend/internal/services/contracts"

	"github.com/joho/godotenv"
	"gorm.io/gorm"
	catalogm "psychic-homily-backend/internal/models/catalog"
)

// CLI flags
var (
	dryRun     bool
	artistName string
	confirm    bool
	limitFlag  int
	verbose    bool
	delayMs    int
)

func init() {
	flag.BoolVar(&dryRun, "dry-run", true, "Show what would be imported without writing to DB (default: true)")
	flag.StringVar(&artistName, "artist", "", "Process a single artist by name")
	flag.BoolVar(&confirm, "confirm", false, "Prompt for confirmation before each import")
	flag.IntVar(&limitFlag, "limit", 0, "Maximum number of artists to process (0 = all)")
	flag.BoolVar(&verbose, "verbose", false, "Show detailed output including skipped items")
	flag.IntVar(&delayMs, "delay", 1500, "Delay between Bandcamp requests in milliseconds")
}

// BandcampRelease represents a release discovered from a Bandcamp page
type BandcampRelease struct {
	Title       string
	URL         string
	ReleaseType catalogm.ReleaseType
	ReleaseDate *string
	ReleaseYear *int
	CoverArtURL *string
	ArtistName  string
	LabelName   *string
	Tracks      int
}

// BandcampJSONLD represents the JSON-LD structured data from a Bandcamp release page
type BandcampJSONLD struct {
	Type           string              `json:"@type"`
	Name           string              `json:"name"`
	DatePublished  string              `json:"datePublished"`
	Image          interface{}         `json:"image"` // can be string or array
	NumTracks      int                 `json:"numTracks"`
	ByArtist       *BandcampJSONArtist `json:"byArtist"`
	RecordLabel    *BandcampJSONLabel  `json:"recordLabel"`
	Track          *BandcampJSONTrack  `json:"track"`
	URL            string              `json:"@id"`
	AdditionalType string              `json:"additionalType"`
}

type BandcampJSONArtist struct {
	Type string `json:"@type"`
	Name string `json:"name"`
}

type BandcampJSONLabel struct {
	Type string `json:"@type"`
	Name string `json:"name"`
}

type BandcampJSONTrack struct {
	NumberOfItems int `json:"numberOfItems"`
}

// MatchResult represents the outcome of trying to match a Bandcamp release to our DB
type MatchResult struct {
	BCRelease     BandcampRelease
	MatchType     string // "MATCH", "NEW", "SKIP"
	ExistingID    uint   // set when MatchType == "MATCH"
	ExistingTitle string // set when MatchType == "MATCH"
	Reason        string // explanation for SKIP
}

func main() {
	flag.Parse()

	fmt.Println("=== Bandcamp Enrichment Tool ===")
	if dryRun {
		fmt.Println("Mode: DRY RUN (no database writes)")
	} else {
		fmt.Println("Mode: LIVE (will write to database)")
	}
	fmt.Println()

	database := connectToDatabase()
	releaseService := catalog.NewReleaseService(database)
	labelService := catalog.NewLabelService(database)

	// Get artists with Bandcamp URLs
	artists := getArtistsWithBandcamp(database)
	if len(artists) == 0 {
		fmt.Println("No artists found with Bandcamp URLs.")
		return
	}

	fmt.Printf("Found %d artists with Bandcamp URLs\n\n", len(artists))

	// Apply limit
	if limitFlag > 0 && limitFlag < len(artists) {
		artists = artists[:limitFlag]
		fmt.Printf("Processing first %d artists (--limit %d)\n\n", limitFlag, limitFlag)
	}

	// Track stats
	var totalMatched, totalNew, totalSkipped, totalErrors int

	for i, artist := range artists {
		fmt.Printf("--- [%d/%d] %s ---\n", i+1, len(artists), artist.Name)

		bandcampURL := getBandcampURL(&artist)
		if bandcampURL == "" {
			fmt.Println("  [SKIP] No valid Bandcamp URL found")
			totalSkipped++
			continue
		}
		fmt.Printf("  URL: %s\n", bandcampURL)

		// Fetch releases from Bandcamp
		releases, err := fetchBandcampReleases(bandcampURL, artist.Name)
		if err != nil {
			fmt.Printf("  [ERROR] Failed to fetch Bandcamp page: %v\n", err)
			totalErrors++
			continue
		}

		if len(releases) == 0 {
			fmt.Println("  No releases found on Bandcamp page")
			continue
		}

		fmt.Printf("  Found %d releases on Bandcamp\n", len(releases))

		// Get existing releases for this artist
		existingReleases := getExistingReleases(database, artist.ID)

		// Match each Bandcamp release
		for _, bcRelease := range releases {
			result := matchRelease(bcRelease, existingReleases, database)
			printMatchResult(result)

			switch result.MatchType {
			case "MATCH":
				totalMatched++
				if !dryRun {
					if confirm && !promptConfirm(fmt.Sprintf("Add Bandcamp link to '%s'?", result.ExistingTitle)) {
						fmt.Println("    Skipped by user")
						continue
					}
					err := addBandcampLink(database, releaseService, result.ExistingID, result.BCRelease.URL)
					if err != nil {
						fmt.Printf("    [ERROR] Failed to add link: %v\n", err)
						totalErrors++
					} else {
						fmt.Println("    Link added successfully")
					}
				}
			case "NEW":
				totalNew++
				if !dryRun {
					if confirm && !promptConfirm(fmt.Sprintf("Create new release '%s'?", result.BCRelease.Title)) {
						fmt.Println("    Skipped by user")
						continue
					}
					err := createNewRelease(database, releaseService, labelService, artist.ID, result.BCRelease)
					if err != nil {
						fmt.Printf("    [ERROR] Failed to create release: %v\n", err)
						totalErrors++
					} else {
						fmt.Println("    Release created successfully")
					}
				}
			case "SKIP":
				totalSkipped++
			}
		}

		// Rate limit between artists
		if i < len(artists)-1 {
			time.Sleep(time.Duration(delayMs) * time.Millisecond)
		}

		fmt.Println()
	}

	// Summary
	fmt.Println("=== Summary ===")
	fmt.Printf("  Matched (added Bandcamp links): %d\n", totalMatched)
	fmt.Printf("  New (created releases):         %d\n", totalNew)
	fmt.Printf("  Skipped:                        %d\n", totalSkipped)
	fmt.Printf("  Errors:                         %d\n", totalErrors)
	if dryRun {
		fmt.Println("\n  (DRY RUN - no changes were made)")
		fmt.Println("  Run with --dry-run=false to apply changes")
	}
}

// getArtistsWithBandcamp queries for artists that have a Bandcamp URL
func getArtistsWithBandcamp(database *gorm.DB) []catalogm.Artist {
	var artists []catalogm.Artist

	query := database.Model(&catalogm.Artist{})

	if artistName != "" {
		query = query.Where("LOWER(name) = LOWER(?)", artistName)
	} else {
		// Artists with bandcamp social link OR bandcamp_embed_url
		query = query.Where("bandcamp IS NOT NULL AND bandcamp != '' OR bandcamp_embed_url IS NOT NULL AND bandcamp_embed_url != ''")
	}

	query = query.Order("name ASC")

	if err := query.Find(&artists).Error; err != nil {
		log.Fatalf("Failed to query artists: %v", err)
	}

	// If filtering by name, check that the artist actually has a Bandcamp URL
	if artistName != "" {
		filtered := make([]catalogm.Artist, 0)
		for _, a := range artists {
			if getBandcampURL(&a) != "" {
				filtered = append(filtered, a)
			} else {
				fmt.Printf("Artist '%s' found but has no Bandcamp URL\n", a.Name)
			}
		}
		artists = filtered
	}

	return artists
}

// getBandcampURL extracts the Bandcamp URL from an artist's social links or embed URL
func getBandcampURL(artist *catalogm.Artist) string {
	// Prefer social.bandcamp (direct URL)
	if artist.Social.Bandcamp != nil && *artist.Social.Bandcamp != "" {
		url := *artist.Social.Bandcamp
		// Ensure it's a proper URL
		if !strings.HasPrefix(url, "http") {
			url = "https://" + url
		}
		return strings.TrimRight(url, "/")
	}

	// Fall back to bandcamp_embed_url — extract the artist domain from it
	if artist.BandcampEmbedURL != nil && *artist.BandcampEmbedURL != "" {
		embedURL := *artist.BandcampEmbedURL
		// Typical embed: https://bandcamp.com/EmbeddedPlayer/album=XXXXX
		// or: https://artist.bandcamp.com/...
		// We need the artist's bandcamp domain, which we can't always derive from embed URL
		// But if it contains the artist subdomain, we can extract it
		if strings.Contains(embedURL, ".bandcamp.com") {
			parts := strings.SplitN(embedURL, ".bandcamp.com", 2)
			if len(parts) > 0 {
				domain := parts[0]
				// Extract just the subdomain
				if idx := strings.LastIndex(domain, "/"); idx >= 0 {
					domain = domain[idx+1:]
				}
				if idx := strings.LastIndex(domain, "//"); idx >= 0 {
					domain = domain[idx+2:]
				}
				if domain != "" && domain != "bandcamp" {
					return "https://" + domain + ".bandcamp.com"
				}
			}
		}
	}

	return ""
}

// fetchBandcampReleases fetches the artist's Bandcamp /music page and extracts release URLs,
// then fetches each release page for structured data
func fetchBandcampReleases(baseURL string, artistName string) ([]BandcampRelease, error) {
	musicURL := baseURL + "/music"

	body, err := httpGet(musicURL)
	if err != nil {
		// If /music 404s, try the base URL (some artists don't have a /music page)
		body, err = httpGet(baseURL)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch %s: %w", baseURL, err)
		}
	}

	// Extract release URLs from the page
	releaseURLs := extractReleaseURLs(body, baseURL)

	if len(releaseURLs) == 0 {
		// Try to extract JSON-LD directly from the main page (single-release artists)
		jsonLD, err := extractJSONLD(body)
		if err == nil && jsonLD != nil && (jsonLD.Type == "MusicAlbum" || jsonLD.Type == "MusicRecording") {
			release := jsonLDToRelease(jsonLD, baseURL, artistName)
			if release != nil {
				return []BandcampRelease{*release}, nil
			}
		}
		return nil, nil
	}

	var releases []BandcampRelease
	for i, releaseURL := range releaseURLs {
		if i > 0 {
			// Rate limit between individual release page fetches
			time.Sleep(time.Duration(delayMs) * time.Millisecond)
		}

		if verbose {
			fmt.Printf("    Fetching: %s\n", releaseURL)
		}

		releaseBody, err := httpGet(releaseURL)
		if err != nil {
			if verbose {
				fmt.Printf("    [WARN] Failed to fetch %s: %v\n", releaseURL, err)
			}
			continue
		}

		jsonLD, err := extractJSONLD(releaseBody)
		if err != nil || jsonLD == nil {
			// Try fallback extraction from page content
			release := extractReleaseFromHTML(releaseBody, releaseURL, artistName)
			if release != nil {
				releases = append(releases, *release)
			}
			continue
		}

		release := jsonLDToRelease(jsonLD, releaseURL, artistName)
		if release != nil {
			releases = append(releases, *release)
		}
	}

	return releases, nil
}

// extractReleaseURLs finds all release links on a Bandcamp artist/music page
func extractReleaseURLs(html string, baseURL string) []string {
	// Bandcamp music pages have links like /album/album-name or /track/track-name
	// within elements like <a href="/album/...">
	albumPattern := regexp.MustCompile(`href="(/(?:album|track)/[^"]+)"`)
	matches := albumPattern.FindAllStringSubmatch(html, -1)

	seen := make(map[string]bool)
	var urls []string

	for _, match := range matches {
		path := match[1]
		fullURL := baseURL + path
		if !seen[fullURL] {
			seen[fullURL] = true
			urls = append(urls, fullURL)
		}
	}

	return urls
}

// extractJSONLD finds and parses the JSON-LD script tag from a Bandcamp page
func extractJSONLD(html string) (*BandcampJSONLD, error) {
	// Find <script type="application/ld+json">...</script>
	pattern := regexp.MustCompile(`(?s)<script\s+type="application/ld\+json"\s*>(.*?)</script>`)
	matches := pattern.FindAllStringSubmatch(html, -1)

	for _, match := range matches {
		jsonStr := strings.TrimSpace(match[1])
		var data BandcampJSONLD
		if err := json.Unmarshal([]byte(jsonStr), &data); err != nil {
			continue
		}
		if data.Type == "MusicAlbum" || data.Type == "MusicRecording" {
			return &data, nil
		}
	}

	return nil, fmt.Errorf("no MusicAlbum or MusicRecording JSON-LD found")
}

// jsonLDToRelease converts JSON-LD data to a BandcampRelease
func jsonLDToRelease(jsonLD *BandcampJSONLD, url string, fallbackArtist string) *BandcampRelease {
	if jsonLD.Name == "" {
		return nil
	}

	release := &BandcampRelease{
		Title:      jsonLD.Name,
		URL:        url,
		ArtistName: fallbackArtist,
	}

	// Determine release type
	if jsonLD.Type == "MusicRecording" {
		release.ReleaseType = catalogm.ReleaseTypeSingle
	} else {
		// Determine LP vs EP based on track count
		trackCount := jsonLD.NumTracks
		if jsonLD.Track != nil && jsonLD.Track.NumberOfItems > 0 {
			trackCount = jsonLD.Track.NumberOfItems
		}
		release.Tracks = trackCount

		if trackCount > 0 && trackCount <= 3 {
			release.ReleaseType = catalogm.ReleaseTypeSingle
		} else if trackCount > 0 && trackCount <= 6 {
			release.ReleaseType = catalogm.ReleaseTypeEP
		} else {
			release.ReleaseType = catalogm.ReleaseTypeLP
		}
	}

	// Parse release date
	if jsonLD.DatePublished != "" {
		release.ReleaseDate = &jsonLD.DatePublished
		// Extract year
		if len(jsonLD.DatePublished) >= 4 {
			yearStr := jsonLD.DatePublished[:4]
			if year, err := strconv.Atoi(yearStr); err == nil {
				release.ReleaseYear = &year
			}
		}
		// Try other date formats
		if release.ReleaseYear == nil {
			for _, layout := range []string{"02 Jan 2006", "January 2, 2006", "2006-01-02"} {
				if t, err := time.Parse(layout, jsonLD.DatePublished); err == nil {
					year := t.Year()
					release.ReleaseYear = &year
					dateStr := t.Format("2006-01-02")
					release.ReleaseDate = &dateStr
					break
				}
			}
		}
	}

	// Extract cover art URL
	switch img := jsonLD.Image.(type) {
	case string:
		release.CoverArtURL = &img
	case []interface{}:
		if len(img) > 0 {
			if s, ok := img[0].(string); ok {
				release.CoverArtURL = &s
			}
		}
	}

	// Extract artist name from JSON-LD if available
	if jsonLD.ByArtist != nil && jsonLD.ByArtist.Name != "" {
		release.ArtistName = jsonLD.ByArtist.Name
	}

	// Extract label name
	if jsonLD.RecordLabel != nil && jsonLD.RecordLabel.Name != "" {
		release.LabelName = &jsonLD.RecordLabel.Name
	}

	return release
}

// extractReleaseFromHTML is a fallback that tries to extract release info from HTML
// when JSON-LD is not available
func extractReleaseFromHTML(html string, url string, artistName string) *BandcampRelease {
	// Try to get title from <title> tag or og:title
	titlePattern := regexp.MustCompile(`<title>([^<]+)</title>`)
	ogTitlePattern := regexp.MustCompile(`<meta\s+property="og:title"\s+content="([^"]+)"`)

	var title string
	if m := ogTitlePattern.FindStringSubmatch(html); len(m) > 1 {
		title = m[1]
	} else if m := titlePattern.FindStringSubmatch(html); len(m) > 1 {
		title = m[1]
	}

	if title == "" {
		return nil
	}

	// Clean up title — Bandcamp titles often have "| ArtistName" suffix
	if idx := strings.LastIndex(title, "|"); idx > 0 {
		title = strings.TrimSpace(title[:idx])
	}

	// Determine type from URL
	releaseType := catalogm.ReleaseTypeLP
	if strings.Contains(url, "/track/") {
		releaseType = catalogm.ReleaseTypeSingle
	}

	// Try to get release date
	var releaseYear *int
	datePattern := regexp.MustCompile(`(?i)release[sd]?\s+(\w+\s+\d{1,2},?\s+\d{4}|\d{4})`)
	if m := datePattern.FindStringSubmatch(html); len(m) > 1 {
		if year, err := strconv.Atoi(m[1]); err == nil {
			releaseYear = &year
		} else if t, err := time.Parse("January 2, 2006", m[1]); err == nil {
			y := t.Year()
			releaseYear = &y
		}
	}

	// Try to get cover art
	var coverArt *string
	ogImagePattern := regexp.MustCompile(`<meta\s+property="og:image"\s+content="([^"]+)"`)
	if m := ogImagePattern.FindStringSubmatch(html); len(m) > 1 {
		coverArt = &m[1]
	}

	return &BandcampRelease{
		Title:       title,
		URL:         url,
		ReleaseType: releaseType,
		ReleaseYear: releaseYear,
		CoverArtURL: coverArt,
		ArtistName:  artistName,
	}
}

// matchRelease tries to match a Bandcamp release against existing releases in the DB
func matchRelease(bcRelease BandcampRelease, existingReleases []existingRelease, database *gorm.DB) MatchResult {
	normalizedBC := normalizeTitle(bcRelease.Title)

	for _, existing := range existingReleases {
		normalizedExisting := normalizeTitle(existing.Title)

		// Exact match (normalized)
		if normalizedBC == normalizedExisting {
			// Check if this release already has a Bandcamp link
			if hasBandcampLink(database, existing.ID) {
				return MatchResult{
					BCRelease: bcRelease,
					MatchType: "SKIP",
					Reason:    fmt.Sprintf("already has Bandcamp link (matched: %s)", existing.Title),
				}
			}
			return MatchResult{
				BCRelease:     bcRelease,
				MatchType:     "MATCH",
				ExistingID:    existing.ID,
				ExistingTitle: existing.Title,
			}
		}
	}

	// No match found — this is a new release
	return MatchResult{
		BCRelease: bcRelease,
		MatchType: "NEW",
	}
}

type existingRelease struct {
	ID    uint
	Title string
}

// getExistingReleases gets all releases for an artist from the DB
func getExistingReleases(database *gorm.DB, artistID uint) []existingRelease {
	var releases []existingRelease

	database.Table("releases").
		Select("releases.id, releases.title").
		Joins("JOIN artist_releases ON artist_releases.release_id = releases.id").
		Where("artist_releases.artist_id = ?", artistID).
		Find(&releases)

	return releases
}

// hasBandcampLink checks if a release already has a Bandcamp external link
func hasBandcampLink(database *gorm.DB, releaseID uint) bool {
	var count int64
	database.Model(&catalogm.ReleaseExternalLink{}).
		Where("release_id = ? AND LOWER(platform) = 'bandcamp'", releaseID).
		Count(&count)
	return count > 0
}

// normalizeTitle normalizes a release title for comparison
func normalizeTitle(title string) string {
	// Lowercase
	normalized := strings.ToLower(title)

	// Remove common suffixes/prefixes
	suffixes := []string{
		" (deluxe edition)", " (deluxe)", " (remastered)", " (remaster)",
		" (expanded edition)", " (expanded)", " (bonus tracks)", " (bonus track edition)",
		" (anniversary edition)", " [deluxe]", " [remastered]", " [expanded]",
	}
	for _, suffix := range suffixes {
		normalized = strings.TrimSuffix(normalized, suffix)
	}

	// Remove punctuation except spaces
	var result strings.Builder
	for _, r := range normalized {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == ' ' {
			result.WriteRune(r)
		}
	}
	normalized = result.String()

	// Collapse multiple spaces
	spacePattern := regexp.MustCompile(`\s+`)
	normalized = spacePattern.ReplaceAllString(normalized, " ")

	return strings.TrimSpace(normalized)
}

// addBandcampLink adds a Bandcamp external link to an existing release
func addBandcampLink(database *gorm.DB, releaseService *catalog.ReleaseService, releaseID uint, bandcampURL string) error {
	_, err := releaseService.AddExternalLink(releaseID, "bandcamp", bandcampURL)
	return err
}

// createNewRelease creates a new release from Bandcamp data
func createNewRelease(database *gorm.DB, releaseService *catalog.ReleaseService, labelService *catalog.LabelService, artistID uint, bcRelease BandcampRelease) error {
	req := &contracts.CreateReleaseRequest{
		Title:       bcRelease.Title,
		ReleaseType: string(bcRelease.ReleaseType),
		ReleaseYear: bcRelease.ReleaseYear,
		ReleaseDate: bcRelease.ReleaseDate,
		CoverArtURL: bcRelease.CoverArtURL,
		Artists: []contracts.CreateReleaseArtistEntry{
			{
				ArtistID: artistID,
				Role:     string(catalogm.ArtistReleaseRoleMain),
			},
		},
		ExternalLinks: []contracts.CreateReleaseLinkEntry{
			{
				Platform: "bandcamp",
				URL:      bcRelease.URL,
			},
		},
	}

	createdRelease, err := releaseService.CreateRelease(req)
	if err != nil {
		return fmt.Errorf("failed to create release: %w", err)
	}

	// If there's a label on the Bandcamp release, create or find it and associate
	if bcRelease.LabelName != nil && *bcRelease.LabelName != "" {
		err := handleLabelAssociation(database, labelService, createdRelease.ID, artistID, *bcRelease.LabelName)
		if err != nil {
			// Non-fatal: log but don't fail the release creation
			fmt.Printf("    [WARN] Failed to associate label '%s': %v\n", *bcRelease.LabelName, err)
		}
	}

	return nil
}

// handleLabelAssociation creates a label if needed and associates it with the release and artist
func handleLabelAssociation(database *gorm.DB, labelService *catalog.LabelService, releaseID uint, artistID uint, labelName string) error {
	// Skip if label name matches the artist name (Bandcamp uses artist name as label for self-released)
	var artist catalogm.Artist
	if err := database.First(&artist, artistID).Error; err == nil {
		if strings.EqualFold(artist.Name, labelName) {
			if verbose {
				fmt.Printf("    [INFO] Skipping label '%s' (same as artist name, likely self-released)\n", labelName)
			}
			return nil
		}
	}

	// Check if label already exists (case-insensitive)
	var existingLabel catalogm.Label
	err := database.Where("LOWER(name) = LOWER(?)", labelName).First(&existingLabel).Error
	if err == nil {
		// Label exists — associate with release and artist
		return associateLabel(database, existingLabel.ID, releaseID, artistID)
	}

	if err != gorm.ErrRecordNotFound {
		return fmt.Errorf("failed to check for existing label: %w", err)
	}

	// Create new label
	newLabel, err := labelService.CreateLabel(&contracts.CreateLabelRequest{
		Name: labelName,
	})
	if err != nil {
		return fmt.Errorf("failed to create label: %w", err)
	}

	fmt.Printf("    [NEW LABEL] Created label: %s (ID: %d)\n", labelName, newLabel.ID)
	return associateLabel(database, newLabel.ID, releaseID, artistID)
}

// associateLabel creates release_labels and artist_labels junction entries
func associateLabel(database *gorm.DB, labelID uint, releaseID uint, artistID uint) error {
	// Create release_labels entry (skip if exists)
	rl := catalogm.ReleaseLabel{
		ReleaseID: releaseID,
		LabelID:   labelID,
	}
	if err := database.Where("release_id = ? AND label_id = ?", releaseID, labelID).FirstOrCreate(&rl).Error; err != nil {
		return fmt.Errorf("failed to associate release with label: %w", err)
	}

	// Create artist_labels entry (skip if exists)
	al := catalogm.ArtistLabel{
		ArtistID: artistID,
		LabelID:  labelID,
	}
	if err := database.Where("artist_id = ? AND label_id = ?", artistID, labelID).FirstOrCreate(&al).Error; err != nil {
		return fmt.Errorf("failed to associate artist with label: %w", err)
	}

	return nil
}

// printMatchResult prints a formatted match result
func printMatchResult(result MatchResult) {
	switch result.MatchType {
	case "MATCH":
		fmt.Printf("  [MATCH] '%s' -> existing release '%s' (ID: %d)\n",
			result.BCRelease.Title, result.ExistingTitle, result.ExistingID)
	case "NEW":
		yearStr := "unknown year"
		if result.BCRelease.ReleaseYear != nil {
			yearStr = strconv.Itoa(*result.BCRelease.ReleaseYear)
		}
		fmt.Printf("  [NEW]   '%s' (%s, %s)\n",
			result.BCRelease.Title, result.BCRelease.ReleaseType, yearStr)
		if result.BCRelease.LabelName != nil {
			fmt.Printf("          Label: %s\n", *result.BCRelease.LabelName)
		}
	case "SKIP":
		if verbose {
			fmt.Printf("  [SKIP]  '%s' — %s\n", result.BCRelease.Title, result.Reason)
		}
	}
}

// promptConfirm asks the user for confirmation
func promptConfirm(message string) bool {
	reader := bufio.NewReader(os.Stdin)
	fmt.Printf("    %s [y/N] ", message)
	response, err := reader.ReadString('\n')
	if err != nil {
		return false
	}
	response = strings.TrimSpace(strings.ToLower(response))
	return response == "y" || response == "yes"
}

// httpGet performs an HTTP GET request with a user agent and timeout
func httpGet(url string) (string, error) {
	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return "", err
	}

	// Set a reasonable user agent
	req.Header.Set("User-Agent", "PsychicHomily-MusicCatalog/1.0 (+https://psychichomily.com)")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.5")

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	return string(body), nil
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
