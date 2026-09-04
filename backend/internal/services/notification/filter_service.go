package notification

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/getsentry/sentry-go"
	"github.com/lib/pq"
	"gorm.io/gorm"

	"psychic-homily-backend/db"
	apperrors "psychic-homily-backend/internal/errors"
	catalogm "psychic-homily-backend/internal/models/catalog"
	engagementm "psychic-homily-backend/internal/models/engagement"
	notificationm "psychic-homily-backend/internal/models/notification"
	"psychic-homily-backend/internal/services/contracts"
	"psychic-homily-backend/internal/services/engagement"
	"psychic-homily-backend/internal/services/shared"
	"psychic-homily-backend/internal/utils"
)

// NotificationFilterService handles notification filter CRUD, matching, and delivery.
type NotificationFilterService struct {
	db           *gorm.DB
	emailService contracts.EmailServiceInterface
	jwtSecret    string // for HMAC unsubscribe URLs
	frontendURL  string

	// venueAlertFailures records, per venue-day batch, when its flush FIRST
	// started failing (PSY-1895). It is what lets the poison-pill bound measure
	// how long a group has been broken rather than how old its rows are — see
	// noteVenueAlertGroupFailure for why that distinction decides whether an
	// outage recovery delivers a backlog or destroys it.
	//
	// Entries exist only while a group is failing and are removed on success or
	// on retirement, so the map is bounded by the number of simultaneously-broken
	// venue-days. Guarded because the flush poller and the request-path methods
	// share one service instance.
	venueAlertFailuresMu sync.Mutex
	venueAlertFailures   map[venueAlertGroupKey]time.Time
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
	return s.createFilter(s.db, userID, input, notificationm.FilterSourceUser)
}

// createFilter inserts a filter with the given ownership source. Shared by
// settings CreateFilter (source=user) and QuickCreateFilter (source=managed)
// so ownership is stamped in the same write as the row (PSY-1467).
func (s *NotificationFilterService) createFilter(
	tx *gorm.DB,
	userID uint,
	input contracts.CreateFilterInput,
	source string,
) (*notificationm.NotificationFilter, error) {
	if tx == nil {
		return nil, apperrors.ErrFilterInternal(fmt.Errorf("database not initialized"))
	}

	if !hasAnyCriteria(input) {
		return nil, apperrors.ErrFilterValidation("at least one filter criteria is required")
	}

	var count int64
	if err := tx.Model(&notificationm.NotificationFilter{}).Where("user_id = ?", userID).Count(&count).Error; err != nil {
		return nil, apperrors.ErrFilterInternal(fmt.Errorf("failed to count filters: %w", err))
	}
	if count >= maxFiltersPerUser {
		return nil, apperrors.ErrFilterValidation(fmt.Sprintf("maximum of %d filters per user", maxFiltersPerUser))
	}

	now := time.Now().UTC()
	filter := notificationm.NotificationFilter{
		UserID:        userID,
		Name:          input.Name,
		Source:        source,
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

	if err := tx.Create(&filter).Error; err != nil {
		return nil, apperrors.ErrFilterInternal(fmt.Errorf("failed to create filter: %w", err))
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
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperrors.ErrFilterNotFound()
		}
		return nil, apperrors.ErrFilterInternal(fmt.Errorf("failed to get filter: %w", err))
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

	// Any settings edit of a managed quick subscription promotes it to a
	// user-owned advanced filter so NotifyMeButton no longer owns its lifecycle
	// (PSY-1467). Pause-only (is_active) via email unsubscribe uses PauseFilter,
	// not this path.
	criteriaEdited := input.Name != nil || input.ArtistIDs != nil || input.VenueIDs != nil ||
		input.LabelIDs != nil || input.TagIDs != nil || input.ExcludeTagIDs != nil ||
		input.Cities != nil || input.PriceMaxCents != nil ||
		input.NotifyEmail != nil || input.NotifyInApp != nil
	if criteriaEdited && filter.Source == notificationm.FilterSourceManaged {
		updates["source"] = notificationm.FilterSourceUser
	}

	if err := s.db.Model(&filter).Updates(updates).Error; err != nil {
		return nil, apperrors.ErrFilterInternal(fmt.Errorf("failed to update filter: %w", err))
	}

	// Reload
	if err := s.db.First(&filter, filterID).Error; err != nil {
		return nil, apperrors.ErrFilterInternal(fmt.Errorf("failed to reload filter: %w", err))
	}

	return &filter, nil
}

// DeleteFilter deletes a filter owned by the user.
func (s *NotificationFilterService) DeleteFilter(userID uint, filterID uint) error {
	if s.db == nil {
		return apperrors.ErrFilterInternal(fmt.Errorf("database not initialized"))
	}

	result := s.db.Where("id = ? AND user_id = ?", filterID, userID).Delete(&notificationm.NotificationFilter{})
	if result.Error != nil {
		return apperrors.ErrFilterInternal(fmt.Errorf("failed to delete filter: %w", result.Error))
	}
	if result.RowsAffected == 0 {
		return apperrors.ErrFilterNotFound()
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
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("filter not found")
		}
		return nil, fmt.Errorf("failed to get filter: %w", err)
	}
	return &filter, nil
}

// ──────────────────────────────────────────────
// Quick create
// ──────────────────────────────────────────────

// QuickCreateFilter creates a managed filter from a single entity shortcut.
// E.g., "Notify me about Deafheaven shows" creates a filter with artist_ids=[42]
// and source=managed. Idempotent: returns the existing managed row if one already
// covers this entity. Inserts source=managed in a single write inside a
// transaction so a failed follow-up update cannot leave a ghost user-owned row
// (PSY-1467 adversarial review).
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

	var created *notificationm.NotificationFilter
	err := s.db.Transaction(func(tx *gorm.DB) error {
		existing, err := s.findManagedQuickFilterTx(tx, userID, entityType, entityID)
		if err != nil {
			return err
		}
		if existing != nil {
			created = existing
			return nil
		}
		filter, err := s.createFilter(tx, userID, input, notificationm.FilterSourceManaged)
		if err != nil {
			return err
		}
		created = filter
		return nil
	})
	if err != nil {
		return nil, err
	}
	return created, nil
}

// findManagedQuickFilterTx returns the user's active managed quick subscription
// for the given entity, or nil if none. Matches only single-entity managed rows
// so a settings-promoted compound filter is never treated as the quick toggle's
// own.
func (s *NotificationFilterService) findManagedQuickFilterTx(
	tx *gorm.DB,
	userID uint,
	entityType string,
	entityID uint,
) (*notificationm.NotificationFilter, error) {
	var filters []notificationm.NotificationFilter
	q := tx.Where("user_id = ? AND source = ? AND is_active = TRUE", userID, notificationm.FilterSourceManaged)
	entityIDInt64 := int64(entityID)
	switch entityType {
	case "artist":
		q = q.Where("artist_ids @> ARRAY[?]::bigint[]", entityIDInt64)
	case "venue":
		q = q.Where("venue_ids @> ARRAY[?]::bigint[]", entityIDInt64)
	case "label":
		q = q.Where("label_ids @> ARRAY[?]::bigint[]", entityIDInt64)
	case "tag":
		q = q.Where("tag_ids @> ARRAY[?]::bigint[]", entityIDInt64)
	default:
		return nil, nil
	}
	if err := q.Order("created_at ASC").Find(&filters).Error; err != nil {
		return nil, apperrors.ErrFilterInternal(fmt.Errorf("failed to look up managed filter: %w", err))
	}
	for i := range filters {
		if isSingleEntityManagedFilter(&filters[i], entityType, entityID) {
			return &filters[i], nil
		}
	}
	return nil, nil
}

