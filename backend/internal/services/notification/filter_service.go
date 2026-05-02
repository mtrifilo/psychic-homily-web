package notification

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/getsentry/sentry-go"
	"github.com/lib/pq"
	"gorm.io/gorm"

	"psychic-homily-backend/db"
	catalogm "psychic-homily-backend/internal/models/catalog"
	notificationm "psychic-homily-backend/internal/models/notification"
	"psychic-homily-backend/internal/services/contracts"
)

// NotificationFilterService handles notification filter CRUD, matching, and delivery.
type NotificationFilterService struct {
	db           *gorm.DB
	emailService contracts.EmailServiceInterface
	jwtSecret    string // for HMAC unsubscribe URLs
	frontendURL  string
}

// NewNotificationFilterService creates a new notification filter service.
func NewNotificationFilterService(database *gorm.DB, emailService contracts.EmailServiceInterface, jwtSecret, frontendURL string) *NotificationFilterService {
	if database == nil {
		database = db.GetDB()
	}
	return &NotificationFilterService{
		db:           database,
		emailService: emailService,
		jwtSecret:    jwtSecret,
		frontendURL:  frontendURL,
	}
}

// maxFiltersPerUser is the maximum number of filters a user can create.
const maxFiltersPerUser = 50

// maxFilterEmailsPerDay is the maximum filter notification emails per user per day.
const maxFilterEmailsPerDay = 10

// ──────────────────────────────────────────────
// CRUD operations
// ──────────────────────────────────────────────

// CreateFilter creates a new notification filter for a user.
func (s *NotificationFilterService) CreateFilter(userID uint, input contracts.CreateFilterInput) (*notificationm.NotificationFilter, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	// Validate at least one criteria is set
	if !hasAnyCriteria(input) {
		return nil, fmt.Errorf("at least one filter criteria is required")
	}

	// Check filter count limit
	var count int64
	if err := s.db.Model(&notificationm.NotificationFilter{}).Where("user_id = ?", userID).Count(&count).Error; err != nil {
		return nil, fmt.Errorf("failed to count filters: %w", err)
	}
	if count >= maxFiltersPerUser {
		return nil, fmt.Errorf("maximum of %d filters per user", maxFiltersPerUser)
	}

	now := time.Now().UTC()
	filter := notificationm.NotificationFilter{
		UserID:        userID,
		Name:          input.Name,
		IsActive:      true,
		ArtistIDs:     toInt64Array(input.ArtistIDs),
		VenueIDs:      toInt64Array(input.VenueIDs),
		LabelIDs:      toInt64Array(input.LabelIDs),
		TagIDs:        toInt64Array(input.TagIDs),
		ExcludeTagIDs: toInt64Array(input.ExcludeTagIDs),
		PriceMaxCents: input.PriceMaxCents,
		NotifyEmail:   input.NotifyEmail,
		NotifyInApp:   input.NotifyInApp,
		CreatedAt:     now,
		UpdatedAt:     now,
	}

	if len(input.Cities) > 0 {
		raw := json.RawMessage(input.Cities)
		filter.Cities = &raw
	}

	if err := s.db.Create(&filter).Error; err != nil {
		return nil, fmt.Errorf("failed to create filter: %w", err)
	}

	return &filter, nil
}

