package engagement

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/getsentry/sentry-go"
	"gorm.io/gorm"

	"psychic-homily-backend/internal/models"
	"psychic-homily-backend/internal/services/contracts"
)

// CommentNotificationService sends emails to subscribers and @-mentioned users on
// new comments. Both entry points are fire-and-forget: the caller should invoke
// them in a goroutine and ignore the returned error (logged internally).
type CommentNotificationService struct {
	db           *gorm.DB
	emailService contracts.EmailServiceInterface
	jwtSecret    string
	frontendURL  string
}

// NewCommentNotificationService constructs the service. All args required
// except jwtSecret/frontendURL, which must be non-empty in production (used
// to mint HMAC unsubscribe URLs).
func NewCommentNotificationService(
	db *gorm.DB,
	emailService contracts.EmailServiceInterface,
	jwtSecret, frontendURL string,
) *CommentNotificationService {
	return &CommentNotificationService{
		db:           db,
		emailService: emailService,
		jwtSecret:    jwtSecret,
		frontendURL:  frontendURL,
	}
}

// subscriberDedupWindow is the cool-down between subscriber emails per
// (user, entity) — a user won't get multiple comment-subscription emails on
// the same entity within this window. Mentions have no such dedup.
const subscriberDedupWindow = 1 * time.Hour

// commentExcerptMaxChars is the max plain-text excerpt rendered into emails.
const commentExcerptMaxChars = 200

// ─────────────────────────────────────────────────────────────
// Mention regex
// ─────────────────────────────────────────────────────────────

// mentionRegex matches `@username` where username matches Psychic Homily
// rules (3-30 chars, alphanumeric + underscore + hyphen). The leading
// lookbehind is simulated via the "preceded by non-alphanumeric or start"
// check in parseMentions, because Go's regexp package lacks lookbehind.
//
// The regex itself captures candidate @handles; parseMentions filters out
// matches where the '@' is preceded by a non-whitespace character (as in
// email addresses or URL fragments like `user@host` / `@example.com/path`).
var mentionRegex = regexp.MustCompile(`@([a-zA-Z0-9][a-zA-Z0-9_-]{2,29})`)

// urlLikeRegex matches anything that looks like an http(s):// URL so we can
// strip URLs before running mention extraction — this prevents a URL whose
// path happens to contain `@something` from being treated as a mention.
var urlLikeRegex = regexp.MustCompile(`https?://\S+`)

