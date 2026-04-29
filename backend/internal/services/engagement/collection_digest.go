package engagement

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/getsentry/sentry-go"
	"gorm.io/gorm"

	"psychic-homily-backend/db"
	"psychic-homily-backend/internal/config"
	"psychic-homily-backend/internal/models"
	"psychic-homily-backend/internal/services/contracts"
)

// DefaultCollectionDigestInterval is how often the digest job runs.
// Set to 168h (7 days) to deliver one email per user per week. Weekly
// (rather than daily) cadence is the spam-prevention default — the digest
// is opt-IN and weekly, so a user who subscribes to a busy collection
// can't accidentally get a deluge. The COLLECTION_DIGEST_INTERVAL_HOURS
// env var still overrides this for local dogfooding.
const DefaultCollectionDigestInterval = 168 * time.Hour

// CollectionDigestService is a ticker-based background service that batches
// "items added to subscribed collections" into a single weekly email per user.
// PSY-350.
//
// Idempotent across restarts: the per-subscriber `last_digest_sent_at`
// cursor is updated atomically per user as part of the send. A crash
// before the email goes out leaves the cursor unchanged, so the next
// cycle re-includes the same items.
type CollectionDigestService struct {
	db           *gorm.DB
	emailService contracts.EmailServiceInterface
	interval     time.Duration
	stopCh       chan struct{}
	wg           sync.WaitGroup
	logger       *slog.Logger
	frontendURL  string
	// backendURL is the public API URL (e.g. https://api.psychichomily.com).
	// Used for the unsubscribe URL placed in the email body and the
	// `List-Unsubscribe` header — the same chi route serves both manual
	// GET (HTML confirmation page) and RFC 8058 POST one-click unsubscribe.
	backendURL string
	jwtSecret  string
}

// NewCollectionDigestService creates a new collection digest service.
func NewCollectionDigestService(database *gorm.DB, emailService contracts.EmailServiceInterface, cfg *config.Config) *CollectionDigestService {
	if database == nil {
		database = db.GetDB()
	}

	interval := DefaultCollectionDigestInterval

	// Allow override for local development / dogfooding via env var.
	// Hours is the natural unit since the default is weekly (168h).
	if envInterval := os.Getenv("COLLECTION_DIGEST_INTERVAL_HOURS"); envInterval != "" {
		if hours, err := strconv.Atoi(envInterval); err == nil && hours > 0 {
			interval = time.Duration(hours) * time.Hour
		}
	}

	return &CollectionDigestService{
		db:           database,
		emailService: emailService,
		interval:     interval,
		stopCh:       make(chan struct{}),
		logger:       slog.Default(),
		frontendURL:  cfg.Email.FrontendURL,
		backendURL:   deriveBackendURL(cfg.Email.FrontendURL),
		jwtSecret:    cfg.JWT.SecretKey,
	}
}

// deriveBackendURL maps a frontend URL to the corresponding public API URL.
// Mirrors handlers.getAPIBaseURL — pulled inline to avoid cross-package
// imports. Defaults to localhost:8080 in dev.
func deriveBackendURL(frontendURL string) string {
	switch frontendURL {
	case "https://psychichomily.com":
		return "https://api.psychichomily.com"
	case "https://stage.psychichomily.com":
		return "https://api-stage.psychichomily.com"
	default:
		return "http://localhost:8080"
	}
}

// Start begins the background digest job.
func (s *CollectionDigestService) Start(ctx context.Context) {
	s.wg.Add(1)
	go s.run(ctx)
	s.logger.Info("collection digest service started",
		"interval_hours", s.interval.Hours(),
	)
}

// Stop gracefully stops the digest job.
func (s *CollectionDigestService) Stop() {
	close(s.stopCh)
	s.wg.Wait()
	s.logger.Info("collection digest service stopped")
}

