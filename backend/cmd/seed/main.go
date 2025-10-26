package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"psychic-homily-backend/db"
	"psychic-homily-backend/internal/config"
	"psychic-homily-backend/internal/models"

	"github.com/goccy/go-yaml"
	"github.com/joho/godotenv"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type VenueData struct {
	Name    string `yaml:"name"`
	Address string `yaml:"address"`
	City    string `yaml:"city"`
	State   string `yaml:"state"`
	Zip     string `yaml:"zip"`
	Social  struct {
		Instagram string `yaml:"instagram"`
		Website   string `yaml:"website"`
	} `yaml:"social"`
}

type ArtistData struct {
	Name        string `yaml:"name"`
	ArizonaBand bool   `yaml:"arizona-band"`
	Social      struct {
		Instagram string `yaml:"instagram"`
		Website   string `yaml:"website"`
	} `yaml:"social"`
	URL string `yaml:"url"`
}

type ShowData struct {
	Title          string   `yaml:"title"`
	Date           string   `yaml:"date"`       // Hugo creation date
	EventDate      string   `yaml:"event_date"` // Actual show date
	Draft          bool     `yaml:"draft"`
	Venues         []string `yaml:"venues"` // Array of venue slugs
	City           string   `yaml:"city"`
	State          string   `yaml:"state"`
	Price          string   `yaml:"price"` // String, can be empty
	AgeRequirement string   `yaml:"age_requirement"`
	Bands          []string `yaml:"bands"` // Array of band slugs (order matters!)
}

func main() {
	fmt.Println("Seeding database...")
	db := connectToDatabase()

	// Seed venues
	fmt.Println("Seeding venues...")
	venues := getVenueData()
	venueModels := make([]*models.Venue, 0, len(venues))

	for _, venue := range venues {
		venueModel := &models.Venue{
			Name:    venue.Name,
			Address: &venue.Address,
			City:    venue.City,
			State:   venue.State,
			Zipcode: &venue.Zip,
			Social: models.Social{
				Instagram: &venue.Social.Instagram,
				Website:   &venue.Social.Website,
			},
		}
		venueModels = append(venueModels, venueModel)
	}

	// Insert venues one by one, skipping duplicates
	// Venue uniqueness is on LOWER(name), LOWER(city) per migration 000004
	var venuesCreated int64
	for _, venueModel := range venueModels {
		// Check if venue already exists (case-insensitive)
		var existing models.Venue
		result := db.Where("LOWER(name) = LOWER(?) AND LOWER(city) = LOWER(?)", venueModel.Name, venueModel.City).First(&existing)
		if result.Error == nil {
			// Venue exists, skip
			continue
		}
		// Create new venue
		if err := db.Create(venueModel).Error; err != nil {
			log.Printf("Warning: Failed to create venue %s: %v", venueModel.Name, err)
			continue
		}
		venuesCreated++
	}

	fmt.Printf("✅ Successfully processed %d venues (%d created)\n", len(venues), venuesCreated)

	// Seed artists
	fmt.Println("Seeding artists...")
	artists := getArtistData()
	artistModels := make([]*models.Artist, 0, len(artists))

	for _, artist := range artists {
		// Set state to "AZ" only if it's an Arizona band
		var state *string
		if artist.ArizonaBand {
			az := "AZ"
			state = &az
		}

		artistModel := &models.Artist{
			Name:  artist.Name,
			State: state,
			Social: models.Social{
				Instagram: &artist.Social.Instagram,
				Website:   &artist.Social.Website,
			},
		}
		artistModels = append(artistModels, artistModel)
	}

	// Use Upsert to handle duplicates gracefully
	artistResult := db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "name"}}, // Conflict on name field
		DoNothing: true,                            // Skip if artist already exists
	}).Create(artistModels)

	if artistResult.Error != nil {
		log.Fatalf("Failed to create artists: %v", artistResult.Error)
	}

	fmt.Printf("✅ Successfully processed %d artists (%d created)\n", len(artists), artistResult.RowsAffected)

	// Seed shows
	fmt.Println("Seeding shows...")
	shows := getShowData()
	showCount := 0
	successCount := 0

	for _, show := range shows {
		// Skip draft shows
		if show.Draft {
			fmt.Printf("⏭️  Skipping draft show: %s\n", show.Title)
			continue
		}

		showCount++
		if err := createShowWithAssociations(db, show); err != nil {
			log.Printf("❌ Failed to create show '%s': %v", show.Title, err)
		} else {
			successCount++
			fmt.Printf("✅ Created show: %s\n", show.Title)
		}
	}

	fmt.Printf("🎉 Database seeding completed!\n")
	fmt.Printf("📊 Summary: %d venues, %d artists, %d/%d shows\n",
		len(venues), len(artists), successCount, showCount)
}