// isSingleEntityManagedFilter reports whether f is a quick-toggle-shaped managed
// subscription for exactly one entity of the given type (no other criteria).
func isSingleEntityManagedFilter(f *notificationm.NotificationFilter, entityType string, entityID uint) bool {
	if f == nil || f.Source != notificationm.FilterSourceManaged {
		return false
	}
	id := int64(entityID)
	hasOnly := func(arr pq.Int64Array) bool {
		return len(arr) == 1 && arr[0] == id
	}
	switch entityType {
	case "artist":
		return hasOnly(f.ArtistIDs) && len(f.VenueIDs) == 0 && len(f.LabelIDs) == 0 &&
			len(f.TagIDs) == 0 && len(f.ExcludeTagIDs) == 0 && f.Cities == nil && f.PriceMaxCents == nil
	case "venue":
		return hasOnly(f.VenueIDs) && len(f.ArtistIDs) == 0 && len(f.LabelIDs) == 0 &&
			len(f.TagIDs) == 0 && len(f.ExcludeTagIDs) == 0 && f.Cities == nil && f.PriceMaxCents == nil
	case "label":
		return hasOnly(f.LabelIDs) && len(f.ArtistIDs) == 0 && len(f.VenueIDs) == 0 &&
			len(f.TagIDs) == 0 && len(f.ExcludeTagIDs) == 0 && f.Cities == nil && f.PriceMaxCents == nil
	case "tag":
		return hasOnly(f.TagIDs) && len(f.ArtistIDs) == 0 && len(f.VenueIDs) == 0 &&
			len(f.LabelIDs) == 0 && len(f.ExcludeTagIDs) == 0 && f.Cities == nil && f.PriceMaxCents == nil
	default:
		return false
	}
}

// ──────────────────────────────────────────────
// Matching engine
// ──────────────────────────────────────────────

// filterMatch holds the result of a matching query row.
type filterMatch struct {
	FilterID    uint   `gorm:"column:filter_id"`
	UserID      uint   `gorm:"column:user_id"`
	FilterName  string `gorm:"column:name"`
	Source      string `gorm:"column:source"`
	NotifyEmail bool   `gorm:"column:notify_email"`
	NotifyInApp bool   `gorm:"column:notify_in_app"`
}