// run is the main loop for the digest service.
func (s *CollectionDigestService) run(ctx context.Context) {
	defer s.wg.Done()

	// Run immediately on startup so admins exercising the service don't have
	// to wait a full interval to see output. The job is idempotent — running
	// twice in a row sends nothing the second time because cursors moved.
	s.runDigestCycle()

	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			s.logger.Info("collection digest service context cancelled")
			return
		case <-s.stopCh:
			s.logger.Info("collection digest service received stop signal")
			return
		case <-ticker.C:
			s.runDigestCycle()
		}
	}
}

// digestCandidate is the result of the per-(user,collection) candidate query —
// each row is one item added to one of the user's subscribed collections
// since the user was last digested. Sorted by user, then collection, then
// item created_at so the email rendering loop is straightforward.
type digestCandidate struct {
	UserID          uint
	UserEmail       string
	CollectionID    uint
	CollectionTitle string
	CollectionSlug  string
	EntityType      string
	EntityID        uint
	ItemCreatedAt   time.Time
	AddedByUsername *string
	AddedByFirst    *string
	AddedByEmail    *string
}

// runDigestCycle sends one digest email per user with new items in their
// subscribed collections. PSY-350.
//
// Algorithm:
//  1. Query candidates: for each (subscriber, item) pair where
//     item.created_at > effective_cursor AND item.added_by_user_id <> subscriber.user_id.
//     The effective cursor is COALESCE(last_digest_sent_at, last_visited_at,
//     subscription.created_at) — so a user who's never been digested still
//     gets the items they haven't seen yet.
//  2. Group rows by user, then by collection.
//  3. For each user with at least one item: render and send one email,
//     then bump the user's `last_digest_sent_at` on every subscription
//     row that contributed.
//
// All errors are logged; the cycle keeps going.
func (s *CollectionDigestService) runDigestCycle() {
	s.logger.Info("starting collection digest cycle")

	now := time.Now().UTC()

	candidates, err := s.queryCandidates(now)
	if err != nil {
		s.logger.Error("failed to query digest candidates", "error", err)
		return
	}
	if len(candidates) == 0 {
		s.logger.Info("no collection digest items to send")
		return
	}

	// Group by user, then by collection. Collection ordering is by first
	// appearance (i.e. earliest item) for stability in tests.
	type collectionBucket struct {
		title string
		slug  string
		items []digestCandidate
	}
	type userBucket struct {
		userID    uint
		userEmail string
		// collection metadata + items, ordered by collection appearance.
		collectionOrder []uint
		byCollection    map[uint]*collectionBucket
	}

	usersByID := make(map[uint]*userBucket)
	userOrder := make([]uint, 0)

	for _, c := range candidates {
		ub, ok := usersByID[c.UserID]
		if !ok {
			ub = &userBucket{
				userID:       c.UserID,
				userEmail:    c.UserEmail,
				byCollection: make(map[uint]*collectionBucket),
			}
			usersByID[c.UserID] = ub
			userOrder = append(userOrder, c.UserID)
		}
		cb, ok := ub.byCollection[c.CollectionID]
		if !ok {
			cb = &collectionBucket{
				title: c.CollectionTitle,
				slug:  c.CollectionSlug,
				items: []digestCandidate{},
			}
			ub.byCollection[c.CollectionID] = cb
			ub.collectionOrder = append(ub.collectionOrder, c.CollectionID)
		}
		cb.items = append(cb.items, c)
	}

	sentCount := 0
	errorCount := 0
	skippedNoEmail := 0

	for _, userID := range userOrder {
		ub := usersByID[userID]
		if ub.userEmail == "" {
			skippedNoEmail++
			continue
		}

		groups := make([]contracts.CollectionDigestGroup, 0, len(ub.collectionOrder))
		for _, cid := range ub.collectionOrder {
			cb := ub.byCollection[cid]
			collectionURL := s.buildCollectionURL(cb.slug)
			items := make([]contracts.CollectionDigestEntry, 0, len(cb.items))
			for _, it := range cb.items {
				items = append(items, contracts.CollectionDigestEntry{
					EntityType: it.EntityType,
					EntityName: s.resolveEntityName(it.EntityType, it.EntityID),
					EntityURL:  s.buildEntityURL(it.EntityType, it.EntityID),
					AddedBy:    digestDisplayName(it.AddedByUsername, it.AddedByFirst, it.AddedByEmail),
				})
			}
			groups = append(groups, contracts.CollectionDigestGroup{
				CollectionTitle: cb.title,
				CollectionURL:   collectionURL,
				Items:           items,
			})
		}

		if len(groups) == 0 {
			continue
		}

		// Unsubscribe URL points at the BACKEND so the same path serves
		// both the manual-click HTML confirmation page (GET) and the
		// RFC 8058 / RFC 2369 one-click POST. No SPA round-trip.
		unsubURL := GenerateCollectionDigestUnsubscribeURL(s.backendURL, ub.userID, s.jwtSecret)

		if s.emailService != nil && s.emailService.IsConfigured() {
			if err := s.emailService.SendCollectionDigestEmail(ub.userEmail, groups, unsubURL); err != nil {
				sentry.WithScope(func(scope *sentry.Scope) {
					scope.SetTag("service", "collection_digest")
					sentry.CaptureException(err)
				})
				s.logger.Error("failed to send collection digest email",
					"user_id", ub.userID, "error", err)
				errorCount++
				continue
			}
			sentCount++
		} else {
			// Email service not configured — still bump cursors so we don't
			// keep accumulating "queued" items forever. Mirrors the
			// CommentNotificationService convention.
			s.logger.Info("email service not configured, marking digest cursor anyway",
				"user_id", ub.userID, "items", len(candidates))
		}

		// Bump cursor on every subscription row that contributed an item.
		// Done after a successful send (or after a deliberate skip when
		// email is not configured) so a transient send error in a future
		// cycle doesn't lose items.
		collectionIDs := ub.collectionOrder
		if err := s.markDigested(ub.userID, collectionIDs, now); err != nil {
			s.logger.Error("failed to update last_digest_sent_at",
				"user_id", ub.userID, "error", err)
			// Don't bump errorCount — the email did go out; this is a
			// surfacing-only failure.
		}
	}

	s.logger.Info("collection digest cycle completed",
		"users_with_items", len(userOrder),
		"sent", sentCount,
		"errors", errorCount,
		"skipped_no_email", skippedNoEmail,
	)
}

