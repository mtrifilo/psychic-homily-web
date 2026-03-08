package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"

	"github.com/joho/godotenv"
	"gorm.io/gorm"

	"psychic-homily-backend/db"
	"psychic-homily-backend/internal/config"
	"psychic-homily-backend/internal/models"
	"psychic-homily-backend/internal/services"
	"psychic-homily-backend/internal/utils"
)

// --- CLI flags ---

var (
	dryRun      bool
	filePath    string
	interactive bool
	confirm     bool
	verbose     bool
)

func init() {
	flag.BoolVar(&dryRun, "dry-run", true, "Show what would happen without DB writes (default: true)")
	flag.StringVar(&filePath, "file", "", "Path to JSON file with festival data")
	flag.BoolVar(&interactive, "interactive", false, "Interactive entry mode")
	flag.BoolVar(&confirm, "confirm", false, "Prompt before each operation")
	flag.BoolVar(&verbose, "verbose", false, "Show detailed output")
}

// --- JSON input types ---

// FestivalInput represents the JSON structure for festival data import
type FestivalInput struct {
	Name         string              `json:"name"`
	SeriesSlug   string              `json:"series_slug"`
	EditionYear  int                 `json:"edition_year"`
	City         string              `json:"city"`
	State        string              `json:"state"`
	Country      string              `json:"country"`
	StartDate    string              `json:"start_date"`
	EndDate      string              `json:"end_date"`
	LocationName string              `json:"location_name"`
	Website      string              `json:"website"`
	TicketURL    string              `json:"ticket_url"`
	Description  string              `json:"description"`
	Venues       []VenueInput        `json:"venues"`
	Lineup       []LineupArtistInput `json:"lineup"`
}

// VenueInput represents a venue in the JSON input
type VenueInput struct {
	Name      string `json:"name"`
	IsPrimary bool   `json:"is_primary"`
}

// LineupArtistInput represents an artist in the lineup JSON input
type LineupArtistInput struct {
	Artist      string `json:"artist"`
	BillingTier string `json:"billing_tier"`
	Day         string `json:"day"`
	Position    int    `json:"position"`
	Stage       string `json:"stage"`
	SetTime     string `json:"set_time"`
}

// --- Stats tracking ---

type importStats struct {
	festivalCreated   bool
	artistsMatched    int
	artistsCreated    int
	artistsSkipped    int
	venuesMatched     int
	venuesSkipped     int
	lineupEntriesAdded int
	errors            int
}

func main() {
	flag.Parse()

	fmt.Println("=== Festival Data Entry Tool ===")
	fmt.Println()

	if !interactive && filePath == "" {
		fmt.Println("Usage:")
		fmt.Println("  go run ./cmd/festival-entry/ --file festival.json --dry-run")
		fmt.Println("  go run ./cmd/festival-entry/ --file festival.json --dry-run=false")
		fmt.Println("  go run ./cmd/festival-entry/ --file festival.json --confirm")
		fmt.Println("  go run ./cmd/festival-entry/ --interactive")
		fmt.Println()
		fmt.Println("Flags:")
		flag.PrintDefaults()
		os.Exit(1)
	}

	if dryRun {
		fmt.Println("[DRY RUN] No database writes will be performed.")
	} else {
		fmt.Println("[LIVE] Database writes ENABLED.")
	}
	fmt.Println()

	// Connect to database
	database := connectToDatabase()
	festivalService := services.NewFestivalService(database)

	if interactive {
		runInteractiveMode(database, festivalService)
	} else {
		runFileImport(database, festivalService, filePath)
	}
}

// --- File import mode ---

func runFileImport(database *gorm.DB, festivalService *services.FestivalService, path string) {
	data, err := os.ReadFile(path)
	if err != nil {
		log.Fatalf("Failed to read file %s: %v", path, err)
	}

	var input FestivalInput
	if err := json.Unmarshal(data, &input); err != nil {
		log.Fatalf("Failed to parse JSON: %v", err)
	}

	// Validate required fields
	if input.Name == "" || input.SeriesSlug == "" || input.EditionYear == 0 ||
		input.StartDate == "" || input.EndDate == "" {
		log.Fatalf("Missing required fields: name, series_slug, edition_year, start_date, end_date")
	}

	stats := &importStats{}

	fmt.Printf("Festival: %s (%s %d)\n", input.Name, input.SeriesSlug, input.EditionYear)
	fmt.Printf("Dates:    %s to %s\n", input.StartDate, input.EndDate)
	if input.City != "" {
		fmt.Printf("Location: %s, %s, %s\n", input.City, input.State, input.Country)
	}
	if input.LocationName != "" {
		fmt.Printf("Venue:    %s\n", input.LocationName)
	}
	fmt.Printf("Lineup:   %d artist(s)\n", len(input.Lineup))
	fmt.Printf("Venues:   %d venue(s)\n", len(input.Venues))
	fmt.Println()

	// Step 1: Create the festival
	festivalID := createFestivalFromInput(database, festivalService, &input, stats)
	if festivalID == 0 && !dryRun {
		fmt.Println("[ERROR] Failed to create festival. Aborting.")
		return
	}

	// Step 2: Process venues
	if len(input.Venues) > 0 {
		fmt.Println("--- Venues ---")
		for _, venueInput := range input.Venues {
			processVenue(database, festivalService, festivalID, venueInput, stats)
		}
		fmt.Println()
	}

	// Step 3: Process lineup
	if len(input.Lineup) > 0 {
		fmt.Println("--- Lineup ---")
		for _, lineupEntry := range input.Lineup {
			processLineupArtist(database, festivalService, festivalID, lineupEntry, stats)
		}
		fmt.Println()
	}

	// Print summary
	printSummary(stats)
}