// effectiveShowPriceCents is the single price a price_max_cents filter judges a
// show by, in cents, or nil when the show records no price at all.
//
// The advance price when there is one, the door price when that is the only
// price recorded (PSY-1864). The fallback is load-bearing rather than tidy: the
// matching predicate reads a nil price as "cannot exclude", so without it a
// door-only show reports "price unknown" and alerts EVERY user whose filter
// says "max $10", however expensive the door is. shows.door_price is what made
// that row shape reachable, so the fallback ships alongside it.
//
// When BOTH prices are known this is the ADVANCE price. DECIDED, not merely
// inherited (PSY-1962): a price ceiling states what a user is willing to spend
// to get in, and the advance price is what they would actually pay, since a
// door price above it is the price of choosing not to buy ahead. Judging by the
// HIGHEST number they could pay would suppress an alert for a show they can
// afford, and a filter that hides a $35-advance show because its door is $40 is
// wrong in the direction that costs the user the show.
//
// The read surfaces went the other way in the same change and show BOTH numbers,
// because a list has room to state a fact and a filter has to reduce it to one.
func effectiveShowPriceCents(show *catalogm.Show) *int {
	price := show.Price
	if price == nil {
		price = show.DoorPrice
	}
	if price == nil {
		return nil
	}
	cents := int(*price * 100)
	return &cents
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

	// Re-read the canonical row, and fence the whole fanout on it.
	//
	// Callers do not agree on how complete a Show they hand over. The outbox
	// poller passes the row it loaded; both admin approve handlers BUILD A
	// PARTIAL LITERAL (id, title, event date, price, slug, city, state) with no
	// Status and no SubmittedBy. Two facts every pass below depends on are
	// therefore absent on half the call sites: whether the show is publicly
	// visible, and who submitted it (which is what keeps the submitter from being
	// notified about their own show).
	//
	// Reading them here rather than in each pass means the answer cannot differ
	// between passes, and it means the visibility fence actually covers all three
	// fanouts. Announcing a private, pending or rejected show would either leak a
	// user's own list or advertise something moderation has not passed, and a
	// guard that only one of three fanouts respects is not a guard.
	//
	// Neither of today's callers can reach this with an unapproved show, so this
	// is defence in depth rather than a live bug: MatchAndNotify is exported, and
	// the cost of a future caller getting it wrong is mail to strangers.
	//
	// A missing row means the show was deleted between the trigger and this
	// goroutine, which is a reason to say nothing.
	canonical, err := s.loadShowForAlert(show.ID)
	if err != nil {
		return err
	}
	if canonical == nil {
		return nil
	}
	if canonical.Status != catalogm.ShowStatusApproved {
		log.Printf("notification match: refusing to announce show %d with status %q",
			canonical.ID, canonical.Status)
		return nil
	}
	show = canonical

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

	priceCents := effectiveShowPriceCents(show)

	// Run the matching query
	matches, err := s.findMatchingFilters(show.ID, showArtistIDs, showVenueIDs, artistLabelIDs, artistTagIDs, cityJSON, priceCents)
	if err != nil {
		return fmt.Errorf("failed to find matching filters: %w", err)
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

	// Follow-driven fanouts run AFTER the filter pass — even when no filter
	// matched — so their cross-system dedup can defer to filter notifications
	// already logged for this show.
	//
	// Artist follows before scene follows, most specific first: one user gets one
	// notification per show, and "a band you follow announced a show" is a better
	// use of that single slot than "something is on in your city" (PSY-1896).
	s.notifyArtistFollowers(show, showArtistIDs)

	// Venue follows ACCRUE here rather than notifying (PSY-1895). Venue alerts
	// are coalesced to one per venue per venue-local day, so this call records an
	// observation and the flush poller decides who hears about the day's set.
	//
	// Between the artist pass and the scene pass, which is where the per-show
	// fanout would have sat, so the specific-to-general ordering is unchanged:
	// filters, then the band you follow, then the room you follow, then your
	// city. Sitting AFTER the artist pass also matters for a reason accrual
	// itself does not care about — the flush's cross-system dedup reads the rows
	// the earlier passes wrote, so running before them would leave it looking at
	// an empty log and repeating what a more specific alert had just said.
	s.accrueVenueShowAlerts(show, showVenueIDs)

	s.notifySceneFollowers(show, showArtistIDs)

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
	// The NOT EXISTS arm is the filter pass's half of the one-notification-per-
	// (user, show) rule, and it consults the SHARED shape predicate: a user the
	// artist-follow pass already reached must not also be matched here on a later
	// run (an outbox re-process, or an edit followed by a re-approve).
	query := fmt.Sprintf(`
		SELECT nf.id as filter_id, nf.user_id, nf.name, nf.source, nf.notify_email, nf.notify_in_app
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
		      WHERE nl.user_id = nf.user_id
		        AND nl.entity_id = ?
		        AND %s
		  )
		ORDER BY nf.id ASC
	`, notifiedAboutShow("nl"))

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

// processUserMatches handles matched filters for a single user: bumps match
// metadata on every matching filter, then delivers at most one user-visible
// notification for the show (PSY-1467 — managed + advanced overlap must not
// spam). Delivery prefers a managed match when present so the log attributes
// to the quick subscription; otherwise the first match wins.
func (s *NotificationFilterService) processUserMatches(userID uint, show *catalogm.Show, matches []filterMatch) {
	if len(matches) == 0 {
		return
	}
	now := time.Now().UTC()

	for _, match := range matches {
		s.db.Model(&notificationm.NotificationFilter{}).
			Where("id = ?", match.FilterID).
			Updates(map[string]interface{}{
				"last_matched_at": now,
				"match_count":     gorm.Expr("match_count + 1"),
			})
	}

	// Cross-filter + cross-system dedup: one user-visible notification per
	// (user, show), across filters, artist follows and scene follows.
	var existing int64
	if err := s.db.Model(&notificationm.NotificationLog{}).
		Where("user_id = ? AND entity_id = ?", userID, show.ID).
		Where(notifiedAboutShow("notification_log")).
		Count(&existing).Error; err != nil {
		log.Printf("failed to dedup notification for user %d, show %d: %v", userID, show.ID, err)
		return
	}
	if existing > 0 {
		return
	}

	delivery := pickDeliveryMatch(matches)
	logEntry := notificationm.NotificationLog{
		UserID:     userID,
		FilterID:   &delivery.FilterID,
		EntityType: notificationm.NotificationEntityShow,
		EntityID:   show.ID,
		Channel:    notificationm.NotificationChannelEmail,
		SentAt:     now,
	}
	if err := s.db.Create(&logEntry).Error; err != nil {
		log.Printf("failed to insert notification log for user %d, filter %d, show %d: %v",
			userID, delivery.FilterID, show.ID, err)
		return
	}

	if delivery.NotifyEmail && s.emailService != nil && s.emailService.IsConfigured() {
		s.sendFilterEmail(userID, delivery.FilterID, delivery.FilterName, show)
	}
}

// pickDeliveryMatch chooses which matched filter owns the single user-visible
// notification. Prefer managed+email, then any managed, then any email-enabled
// user filter, then the lowest-id match (query is ORDER BY nf.id).
func pickDeliveryMatch(matches []filterMatch) filterMatch {
	for _, m := range matches {
		if m.Source == notificationm.FilterSourceManaged && m.NotifyEmail {
			return m
		}
	}
	for _, m := range matches {
		if m.Source == notificationm.FilterSourceManaged {
			return m
		}
	}
	for _, m := range matches {
		if m.NotifyEmail {
			return m
		}
	}
	return matches[0]
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

	c := s.showEmailContent(show)

	// Unsubscribe URL (HMAC-signed)
	unsubscribeURL := GenerateFilterUnsubscribeURL(s.frontendURL, filterID, s.jwtSecret)

	html, err := buildFilterEmailHTML(filterName, show.Title, c.date, c.venueText, c.artistText, c.priceText, c.showURL, unsubscribeURL)
	if err != nil {
		log.Printf("failed to render filter notification email for user %d: %v", userID, err)
		return
	}

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

// showEmailContentParts is the show-derived content shared by the filter and
// scene-follow notification emails.
type showEmailContentParts struct {
	date       string
	venueText  string
	artistText string
	priceText  string
	showURL    string
}

// showEmailContent builds the show-derived email fields (extracted from
// sendFilterEmail for reuse by the scene-follow email, PSY-1341). Venue
// timezone rendering per PSY-996.
func (s *NotificationFilterService) showEmailContent(show *catalogm.Show) showEmailContentParts {
	type venueRow struct {
		Name     string
		Timezone *string
		State    string
	}
	var venueRows []venueRow
	s.db.Table("show_venues").
		Select("venues.name, venues.timezone, venues.state").
		Joins("JOIN venues ON venues.id = show_venues.venue_id").
		Where("show_venues.show_id = ?", show.ID).
		Scan(&venueRows)

	venueNames := make([]string, 0, len(venueRows))
	var venueTZ *string
	venueState := derefString(show.State)
	for i, v := range venueRows {
		venueNames = append(venueNames, v.Name)
		if i == 0 {
			venueTZ = v.Timezone
			if v.State != "" {
				venueState = v.State
			}
		}
	}

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

	// Through the shared derivation, like every other read surface (PSY-1962).
	// This one line feeds THREE emails -- the filter alert, the scene-follow
	// alert and the artist-follow alert -- and it used to render show.Price
	// alone with "$%.0f", so a door-only show emailed no price at all, a
	// $35/$40 show emailed "$35", and a FREE show emailed "$0".
	//
	// An email is the ICS feed's twin for the reason that made this urgent: it
	// lands in an inbox and stays there, so the number in it outlives the page
	// view that produced it and is what the reader budgets against.
	priceText := shared.ShowPriceText(show.Price, show.DoorPrice)

	return showEmailContentParts{
		date:       show.EventDate.In(utils.EventLocation(venueTZ, venueState)).Format("Monday, January 2, 2006"),
		venueText:  strings.Join(venueNames, ", "),
		artistText: strings.Join(artistNames, ", "),
		priceText:  priceText,
		showURL:    showURL,
	}
}

// sendSceneFollowEmail mirrors sendFilterEmail for a scene follow (PSY-1341):
// same per-user daily rate limit, scene name in place of the filter name, and
// the manage page in place of a filter-scoped one-click unsubscribe (scene
// follows have no filter row to sign; the weekly-digest ticket owns richer
// unsubscribe scoping).
func (s *NotificationFilterService) sendSceneFollowEmail(userID uint, sceneName string, show *catalogm.Show) {
	var emailCount int64
	dayAgo := time.Now().UTC().Add(-24 * time.Hour)
	s.db.Model(&notificationm.NotificationLog{}).
		Where("user_id = ? AND channel = ? AND sent_at > ?", userID, "email", dayAgo).
		Count(&emailCount)
	if emailCount >= int64(maxFilterEmailsPerDay) {
		log.Printf("rate limit: skipping scene-follow email for user %d (sent %d today)", userID, emailCount)
		return
	}

	var email string
	if err := s.db.Table("users").Where("id = ?", userID).Pluck("email", &email).Error; err != nil || email == "" {
		log.Printf("failed to get email for user %d: %v", userID, err)
		return
	}

	c := s.showEmailContent(show)
	manageURL := fmt.Sprintf("%s/following?tab=scene", s.frontendURL)
	html, err := buildFilterEmailHTML(
		fmt.Sprintf("%s scene", sceneName),
		show.Title, c.date, c.venueText, c.artistText, c.priceText, c.showURL, manageURL,
	)
	if err != nil {
		log.Printf("failed to render scene-follow email for user %d: %v", userID, err)
		return
	}
	subject := fmt.Sprintf("New show in %s", sceneName)
	if err := s.sendEmail(email, subject, html, manageURL); err != nil {
		sentry.WithScope(func(scope *sentry.Scope) {
			scope.SetTag("service", "notification_filter")
			scope.SetTag("email_type", "scene_follow")
			sentry.CaptureException(err)
		})
		log.Printf("failed to send scene-follow email to %s: %v", email, err)
	}
}

// sendEmail sends an email via the email service.
func (s *NotificationFilterService) sendEmail(to, subject, html, unsubscribeURL string) error {
	return s.emailService.SendFilterNotificationEmail(to, subject, html, unsubscribeURL)
}

// ──────────────────────────────────────────────
// Notification log
// ──────────────────────────────────────────────

// GetUserNotifications returns viewer's own INBOX rows, paginated.
//
// Not every notification_log row for the user: inboxVisibleRows filters out the
// ones that are not bell entries (see it for which and why), and
// inboxRowsVisibleTo drops the rows leading to an entity viewer may no longer
// see: a withdrawn show, and a collection turned private.
// A caller counting rows here against a raw count of the table will
// disagree, on purpose.
//
// Show-filter and scene-follow rows are returned as-is. Three kinds are enriched
// in batched follow-up passes so the bell/inbox can render a readable line with
// a working link target: comment-driven rows (PSY-595), request-fulfillment rows
// (PSY-890), and artist show-alert rows (PSY-1896).
func (s *NotificationFilterService) GetUserNotifications(viewer contracts.ShowViewer, limit, offset int) ([]contracts.NotificationLogEntry, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}
	// The zero viewer is a construction bug: ShowViewer{} is the codebase's
	// spelling for the public tier on the listing gates, and these methods are
	// self-scoped, so it would otherwise mean "user 0" and answer emptily forever.
	if viewer.UserID == 0 {
		return nil, fmt.Errorf("GetUserNotifications is self-scoped: the viewer carries no user id")
	}
	userID := viewer.UserID

	var logs []struct {
		notificationm.NotificationLog
		FilterName string `gorm:"column:filter_name"`
	}

	// Scene-follow rows (PSY-1341) are show rows with a NULL filter_id — the
	// first such rows ever, so the bell UI's filter-name slot would render
	// bare "show" for them. Re-derive the scene display name from the show's
	// venues (same join branches as sceneFollowersForShow); a since-merged
	// scene row just falls back to the generic label.
	sceneNameSubquery := `(
		SELECT sc.city || ', ' || sc.state || ' scene'
		FROM show_venues sv
		JOIN venues v ON v.id = sv.venue_id
		JOIN scenes sc ON (
			(v.metro IS NOT NULL AND sc.metro = v.metro)
			OR (sc.metro IS NULL
				AND LOWER(TRIM(sc.city)) = LOWER(TRIM(v.city))
				AND LOWER(TRIM(sc.state)) = LOWER(TRIM(v.state)))
		)
		WHERE sv.show_id = nl.entity_id
		LIMIT 1
	)`
	inboxVisibleSQL, inboxVisibleArgs := inboxRowsVisibleTo("nl", viewer)
	err := s.db.Table("notification_log nl").
		Select("nl.*, COALESCE(nf.name, CASE WHEN nl.entity_type = 'show' AND nl.filter_id IS NULL THEN "+sceneNameSubquery+" END, '') as filter_name").
		Joins("LEFT JOIN notification_filters nf ON nf.id = nl.filter_id").
		Where("nl.user_id = ?", userID).
		Where(inboxVisibleRows("nl")).
		Where(inboxVisibleSQL, inboxVisibleArgs...).
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
			ID:              l.ID,
			FilterID:        l.FilterID,
			FilterName:      l.FilterName,
			EntityType:      l.EntityType,
			EntityID:        l.EntityID,
			SubjectEntityID: l.SubjectEntityID,
			Channel:         l.Channel,
			SentAt:          l.SentAt,
			ReadAt:          l.ReadAt,
			// Formatted in UTC, which is where a DATE column arrives from the
			// driver — the value carries a calendar day and no instant, so any
			// other zone would shift it by one.
			AlertBucket: formatAlertBucket(l.AlertBucket),
		}
	}

	// Enrich comment-driven, request-driven + follow-alert rows in batched passes.
	s.enrichCommentNotifications(entries, viewer)
	s.enrichRequestNotifications(entries)
	s.enrichArtistShowAlertNotifications(entries, viewer)
	s.enrichVenueShowAlertNotifications(userID, entries)
	return entries, nil
}

