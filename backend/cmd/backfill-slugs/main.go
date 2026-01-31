package main

import (
	"fmt"
	"log"

	"psychic-homily-backend/db"
	"psychic-homily-backend/internal/config"
	"psychic-homily-backend/internal/models"
	"psychic-homily-backend/internal/utils"

	"github.com/joho/godotenv"
)

func main() {
	// Load environment variables
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found, using environment variables")
	}

	// Load configuration
	cfg := config.Load()

	// Initialize database
	if err := db.InitDB(cfg.DatabaseURL); err != nil {
		log.Fatal("Failed to connect to database:", err)
	}

	database := db.GetDB()

	fmt.Println("Starting slug backfill...")

	// Backfill artist slugs
	fmt.Println("\n=== Backfilling Artist Slugs ===")
	var artists []models.Artist
	database.Where("slug IS NULL OR slug = ''").Find(&artists)
	fmt.Printf("Found %d artists without slugs\n", len(artists))

	for _, artist := range artists {
		baseSlug := utils.GenerateArtistSlug(artist.Name)
		slug := utils.GenerateUniqueSlug(baseSlug, func(candidate string) bool {
			var count int64
			database.Model(&models.Artist{}).Where("slug = ? AND id != ?", candidate, artist.ID).Count(&count)
			return count > 0
		})
		if err := database.Model(&artist).Update("slug", slug).Error; err != nil {
			log.Printf("Error updating artist %d (%s): %v\n", artist.ID, artist.Name, err)
			continue
		}
		fmt.Printf("  Artist %d: %s -> %s\n", artist.ID, artist.Name, slug)
	}

	// Backfill venue slugs
	fmt.Println("\n=== Backfilling Venue Slugs ===")
	var venues []models.Venue
	database.Where("slug IS NULL OR slug = ''").Find(&venues)
	fmt.Printf("Found %d venues without slugs\n", len(venues))

	for _, venue := range venues {
		baseSlug := utils.GenerateVenueSlug(venue.Name, venue.City, venue.State)
		slug := utils.GenerateUniqueSlug(baseSlug, func(candidate string) bool {
			var count int64
			database.Model(&models.Venue{}).Where("slug = ? AND id != ?", candidate, venue.ID).Count(&count)
			return count > 0
		})
		if err := database.Model(&venue).Update("slug", slug).Error; err != nil {
			log.Printf("Error updating venue %d (%s): %v\n", venue.ID, venue.Name, err)
			continue
		}
		fmt.Printf("  Venue %d: %s (%s, %s) -> %s\n", venue.ID, venue.Name, venue.City, venue.State, slug)
	}

	// Backfill show slugs
	fmt.Println("\n=== Backfilling Show Slugs ===")
	var shows []models.Show
	database.Preload("Artists").Preload("Venues").Where("slug IS NULL OR slug = ''").Find(&shows)
	fmt.Printf("Found %d shows without slugs\n", len(shows))

	for _, show := range shows {
		headlinerName := "unknown"
		venueName := "unknown"

		// Get headliner from show_artists
		var showArtist models.ShowArtist
		if err := database.Where("show_id = ? AND set_type = ?", show.ID, "headliner").First(&showArtist).Error; err == nil {
			var artist models.Artist
			if err := database.First(&artist, showArtist.ArtistID).Error; err == nil {
				headlinerName = artist.Name
			}
		} else if len(show.Artists) > 0 {
			headlinerName = show.Artists[0].Name
		}

		// Get first venue
		if len(show.Venues) > 0 {
			venueName = show.Venues[0].Name
		}

		baseSlug := utils.GenerateShowSlug(show.EventDate, headlinerName, venueName)
		slug := utils.GenerateUniqueSlug(baseSlug, func(candidate string) bool {
			var count int64
			database.Model(&models.Show{}).Where("slug = ? AND id != ?", candidate, show.ID).Count(&count)
			return count > 0
		})
		if err := database.Model(&show).Update("slug", slug).Error; err != nil {
			log.Printf("Error updating show %d: %v\n", show.ID, err)
			continue
		}
		fmt.Printf("  Show %d: %s\n", show.ID, slug)
	}

	fmt.Println("\n=== Backfill Complete ===")

	// Summary
	var artistCount, venueCount, showCount int64
	database.Model(&models.Artist{}).Where("slug IS NOT NULL AND slug != ''").Count(&artistCount)
	database.Model(&models.Venue{}).Where("slug IS NOT NULL AND slug != ''").Count(&venueCount)
	database.Model(&models.Show{}).Where("slug IS NOT NULL AND slug != ''").Count(&showCount)

	fmt.Printf("\nFinal counts with slugs:\n")
	fmt.Printf("  Artists: %d\n", artistCount)
	fmt.Printf("  Venues: %d\n", venueCount)
	fmt.Printf("  Shows: %d\n", showCount)
}