// queryCandidates loads every (user, item) pair eligible for the current
// digest cycle.
//
// `now` is the cycle's reference time — items created after `now` are NOT
// included so a long-running cycle doesn't double-include items added during
// the cycle itself.
func (s *CollectionDigestService) queryCandidates(now time.Time) ([]digestCandidate, error) {
	type row struct {
		UserID          uint
		UserEmail       *string
		CollectionID    uint
		CollectionTitle string
		CollectionSlug  string
		EntityType      string
		EntityID        uint
		ItemCreatedAt   time.Time
		AddedByUsername *string
		AddedByFirst    *string
		AddedByEmail    *string
	}

	var rows []row

	// Effective cursor: the most recent of the digest cursor and the
	// subscription's created_at. We pick the *more recent* so re-subscribing
	// after unsubscribing gives a clean slate. last_visited_at is
	// intentionally NOT used as a cursor — viewing the collection in the UI
	// already updates last_visited_at so library "new since visit" badges
	// can clear, but we don't want viewing to also suppress the email.
	//
	// notify_on_collection_digest is opt-IN: the column default is FALSE so
	// users must explicitly enable the digest from the notification settings
	// page (PSY-350 / PSY-515). A subscriber with no user_preferences row
	// COALESCEs to FALSE and is excluded.
	err := s.db.Raw(`
		SELECT
			cs.user_id,
			u.email AS user_email,
			c.id AS collection_id,
			c.title AS collection_title,
			c.slug AS collection_slug,
			ci.entity_type,
			ci.entity_id,
			ci.created_at AS item_created_at,
			added_by.username AS added_by_username,
			added_by.first_name AS added_by_first,
			added_by.email AS added_by_email
		FROM collection_subscribers cs
		JOIN users u ON u.id = cs.user_id
		JOIN collections c ON c.id = cs.collection_id
		JOIN collection_items ci
			ON ci.collection_id = cs.collection_id
			AND ci.added_by_user_id <> cs.user_id
			AND ci.created_at > GREATEST(COALESCE(cs.last_digest_sent_at, cs.created_at), cs.created_at)
			AND ci.created_at <= ?
		LEFT JOIN users added_by ON added_by.id = ci.added_by_user_id
		LEFT JOIN user_preferences up ON up.user_id = cs.user_id
		WHERE u.is_active = TRUE
			AND u.deleted_at IS NULL
			AND COALESCE(up.notify_on_collection_digest, FALSE) = TRUE
		ORDER BY cs.user_id ASC, c.id ASC, ci.created_at ASC
	`, now).Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("digest candidate query: %w", err)
	}

	out := make([]digestCandidate, 0, len(rows))
	for _, r := range rows {
		email := ""
		if r.UserEmail != nil {
			email = *r.UserEmail
		}
		out = append(out, digestCandidate{
			UserID:          r.UserID,
			UserEmail:       email,
			CollectionID:    r.CollectionID,
			CollectionTitle: r.CollectionTitle,
			CollectionSlug:  r.CollectionSlug,
			EntityType:      r.EntityType,
			EntityID:        r.EntityID,
			ItemCreatedAt:   r.ItemCreatedAt,
			AddedByUsername: r.AddedByUsername,
			AddedByFirst:    r.AddedByFirst,
			AddedByEmail:    r.AddedByEmail,
		})
	}
	return out, nil
}