// formatAlertBucket renders a coalesced alert's day as YYYY-MM-DD, or "" when
// the row has no bucket (every per-event writer).
//
// Formatted from the WALL CLOCK, with no zone conversion. A DATE carries
// calendar fields and no instant, so converting it is not a normalization — it
// is a reinterpretation that moves the day. The driver happens to decode DATE at
// midnight UTC today, which makes .UTC() a no-op; the moment a connection
// TimeZone east of UTC changed that, .UTC() would roll the value back to the
// previous day, and every enrichment lookup keyed on this string would miss its
// batch and render "0 new shows".
func formatAlertBucket(bucket *time.Time) string {
	if bucket == nil {
		return ""
	}
	return bucket.Format(alertDayLayout)
}

// ──────────────────────────────────────────────
// notification_log row shapes
//
// Three predicates over this table have to agree about which rows mean what.
// They are built here, from the model's constants, so that adding a fourth
// alert type is ONE edit rather than a hunt through five query sites that fail
// silently when one is missed.
//
// Each returns a literal SQL fragment. Nothing caller-supplied reaches them:
// the only interpolations are an alias chosen at the call site and constants
// from models/notification.
// ──────────────────────────────────────────────

// emailLaneAlertTypes are the entity types whose writer uses ONE ROW PER
// CHANNEL LANE, where the channel='email' row stands for a message already in
// the recipient's mailbox rather than for an inbox entry.
//
// A new two-lane alert type MUST be listed here. Leaving it off does not fail:
// its email-lane rows simply start appearing in the bell and inflating the
// unread badge, which is the kind of bug nobody reports as a bug.
var emailLaneAlertTypes = []string{
	notificationm.NotificationEntityArtistShowAlert,
	notificationm.NotificationEntityVenueShowAlert,
}

// showAlertEntityTypes are the entity types that mean "this user has been told
// about a show", where entity_id is the show id.
//
// NotificationEntityShow is deliberately NOT in this list: it needs a channel
// qualifier the others do not (see notifiedAboutShow), so it is spelled there.
//
// NotificationEntityVenueShowAlert MUST NEVER BE ADDED HERE, and it is the one
// entry that would look like it belongs. Its entity_id is a VENUE id, not a show
// id, because a venue alert is coalesced over a whole day and has no single show
// to point at. Listing it would make notifiedAboutShow compare venue ids against
// show ids: venue 42 would read as "already told about show 42" and silence a
// filter match, an artist alert or a scene alert for an unrelated event, for
// every user who follows that venue. TestShowAlertEntityTypesExcludeVenueAlerts
// is what stops the well-meaning edit.
var showAlertEntityTypes = []string{
	notificationm.NotificationEntityArtistShowAlert,
}

// inboxVisibleRows is the predicate that hides notification_log rows which are
// NOT bell entries, for a table alias.
//
// The bell is otherwise channel-agnostic, deliberately: the show-filter and
// scene-follow writers stamp channel='email' on rows that are a user's only
// in-app record, so a `channel = 'in_app'` read would empty the inbox. Adding
// one is a standing trap, and this predicate is narrow precisely so it does not
// become that read. It excludes only the email LANE of a two-lane alert type
// (PSY-1896), whose in-app sibling is the bell entry.
//
// Applied to every read of the log: the list, the unread count, and mark-all.
// Leaving it off any one of them produces a count that disagrees with the list.
func inboxVisibleRows(alias string) string {
	clauses := make([]string, 0, len(emailLaneAlertTypes))
	for _, entityType := range emailLaneAlertTypes {
		clauses = append(clauses, fmt.Sprintf(
			"NOT (%[1]s.entity_type = '%[2]s' AND %[1]s.channel = '%[3]s')",
			alias, entityType, notificationm.NotificationChannelEmail))
	}
	if len(clauses) == 0 {
		// "1 = 1", not "TRUE": GORM only treats a bare string as raw SQL when it
		// contains a space, a `?` or an `@`, so a single-token condition with no
		// arguments is not the no-op it looks like.
		return "1 = 1"
	}
	return strings.Join(clauses, " AND ")
}

// inboxRowsVisibleTo is the predicate that hides the inbox rows which lead to an
// entity viewer may not see.
//
// Suppression, not de-identification. A row kept and blanked still holds a
// position in the list and still counts toward the badge, and the recipient
// knows what they follow — so the entry's mere presence is the disclosure. The
// rows are left in the table and the gate re-evaluated on every read, so
// republishing the entity brings them back, unread and in place.
//
// That restoration covers the rows that EXIST. It is not a promise that the
// inbox is complete: the comment fan-out refuses to write a row for a recipient
// who cannot see the parent (CommentEntityRecipientsSQL), so activity during the
// gated window was never minted and republication has nothing to restore for it.
// Read-time suppression is reversible; the write-time gate is not.
//
// Applied to all three reads inboxVisibleRows covers — the list, the unread
// count, mark-all — AND to BOTH mark-read writes. Those two return the number of
// rows they touched, so a mark-all that flipped rows the list never showed would
// publish the withheld count as arithmetic. (MarkNotificationsRead carries THIS
// predicate but not inboxVisibleRows; that asymmetry is pre-existing and means an
// explicitly-named email-lane id still flips and still counts. Out of scope here,
// and named so it is not mistaken for coverage.)
//
// TWO FAMILIES OF ROW reach a gated entity, and they reach it differently.
// Getting the entity_type test wrong is not a style matter: entity_id means a
// DIFFERENT THING per entity_type, so a lookup that skipped it would decide a
// row by whatever unrelated record happened to share that number.
//
//   - comment_reply / comment_mention carry a COMMENT id, and the comment names
//     its own parent — which may be a show, a collection, or an entity type with
//     no rule at all. Reached through the comments table, and judged by
//     shared.VisibleCommentEntitySQL rather than by a show test written here, so
//     this arm cannot fall behind the registry the way it did for collections.
//   - show, artist_show_alert carry a SHOW id directly — the show-filter matcher
//     and scene-follow writer for the first (scene_follow_notify.go), the artist
//     alert fan-out for the second. Reached with no join at all. No
//     notification entity type carries a COLLECTION id, which is why this arm
//     stays show-only; TestEveryInboxEntityTypeHasADisposition is what keeps
//     that true.
//
// The show-typed rows matter as much as the rest even though the API sends no
// title for them: entity_id IS the withdrawn show's id, and for a scene-follow
// row filter_name is re-derived AT READ TIME from the show's current venues, so
// the row keeps publishing which metro a private show is in and follows it if
// the venue changes.
//
// NotificationEntityVenueShowAlert MUST NEVER be added to the second arm, for
// the reason showAlertEntityTypes gives at length: its entity_id is a VENUE id.
// Its show titles are fenced separately, inside the batch-membership query in
// enrichVenueShowAlertNotifications, because one venue-day row summarises MANY
// shows and gating the row would hide the ones that are still public.
//
// That fence is deliberately the PUBLIC TIER for every caller, unlike the arms
// here, which carry the viewer. A venue-day summary is a listing that merely
// CONTAINS shows rather than a show's own surface, and services/shared's rule
// puts those on the public tier so their contents do not vary by credential. The
// consequence is narrower than the detail route and fails closed: a submitter
// sees their own withdrawn show at /shows/{id} but not in that summary's count.
//
// Each arm FAILS CLOSED on an entity that is gated OR gone, because both EXISTS
// forms answer false for a missing row — which is what lets a withdrawn show and
// a deleted one answer alike, and a private collection and a hard-deleted one
// likewise. The comment arm stays deliberately permissive in the one place
// nothing is at stake: a comment row deleted since the notification was minted
// still passes, and the inbox already degrades to a plain "new comment" line for
// it. A comment that DOES exist and names an entity type nobody dispositioned
// does not pass, because the composite's allowlist refuses it.
//
// It is an ADAPTER, not a second rule: the visibility decisions are
// services/shared's, and what this adds is the walk from a notification row to
// the entity. That vocabulary belongs to this package's table, which is why the
// adapter lives here rather than beside the rules.
func inboxRowsVisibleTo(alias string, viewer contracts.ShowViewer) (string, []interface{}) {
	// NO ADMIN SHORT-CIRCUIT. `1 = 1` is right only while every arm judges shows,
	// which an admin sees all of. The comment arm also judges COLLECTIONS, and no
	// collection detail or listing read grants an admin a private one
	// (services/shared/collection_visibility.go, which names the admin surfaces
	// that do and why they are moderation powers rather than a tier).
	// A blanket bypass here would extend those powers to a passive feed. The show
	// arms still short-circuit internally, so an admin's statement is the same
	// shape as any other caller's plus the collection probe.
	commentGate, commentGateArgs := shared.VisibleCommentEntitySQL(
		"inbox_comment.entity_type", "inbox_comment.entity_id", viewer)
	directGate, directGateArgs := shared.VisibleShowExistsSQL(alias+".entity_id", viewer)

	commentArm, commentArgs := entityTypeArm(alias, commentNotificationEntityTypeList,
		"NOT EXISTS (SELECT 1 FROM comments inbox_comment"+
			" WHERE inbox_comment.id = "+alias+".entity_id"+
			" AND NOT "+commentGate+")", commentGateArgs)
	directArm, directArgs := entityTypeArm(alias, showIDBearingEntityTypeList, directGate, directGateArgs)

	// Argument order follows the statement text: the comment arm is written
	// first, so its binds come first. Each arm reports its OWN args, because an
	// arm that drops its gate must drop that gate's binds with it.
	args := make([]interface{}, 0, len(commentArgs)+len(directArgs))
	args = append(args, commentArgs...)
	args = append(args, directArgs...)
	return "(" + commentArm + " AND " + directArm + ")", args
}