// UpdateFilter updates an existing filter owned by the user.
func (s *NotificationFilterService) UpdateFilter(userID uint, filterID uint, input contracts.UpdateFilterInput) (*notificationm.NotificationFilter, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	var filter notificationm.NotificationFilter
	if err := s.db.Where("id = ? AND user_id = ?", filterID, userID).First(&filter).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("filter not found")
		}
		return nil, fmt.Errorf("failed to get filter: %w", err)
	}

	updates := map[string]interface{}{
		"updated_at": time.Now().UTC(),
	}

	if input.Name != nil {
		updates["name"] = *input.Name
	}
	if input.IsActive != nil {
		updates["is_active"] = *input.IsActive
	}
	if input.ArtistIDs != nil {
		updates["artist_ids"] = toInt64Array(*input.ArtistIDs)
	}
	if input.VenueIDs != nil {
		updates["venue_ids"] = toInt64Array(*input.VenueIDs)
	}
	if input.LabelIDs != nil {
		updates["label_ids"] = toInt64Array(*input.LabelIDs)
	}
	if input.TagIDs != nil {
		updates["tag_ids"] = toInt64Array(*input.TagIDs)
	}
	if input.ExcludeTagIDs != nil {
		updates["exclude_tag_ids"] = toInt64Array(*input.ExcludeTagIDs)
	}
	if input.Cities != nil {
		updates["cities"] = *input.Cities
	}
	if input.PriceMaxCents != nil {
		updates["price_max_cents"] = *input.PriceMaxCents
	}
	if input.NotifyEmail != nil {
		updates["notify_email"] = *input.NotifyEmail
	}
	if input.NotifyInApp != nil {
		updates["notify_in_app"] = *input.NotifyInApp
	}

	if err := s.db.Model(&filter).Updates(updates).Error; err != nil {
		return nil, fmt.Errorf("failed to update filter: %w", err)
	}

	// Reload
	if err := s.db.First(&filter, filterID).Error; err != nil {
		return nil, fmt.Errorf("failed to reload filter: %w", err)
	}

	return &filter, nil
}

// DeleteFilter deletes a filter owned by the user.
func (s *NotificationFilterService) DeleteFilter(userID uint, filterID uint) error {
	if s.db == nil {
		return fmt.Errorf("database not initialized")
	}

	result := s.db.Where("id = ? AND user_id = ?", filterID, userID).Delete(&notificationm.NotificationFilter{})
	if result.Error != nil {
		return fmt.Errorf("failed to delete filter: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("filter not found")
	}
	return nil
}

// GetUserFilters returns all filters for a user.
func (s *NotificationFilterService) GetUserFilters(userID uint) ([]notificationm.NotificationFilter, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	var filters []notificationm.NotificationFilter
	if err := s.db.Where("user_id = ?", userID).Order("created_at DESC").Find(&filters).Error; err != nil {
		return nil, fmt.Errorf("failed to get filters: %w", err)
	}
	return filters, nil
}

// GetFilter returns a single filter owned by the user.
func (s *NotificationFilterService) GetFilter(userID uint, filterID uint) (*notificationm.NotificationFilter, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	var filter notificationm.NotificationFilter
	if err := s.db.Where("id = ? AND user_id = ?", filterID, userID).First(&filter).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("filter not found")
		}
		return nil, fmt.Errorf("failed to get filter: %w", err)
	}
	return &filter, nil
}

// ──────────────────────────────────────────────
// Quick create
// ──────────────────────────────────────────────

// QuickCreateFilter creates a filter from a single entity shortcut.
// E.g., "Notify me about Deafheaven shows" creates a filter with artist_ids=[42].
func (s *NotificationFilterService) QuickCreateFilter(userID uint, entityType string, entityID uint) (*notificationm.NotificationFilter, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	input := contracts.CreateFilterInput{
		NotifyEmail: true,
		NotifyInApp: true,
	}

	entityIDInt64 := int64(entityID)

	switch entityType {
	case "artist":
		// Look up artist name for filter name
		var name string
		if err := s.db.Table("artists").Where("id = ?", entityID).Pluck("name", &name).Error; err != nil {
			return nil, fmt.Errorf("artist not found")
		}
		input.Name = fmt.Sprintf("%s shows", name)
		input.ArtistIDs = []int64{entityIDInt64}
	case "venue":
		var name string
		if err := s.db.Table("venues").Where("id = ?", entityID).Pluck("name", &name).Error; err != nil {
			return nil, fmt.Errorf("venue not found")
		}
		input.Name = fmt.Sprintf("Shows at %s", name)
		input.VenueIDs = []int64{entityIDInt64}
	case "label":
		var name string
		if err := s.db.Table("labels").Where("id = ?", entityID).Pluck("name", &name).Error; err != nil {
			return nil, fmt.Errorf("label not found")
		}
		input.Name = fmt.Sprintf("%s artists", name)
		input.LabelIDs = []int64{entityIDInt64}
	case "tag":
		var name string
		if err := s.db.Table("tags").Where("id = ?", entityID).Pluck("name", &name).Error; err != nil {
			return nil, fmt.Errorf("tag not found")
		}
		input.Name = fmt.Sprintf("%s shows", name)
		input.TagIDs = []int64{entityIDInt64}
	default:
		return nil, fmt.Errorf("invalid entity type: %s (must be artist, venue, label, or tag)", entityType)
	}

	return s.CreateFilter(userID, input)
}