// parseMentions extracts unique lowercase usernames from a comment body.
//
// Rules:
//  1. Strip URLs first — anything inside an http(s):// URL is ignored.
//  2. Strip email-address-like substrings (localpart@domain) — these are NOT
//     mentions.
//  3. After stripping, match `@username` where the character immediately
//     before `@` is either start-of-string or whitespace/punctuation (NOT
//     an alphanumeric, underscore, or hyphen — which would indicate we're
//     still inside a word, e.g. part of a badly-stripped email).
//  4. Usernames are de-duped (case-insensitive) preserving first-occurrence
//     order so notification order is stable for tests.
func parseMentions(body string) []string {
	if body == "" {
		return nil
	}

	// 1. Strip URLs.
	cleaned := urlLikeRegex.ReplaceAllString(body, " ")

	// 2. Strip email addresses — anything matching [localpart]@[domain]
	//    where localpart has at least one non-@ char and domain has a dot.
	//    We replace with a space so positions stay reasonable.
	emailRegex := regexp.MustCompile(`[A-Za-z0-9._%+\-]+@[A-Za-z0-9.\-]+\.[A-Za-z]{2,}`)
	cleaned = emailRegex.ReplaceAllString(cleaned, " ")

	// 3. Find mention candidates on the cleaned body, and enforce the
	//    "preceded by non-word char" rule manually (Go regexp has no
	//    lookbehind).
	matches := mentionRegex.FindAllStringSubmatchIndex(cleaned, -1)
	if len(matches) == 0 {
		return nil
	}

	seen := make(map[string]struct{}, len(matches))
	out := make([]string, 0, len(matches))
	for _, m := range matches {
		// m[0] = start of full match (`@`); m[2]..m[3] = capture group.
		atPos := m[0]
		if atPos > 0 {
			prev := cleaned[atPos-1]
			if isWordChar(prev) {
				// E.g. text like `foo@bar` where the email regex above
				// didn't match (no TLD). Skip.
				continue
			}
		}
		username := strings.ToLower(cleaned[m[2]:m[3]])
		if _, dup := seen[username]; dup {
			continue
		}
		seen[username] = struct{}{}
		out = append(out, username)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func isWordChar(b byte) bool {
	return (b >= 'a' && b <= 'z') ||
		(b >= 'A' && b <= 'Z') ||
		(b >= '0' && b <= '9') ||
		b == '_' || b == '-'
}

// ─────────────────────────────────────────────────────────────
// Helpers
// ─────────────────────────────────────────────────────────────

// stripMarkdownToPlain does a best-effort conversion of markdown body to
// plain text suitable for an email excerpt. This is intentionally simple —
// we only need "readable preview", not perfect rendering.
func stripMarkdownToPlain(body string) string {
	// Remove code fences.
	body = regexp.MustCompile("```[\\s\\S]*?```").ReplaceAllString(body, " ")
	// Remove inline code.
	body = regexp.MustCompile("`[^`]*`").ReplaceAllString(body, " ")
	// Remove markdown link syntax `[text](url)` → keep text.
	body = regexp.MustCompile(`\[([^\]]+)\]\([^\)]+\)`).ReplaceAllString(body, "$1")
	// Remove heading / quote / list markers at line start.
	body = regexp.MustCompile(`(?m)^\s*(#{1,6}|[>*\-+])\s+`).ReplaceAllString(body, "")
	// Collapse whitespace.
	body = regexp.MustCompile(`\s+`).ReplaceAllString(body, " ")
	return strings.TrimSpace(body)
}

// buildExcerpt returns a plain-text preview capped at commentExcerptMaxChars.
func buildExcerpt(body string) string {
	plain := stripMarkdownToPlain(body)
	runes := []rune(plain)
	if len(runes) <= commentExcerptMaxChars {
		return plain
	}
	return string(runes[:commentExcerptMaxChars]) + "…"
}

// buildEntityURL returns the frontend URL for the parent entity, or a
// best-effort fallback if the entity's slug cannot be resolved.
func (s *CommentNotificationService) buildEntityURL(entityType models.CommentEntityType, entityID uint) string {
	tableName, ok := models.ValidCommentEntityTypes[entityType]
	if !ok {
		return s.frontendURL
	}
	var slug string
	// Not every entity has a slug — pluck it, ignore errors, fall back to ID.
	_ = s.db.Table(tableName).Where("id = ?", entityID).Pluck("slug", &slug).Error
	var pathSegment string
	switch entityType {
	case models.CommentEntityArtist:
		pathSegment = "artists"
	case models.CommentEntityVenue:
		pathSegment = "venues"
	case models.CommentEntityShow:
		pathSegment = "shows"
	case models.CommentEntityRelease:
		pathSegment = "releases"
	case models.CommentEntityLabel:
		pathSegment = "labels"
	case models.CommentEntityFestival:
		pathSegment = "festivals"
	case models.CommentEntityCollection:
		pathSegment = "collections"
	default:
		pathSegment = tableName
	}
	if slug != "" {
		return fmt.Sprintf("%s/%s/%s", s.frontendURL, pathSegment, slug)
	}
	return fmt.Sprintf("%s/%s/%d", s.frontendURL, pathSegment, entityID)
}

// buildEntityName returns a display name for the entity (name/title) for use
// in email subjects/bodies. Falls back to "<entity_type> #<id>".
func (s *CommentNotificationService) buildEntityName(entityType models.CommentEntityType, entityID uint) string {
	tableName, ok := models.ValidCommentEntityTypes[entityType]
	if !ok {
		return fmt.Sprintf("%s #%d", entityType, entityID)
	}
	// Most entities have a `name` column; shows have `title` instead.
	column := "name"
	if entityType == models.CommentEntityShow {
		column = "title"
	}
	var name string
	if err := s.db.Table(tableName).Where("id = ?", entityID).Pluck(column, &name).Error; err == nil && name != "" {
		return name
	}
	return fmt.Sprintf("%s #%d", entityType, entityID)
}

// buildCommentURL returns the URL to the specific comment on its entity page.
// We anchor via `#comment-<id>` — the frontend handles scrolling.
func (s *CommentNotificationService) buildCommentURL(entityType models.CommentEntityType, entityID, commentID uint) string {
	return fmt.Sprintf("%s#comment-%d", s.buildEntityURL(entityType, entityID), commentID)
}

// displayName returns a friendly name for a user: username first, else
// first name, else "A contributor".
func displayName(u *models.User) string {
	if u == nil {
		return "A contributor"
	}
	if u.Username != nil && *u.Username != "" {
		return *u.Username
	}
	if u.FirstName != nil && *u.FirstName != "" {
		return *u.FirstName
	}
	return "A contributor"
}

// ─────────────────────────────────────────────────────────────
// NotifySubscribers
// ─────────────────────────────────────────────────────────────

// NotifySubscribers fans out emails to all users subscribed to the parent
// entity, skipping the comment author and subscribers whose
// `last_notified_at` is within the dedup window. Updates `last_notified_at`
// on each subscription row after a successful send.
//
// Callers MUST invoke this in a goroutine — errors are logged only.
func (s *CommentNotificationService) NotifySubscribers(commentID uint) error {
	if s.db == nil {
		return errors.New("database not initialized")
	}

	// 1. Load the comment + author.
	var comment models.Comment
	if err := s.db.Preload("User").First(&comment, commentID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return errors.New("comment not found")
		}
		return fmt.Errorf("failed to load comment: %w", err)
	}

	// 2. Query subscribers via joined preferences. Skip:
	//    - comment author (no self-notify)
	//    - users with notify_on_comment_subscription = false
	//    - subscribers whose last_notified_at is within the dedup window
	cutoff := time.Now().UTC().Add(-subscriberDedupWindow)

	type row struct {
		UserID                      uint
		Email                       *string
		Username                    *string
		FirstName                   *string
		NotifyOnCommentSubscription bool
	}
	var rows []row
	err := s.db.Table("comment_subscriptions cs").
		Select(`cs.user_id,
			u.email,
			u.username,
			u.first_name,
			COALESCE(up.notify_on_comment_subscription, TRUE) AS notify_on_comment_subscription`).
		Joins("JOIN users u ON u.id = cs.user_id").
		Joins("LEFT JOIN user_preferences up ON up.user_id = cs.user_id").
		Where("cs.entity_type = ? AND cs.entity_id = ?", string(comment.EntityType), comment.EntityID).
		Where("cs.user_id <> ?", comment.UserID).
		Where("cs.last_notified_at IS NULL OR cs.last_notified_at < ?", cutoff).
		Where("u.is_active = TRUE").
		Where("u.deleted_at IS NULL").
		Scan(&rows).Error
	if err != nil {
		return fmt.Errorf("failed to load subscribers: %w", err)
	}

	if len(rows) == 0 {
		return nil
	}

	// Sort for stable test output.
	sort.Slice(rows, func(i, j int) bool { return rows[i].UserID < rows[j].UserID })

	// Pre-compute shared email content.
	entityType := string(comment.EntityType)
	entityName := s.buildEntityName(comment.EntityType, comment.EntityID)
	entityURL := s.buildEntityURL(comment.EntityType, comment.EntityID)
	excerpt := buildExcerpt(comment.Body)
	commenterName := displayName(&comment.User)
	now := time.Now().UTC()

	for _, r := range rows {
		if !r.NotifyOnCommentSubscription {
			continue
		}
		if r.Email == nil || *r.Email == "" {
			continue
		}

		unsubURL := GenerateCommentSubscriptionUnsubscribeURL(
			s.frontendURL, r.UserID, entityType, comment.EntityID, s.jwtSecret,
		)

		if s.emailService != nil && s.emailService.IsConfigured() {
			if sendErr := s.emailService.SendCommentNotification(
				*r.Email,
				commenterName,
				entityType,
				entityName,
				excerpt,
				entityURL,
				unsubURL,
			); sendErr != nil {
				sentry.WithScope(func(scope *sentry.Scope) {
					scope.SetTag("service", "comment_notification")
					scope.SetTag("notification_type", "subscriber")
					sentry.CaptureException(sendErr)
				})
				log.Printf("failed to send comment notification email to user %d: %v", r.UserID, sendErr)
				// Don't update last_notified_at on failure.
				continue
			}
		}

		// Bump last_notified_at even if email is not configured — the row
		// still reflects that we "attempted to notify" for this cycle.
		if upErr := s.db.Model(&models.CommentSubscription{}).
			Where("user_id = ? AND entity_type = ? AND entity_id = ?",
				r.UserID, entityType, comment.EntityID).
			Update("last_notified_at", now).Error; upErr != nil {
			log.Printf("failed to update last_notified_at for user %d: %v", r.UserID, upErr)
		}
	}

	return nil
}

// ─────────────────────────────────────────────────────────────
// NotifyMentioned
// ─────────────────────────────────────────────────────────────

// NotifyMentioned parses @mentions in a comment body and emails each mentioned
// user who has notify_on_mention=true. No dedup — each mention is one-off by
// definition. Mentioned users don't have to be subscribed to the entity.
//
// Callers MUST invoke this in a goroutine — errors are logged only.
func (s *CommentNotificationService) NotifyMentioned(commentID uint) error {
	if s.db == nil {
		return errors.New("database not initialized")
	}

	var comment models.Comment
	if err := s.db.Preload("User").First(&comment, commentID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return errors.New("comment not found")
		}
		return fmt.Errorf("failed to load comment: %w", err)
	}

	mentions := parseMentions(comment.Body)
	if len(mentions) == 0 {
		return nil
	}

	// Case-insensitive author username — skip self-mentions.
	var authorUsername string
	if comment.User.Username != nil {
		authorUsername = strings.ToLower(*comment.User.Username)
	}

	// Look up each mentioned user (case-insensitive username match).
	type row struct {
		UserID          uint
		Email           *string
		Username        *string
		FirstName       *string
		NotifyOnMention bool
	}
	var rows []row
	err := s.db.Table("users u").
		Select(`u.id AS user_id,
			u.email,
			u.username,
			u.first_name,
			COALESCE(up.notify_on_mention, TRUE) AS notify_on_mention`).
		Joins("LEFT JOIN user_preferences up ON up.user_id = u.id").
		Where("LOWER(u.username) IN ?", mentions).
		Where("u.is_active = TRUE").
		Where("u.deleted_at IS NULL").
		Scan(&rows).Error
	if err != nil {
		return fmt.Errorf("failed to resolve mentioned users: %w", err)
	}

	if len(rows) == 0 {
		return nil
	}

	entityType := string(comment.EntityType)
	entityName := s.buildEntityName(comment.EntityType, comment.EntityID)
	commentURL := s.buildCommentURL(comment.EntityType, comment.EntityID, comment.ID)
	excerpt := buildExcerpt(comment.Body)
	mentionerName := displayName(&comment.User)

	for _, r := range rows {
		if r.UserID == comment.UserID {
			continue // self-mention
		}
		if r.Username != nil && strings.ToLower(*r.Username) == authorUsername && authorUsername != "" {
			continue
		}
		if !r.NotifyOnMention {
			continue
		}
		if r.Email == nil || *r.Email == "" {
			continue
		}

		unsubURL := GenerateMentionUnsubscribeURL(s.frontendURL, r.UserID, s.jwtSecret)

		if s.emailService != nil && s.emailService.IsConfigured() {
			if sendErr := s.emailService.SendMentionNotification(
				*r.Email,
				mentionerName,
				entityType,
				entityName,
				excerpt,
				commentURL,
				unsubURL,
			); sendErr != nil {
				sentry.WithScope(func(scope *sentry.Scope) {
					scope.SetTag("service", "comment_notification")
					scope.SetTag("notification_type", "mention")
					sentry.CaptureException(sendErr)
				})
				log.Printf("failed to send mention notification email to user %d: %v", r.UserID, sendErr)
			}
		}
	}

	return nil
}