// entityTypeArm builds "this row's entity_type is not one I judge, OR it passes
// gate", and answers the NO-OP when the type list is empty.
//
// It returns the arm's ARGUMENTS as well as its SQL, and that is the whole point
// of the signature: the no-op branch discards `gate`, so it must discard
// gateArgs too. Computing the args beside the gate and appending them
// unconditionally would leave the statement two placeholders short of its bind
// list, and Postgres rejects the whole query — "bind message supplies 4
// parameters, but the prepared statement requires 2" — on all four log methods
// at once. A guard meant to make the empty case safe would have made it fatal.
//
// The empty case has to be spelled out rather than left to the SQL, because the
// obvious spelling inverts the meaning. `entity_type NOT IN ()` is a syntax
// error, and `NOT IN (NULL)` is never TRUE — which does not disable the arm, it
// applies `gate` to EVERY row regardless of type. For the direct arm that would
// mean judging a venue alert by whether a show with the venue's id is visible,
// the exact confusion this file warns "MUST NEVER" happen. So an empty list
// means "no row is of a type this arm judges", which is TRUE for every row.
func entityTypeArm(alias, entityTypeList, gate string, gateArgs []interface{}) (string, []interface{}) {
	if entityTypeList == "" {
		return "1 = 1", nil
	}
	return "(" + alias + ".entity_type NOT IN (" + entityTypeList + ") OR " + gate + ")", gateArgs
}

// showIDBearingEntityTypeList is the SQL IN-list of notification entity types
// whose entity_id is a SHOW id, for inboxRowsVisibleTo's direct arm.
//
// Spelled here rather than reusing showAlertEntityTypes, which looks like the
// same set and is not: that one answers "has this user been told about this
// show" and excludes NotificationEntityShow because that family needs a channel
// qualifier it does not. A visibility gate wants every row whose entity_id is a
// show id, qualifier or no qualifier, so the two lists are genuinely different
// questions over overlapping members and must not be merged.
//
// NotificationEntityVenueShowAlert is absent and must stay absent: its entity_id
// is a VENUE id, and listing it would gate venue rows on whether a SHOW with the
// venue's id happens to be visible.
var showIDBearingEntityTypeList = shared.SQLQuotedList([]string{
	notificationm.NotificationEntityShow,
	notificationm.NotificationEntityArtistShowAlert,
})

// commentNotificationEntityTypeList is commentNotificationEntityTypes as a SQL
// IN-list, built once.
//
// Derived from the map rather than written out, so adding an entity type there
// reaches this predicate without a second edit. shared.SQLQuotedList carries the
// sorting and the empty-list contract that both this list and the one above
// depend on.
var commentNotificationEntityTypeList = shared.SQLQuotedList(commentNotificationEntityTypeKeys())

// commentNotificationEntityTypeKeys is the map's key set. Extracted rather than
// written out so the list and the writers cannot drift.
func commentNotificationEntityTypeKeys() []string {
	types := make([]string, 0, len(commentNotificationEntityTypes))
	for entityType := range commentNotificationEntityTypes {
		types = append(types, entityType)
	}
	return types
}

// notifiedAboutShow is the predicate for "this user has ALREADY been told about
// this show, by any of the systems that can tell them".
//
// One user gets one notification per show across all three fanouts: criteria
// filters, artist follows, and scene follows. Each stage consults this before
// delivering, so the predicate has to name every shape any of them writes, and
// naming it in one place is what keeps the three stages from drifting apart.
// The scene pass drifting from the filter pass is exactly how a user ends up
// told twice about one show.
//
// The two arms are asymmetric on purpose. Filter and scene rows share
// entity_type='show' and are distinguished from nothing else, so they need the
// channel='email' qualifier that has always marked "delivered" on that path.
// The two-lane alert types need NO channel qualifier: either lane means the
// user was reached, and requiring 'email' there would miss the in-app-only
// recipient, who is the DEFAULT (email is opt-in).
//
// alias is the table or alias the caller queries through.
func notifiedAboutShow(alias string) string {
	clauses := []string{fmt.Sprintf(
		"(%[1]s.entity_type = '%[2]s' AND %[1]s.channel = '%[3]s')",
		alias, notificationm.NotificationEntityShow, notificationm.NotificationChannelEmail)}
	for _, entityType := range showAlertEntityTypes {
		clauses = append(clauses, fmt.Sprintf(
			"%s.entity_type = '%s'", alias, entityType))
	}
	return "(" + strings.Join(clauses, " OR ") + ")"
}