func getVenueData() map[string]VenueData {
	data, err := os.ReadFile("../data/venues.yaml")
	if err != nil {
		log.Fatalf("Failed to read venues.yaml: %v", err)
	}

	var venues map[string]VenueData
	err = yaml.Unmarshal(data, &venues)
	if err != nil {
		log.Fatalf("Failed to unmarshal venues.yaml: %v", err)
	}
	return venues
}

func getArtistData() map[string]ArtistData {
	data, err := os.ReadFile("../data/bands.yaml")
	if err != nil {
		log.Fatalf("Failed to read bands.yaml: %v", err)
	}

	var artists map[string]ArtistData
	err = yaml.Unmarshal(data, &artists)
	if err != nil {
		log.Fatalf("Failed to unmarshal bands.yaml: %v", err)
	}
	return artists
}

func getShowData() []ShowData {
	// Read all show files from content/shows/
	showDir := "../content/shows"
	files, err := os.ReadDir(showDir)
	if err != nil {
		log.Fatalf("Failed to read shows directory: %v", err)
	}

	var shows []ShowData
	for _, file := range files {
		if file.IsDir() || !strings.HasSuffix(file.Name(), ".md") || file.Name() == "_index.md" {
			continue
		}

		// Read the show file
		data, err := os.ReadFile(filepath.Join(showDir, file.Name()))
		if err != nil {
			log.Printf("Warning: Failed to read %s: %v", file.Name(), err)
			continue
		}

		// Parse frontmatter
		show, err := parseShowFrontmatter(data)
		if err != nil {
			log.Printf("Warning: Failed to parse %s: %v", file.Name(), err)
			continue
		}

		shows = append(shows, show)
	}

	return shows
}

func parseShowFrontmatter(data []byte) (ShowData, error) {
	// Split frontmatter from content
	parts := strings.Split(string(data), "---")
	if len(parts) < 3 {
		return ShowData{}, fmt.Errorf("invalid frontmatter format")
	}

	// Parse YAML frontmatter
	var show ShowData
	err := yaml.Unmarshal([]byte(parts[1]), &show)
	if err != nil {
		return ShowData{}, fmt.Errorf("failed to parse YAML: %v", err)
	}

	return show, nil
}