func createFestivalFromInput(database *gorm.DB, festivalService *services.FestivalService, input *FestivalInput, stats *importStats) uint {
	// Check if festival already exists (by series_slug + edition_year)
	existing := findExistingFestival(database, input.SeriesSlug, input.EditionYear)
	if existing != nil {
		fmt.Printf("[EXISTS] Festival '%s' already exists (ID: %d, slug: %s)\n", existing.Name, existing.ID, existing.Slug)
		fmt.Println("         Will add lineup entries to existing festival.")
		fmt.Println()
		return existing.ID
	}

	if confirm && !dryRun {
		if !promptYesNo(fmt.Sprintf("Create festival '%s'?", input.Name)) {
			fmt.Println("[SKIP] Festival creation skipped by user.")
			return 0
		}
	}

	if dryRun {
		fmt.Printf("[DRY RUN] Would create festival: %s (%s %d)\n\n", input.Name, input.SeriesSlug, input.EditionYear)
		return 0
	}

	req := &services.CreateFestivalRequest{
		Name:        input.Name,
		SeriesSlug:  input.SeriesSlug,
		EditionYear: input.EditionYear,
		StartDate:   input.StartDate,
		EndDate:     input.EndDate,
		Status:      string(models.FestivalStatusAnnounced),
	}

	if input.Description != "" {
		req.Description = &input.Description
	}
	if input.LocationName != "" {
		req.LocationName = &input.LocationName
	}
	if input.City != "" {
		req.City = &input.City
	}
	if input.State != "" {
		req.State = &input.State
	}
	if input.Country != "" {
		req.Country = &input.Country
	}
	if input.Website != "" {
		req.Website = &input.Website
	}
	if input.TicketURL != "" {
		req.TicketURL = &input.TicketURL
	}

	festival, err := festivalService.CreateFestival(req)
	if err != nil {
		fmt.Printf("[ERROR] Failed to create festival: %v\n", err)
		stats.errors++
		return 0
	}

	stats.festivalCreated = true
	fmt.Printf("[CREATED] Festival '%s' (ID: %d, slug: %s)\n\n", festival.Name, festival.ID, festival.Slug)
	return festival.ID
}

func processVenue(database *gorm.DB, festivalService *services.FestivalService, festivalID uint, venueInput VenueInput, stats *importStats) {
	// Search for venue by name (case-insensitive)
	venue := findVenueByName(database, venueInput.Name)

	if venue != nil {
		fmt.Printf("  [MATCH] Venue: %s (id=%d, slug=%s)\n", venue.Name, venue.ID, safeSlug(venue.Slug))

		if dryRun {
			fmt.Printf("  [DRY RUN] Would link venue to festival (is_primary=%v)\n", venueInput.IsPrimary)
			stats.venuesMatched++
			return
		}

		if festivalID == 0 {
			stats.venuesMatched++
			return
		}

		if confirm {
			if !promptYesNo(fmt.Sprintf("  Link venue '%s' to festival?", venue.Name)) {
				fmt.Println("  [SKIP] Venue skipped by user.")
				stats.venuesSkipped++
				return
			}
		}

		_, err := festivalService.AddFestivalVenue(festivalID, &services.AddFestivalVenueRequest{
			VenueID:   venue.ID,
			IsPrimary: venueInput.IsPrimary,
		})
		if err != nil {
			fmt.Printf("  [ERROR] Failed to link venue: %v\n", err)
			stats.errors++
			return
		}

		stats.venuesMatched++
		fmt.Println("  [LINKED] Venue linked to festival")
	} else {
		fmt.Printf("  [SKIP] Venue '%s' not found in database — skipping (venues must be created separately)\n", venueInput.Name)
		stats.venuesSkipped++
	}
}