// enrichArtistShowAlertNotifications populates the artist name, show title and
// show URL on artist_show_alert rows (PSY-1896), so the inbox can render
// "<artist> announced a show" with a link to the show rather than the bare
// entity_type the generic branch falls back to.
//
// Two batched lookups, one per table, mirroring enrichRequestNotifications:
// subject_entity_id resolves the artist and entity_id the show. Either coming
// back empty (a merged artist, a deleted show) leaves that half blank and the
// row degrades rather than disappearing — the notification still happened.
func (s *NotificationFilterService) enrichArtistShowAlertNotifications(entries []contracts.NotificationLogEntry, viewer contracts.ShowViewer) {
	showIDs := make([]uint, 0, len(entries))
	artistIDs := make([]uint, 0, len(entries))
	seenShow := make(map[uint]struct{}, len(entries))
	seenArtist := make(map[uint]struct{}, len(entries))
	for _, e := range entries {
		if e.EntityType != notificationm.NotificationEntityArtistShowAlert {
			continue
		}
		if _, dup := seenShow[e.EntityID]; !dup {
			seenShow[e.EntityID] = struct{}{}
			showIDs = append(showIDs, e.EntityID)
		}
		if e.SubjectEntityID == nil {
			continue
		}
		if _, dup := seenArtist[*e.SubjectEntityID]; !dup {
			seenArtist[*e.SubjectEntityID] = struct{}{}
			artistIDs = append(artistIDs, *e.SubjectEntityID)
		}
	}
	if len(showIDs) == 0 {
		return
	}

	type showRow struct {
		ID    uint
		Title string
		Slug  *string
	}
	// FENCED HERE TOO, matching the venue twin below rather than relying on the
	// row having been dropped upstream (PSY-1983). inboxRowsVisibleTo already
	// removes these rows from GetUserNotifications, so today no gated id reaches
	// this query — but the title and URL of a private show are the most
	// sensitive fields this file writes, and a second reader of these rows (a
	// digest, an export, an admin view) would otherwise publish them with
	// nothing here to stop it. One index probe per row is the whole cost.
	visibleShows, visibleShowArgs := shared.VisibleShowPredicateSQL("shows", viewer)
	var shows []showRow
	if err := s.db.Table("shows").
		Select("id, title, slug").
		Where("id IN ?", showIDs).
		Where(visibleShows, visibleShowArgs...).
		Scan(&shows).Error; err != nil {
		log.Printf("warning: failed to load shows for inbox enrichment: %v", err)
		return
	}
	showByID := make(map[uint]showRow, len(shows))
	for _, r := range shows {
		showByID[r.ID] = r
	}

	artistByID := make(map[uint]string, len(artistIDs))
	if len(artistIDs) > 0 {
		var artists []struct {
			ID   uint
			Name string
		}
		if err := s.db.Table("artists").
			Select("id, name").
			Where("id IN ?", artistIDs).
			Scan(&artists).Error; err != nil {
			log.Printf("warning: failed to load artists for inbox enrichment: %v", err)
		}
		for _, a := range artists {
			artistByID[a.ID] = a.Name
		}
	}

	for i := range entries {
		e := &entries[i]
		if e.EntityType != notificationm.NotificationEntityArtistShowAlert {
			continue
		}
		if e.SubjectEntityID != nil {
			e.AlertArtistName = artistByID[*e.SubjectEntityID]
		}
		show, found := showByID[e.EntityID]
		if !found {
			continue
		}
		e.AlertShowTitle = show.Title
		// Same slug-then-id fallback showEmailContent uses: entity slugs are
		// nullable, and "/shows/" with an empty slug resolves to the index.
		if show.Slug != nil && *show.Slug != "" {
			e.AlertShowURL = fmt.Sprintf("%s/shows/%s", s.frontendURL, *show.Slug)
		} else {
			e.AlertShowURL = fmt.Sprintf("%s/shows/%d", s.frontendURL, e.EntityID)
		}
	}
}

// venueAlertSummaryLimit caps how many shows an inbox row names.
//
// A venue-day batch is unbounded — a season's calendar drop is one batch — and
// an inbox row is one or two lines. The count is reported separately and in
// full, so the row can say "and 12 more" rather than lying about the size.
const venueAlertSummaryLimit = 3

// enrichVenueShowAlertNotifications populates the venue name, link, show count
// and show preview on venue_show_alert rows (PSY-1895).
//
// The shows are resolved from venue_show_alert_batch AT READ TIME rather than
// stamped onto the row when it was created, and that is the deliberate half of
// this design: a show announced later the same day joins the batch, so the row
// a user is looking at grows to include it without a second notification ever
// having been sent. A snapshot taken at write time could not do that.
//
// Degrades rather than disappearing, in each direction independently: a merged
// or deleted venue leaves the name and link blank, and a batch whose members
// were all deleted leaves a count of zero. The notification still happened, and
// a row that vanished from history would be a worse lie than a bare one.
//
// # The member query re-applies the DELIVERY rules, and must
//
// Reading the batch live is what lets the row grow, but it also means this query
// is the ONLY thing standing between a member and the reader. Three filters
// therefore mirror what the flush already decided, and each closes a real hole:
//
//   - ANNOUNCEABILITY. Only deleted shows leave the batch (the foreign key
//     cascades). A show that is later un-approved, rejected or cancelled stays,
//     and without this predicate its title would keep rendering in every
//     follower's inbox forever — a non-public show's title, shown to users, on a
//     row moderation has already withdrawn.
//   - VENUE MEMBERSHIP. An ordinary edit replaces a show's venues wholesale
//     without touching the batch, so a show that has moved to another venue would
//     otherwise still be listed under this one.
//   - THE READER'S OWN DEDUP. The flush strips shows the user was already told
//     about by a more specific alert and sizes the EMAIL from what is left. This
//     count and this list have to be sized the same way, or the same flush
//     produces an email saying "New show at X" beside a bell row saying "5 new
//     shows" — four of which it deliberately chose not to re-tell them about.
func (s *NotificationFilterService) enrichVenueShowAlertNotifications(
	userID uint,
	entries []contracts.NotificationLogEntry,
) {
	if len(entries) == 0 {
		return
	}

	venueIDs := make([]uint, 0, len(entries))
	buckets := make([]string, 0, len(entries))
	seenVenue := make(map[uint]struct{}, len(entries))
	seenBucket := make(map[string]struct{}, len(entries))
	for _, e := range entries {
		if e.EntityType != notificationm.NotificationEntityVenueShowAlert || e.AlertBucket == "" {
			continue
		}
		if _, dup := seenVenue[e.EntityID]; !dup {
			seenVenue[e.EntityID] = struct{}{}
			venueIDs = append(venueIDs, e.EntityID)
		}
		if _, dup := seenBucket[e.AlertBucket]; !dup {
			seenBucket[e.AlertBucket] = struct{}{}
			buckets = append(buckets, e.AlertBucket)
		}
	}
	if len(venueIDs) == 0 {
		return
	}

	type venueRow struct {
		ID       uint
		Name     string
		Slug     *string
		Timezone *string
		State    string
	}
	var venues []venueRow
	if err := s.db.Table("venues").
		Select("id, name, slug, timezone, state").
		Where("id IN ?", venueIDs).
		Scan(&venues).Error; err != nil {
		log.Printf("warning: failed to load venues for inbox enrichment: %v", err)
		return
	}
	venueByID := make(map[uint]venueRow, len(venues))
	for _, v := range venues {
		venueByID[v.ID] = v
	}

	// Queried as the CROSS PRODUCT of the page's venues and its buckets, then
	// narrowed to the exact pairs in Go. A tuple IN-list would be tighter, but a
	// page of notifications holds a handful of venue alerts, so the superset is
	// a handful of extra rows against one readable query.
	var members []struct {
		VenueID   uint      `gorm:"column:venue_id"`
		AlertDay  string    `gorm:"column:alert_day"`
		Title     string    `gorm:"column:title"`
		EventDate time.Time `gorm:"column:event_date"`
	}
	if err := s.db.Raw(`
		SELECT b.venue_id, to_char(b.alert_day, 'YYYY-MM-DD') AS alert_day,
		       s.title, s.event_date
		FROM venue_show_alert_batch b
		JOIN shows s ON s.id = b.show_id
		-- The show must still be AT this venue: an ordinary edit replaces
		-- show_venues wholesale and never touches the batch row.
		JOIN show_venues sv ON sv.show_id = s.id AND sv.venue_id = b.venue_id
		-- The reader's OWN alert row for this venue-day. It supplies the instant
		-- the notification was delivered, which is what freezes the dedup below.
		JOIN notification_log own
		  ON own.user_id = ?
		 AND own.entity_type = ?
		 AND own.entity_id = b.venue_id
		 AND own.alert_bucket = b.alert_day
		 AND own.channel = ?
		WHERE b.venue_id = ANY(?) AND b.alert_day = ANY(?::date[])
		  -- The same fence the flush applied before announcing it. Only DELETED
		  -- shows leave the batch on their own, so without this a show pulled by
		  -- moderation keeps its title in every follower's inbox.
		  AND s.status = ? AND s.is_cancelled = false
		  -- ...and the reader's own dedup, so this count cannot disagree with the
		  -- email the same flush sent.
		  --
		  -- Bounded by own.sent_at: only alerts that existed WHEN THIS ROW WAS
		  -- DELIVERED count. Without that bound the dedup is re-evaluated on every
		  -- read against a log that keeps growing, so a later artist alert about a
		  -- show in this batch would retroactively shrink a notification the user
		  -- already received — "5 new shows" decaying to "2", or to an empty row.
		  -- This reproduces what the flush actually saw, which is the only version
		  -- of the count that can be stable.
		  AND NOT EXISTS (
		        SELECT 1 FROM notification_log nl
		        WHERE nl.user_id = own.user_id
		          AND nl.entity_id = s.id
		          AND nl.sent_at <= own.sent_at
		          AND `+notifiedAboutShow("nl")+`
		      )
		ORDER BY b.venue_id, b.alert_day, s.event_date ASC, s.id ASC
	`,
		// Bind order follows the query text: the JOIN's three, then the WHERE.
		userID,
		notificationm.NotificationEntityVenueShowAlert,
		notificationm.NotificationChannelInApp,
		pq.Array(venueIDs),
		pq.Array(buckets),
		catalogm.ShowStatusApproved,
	).Scan(&members).Error; err != nil {
		log.Printf("warning: failed to load venue alert batches for inbox enrichment: %v", err)
		return
	}

	type batchKey struct {
		venueID uint
		day     string
	}
	byBatch := make(map[batchKey][]string, len(venueIDs))
	counts := make(map[batchKey]int, len(venueIDs))
	for _, m := range members {
		key := batchKey{venueID: m.VenueID, day: m.AlertDay}
		counts[key]++
		if len(byBatch[key]) >= venueAlertSummaryLimit {
			continue
		}
		v := venueByID[m.VenueID]
		// Dates render in the VENUE's zone, matching every other show surface:
		// a Phoenix date shown in the reader's zone is the PSY-996 bug.
		label := m.EventDate.In(utils.EventLocation(v.Timezone, v.State)).Format("Mon Jan 2")
		byBatch[key] = append(byBatch[key], fmt.Sprintf("%s %s", label, m.Title))
	}

	for i := range entries {
		e := &entries[i]
		if e.EntityType != notificationm.NotificationEntityVenueShowAlert || e.AlertBucket == "" {
			continue
		}
		key := batchKey{venueID: e.EntityID, day: e.AlertBucket}
		e.AlertShowCount = counts[key]
		summary := byBatch[key]
		if extra := counts[key] - len(summary); extra > 0 {
			summary = append(append([]string{}, summary...), fmt.Sprintf("and %d more", extra))
		}
		e.AlertShowSummary = strings.Join(summary, " · ")

		v, found := venueByID[e.EntityID]
		if !found {
			continue
		}
		e.AlertVenueName = v.Name
		e.AlertVenueURL = entityURL(s.frontendURL, "venues", v.Slug, v.ID)
	}
}