// ─────────────────────────────────────────────────────────────
// HMAC unsubscribe URL helpers (public — used by email templates and the
// unsubscribe HTTP handlers).
// ─────────────────────────────────────────────────────────────

// GenerateCommentSubscriptionUnsubscribeURL mints an HMAC-signed URL that,
// when visited, flips the recipient's `notify_on_comment_subscription`
// preference off. Bound to (userID, entityType, entityID) so the link
// can't be replayed against a different user.
func GenerateCommentSubscriptionUnsubscribeURL(baseURL string, userID uint, entityType string, entityID uint, secret string) string {
	sig := ComputeCommentSubscriptionUnsubscribeSignature(userID, entityType, entityID, secret)
	return fmt.Sprintf(
		"%s/unsubscribe/comment-subscription?uid=%d&entity_type=%s&entity_id=%d&sig=%s",
		baseURL, userID, url.QueryEscape(entityType), entityID, sig,
	)
}

// VerifyCommentSubscriptionUnsubscribeSignature checks the HMAC for a
// comment-subscription unsubscribe request.
func VerifyCommentSubscriptionUnsubscribeSignature(userID uint, entityType string, entityID uint, signature, secret string) bool {
	expected := ComputeCommentSubscriptionUnsubscribeSignature(userID, entityType, entityID, secret)
	return hmac.Equal([]byte(expected), []byte(signature))
}