// ──────────────────────────────────────────────
// Matching engine
// ──────────────────────────────────────────────

// filterMatch holds the result of a matching query row.
type filterMatch struct {
	FilterID    uint   `gorm:"column:filter_id"`
	UserID      uint   `gorm:"column:user_id"`
	FilterName  string `gorm:"column:name"`
	NotifyEmail bool   `gorm:"column:notify_email"`
	NotifyInApp bool   `gorm:"column:notify_in_app"`
}

// MatchAndNotify finds all active filters that match the given show and sends notifications.
// This is designed to be called fire-and-forget from the show approval handler.
func (s *NotificationFilterService) MatchAndNotify(show *catalogm.Show) error {
	if s.db == nil {
		return fmt.Errorf("database not initialized")
	}
	if show == nil {
		return nil
	}

	// Gather the show's related IDs for matching
	showArtistIDs, showVenueIDs, artistLabelIDs, artistTagIDs, err := s.gatherShowRelations(show.ID)
	if err != nil {
		return fmt.Errorf("failed to gather show relations: %w", err)
	}

	// Build city JSON for matching
	var cityJSON []byte
	if show.City != nil && show.State != nil {
		cityJSON, _ = json.Marshal([]map[string]string{
			{"city": *show.City, "state": *show.State},
		})
	}

	// Get show price in cents (nullable)
	var priceCents *int
	if show.Price != nil {
		cents := int(*show.Price * 100)
		priceCents = &cents
	}

	// Run the matching query
	matches, err := s.findMatchingFilters(show.ID, showArtistIDs, showVenueIDs, artistLabelIDs, artistTagIDs, cityJSON, priceCents)
	if err != nil {
		return fmt.Errorf("failed to find matching filters: %w", err)
	}

	if len(matches) == 0 {
		return nil
	}

	// Group matches by user for deduplication and email batching
	userMatches := make(map[uint][]filterMatch)
	for _, m := range matches {
		userMatches[m.UserID] = append(userMatches[m.UserID], m)
	}

	// Process each user's matches
	for userID, userFilterMatches := range userMatches {
		s.processUserMatches(userID, show, userFilterMatches)
	}

	return nil
}

// MatchAndNotifyBatch matches multiple shows against all active filters.
// Used after batch approval.
func (s *NotificationFilterService) MatchAndNotifyBatch(shows []catalogm.Show) error {
	if s.db == nil {
		return fmt.Errorf("database not initialized")
	}

	for i := range shows {
		if err := s.MatchAndNotify(&shows[i]); err != nil {
			// Log but don't fail the batch
			log.Printf("notification filter match error for show %d: %v", shows[i].ID, err)
		}
	}

	return nil
}