// enrichRequestNotifications populates request_title + request_url on
// request_fulfillment_proposed rows (entity_id holds the request_id) via a
// single batched lookup against the requests table. Missing requests (deleted
// after the row was minted) leave the fields empty so the UI degrades to a
// plain "fulfillment proposed" line. PSY-890.
func (s *NotificationFilterService) enrichRequestNotifications(entries []contracts.NotificationLogEntry) {
	if len(entries) == 0 {
		return
	}

	ids := make([]uint, 0, len(entries))
	seen := make(map[uint]struct{}, len(entries))
	for _, e := range entries {
		if e.EntityType != notificationm.NotificationEntityRequestFulfillmentProposed {
			continue
		}
		if _, dup := seen[e.EntityID]; dup {
			continue
		}
		seen[e.EntityID] = struct{}{}
		ids = append(ids, e.EntityID)
	}
	if len(ids) == 0 {
		return
	}

	var rows []struct {
		ID    uint
		Title string
	}
	if err := s.db.Table("requests").
		Select("id, title").
		Where("id IN ?", ids).
		Scan(&rows).Error; err != nil {
		log.Printf("warning: failed to load requests for inbox enrichment: %v", err)
		return
	}
	titleByID := make(map[uint]string, len(rows))
	for _, r := range rows {
		titleByID[r.ID] = r.Title
	}

	for i := range entries {
		e := &entries[i]
		if e.EntityType != notificationm.NotificationEntityRequestFulfillmentProposed {
			continue
		}
		title, found := titleByID[e.EntityID]
		if !found {
			continue // request deleted — leave fields empty; UI degrades gracefully
		}
		e.RequestTitle = title
		e.RequestURL = fmt.Sprintf("%s/requests/%d", s.frontendURL, e.EntityID)
	}
}

// commentInboxExcerptMaxRunes is the rune cap for the bell/inbox excerpt.
// Tighter than the email excerpt (engagement.commentExcerptMaxChars=200)
// since the popover row is two visual lines max.
const commentInboxExcerptMaxRunes = 140

// commentEntityPathAndTable maps the CommentEntityType strings owned by
// the engagement package onto the frontend path segment + DB table name
// the inbox enrichment query needs. The display column is not uniform:
// shows, releases and collections spell it "title", the rest "name".
//
// The list and the entity-type-string values are validated by the
// engagementm.ValidCommentEntityTypes map at row-write time, so any
// entity_type that arrives here is guaranteed to be one of these seven.
func commentEntityPathAndTable(entityType string) (path, table, nameCol string, ok bool) {
	return engagementm.CommentEntityPathAndTable(entityType)
}

// commentNotificationEntityTypes is the set of notification_log.entity_type
// values whose entity_id points at a row in the `comments` table.
var commentNotificationEntityTypes = map[string]bool{
	engagement.NotificationEntityCommentReply:   true,
	engagement.NotificationEntityCommentMention: true,
}

// commentRow is the slim projection used by the enrichment query.
type commentRow struct {
	ID         uint
	EntityType string
	EntityID   uint
	Body       string
	UserID     uint
}

// entityNameRow projects the (id, name|title, slug) tuple from a parent
// entity table during the per-table batch lookup (shared with the
// watching-list enrichment).
type entityNameRow = shared.EntityNameRow

// enrichCommentNotifications walks entries, finds the comment-driven rows,
// then batch-loads (in order): the referenced comments, the commenters'
// display names + usernames, and the parent entities' (name, slug) tuples
// — grouped by table so each table is hit at most once. Missing comments
// (deleted after the row was minted) leave the entry's enrichment fields
// empty; the UI degrades to a plain "new comment" line.
func (s *NotificationFilterService) enrichCommentNotifications(entries []contracts.NotificationLogEntry, viewer contracts.ShowViewer) {
	if len(entries) == 0 {
		return
	}

	commentIDs := uniqueCommentIDs(entries)
	if len(commentIDs) == 0 {
		return
	}

	commentsByID, ok := s.loadCommentRows(commentIDs)
	if !ok {
		return
	}

	userIDs := make([]uint, 0, len(commentsByID))
	for _, c := range commentsByID {
		userIDs = append(userIDs, c.UserID)
	}
	// Public chain (PSY-1940). The notification feed is private to its
	// recipient, but the name it carries is a THIRD PARTY's — the commenter —
	// and it must read the same as the byline on the comment the notification
	// points at, which is public. Resolving it any other way would name someone
	// here in a way the linked comment does not.
	names, _ := shared.BatchResolvePublicUserNames(s.db, userIDs)
	usernames, _ := shared.BatchResolveUserUsernames(s.db, userIDs)

	entitiesByTypeID := s.loadParentEntitiesByType(commentsByID, viewer)

	for i := range entries {
		e := &entries[i]
		if !commentNotificationEntityTypes[e.EntityType] {
			continue
		}
		c, found := commentsByID[e.EntityID]
		if !found {
			continue
		}
		e.CommenterName = names[c.UserID]
		if u, hasUsername := usernames[c.UserID]; hasUsername && u != nil {
			e.CommenterUsername = *u
		}
		e.CommentExcerpt = engagement.BuildExcerpt(c.Body, commentInboxExcerptMaxRunes)
		e.CommentEntityType = c.EntityType
		e.CommentEntityID = c.EntityID
		entityURL, entityName := s.formatEntityURL(c.EntityType, c.EntityID, entitiesByTypeID)
		e.CommentEntityName = entityName
		if entityURL != "" {
			e.CommentURL = fmt.Sprintf("%s#comment-%d", entityURL, c.ID)
		}
	}
}

// uniqueCommentIDs collects the deduplicated set of comment IDs referenced
// by the comment-driven entries.
func uniqueCommentIDs(entries []contracts.NotificationLogEntry) []uint {
	ids := make([]uint, 0, len(entries))
	seen := make(map[uint]struct{}, len(entries))
	for _, e := range entries {
		if !commentNotificationEntityTypes[e.EntityType] {
			continue
		}
		if _, dup := seen[e.EntityID]; dup {
			continue
		}
		seen[e.EntityID] = struct{}{}
		ids = append(ids, e.EntityID)
	}
	return ids
}

