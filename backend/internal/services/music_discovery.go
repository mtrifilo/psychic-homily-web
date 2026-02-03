package services

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/getsentry/sentry-go"

	"psychic-homily-backend/internal/config"
)

// MusicDiscoveryService handles automatic discovery of music platforms for new artists
type MusicDiscoveryService struct {
	internalSecret string
	frontendURL    string
	enabled        bool
	httpClient     *http.Client
}

// NewMusicDiscoveryService creates a new music discovery service
func NewMusicDiscoveryService(cfg *config.Config) *MusicDiscoveryService {
	return &MusicDiscoveryService{
		internalSecret: cfg.MusicDiscovery.InternalAPISecret,
		frontendURL:    cfg.MusicDiscovery.FrontendURL,
		enabled:        cfg.MusicDiscovery.Enabled,
		httpClient: &http.Client{
			Timeout: 60 * time.Second, // Longer timeout since AI discovery can take time
		},
	}
}

// IsConfigured returns true if the music discovery service is properly configured
func (s *MusicDiscoveryService) IsConfigured() bool {
	return s.enabled && s.internalSecret != "" && s.frontendURL != ""
}

// DiscoverMusicForArtist triggers the AI-powered music discovery for an artist
// This is a fire-and-forget operation that runs in a goroutine
func (s *MusicDiscoveryService) DiscoverMusicForArtist(artistID uint, artistName string) {
	if !s.IsConfigured() {
		return
	}

	go s.triggerDiscovery(artistID, artistName)
}

// triggerDiscovery makes the HTTP call to the discovery endpoint
func (s *MusicDiscoveryService) triggerDiscovery(artistID uint, artistName string) {
	url := fmt.Sprintf("%s/api/admin/artists/%d/discover-music", s.frontendURL, artistID)

	// Create empty request body (POST endpoint doesn't require body)
	reqBody, _ := json.Marshal(map[string]interface{}{})

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(reqBody))
	if err != nil {
		sentry.WithScope(func(scope *sentry.Scope) {
			scope.SetTag("service", "music-discovery")
			scope.SetExtra("artist_id", artistID)
			scope.SetExtra("artist_name", artistName)
			sentry.CaptureException(fmt.Errorf("failed to create request: %w", err))
		})
		return
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Internal-Secret", s.internalSecret)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		sentry.WithScope(func(scope *sentry.Scope) {
			scope.SetTag("service", "music-discovery")
			scope.SetExtra("artist_id", artistID)
			scope.SetExtra("artist_name", artistName)
			sentry.CaptureException(fmt.Errorf("discovery endpoint call failed: %w", err))
		})
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		// Parse response to log what was found
		var result struct {
			Success  bool   `json:"success"`
			Platform string `json:"platform"`
			URL      string `json:"url"`
			Error    string `json:"error"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&result); err == nil {
			if result.Success {
				log.Printf("[MusicDiscovery] Found %s for artist %d (%s): %s", result.Platform, artistID, artistName, result.URL)
			} else {
				log.Printf("[MusicDiscovery] No music found for artist %d (%s): %s", artistID, artistName, result.Error)
			}
		}
	} else if resp.StatusCode == 404 {
		log.Printf("[MusicDiscovery] No music found for artist %d (%s)", artistID, artistName)
	} else {
		// Capture 5xx errors to Sentry (service failures)
		if resp.StatusCode >= 500 {
			sentry.WithScope(func(scope *sentry.Scope) {
				scope.SetTag("service", "music-discovery")
				scope.SetExtra("artist_id", artistID)
				scope.SetExtra("artist_name", artistName)
				scope.SetExtra("status_code", resp.StatusCode)
				sentry.CaptureMessage(fmt.Sprintf("Discovery endpoint returned %d", resp.StatusCode))
			})
		}
		log.Printf("[MusicDiscovery] Discovery endpoint returned status %d for artist %d (%s)", resp.StatusCode, artistID, artistName)
	}
}