// gatherShowRelations collects artist IDs, venue IDs, artist label IDs, and artist tag IDs
// for a given show.
func (s *NotificationFilterService) gatherShowRelations(showID uint) (artistIDs, venueIDs, labelIDs, tagIDs pq.Int64Array, err error) {
	// Get artist IDs from show_artists
	err = s.db.Table("show_artists").
		Where("show_id = ?", showID).
		Pluck("artist_id", &artistIDs).Error
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("failed to get show artists: %w", err)
	}

	// Get venue IDs from show_venues
	err = s.db.Table("show_venues").
		Where("show_id = ?", showID).
		Pluck("venue_id", &venueIDs).Error
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("failed to get show venues: %w", err)
	}

	// Get label IDs for all artists on this show (via artist_releases → release_labels).
	// Labels are M2M with releases through the release_labels junction table
	// (not a foreign key on releases). An artist can also be directly signed
	// to labels via artist_labels, which is merged in below.
	if len(artistIDs) > 0 {
		err = s.db.Table("artist_releases").
			Joins("JOIN release_labels ON release_labels.release_id = artist_releases.release_id").
			Where("artist_releases.artist_id IN ?", []int64(artistIDs)).
			Distinct().
			Pluck("release_labels.label_id", &labelIDs).Error
		if err != nil {
			// Non-fatal: labels are optional
			log.Printf("warning: failed to get release labels for show %d: %v", showID, err)
			labelIDs = nil
		}

		// Also include labels the artists are directly signed to (artist_labels).
		var artistLabelIDs pq.Int64Array
		if err2 := s.db.Table("artist_labels").
			Where("artist_id IN ?", []int64(artistIDs)).
			Distinct().
			Pluck("label_id", &artistLabelIDs).Error; err2 != nil {
			log.Printf("warning: failed to get artist direct labels for show %d: %v", showID, err2)
		} else {
			// Merge and dedupe
			seen := make(map[int64]struct{}, len(labelIDs))
			for _, id := range labelIDs {
				seen[id] = struct{}{}
			}
			for _, id := range artistLabelIDs {
				if _, ok := seen[id]; !ok {
					labelIDs = append(labelIDs, id)
					seen[id] = struct{}{}
				}
			}
		}
	}

	// Get tag IDs for all artists on this show (via entity_tags)
	if len(artistIDs) > 0 {
		err = s.db.Table("entity_tags").
			Where("entity_type = ? AND entity_id IN ?", "artist", []int64(artistIDs)).
			Distinct().
			Pluck("tag_id", &tagIDs).Error
		if err != nil {
			// Non-fatal: tags are optional
			log.Printf("warning: failed to get artist tags for show %d: %v", showID, err)
			tagIDs = nil
		}
	}

	return artistIDs, venueIDs, labelIDs, tagIDs, nil
}

// findMatchingFilters runs the matching query against all active filters.
func (s *NotificationFilterService) findMatchingFilters(
	showID uint,
	showArtistIDs, showVenueIDs, artistLabelIDs, artistTagIDs pq.Int64Array,
	cityJSON []byte,
	priceCents *int,
) ([]filterMatch, error) {
	var matches []filterMatch

	// Build the matching query using PostgreSQL array overlap operator (&&).
	// GORM uses ? for parameter binding.
	query := `
		SELECT nf.id as filter_id, nf.user_id, nf.name, nf.notify_email, nf.notify_in_app
		FROM notification_filters nf
		WHERE nf.is_active = TRUE
		  AND (nf.artist_ids IS NULL OR nf.artist_ids && ?::bigint[])
		  AND (nf.venue_ids IS NULL OR nf.venue_ids && ?::bigint[])
		  AND (nf.label_ids IS NULL OR nf.label_ids && ?::bigint[])
		  AND (nf.tag_ids IS NULL OR nf.tag_ids && ?::bigint[])
		  AND (nf.exclude_tag_ids IS NULL OR NOT (nf.exclude_tag_ids && ?::bigint[]))
		  AND (nf.cities IS NULL OR nf.cities @> ?::jsonb)
		  AND (nf.price_max_cents IS NULL OR ?::int IS NULL OR ? <= nf.price_max_cents)
		  AND NOT EXISTS (
		      SELECT 1 FROM notification_log nl
		      WHERE nl.filter_id = nf.id AND nl.entity_type = 'show' AND nl.entity_id = ?
		  )
	`

	// Handle nil arrays — PostgreSQL needs empty arrays, not NULL
	if showArtistIDs == nil {
		showArtistIDs = pq.Int64Array{}
	}
	if showVenueIDs == nil {
		showVenueIDs = pq.Int64Array{}
	}
	if artistLabelIDs == nil {
		artistLabelIDs = pq.Int64Array{}
	}
	if artistTagIDs == nil {
		artistTagIDs = pq.Int64Array{}
	}
	if cityJSON == nil {
		cityJSON = []byte("[]")
	}

	err := s.db.Raw(query,
		showArtistIDs,    // artist_ids &&
		showVenueIDs,     // venue_ids &&
		artistLabelIDs,   // label_ids &&
		artistTagIDs,     // tag_ids &&
		artistTagIDs,     // exclude_tag_ids && (same tag IDs)
		string(cityJSON), // cities @>
		priceCents,       // price_max_cents IS NULL OR ?
		priceCents,       // ? <= nf.price_max_cents
		showID,           // entity_id = ?
	).Scan(&matches).Error

	if err != nil {
		return nil, fmt.Errorf("matching query failed: %w", err)
	}

	return matches, nil
}