// ComputeCommentSubscriptionUnsubscribeSignature hashes (userID, entityType,
// entityID) under secret — stable across processes so the link stays valid
// as long as the JWT secret doesn't rotate.
func ComputeCommentSubscriptionUnsubscribeSignature(userID uint, entityType string, entityID uint, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(fmt.Sprintf("unsubscribe:comment-subscription:%d:%s:%d", userID, entityType, entityID)))
	return hex.EncodeToString(mac.Sum(nil))
}

// GenerateMentionUnsubscribeURL mints an HMAC-signed URL that flips the
// recipient's `notify_on_mention` preference off. Keyed by userID only —
// the preference is account-wide.
func GenerateMentionUnsubscribeURL(baseURL string, userID uint, secret string) string {
	sig := ComputeMentionUnsubscribeSignature(userID, secret)
	return fmt.Sprintf("%s/unsubscribe/mention?uid=%d&sig=%s", baseURL, userID, sig)
}

// VerifyMentionUnsubscribeSignature checks the HMAC for a mention
// unsubscribe request.
func VerifyMentionUnsubscribeSignature(userID uint, signature, secret string) bool {
	expected := ComputeMentionUnsubscribeSignature(userID, secret)
	return hmac.Equal([]byte(expected), []byte(signature))
}

// ComputeMentionUnsubscribeSignature hashes userID under secret.
func ComputeMentionUnsubscribeSignature(userID uint, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(fmt.Sprintf("unsubscribe:mention:%d", userID)))
	return hex.EncodeToString(mac.Sum(nil))
}
