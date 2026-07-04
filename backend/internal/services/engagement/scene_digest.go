package engagement

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/getsentry/sentry-go"
	"gorm.io/gorm"

	"psychic-homily-backend/db"
	"psychic-homily-backend/internal/config"
	engagementm "psychic-homily-backend/internal/models/engagement"
	"psychic-homily-backend/internal/services/contracts"
	"psychic-homily-backend/internal/services/shared"
)

// DefaultSceneDigestInterval is how often the scene digest job runs — 168h
// (weekly), one email per user per week. Weekly (not daily) is the anti-spam
// default: following a scene implicitly subscribes to a recurring email, and a
// user following several busy scenes shouldn't get a deluge.
// SCENE_DIGEST_INTERVAL_HOURS overrides it for local dogfooding.
const DefaultSceneDigestInterval = 168 * time.Hour

// Per-scene content caps for one weekly email — bound the email size for a
// user following many scenes. The scene page is the place to see everything.
const (
	sceneDigestShowsPerScene   = 8
	sceneDigestArtistsPerScene = 8
	sceneDigestWindowDays      = 7 // "this week" (PSY-1309 semantics)
	// Safety bound on scene sections in one email (deliverability + fatigue).
	// The catalog is ~a dozen scene-cities today, so this only bites a
	// pathological follow count; excluded scenes keep their cursor and are
	// retried next cycle. A prioritization/rotation upgrade is a follow-up if
	// global expansion makes it bite in practice.
	sceneDigestMaxScenes = 20
)

// SceneDigestService is a ticker-based background service that batches, per
// user, the this-week shows + new bands for every scene they follow into one
// weekly email (PSY-1342, from the PSY-1314 spike). Modeled on
// CollectionDigestService.
//
// Idempotent across restarts: RunTickerLoop fires immediately on startup, so
// the follow-selection query gates on the per-(scene-follow)
// `scene_digest_sent_at` cursor being older than one interval — a follow
// already digested this week is skipped, so a restart doesn't re-send the
// (cursor-independent) this-week-shows section. The cursor advances only after
// a successful send and ONLY on scenes that contributed content, so a band
// that appears in a scene the user follows but that was empty this cycle is
// still included next cycle.
type SceneDigestService struct {
	db           *gorm.DB
	emailService contracts.EmailServiceInterface
	sceneService contracts.SceneServiceInterface
	interval     time.Duration
	stopCh       chan struct{}
	wg           sync.WaitGroup
	logger       *slog.Logger
	frontendURL  string
	backendURL   string
	jwtSecret    string
}

// NewSceneDigestService creates a new scene digest service. sceneService
// provides the per-scene content queries (this-week shows + new bands) so the
// roster/venue scope logic stays in the catalog package.
func NewSceneDigestService(
	database *gorm.DB,
	emailService contracts.EmailServiceInterface,
	sceneService contracts.SceneServiceInterface,
	cfg *config.Config,
) *SceneDigestService {
	if database == nil {
		database = db.GetDB()
	}

	interval := DefaultSceneDigestInterval
	if envInterval := os.Getenv("SCENE_DIGEST_INTERVAL_HOURS"); envInterval != "" {
		if hours, err := strconv.Atoi(envInterval); err == nil && hours > 0 {
			interval = time.Duration(hours) * time.Hour
		}
	}

	return &SceneDigestService{
		db:           database,
		emailService: emailService,
		sceneService: sceneService,
		interval:     interval,
		stopCh:       make(chan struct{}),
		logger:       slog.Default(),
		frontendURL:  cfg.Email.FrontendURL,
		backendURL:   DeriveBackendURL(cfg.Email.FrontendURL),
		jwtSecret:    cfg.JWT.SecretKey,
	}
}

// Start begins the background digest job.
func (s *SceneDigestService) Start(ctx context.Context) {
	s.wg.Add(1)
	go s.run(ctx)
	s.logger.Info("scene digest service started", "interval_hours", s.interval.Hours())
}

// Stop gracefully stops the digest job.
func (s *SceneDigestService) Stop() {
	close(s.stopCh)
	s.wg.Wait()
	s.logger.Info("scene digest service stopped")
}

// run is the main loop. Runs once on startup (idempotent — a second run in a
// row sends nothing because cursors moved).
func (s *SceneDigestService) run(ctx context.Context) {
	defer s.wg.Done()
	shared.RunTickerLoop(ctx, "scene_digest", s.interval, s.stopCh, true, func(_ context.Context) {
		s.runDigestCycle()
	})
}

// RunDigestCycleNow runs the cycle synchronously (test/admin entry point).
func (s *SceneDigestService) RunDigestCycleNow() {
	s.runDigestCycle()
}

// sceneFollowRow is one opted-in scene follow joined with its registry row and
// digest cursor.
type sceneFollowRow struct {
	UserID     uint
	UserEmail  *string
	SceneID    uint
	City       string
	State      string
	Slug       string
	Cursor     *time.Time // scene_digest_sent_at; NULL until first digest
	FollowedAt time.Time  // user_bookmarks.created_at
}