// processUserMatches handles matched filters for a single user: inserts notification log
// entries and sends emails.
func (s *NotificationFilterService) processUserMatches(userID uint, show *catalogm.Show, matches []filterMatch) {
	now := time.Now().UTC()

	for _, match := range matches {
		// Insert notification_log entry for each match
		logEntry := notificationm.NotificationLog{
			UserID:     userID,
			FilterID:   &match.FilterID,
			EntityType: "show",
			EntityID:   show.ID,
			Channel:    "email",
			SentAt:     now,
		}
		if err := s.db.Create(&logEntry).Error; err != nil {
			log.Printf("failed to insert notification log for user %d, filter %d, show %d: %v",
				userID, match.FilterID, show.ID, err)
			continue
		}

		// Update the filter's last_matched_at and match_count
		s.db.Model(&notificationm.NotificationFilter{}).
			Where("id = ?", match.FilterID).
			Updates(map[string]interface{}{
				"last_matched_at": now,
				"match_count":     gorm.Expr("match_count + 1"),
			})

		// Send email if configured
		if match.NotifyEmail && s.emailService != nil && s.emailService.IsConfigured() {
			s.sendFilterEmail(userID, match.FilterID, match.FilterName, show)
		}
	}
}

// sendFilterEmail sends a notification email for a matched filter.
// Rate limited to maxFilterEmailsPerDay per user per day.
func (s *NotificationFilterService) sendFilterEmail(userID uint, filterID uint, filterName string, show *catalogm.Show) {
	// Check rate limit: max emails per user per day
	var emailCount int64
	dayAgo := time.Now().UTC().Add(-24 * time.Hour)
	s.db.Model(&notificationm.NotificationLog{}).
		Where("user_id = ? AND channel = ? AND sent_at > ?", userID, "email", dayAgo).
		Count(&emailCount)

	if emailCount >= int64(maxFilterEmailsPerDay) {
		log.Printf("rate limit: skipping filter email for user %d (sent %d today)", userID, emailCount)
		return
	}

	// Look up user email
	var email string
	if err := s.db.Table("users").Where("id = ?", userID).Pluck("email", &email).Error; err != nil || email == "" {
		log.Printf("failed to get email for user %d: %v", userID, err)
		return
	}

	// Build show info
	showTitle := show.Title
	showDate := show.EventDate.Format("Monday, January 2, 2006")

	var venueNames []string
	s.db.Table("show_venues").
		Joins("JOIN venues ON venues.id = show_venues.venue_id").
		Where("show_venues.show_id = ?", show.ID).
		Pluck("venues.name", &venueNames)

	var artistNames []string
	s.db.Table("show_artists").
		Joins("JOIN artists ON artists.id = show_artists.artist_id").
		Where("show_artists.show_id = ?", show.ID).
		Order("show_artists.position ASC").
		Pluck("artists.name", &artistNames)

	showSlug := ""
	if show.Slug != nil {
		showSlug = *show.Slug
	}
	showURL := fmt.Sprintf("%s/shows/%s", s.frontendURL, showSlug)
	if showSlug == "" {
		showURL = fmt.Sprintf("%s/shows/%d", s.frontendURL, show.ID)
	}

	// Unsubscribe URL (HMAC-signed)
	unsubscribeURL := GenerateFilterUnsubscribeURL(s.frontendURL, filterID, s.jwtSecret)

	venueText := ""
	if len(venueNames) > 0 {
		venueText = strings.Join(venueNames, ", ")
	}
	artistText := ""
	if len(artistNames) > 0 {
		artistText = strings.Join(artistNames, ", ")
	}
	priceText := ""
	if show.Price != nil {
		priceText = fmt.Sprintf("$%.0f", *show.Price)
	}

	html := buildFilterEmailHTML(filterName, showTitle, showDate, venueText, artistText, priceText, showURL, unsubscribeURL)

	subject := fmt.Sprintf("New show matching \"%s\"", filterName)
	if err := s.sendEmail(email, subject, html, unsubscribeURL); err != nil {
		sentry.WithScope(func(scope *sentry.Scope) {
			scope.SetTag("service", "notification_filter")
			scope.SetTag("email_type", "filter_match")
			sentry.CaptureException(err)
		})
		log.Printf("failed to send filter notification email to %s: %v", email, err)
	}
}