func processLineupArtist(database *gorm.DB, festivalService *services.FestivalService, festivalID uint, entry LineupArtistInput, stats *importStats) {
	// Search for artist by exact name
	artist := findArtistByName(database, entry.Artist)

	if artist != nil {
		fmt.Printf("  [MATCH] %s (id=%d, slug=%s) — %s", artist.Name, artist.ID, safeSlug(artist.Slug), entry.BillingTier)
		if entry.Day != "" {
			fmt.Printf(" — %s", entry.Day)
		}
		fmt.Println()

		if dryRun {
			stats.artistsMatched++
			return
		}

		if festivalID == 0 {
			stats.artistsMatched++
			return
		}

		if confirm {
			if !promptYesNo(fmt.Sprintf("  Add '%s' to lineup?", artist.Name)) {
				fmt.Println("  [SKIP] Artist skipped by user.")
				stats.artistsSkipped++
				return
			}
		}

		addToLineup(festivalService, festivalID, artist.ID, entry, stats)
		stats.artistsMatched++
	} else {
		fmt.Printf("  [NEW] %s — will create new artist — %s", entry.Artist, entry.BillingTier)
		if entry.Day != "" {
			fmt.Printf(" — %s", entry.Day)
		}
		fmt.Println()

		if dryRun {
			stats.artistsCreated++
			return
		}

		if confirm {
			if !promptYesNo(fmt.Sprintf("  Create artist '%s' and add to lineup?", entry.Artist)) {
				fmt.Println("  [SKIP] Artist skipped by user.")
				stats.artistsSkipped++
				return
			}
		}

		// Create the new artist
		artistID, err := createMinimalArtist(database, entry.Artist)
		if err != nil {
			fmt.Printf("  [ERROR] Failed to create artist '%s': %v\n", entry.Artist, err)
			stats.errors++
			return
		}
		fmt.Printf("  [CREATED] Artist '%s' (id=%d)\n", entry.Artist, artistID)
		stats.artistsCreated++

		if festivalID > 0 {
			addToLineup(festivalService, festivalID, artistID, entry, stats)
		}
	}
}

func addToLineup(festivalService *services.FestivalService, festivalID, artistID uint, entry LineupArtistInput, stats *importStats) {
	req := &services.AddFestivalArtistRequest{
		ArtistID:    artistID,
		BillingTier: entry.BillingTier,
		Position:    entry.Position,
	}

	if entry.Day != "" {
		req.DayDate = &entry.Day
	}
	if entry.Stage != "" {
		req.Stage = &entry.Stage
	}
	if entry.SetTime != "" {
		req.SetTime = &entry.SetTime
	}

	_, err := festivalService.AddFestivalArtist(festivalID, req)
	if err != nil {
		fmt.Printf("  [ERROR] Failed to add to lineup: %v\n", err)
		stats.errors++
		return
	}
	stats.lineupEntriesAdded++
}

// --- Interactive mode ---