// loadCommentRows batch-fetches the comments referenced by the inbox
// enrichment pass. Returns ok=false on DB error so the caller can skip
// enrichment without crashing the request.
func (s *NotificationFilterService) loadCommentRows(commentIDs []uint) (map[uint]commentRow, bool) {
	var rows []commentRow
	err := s.db.Table("comments").
		Select("id, entity_type, entity_id, body, user_id").
		Where("id IN ?", commentIDs).
		Scan(&rows).Error
	if err != nil {
		log.Printf("warning: failed to load comments for inbox enrichment: %v", err)
		return nil, false
	}
	byID := make(map[uint]commentRow, len(rows))
	for _, r := range rows {
		byID[r.ID] = r
	}
	return byID, true
}

// loadParentEntitiesByType groups (entityType, entityID) pairs from the
// loaded comments by table and fires one batch SELECT per table — so the
// inbox enrichment runs in O(distinct-entity-tables) DB roundtrips, not
// O(rows). Returns nested map[entityType]map[entityID]entityNameRow.
func (s *NotificationFilterService) loadParentEntitiesByType(comments map[uint]commentRow, viewer contracts.ShowViewer) map[string]map[uint]entityNameRow {
	idsByType := make(map[string]map[uint]struct{})
	for _, c := range comments {
		if _, _, _, ok := commentEntityPathAndTable(c.EntityType); !ok {
			continue
		}
		set, exists := idsByType[c.EntityType]
		if !exists {
			set = make(map[uint]struct{})
			idsByType[c.EntityType] = set
		}
		set[c.EntityID] = struct{}{}
	}

	idListByType := make(map[string][]uint, len(idsByType))
	for entityType, idSet := range idsByType {
		ids := make([]uint, 0, len(idSet))
		for id := range idSet {
			ids = append(ids, id)
		}
		idListByType[entityType] = ids
	}
	return shared.LoadCommentEntityNames(s.db, idListByType, viewer)
}

// formatEntityURL builds the (URL, display-name) pair for a comment's
// parent entity from the pre-loaded per-type map. Falls back to "<type>
// #<id>" + slug-less URL when the entity row wasn't pre-loaded (deleted
// since notification minted, or unknown entity_type).
func (s *NotificationFilterService) formatEntityURL(entityType string, entityID uint, entitiesByTypeID map[string]map[uint]entityNameRow) (url, name string) {
	pathSegment, _, _, ok := commentEntityPathAndTable(entityType)
	if !ok {
		return "", ""
	}
	byID, hasType := entitiesByTypeID[entityType]
	row, hasRow := byID[entityID]
	switch {
	case hasType && hasRow && row.Slug != "":
		url = fmt.Sprintf("%s/%s/%s", s.frontendURL, pathSegment, row.Slug)
	default:
		url = fmt.Sprintf("%s/%s/%d", s.frontendURL, pathSegment, entityID)
	}
	if hasRow && row.Name != "" {
		name = row.Name
	} else {
		name = fmt.Sprintf("%s #%d", entityType, entityID)
	}
	return url, name
}

// GetUnreadCount returns the number of unread notifications for viewer.
//
// Carries the same two predicates the list carries, or the badge and the list
// disagree and the difference is a count of what was withheld.
func (s *NotificationFilterService) GetUnreadCount(viewer contracts.ShowViewer) (int64, error) {
	if s.db == nil {
		return 0, fmt.Errorf("database not initialized")
	}
	// The zero viewer is a construction bug: ShowViewer{} is the codebase's
	// spelling for the public tier on the listing gates, and these methods are
	// self-scoped, so it would otherwise mean "user 0" and answer emptily forever.
	if viewer.UserID == 0 {
		return 0, fmt.Errorf("GetUnreadCount is self-scoped: the viewer carries no user id")
	}

	visibleSQL, visibleArgs := inboxRowsVisibleTo("notification_log", viewer)
	var count int64
	err := s.db.Model(&notificationm.NotificationLog{}).
		Where("user_id = ? AND read_at IS NULL", viewer.UserID).
		Where(inboxVisibleRows("notification_log")).
		Where(visibleSQL, visibleArgs...).
		Count(&count).Error
	return count, err
}

// MarkNotificationsRead flips read_at on the given IDs for viewer.
// Bound by user_id so a user can't mark another user's notifications read.
// Returns the count actually updated.
//
// Rows viewer cannot see are skipped, so the returned count cannot be differenced
// against the list to recover how many were withheld.
func (s *NotificationFilterService) MarkNotificationsRead(viewer contracts.ShowViewer, ids []uint) (int64, error) {
	if s.db == nil {
		return 0, fmt.Errorf("database not initialized")
	}
	// The zero viewer is a construction bug: ShowViewer{} is the codebase's
	// spelling for the public tier on the listing gates, and these methods are
	// self-scoped, so it would otherwise mean "user 0" and answer emptily forever.
	if viewer.UserID == 0 {
		return 0, fmt.Errorf("MarkNotificationsRead is self-scoped: the viewer carries no user id")
	}
	if len(ids) == 0 {
		return 0, nil
	}
	visibleSQL, visibleArgs := inboxRowsVisibleTo("notification_log", viewer)
	now := time.Now().UTC()
	result := s.db.Model(&notificationm.NotificationLog{}).
		Where("user_id = ? AND id IN ? AND read_at IS NULL", viewer.UserID, ids).
		Where(visibleSQL, visibleArgs...).
		Update("read_at", now)
	if result.Error != nil {
		return 0, fmt.Errorf("failed to mark notifications read: %w", result.Error)
	}
	return result.RowsAffected, nil
}

// MarkAllNotificationsRead flips read_at on every unread notification for
// viewer. Returns the count updated.
func (s *NotificationFilterService) MarkAllNotificationsRead(viewer contracts.ShowViewer) (int64, error) {
	if s.db == nil {
		return 0, fmt.Errorf("database not initialized")
	}
	// The zero viewer is a construction bug: ShowViewer{} is the codebase's
	// spelling for the public tier on the listing gates, and these methods are
	// self-scoped, so it would otherwise mean "user 0" and answer emptily forever.
	if viewer.UserID == 0 {
		return 0, fmt.Errorf("MarkAllNotificationsRead is self-scoped: the viewer carries no user id")
	}
	visibleSQL, visibleArgs := inboxRowsVisibleTo("notification_log", viewer)
	now := time.Now().UTC()
	result := s.db.Model(&notificationm.NotificationLog{}).
		Where("user_id = ? AND read_at IS NULL", viewer.UserID).
		// Same predicates as the list and the count, so "mark all read" clears
		// exactly the rows the user could see and its reported count matches the
		// badge that prompted the click. Leaving the hidden email-lane rows and
		// the gated-show rows unread costs nothing: nothing reads their read_at
		// while they are hidden, and a republished show hands the recipient the
		// backlog it withheld rather than a row already marked seen.
		Where(inboxVisibleRows("notification_log")).
		Where(visibleSQL, visibleArgs...).
		Update("read_at", now)
	if result.Error != nil {
		return 0, fmt.Errorf("failed to mark all notifications read: %w", result.Error)
	}
	return result.RowsAffected, nil
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
	// hash.Hash.Write never returns an error; the drop is intentional.
	_, _ = fmt.Fprintf(mac, "unsubscribe:filter:%d", filterID)
	return hex.EncodeToString(mac.Sum(nil))
}

// ──────────────────────────────────────────────
// Email template
// ──────────────────────────────────────────────

// buildFilterEmailHTML renders the show-match body shared by the saved-filter
// and scene-follow alerts.
//
// Every value here is entity text the platform does not author: show titles,
// artist and venue names, and a user-chosen filter name, none charset-restricted
// and all up to 255 chars. Scraped third-party venue-calendar text reaches this
// body automatically, and the message ships from this platform's own
// SPF/DKIM-aligned sender, so a title carrying markup would arrive as a working
// link the recipient has every reason to trust. The template escapes each value
// for the context it lands in.
func buildFilterEmailHTML(filterName, showTitle, showDate, venueText, artistText, priceText, showURL, unsubscribeURL string) (string, error) {
	return renderEmailTemplate(filterEmailTemplate, filterEmailData{
		FilterName:     filterName,
		ShowTitle:      showTitle,
		ShowDate:       showDate,
		VenueText:      venueText,
		ArtistText:     artistText,
		PriceText:      priceText,
		ShowURL:        showURL,
		UnsubscribeURL: unsubscribeURL,
	})
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