// sendEmail sends an email via the email service.
func (s *NotificationFilterService) sendEmail(to, subject, html, unsubscribeURL string) error {
	return s.emailService.SendFilterNotificationEmail(to, subject, html, unsubscribeURL)
}

// ──────────────────────────────────────────────
// Notification log
// ──────────────────────────────────────────────

// GetUserNotifications returns the notification log for a user, paginated.
func (s *NotificationFilterService) GetUserNotifications(userID uint, limit, offset int) ([]contracts.NotificationLogEntry, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	var logs []struct {
		notificationm.NotificationLog
		FilterName string `gorm:"column:filter_name"`
	}

	err := s.db.Table("notification_log nl").
		Select("nl.*, nf.name as filter_name").
		Joins("LEFT JOIN notification_filters nf ON nf.id = nl.filter_id").
		Where("nl.user_id = ?", userID).
		Order("nl.sent_at DESC").
		Limit(limit).
		Offset(offset).
		Find(&logs).Error
	if err != nil {
		return nil, fmt.Errorf("failed to get notifications: %w", err)
	}

	entries := make([]contracts.NotificationLogEntry, len(logs))
	for i, l := range logs {
		entries[i] = contracts.NotificationLogEntry{
			ID:         l.ID,
			FilterID:   l.FilterID,
			FilterName: l.FilterName,
			EntityType: l.EntityType,
			EntityID:   l.EntityID,
			Channel:    l.Channel,
			SentAt:     l.SentAt,
			ReadAt:     l.ReadAt,
		}
	}
	return entries, nil
}

// GetUnreadCount returns the number of unread notifications for a user.
func (s *NotificationFilterService) GetUnreadCount(userID uint) (int64, error) {
	if s.db == nil {
		return 0, fmt.Errorf("database not initialized")
	}

	var count int64
	err := s.db.Model(&notificationm.NotificationLog{}).
		Where("user_id = ? AND read_at IS NULL", userID).
		Count(&count).Error
	return count, err
}

// ──────────────────────────────────────────────
// Unsubscribe
// ──────────────────────────────────────────────

// PauseFilter pauses a filter (sets is_active = false).
// Used by HMAC-signed unsubscribe link in emails.
func (s *NotificationFilterService) PauseFilter(filterID uint) error {
	if s.db == nil {
		return fmt.Errorf("database not initialized")
	}

	result := s.db.Model(&notificationm.NotificationFilter{}).
		Where("id = ?", filterID).
		Update("is_active", false)
	if result.Error != nil {
		return fmt.Errorf("failed to pause filter: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("filter not found")
	}
	return nil
}

// ──────────────────────────────────────────────
// HMAC signature helpers for filter unsubscribe
// ──────────────────────────────────────────────

// GenerateFilterUnsubscribeURL generates an HMAC-signed URL for pausing a filter.
func GenerateFilterUnsubscribeURL(baseURL string, filterID uint, secret string) string {
	sig := ComputeFilterUnsubscribeSignature(filterID, secret)
	return fmt.Sprintf("%s/unsubscribe/filter/%d?sig=%s", baseURL, filterID, sig)
}

// VerifyFilterUnsubscribeSignature checks the HMAC signature for a filter unsubscribe request.
func VerifyFilterUnsubscribeSignature(filterID uint, signature, secret string) bool {
	expected := ComputeFilterUnsubscribeSignature(filterID, secret)
	return hmac.Equal([]byte(expected), []byte(signature))
}

// ComputeFilterUnsubscribeSignature computes HMAC-SHA256 of the filter ID.
func ComputeFilterUnsubscribeSignature(filterID uint, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(fmt.Sprintf("unsubscribe:filter:%d", filterID)))
	return hex.EncodeToString(mac.Sum(nil))
}