func createShowWithAssociations(db *gorm.DB, showData ShowData) error {
	// Parse event date and convert to UTC
	eventDate, err := time.Parse("2006-01-02T15:04:05-07:00", showData.EventDate)
	if err != nil {
		return fmt.Errorf("failed to parse event date: %v", err)
	}

	// Convert to UTC for database storage
	eventDateUTC := eventDate.UTC()

	// Parse price
	var price *float64
	if showData.Price != "" {
		if p, err := strconv.ParseFloat(showData.Price, 64); err == nil {
			price = &p
		}
	}

	// Generate normalized title: "Band1, Band2, Band3 at Venue Name"
	normalizedTitle := generateNormalizedTitle(showData)

	// Create the show
	show := &models.Show{
		Title:          normalizedTitle,
		EventDate:      eventDateUTC,
		City:           &showData.City,
		State:          &showData.State,
		Price:          price,
		AgeRequirement: &showData.AgeRequirement,
	}

	// Use transaction for data consistency
	return db.Transaction(func(tx *gorm.DB) error {
		// Create the show
		if err := tx.Create(show).Error; err != nil {
			return fmt.Errorf("failed to create show: %v", err)
		}

		// Associate venues
		for _, venueSlug := range showData.Venues {
			var venue models.Venue
			// Try to find venue by name (normalized)
			venueName := normalizeVenueName(venueSlug)

			// Try exact match first
			result := tx.Where("LOWER(name) = LOWER(?)", venueName).First(&venue)
			if result.Error != nil {
				// Try partial match for cases like venue name variations
				result = tx.Where("LOWER(name) LIKE LOWER(?)", "%"+venueName+"%").First(&venue)
				if result.Error != nil {
					log.Printf("Warning: Venue not found: %s (slug: %s)", venueName, venueSlug)
					continue
				}
			}

			// Create show-venue association
			showVenue := models.ShowVenue{
				ShowID:  show.ID,
				VenueID: venue.ID,
			}
			if err := tx.Create(&showVenue).Error; err != nil {
				return fmt.Errorf("failed to create show-venue association: %v", err)
			}
		}

		// Associate artists in order
		for position, artistSlug := range showData.Bands {
			var artist models.Artist
			// Try to find artist by name (normalized)
			artistName := normalizeArtistName(artistSlug)

			// Try exact match first
			result := tx.Where("LOWER(name) = LOWER(?)", artistName).First(&artist)
			if result.Error != nil {
				// Try partial match for cases like "Fashion Club (LA)" vs "Fashion Club"
				result = tx.Where("LOWER(name) LIKE LOWER(?)", "%"+artistName+"%").First(&artist)
				if result.Error != nil {
					log.Printf("Warning: Artist not found: %s (slug: %s)", artistName, artistSlug)
					continue
				}
			}

			// Determine set type based on position
			setType := "opener"
			if position == 0 {
				setType = "headliner"
			}

			// Create show-artist association with position
			showArtist := models.ShowArtist{
				ShowID:   show.ID,
				ArtistID: artist.ID,
				Position: position,
				SetType:  setType,
			}
			if err := tx.Create(&showArtist).Error; err != nil {
				return fmt.Errorf("failed to create show-artist association: %v", err)
			}
		}

		return nil
	})
}

// Helper functions for name normalization
func normalizeVenueName(slug string) string {
	// Convert slug to display name
	// e.g., "club-congress" -> "Club Congress"
	words := strings.Split(slug, "-")
	for i, word := range words {
		if len(word) > 0 {
			words[i] = cases.Title(language.English).String(word)
		}
	}
	return strings.Join(words, " ")
}

func normalizeArtistName(slug string) string {
	// Convert slug to display name
	// e.g., "where's-lucy?" -> "Where's Lucy?"
	words := strings.Split(slug, "-")
	for i, word := range words {
		if len(word) > 0 {
			words[i] = cases.Title(language.English).String(word)
		}
	}
	return strings.Join(words, " ")
}

func generateNormalizedTitle(showData ShowData) string {
	// Build band list in order
	var bandNames []string
	for _, bandSlug := range showData.Bands {
		bandName := normalizeArtistName(bandSlug)
		bandNames = append(bandNames, bandName)
	}

	// Join bands with commas
	bandList := strings.Join(bandNames, ", ")

	// Get venue name (use first venue if multiple)
	var venueName string
	if len(showData.Venues) > 0 {
		venueName = normalizeVenueName(showData.Venues[0])
	}

	// Format: "Band1, Band2, Band3 at Venue Name"
	if venueName != "" {
		return fmt.Sprintf("%s at %s", bandList, venueName)
	}

	return bandList
}

func connectToDatabase() *gorm.DB {
	envFile := fmt.Sprintf(".env.%s", config.GetEnv("NODE_ENV", "development"))

	if err := godotenv.Load(envFile); err != nil {
		log.Printf("Warning: %s file not found, trying .env: %v", envFile, err)
		// Fallback to .env if environment-specific file doesn't exist
		if err := godotenv.Load(); err != nil {
			log.Printf("Warning: no .env file found: %v", err)
		}
	}

	// Load configuration
	cfg := config.Load()

	// Connect to database
	if err := db.Connect(cfg); err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}

	return db.GetDB()
}