// markDigested bumps the per-subscription cursor for every collection that
// contributed an item to the user's digest. Wrapped in a transaction so the
// cursor advances atomically.
func (s *CollectionDigestService) markDigested(userID uint, collectionIDs []uint, now time.Time) error {
	if len(collectionIDs) == 0 {
		return nil
	}
	return s.db.Transaction(func(tx *gorm.DB) error {
		return tx.Model(&models.CollectionSubscriber{}).
			Where("user_id = ? AND collection_id IN ?", userID, collectionIDs).
			Update("last_digest_sent_at", now).Error
	})
}

// resolveEntityName returns a display name for a collection item entity.
// Mirrors collection.go's resolveEntityNameAndSlug, but only returns the name
// since URL building is centralized below.
func (s *CollectionDigestService) resolveEntityName(entityType string, entityID uint) string {
	switch entityType {
	case models.CollectionEntityArtist:
		var name string
		_ = s.db.Table("artists").Where("id = ?", entityID).Pluck("name", &name).Error
		if name != "" {
			return name
		}
	case models.CollectionEntityVenue:
		var name string
		_ = s.db.Table("venues").Where("id = ?", entityID).Pluck("name", &name).Error
		if name != "" {
			return name
		}
	case models.CollectionEntityShow:
		var title string
		_ = s.db.Table("shows").Where("id = ?", entityID).Pluck("title", &title).Error
		if title != "" {
			return title
		}
	case models.CollectionEntityRelease:
		var title string
		_ = s.db.Table("releases").Where("id = ?", entityID).Pluck("title", &title).Error
		if title != "" {
			return title
		}
	case models.CollectionEntityLabel:
		var name string
		_ = s.db.Table("labels").Where("id = ?", entityID).Pluck("name", &name).Error
		if name != "" {
			return name
		}
	case models.CollectionEntityFestival:
		var name string
		_ = s.db.Table("festivals").Where("id = ?", entityID).Pluck("name", &name).Error
		if name != "" {
			return name
		}
	}
	return fmt.Sprintf("%s #%d", entityType, entityID)
}