// ──────────────────────────────────────────────
// Email template
// ──────────────────────────────────────────────

func buildFilterEmailHTML(filterName, showTitle, showDate, venueText, artistText, priceText, showURL, unsubscribeURL string) string {
	venueSection := ""
	if venueText != "" {
		venueSection = fmt.Sprintf(`<p style="font-size: 15px; color: #444; margin: 4px 0;"><strong>Venue:</strong> %s</p>`, venueText)
	}
	artistSection := ""
	if artistText != "" {
		artistSection = fmt.Sprintf(`<p style="font-size: 15px; color: #444; margin: 4px 0;"><strong>Artists:</strong> %s</p>`, artistText)
	}
	priceSection := ""
	if priceText != "" {
		priceSection = fmt.Sprintf(`<p style="font-size: 15px; color: #444; margin: 4px 0;"><strong>Price:</strong> %s</p>`, priceText)
	}

	return fmt.Sprintf(`
<!DOCTYPE html>
<html>
<head>
    <meta charset="utf-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
</head>
<body style="font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, 'Helvetica Neue', Arial, sans-serif; line-height: 1.6; color: #333; max-width: 600px; margin: 0 auto; padding: 20px;">
    <div style="text-align: center; margin-bottom: 30px;">
        <h1 style="color: #1a1a1a; margin: 0;">Psychic Homily</h1>
    </div>

    <div style="background: #f9f9f9; border-radius: 8px; padding: 30px; margin-bottom: 20px;">
        <h2 style="margin-top: 0; color: #1a1a1a;">New show matching "%s"</h2>
        <p style="font-size: 18px; font-weight: 600; color: #1a1a1a; margin: 8px 0;">%s</p>
        <p style="font-size: 15px; color: #444; margin: 4px 0;"><strong>Date:</strong> %s</p>
        %s
        %s
        %s
        <p style="text-align: center; margin: 30px 0;">
            <a href="%s" style="display: inline-block; background: #f97316; color: white; text-decoration: none; padding: 12px 30px; border-radius: 6px; font-weight: 600;">View Show</a>
        </p>
    </div>

    <div style="text-align: center; font-size: 12px; color: #999;">
        <p>Don't want these notifications? <a href="%s" style="color: #666;">Pause this filter</a></p>
    </div>
</body>
</html>
`, filterName, showTitle, showDate, venueSection, artistSection, priceSection, showURL, unsubscribeURL)
}

// ──────────────────────────────────────────────
// Helpers
// ──────────────────────────────────────────────

// hasAnyCriteria returns true if at least one filter criteria is set.
func hasAnyCriteria(input contracts.CreateFilterInput) bool {
	if len(input.ArtistIDs) > 0 {
		return true
	}
	if len(input.VenueIDs) > 0 {
		return true
	}
	if len(input.LabelIDs) > 0 {
		return true
	}
	if len(input.TagIDs) > 0 {
		return true
	}
	if len(input.ExcludeTagIDs) > 0 {
		return true
	}
	if len(input.Cities) > 0 {
		return true
	}
	if input.PriceMaxCents != nil {
		return true
	}
	return false
}

// toInt64Array converts a slice of int64 to pq.Int64Array, returning nil for empty slices.
func toInt64Array(ids []int64) pq.Int64Array {
	if len(ids) == 0 {
		return nil
	}
	return pq.Int64Array(ids)
}