// runDigestCycle sends one digest email per opted-in user, batching the
// this-week shows + new bands for each scene they follow. Empty scene sections
// are skipped; a user with no non-empty section gets no email. Cursors advance
// only on scenes that contributed. All errors are logged; the cycle keeps going.
func (s *SceneDigestService) runDigestCycle() {
	s.logger.Info("starting scene digest cycle")
	now := time.Now().UTC()

	// Only follows not digested within the last interval are due — this is what
	// makes the cycle idempotent across restarts (the ticker runs immediately
	// on startup, and the this-week-shows section isn't otherwise cursor-gated).
	follows, err := s.queryFollows(now.Add(-s.interval))
	if err != nil {
		s.logger.Error("failed to query scene follows", "error", err)
		return
	}
	if len(follows) == 0 {
		s.logger.Info("no opted-in scene follows to digest")
		return
	}

	// Group by user, preserving first-seen order.
	type userBucket struct {
		email   string
		follows []sceneFollowRow
	}
	byUser := make(map[uint]*userBucket)
	order := make([]uint, 0)
	for _, f := range follows {
		ub, ok := byUser[f.UserID]
		if !ok {
			email := ""
			if f.UserEmail != nil {
				email = *f.UserEmail
			}
			ub = &userBucket{email: email}
			byUser[f.UserID] = ub
			order = append(order, f.UserID)
		}
		ub.follows = append(ub.follows, f)
	}

	sent, errors, skippedNoEmail := 0, 0, 0
	for _, userID := range order {
		ub := byUser[userID]

		groups := make([]contracts.SceneDigestGroup, 0, len(ub.follows))
		contributing := make([]uint, 0, len(ub.follows))
		for _, f := range ub.follows {
			// Safety bound: cap scene sections per email. Excluded scenes are
			// NOT added to `contributing`, so their cursors don't advance and
			// they're retried next cycle (their content isn't lost).
			if len(groups) >= sceneDigestMaxScenes {
				break
			}
			group, ok := s.buildSceneGroup(f, now)
			if !ok {
				continue
			}
			groups = append(groups, group)
			contributing = append(contributing, f.SceneID)
		}
		if len(groups) == 0 {
			continue // nothing new in any followed scene this week
		}

		if ub.email == "" {
			skippedNoEmail++
			// Still advance cursors so we don't re-scan the same window forever
			// for an emailless account (mirrors the collection digest).
			if err := s.markDigested(userID, contributing, now); err != nil {
				s.logger.Error("failed to advance scene digest cursor (no email)", "user_id", userID, "error", err)
			}
			continue
		}

		unsubURL := GenerateScopedUnsubscribeURL(s.backendURL, userID, UnsubscribeScopeSceneDigest, s.jwtSecret)

		if s.emailService != nil && s.emailService.IsConfigured() {
			if err := s.emailService.SendSceneDigestEmail(ub.email, groups, unsubURL); err != nil {
				sentry.WithScope(func(scope *sentry.Scope) {
					scope.SetTag("service", "scene_digest")
					sentry.CaptureException(err)
				})
				s.logger.Error("failed to send scene digest email", "user_id", userID, "error", err)
				errors++
				continue // do NOT advance the cursor — retry next cycle
			}
			sent++
		} else {
			s.logger.Info("email service not configured, advancing scene digest cursor anyway", "user_id", userID)
		}

		if err := s.markDigested(userID, contributing, now); err != nil {
			s.logger.Error("failed to advance scene digest cursor", "user_id", userID, "error", err)
			// Email went out; surfacing-only failure, don't count as an error.
		}
	}

	s.logger.Info("scene digest cycle completed",
		"users_with_content", len(order), "sent", sent, "errors", errors, "skipped_no_email", skippedNoEmail)
}