func runInteractiveMode(database *gorm.DB, festivalService *services.FestivalService) {
	reader := bufio.NewReader(os.Stdin)
	stats := &importStats{}

	fmt.Println("=== Interactive Festival Entry ===")
	fmt.Println("Enter festival details. Press Ctrl+C to cancel at any time.")
	fmt.Println()

	// Gather festival details
	name := promptString(reader, "Festival name")
	seriesSlug := promptString(reader, "Series slug (e.g., m3f, levitation)")
	editionYearStr := promptString(reader, "Edition year")
	editionYear, err := strconv.Atoi(editionYearStr)
	if err != nil {
		log.Fatalf("Invalid year: %s", editionYearStr)
	}

	startDate := promptString(reader, "Start date (YYYY-MM-DD)")
	endDate := promptString(reader, "End date (YYYY-MM-DD)")
	city := promptStringOptional(reader, "City (optional)")
	state := promptStringOptional(reader, "State (optional)")
	country := promptStringOptional(reader, "Country (optional, default: US)")
	if country == "" {
		country = "US"
	}
	locationName := promptStringOptional(reader, "Location name (optional, e.g., Margaret T. Hance Park)")
	website := promptStringOptional(reader, "Website URL (optional)")
	ticketURL := promptStringOptional(reader, "Ticket URL (optional)")
	description := promptStringOptional(reader, "Description (optional)")

	input := &FestivalInput{
		Name:         name,
		SeriesSlug:   seriesSlug,
		EditionYear:  editionYear,
		StartDate:    startDate,
		EndDate:      endDate,
		City:         city,
		State:        state,
		Country:      country,
		LocationName: locationName,
		Website:      website,
		TicketURL:    ticketURL,
		Description:  description,
	}

	fmt.Println()
	fmt.Printf("Festival: %s (%s %d)\n", name, seriesSlug, editionYear)
	fmt.Printf("Dates:    %s to %s\n", startDate, endDate)
	fmt.Println()

	// Create festival
	festivalID := createFestivalFromInput(database, festivalService, input, stats)
	if festivalID == 0 && !dryRun {
		fmt.Println("[ERROR] Failed to create festival. Aborting.")
		return
	}

	// Add artists interactively
	fmt.Println()
	fmt.Println("--- Add Artists to Lineup ---")
	fmt.Println("Enter artist details. Type 'done' for artist name to finish.")
	fmt.Println()

	for {
		artistName := promptString(reader, "Artist name (or 'done')")
		if strings.EqualFold(artistName, "done") {
			break
		}

		billingTier := promptStringWithDefault(reader, "Billing tier (headliner/sub_headliner/mid_card/undercard/local/dj/host)", "mid_card")
		day := promptStringOptional(reader, "Day (YYYY-MM-DD, optional)")
		posStr := promptStringWithDefault(reader, "Position (0 = first in tier)", "0")
		position, _ := strconv.Atoi(posStr)
		stage := promptStringOptional(reader, "Stage (optional)")

		entry := LineupArtistInput{
			Artist:      artistName,
			BillingTier: billingTier,
			Day:         day,
			Position:    position,
			Stage:       stage,
		}

		processLineupArtist(database, festivalService, festivalID, entry, stats)
		fmt.Println()
	}

	// Print summary
	fmt.Println()
	printSummary(stats)
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

func findExistingFestival(database *gorm.DB, seriesSlug string, editionYear int) *models.Festival {
	var festival models.Festival
	err := database.Where("series_slug = ? AND edition_year = ?", seriesSlug, editionYear).First(&festival).Error
	if err != nil {
		return nil
	}
	return &festival
}

func findArtistByName(database *gorm.DB, name string) *models.Artist {
	var artist models.Artist
	err := database.Where("LOWER(name) = LOWER(?)", name).First(&artist).Error
	if err != nil {
		return nil
	}
	return &artist
}

func findVenueByName(database *gorm.DB, name string) *models.Venue {
	var venue models.Venue
	err := database.Where("LOWER(name) = LOWER(?)", name).First(&venue).Error
	if err != nil {
		return nil
	}
	return &venue
}

func createMinimalArtist(database *gorm.DB, name string) (uint, error) {
	baseSlug := utils.GenerateArtistSlug(name)
	slug := utils.GenerateUniqueSlug(baseSlug, func(candidate string) bool {
		var count int64
		database.Model(&models.Artist{}).Where("slug = ?", candidate).Count(&count)
		return count > 0
	})

	artist := &models.Artist{
		Name: name,
		Slug: &slug,
	}

	if err := database.Create(artist).Error; err != nil {
		return 0, fmt.Errorf("failed to create artist: %w", err)
	}

	return artist.ID, nil
}

func safeSlug(slug *string) string {
	if slug == nil {
		return ""
	}
	return *slug
}

// --- Display helpers ---

func printSummary(stats *importStats) {
	fmt.Println("=== Summary ===")
	if stats.festivalCreated {
		fmt.Println("Festival:           Created")
	}
	fmt.Printf("Artists matched:     %d\n", stats.artistsMatched)
	fmt.Printf("Artists created:     %d\n", stats.artistsCreated)
	fmt.Printf("Artists skipped:     %d\n", stats.artistsSkipped)
	fmt.Printf("Venues matched:      %d\n", stats.venuesMatched)
	fmt.Printf("Venues skipped:      %d\n", stats.venuesSkipped)
	fmt.Printf("Lineup entries:      %d\n", stats.lineupEntriesAdded)
	fmt.Printf("Errors:              %d\n", stats.errors)
	if dryRun {
		fmt.Println("\n[DRY RUN] No changes were made. Run with --dry-run=false to import.")
	}
}

func promptYesNo(question string) bool {
	reader := bufio.NewReader(os.Stdin)
	fmt.Printf("%s [y/N]: ", question)
	answer, _ := reader.ReadString('\n')
	answer = strings.TrimSpace(strings.ToLower(answer))
	return answer == "y" || answer == "yes"
}

func promptString(reader *bufio.Reader, label string) string {
	for {
		fmt.Printf("%s: ", label)
		line, _ := reader.ReadString('\n')
		line = strings.TrimSpace(line)
		if line != "" {
			return line
		}
		fmt.Println("  (required, please enter a value)")
	}
}

func promptStringOptional(reader *bufio.Reader, label string) string {
	fmt.Printf("%s: ", label)
	line, _ := reader.ReadString('\n')
	return strings.TrimSpace(line)
}

func promptStringWithDefault(reader *bufio.Reader, label string, defaultVal string) string {
	fmt.Printf("%s [%s]: ", label, defaultVal)
	line, _ := reader.ReadString('\n')
	line = strings.TrimSpace(line)
	if line == "" {
		return defaultVal
	}
	return line
}