// buildEntityURL returns the frontend URL for an entity by type+ID. Falls
// back to ID if no slug column exists or the row is missing.
func (s *CollectionDigestService) buildEntityURL(entityType string, entityID uint) string {
	pathSegment, tableName := digestEntityPathAndTable(entityType)
	if tableName == "" {
		return s.frontendURL
	}
	var slug string
	_ = s.db.Table(tableName).Where("id = ?", entityID).Pluck("slug", &slug).Error
	if slug != "" {
		return fmt.Sprintf("%s/%s/%s", s.frontendURL, pathSegment, slug)
	}
	return fmt.Sprintf("%s/%s/%d", s.frontendURL, pathSegment, entityID)
}

// buildCollectionURL returns the collection's frontend URL.
func (s *CollectionDigestService) buildCollectionURL(slug string) string {
	if slug == "" {
		return fmt.Sprintf("%s/collections", s.frontendURL)
	}
	return fmt.Sprintf("%s/collections/%s", s.frontendURL, slug)
}

// digestEntityPathAndTable returns the URL path segment + DB table name for
// a collection entity type. Empty values signal "unknown type".
func digestEntityPathAndTable(entityType string) (string, string) {
	switch entityType {
	case models.CollectionEntityArtist:
		return "artists", "artists"
	case models.CollectionEntityVenue:
		return "venues", "venues"
	case models.CollectionEntityShow:
		return "shows", "shows"
	case models.CollectionEntityRelease:
		return "releases", "releases"
	case models.CollectionEntityLabel:
		return "labels", "labels"
	case models.CollectionEntityFestival:
		return "festivals", "festivals"
	}
	return "", ""
}

// digestDisplayName returns a friendly name for the user who added an item.
// Username first, then first name, then email local-part, then "a contributor".
// Pulled out for unit testability.
func digestDisplayName(username, firstName, email *string) string {
	if username != nil && *username != "" {
		return *username
	}
	if firstName != nil && *firstName != "" {
		return *firstName
	}
	if email != nil && *email != "" {
		// Take everything before the @ as a fallback handle.
		for i, ch := range *email {
			if ch == '@' {
				if i > 0 {
					return (*email)[:i]
				}
				break
			}
		}
	}
	return "a contributor"
}

// RunDigestCycleNow runs the digest cycle synchronously (test/admin entry
// point — mirrors ReminderService.RunReminderCycleNow).
func (s *CollectionDigestService) RunDigestCycleNow() {
	s.runDigestCycle()
}

// ─────────────────────────────────────────────────────────────
// HMAC unsubscribe URL helpers (public — used by email and the
// unsubscribe HTTP handler).
// ─────────────────────────────────────────────────────────────

// GenerateCollectionDigestUnsubscribeURL mints an HMAC-signed URL that flips
// the recipient's `notify_on_collection_digest` preference off when visited.
// Account-wide (preference flag, not per-collection) — same shape as
// GenerateMentionUnsubscribeURL.
//
// `baseURL` should be the public API/backend URL (NOT the frontend) — the
// chi route at /unsubscribe/collection-digest serves an HTML confirmation
// page on GET and accepts an RFC 8058 one-click POST. See
// handlers.UnsubscribeCollectionDigestPageHandler.
func GenerateCollectionDigestUnsubscribeURL(baseURL string, userID uint, secret string) string {
	sig := ComputeCollectionDigestUnsubscribeSignature(userID, secret)
	return fmt.Sprintf("%s/unsubscribe/collection-digest?uid=%d&sig=%s", baseURL, userID, sig)
}

// VerifyCollectionDigestUnsubscribeSignature checks the HMAC for an unsubscribe request.
func VerifyCollectionDigestUnsubscribeSignature(userID uint, signature, secret string) bool {
	expected := ComputeCollectionDigestUnsubscribeSignature(userID, secret)
	return hmac.Equal([]byte(expected), []byte(signature))
}

// ComputeCollectionDigestUnsubscribeSignature hashes userID under secret.
// Bound to the "collection-digest" domain string so a signature minted for
// one notification type can't be replayed against another.
func ComputeCollectionDigestUnsubscribeSignature(userID uint, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(fmt.Sprintf("unsubscribe:collection-digest:%d", userID)))
	return hex.EncodeToString(mac.Sum(nil))
}