// buildSceneGroup assembles one scene's section: this-week shows (a forward
// snapshot, NOT cursor-gated) + new bands based there since the cursor. Returns
// ok=false when both are empty (the section is skipped). Content-query errors
// degrade that stream to empty rather than failing the user's whole digest.
func (s *SceneDigestService) buildSceneGroup(f sceneFollowRow, now time.Time) (contracts.SceneDigestGroup, bool) {
	shows, err := s.sceneService.GetSceneUpcomingShows(f.City, f.State, sceneDigestWindowDays, sceneDigestShowsPerScene)
	if err != nil {
		// e.g. the scene dipped below the venue threshold — treat as no shows.
		s.logger.Warn("scene digest: upcoming shows unavailable", "scene_id", f.SceneID, "error", err)
		shows = nil
	}

	// New bands since the cursor; first cycle looks back to the follow time.
	since := f.FollowedAt
	if f.Cursor != nil {
		since = *f.Cursor
	}
	newArtists, newArtistsTotal, err := s.sceneService.GetSceneNewArtistsSince(f.City, f.State, since, now, sceneDigestArtistsPerScene)
	if err != nil {
		s.logger.Warn("scene digest: new artists unavailable", "scene_id", f.SceneID, "error", err)
		newArtists, newArtistsTotal = nil, 0
	}

	if len(shows) == 0 && len(newArtists) == 0 {
		return contracts.SceneDigestGroup{}, false
	}

	group := contracts.SceneDigestGroup{
		SceneName:      fmt.Sprintf("%s, %s", f.City, f.State),
		SceneURL:       fmt.Sprintf("%s/scenes/%s", s.frontendURL, f.Slug),
		MoreNewArtists: newArtistsTotal - len(newArtists), // >0 → "+N more"
	}
	for _, sh := range shows {
		group.Shows = append(group.Shows, contracts.SceneDigestShow{
			DisplayTitle: sceneShowDisplayTitle(sh.Title, sh.ArtistNames),
			Date:         formatDigestDate(sh.EventDate),
			VenueName:    sh.VenueName,
			ShowURL:      s.showURL(sh.Slug, sh.ID),
		})
	}
	for _, a := range newArtists {
		group.NewArtists = append(group.NewArtists, contracts.SceneDigestArtist{
			Name:      a.Name,
			ArtistURL: s.artistURL(a.Slug, a.ID),
		})
	}
	return group, true
}

// queryFollows loads every opted-in scene follow DUE for a digest — its cursor
// is NULL (never digested) or older than one interval — with its registry row.
// The interval gate is what makes the cycle idempotent across restarts: a
// follow digested within the last interval is skipped, so the immediate
// startup run doesn't re-send its this-week-shows section. Ordered by user so
// the grouping loop is straightforward.
func (s *SceneDigestService) queryFollows(cutoff time.Time) ([]sceneFollowRow, error) {
	var rows []sceneFollowRow
	err := s.db.Raw(`
		SELECT b.user_id,
		       u.email AS user_email,
		       sc.id AS scene_id,
		       sc.city, sc.state, sc.slug,
		       b.scene_digest_sent_at AS cursor,
		       b.created_at AS followed_at
		FROM user_bookmarks b
		JOIN users u ON u.id = b.user_id
		JOIN scenes sc ON sc.id = b.entity_id
		LEFT JOIN user_preferences up ON up.user_id = b.user_id
		WHERE b.entity_type = 'scene' AND b.action = 'follow'
		  AND u.is_active = TRUE
		  AND u.deleted_at IS NULL
		  AND COALESCE(up.notify_on_scene_digest, FALSE) = TRUE
		  AND (b.scene_digest_sent_at IS NULL OR b.scene_digest_sent_at <= ?)
		ORDER BY b.user_id ASC, sc.city ASC, sc.state ASC, sc.id ASC
	`, cutoff).Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("scene follow candidate query: %w", err)
	}
	return rows, nil
}

// markDigested advances the per-(scene-follow) cursor for every scene that
// contributed content to this user's digest.
func (s *SceneDigestService) markDigested(userID uint, sceneIDs []uint, now time.Time) error {
	if len(sceneIDs) == 0 {
		return nil
	}
	return s.db.Model(&engagementm.UserBookmark{}).
		Where("user_id = ? AND entity_type = ? AND action = ? AND entity_id IN ?",
			userID, engagementm.BookmarkEntityScene, engagementm.BookmarkActionFollow, sceneIDs).
		Update("scene_digest_sent_at", now).Error
}

func (s *SceneDigestService) showURL(slug string, id uint) string {
	if slug != "" {
		return fmt.Sprintf("%s/shows/%s", s.frontendURL, slug)
	}
	return fmt.Sprintf("%s/shows/%d", s.frontendURL, id)
}

func (s *SceneDigestService) artistURL(slug string, id uint) string {
	if slug != "" {
		return fmt.Sprintf("%s/artists/%s", s.frontendURL, slug)
	}
	return fmt.Sprintf("%s/artists/%d", s.frontendURL, id)
}

// sceneShowDisplayTitle mirrors the FE showDisplayTitle convention (PSY-1328):
// title → capped bill → "Untitled Show". Backend-side for the digest email.
func sceneShowDisplayTitle(title string, artistNames []string) string {
	if t := strings.TrimSpace(title); t != "" {
		return t
	}
	names := make([]string, 0, len(artistNames))
	for _, n := range artistNames {
		if t := strings.TrimSpace(n); t != "" {
			names = append(names, t)
		}
	}
	if len(names) == 0 {
		return "Untitled Show"
	}
	const maxNames = 3
	if len(names) > maxNames {
		return fmt.Sprintf("%s +%d more", strings.Join(names[:maxNames], ", "), len(names)-maxNames)
	}
	return strings.Join(names, ", ")
}

// formatDigestDate turns an ISO date-only string (YYYY-MM-DD) into "Mon, Jan 2".
// Date-only, so no timezone nuance. Falls back to the raw value if unparseable.
func formatDigestDate(iso string) string {
	t, err := time.Parse("2006-01-02", iso)
	if err != nil {
		return iso
	}
	return t.Format("Mon, Jan 2")
}
