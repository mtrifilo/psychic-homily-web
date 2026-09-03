package user

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"time"

	"gorm.io/gorm"

	"psychic-homily-backend/db"
	apperrors "psychic-homily-backend/internal/errors"
	adminm "psychic-homily-backend/internal/models/admin"
	authm "psychic-homily-backend/internal/models/auth"
	catalogm "psychic-homily-backend/internal/models/catalog"
	communitym "psychic-homily-backend/internal/models/community"
	engagementm "psychic-homily-backend/internal/models/engagement"
	"psychic-homily-backend/internal/services/contracts"
	svcsengagement "psychic-homily-backend/internal/services/engagement"
	"psychic-homily-backend/internal/services/shared"
	"psychic-homily-backend/internal/utils"
)

// profileMarkdown renders profile-section Content to sanitized HTML. The
// renderer is stateless after construction (goldmark + bluemonday), so a single
// package-level instance is shared by the package-level buildSectionResponse
// helpers — same policy used by tag/collection descriptions (PSY-747).
var profileMarkdown = utils.NewMarkdownRenderer()

// ContributorProfileService handles contributor profile and contribution history operations.
type ContributorProfileService struct {
	db *gorm.DB
}

// NewContributorProfileService creates a new contributor profile service.
func NewContributorProfileService(database *gorm.DB) *ContributorProfileService {
	if database == nil {
		database = db.GetDB()
	}
	return &ContributorProfileService{
		db: database,
	}
}

// binaryOnlyFields are privacy fields that only support visible/hidden (not count_only).
var binaryOnlyFields = map[string]bool{
	"last_active":      true,
	"profile_sections": true,
}

// ValidatePrivacySettings checks that all fields have valid values.
func ValidatePrivacySettings(ps contracts.PrivacySettings) error {
	fields := map[string]contracts.PrivacyLevel{
		"contributions":    ps.Contributions,
		"saved_shows":      ps.SavedShows,
		"following":        ps.Following,
		"collections":      ps.Collections,
		"last_active":      ps.LastActive,
		"profile_sections": ps.ProfileSections,
	}
	for name, level := range fields {
		if level != contracts.PrivacyVisible && level != contracts.PrivacyCountOnly && level != contracts.PrivacyHidden {
			return fmt.Errorf("invalid privacy level %q for field %q", level, name)
		}
		if binaryOnlyFields[name] && level == contracts.PrivacyCountOnly {
			return fmt.Errorf("field %q only supports 'visible' or 'hidden'", name)
		}
	}
	return nil
}

// parsePrivacySettings extracts contracts.PrivacySettings from a user's JSONB column.
func parsePrivacySettings(raw *json.RawMessage) contracts.PrivacySettings {
	if raw == nil {
		return contracts.DefaultPrivacySettings()
	}
	var ps contracts.PrivacySettings
	if err := json.Unmarshal(*raw, &ps); err != nil {
		return contracts.DefaultPrivacySettings()
	}
	return ps
}

// contributionRow is a raw scan target for the UNION query.
type contributionRow struct {
	ID         uint
	Action     string
	EntityType string
	EntityID   uint
	Metadata   *json.RawMessage
	CreatedAt  time.Time
	Source     string
}

// contributionStatCounter selects the counter an audit action feeds.
type contributionStatCounter func(*contracts.ContributionStats) *int64

func moderationActionsCounter(s *contracts.ContributionStats) *int64 { return &s.ModerationActions }
func releasesCreatedCounter(s *contracts.ContributionStats) *int64   { return &s.ReleasesCreated }
func labelsCreatedCounter(s *contracts.ContributionStats) *int64     { return &s.LabelsCreated }
func festivalsCreatedCounter(s *contracts.ContributionStats) *int64  { return &s.FestivalsCreated }

// contributionStatActions is every audit_log action GetContributionStats reads,
// mapped to the counter it feeds.
//
// IT IS ALSO THE QUERY'S OWN ALLOWLIST, and that is the reason it is a map
// rather than a switch. The visibility condition spliced into that query is a
// per-row correlated EXISTS, up to three of them for a collection-typed row,
// one of which joins collection_items, so a group nobody consumes pays the
// whole subplan to produce a number that is discarded. On a heavy contributor
// the comment and subscription rows that pay it outnumber the rows that are
// counted, and this is an anonymous, uncached read.
//
// None of these actions is written with entity_type "collection" by any writer
// today, so the filter also makes the collection arm of the condition
// unreachable. That is an observation about the writers with no guard behind it,
// and it is stated because the failure direction is safe: an action that started
// carrying a collection id would be DECIDED by the arm rather than skipping it.
//
// An audit action added to the WRITERS without an entry in this map counts zero,
// which is also the safe direction, and the disposition test forces whatever
// counter it was meant to feed to record a position before it can ship.
var contributionStatActions = map[string]contributionStatCounter{
	// Moderation, not content creation.
	"approve_show":          moderationActionsCounter,
	"reject_show":           moderationActionsCounter,
	"verify_venue":          moderationActionsCounter,
	"approve_venue_edit":    moderationActionsCounter,
	"reject_venue_edit":     moderationActionsCounter,
	"dismiss_report":        moderationActionsCounter,
	"resolve_report":        moderationActionsCounter,
	"dismiss_artist_report": moderationActionsCounter,
	"resolve_artist_report": moderationActionsCounter,

	// Content creation. PSY-618 moved edit_<type> rows to
	// entity_edit_audit_logs, which is counted separately below.
	"create_release":         releasesCreatedCounter,
	"create_label":           labelsCreatedCounter,
	"create_festival":        festivalsCreatedCounter,
	"add_festival_artist":    festivalsCreatedCounter,
	"remove_festival_artist": festivalsCreatedCounter,
	"update_festival_artist": festivalsCreatedCounter,
	"add_festival_venue":     festivalsCreatedCounter,
	"remove_festival_venue":  festivalsCreatedCounter,
}

// contributionStatActionNames is the map's keys, sorted so the emitted statement
// is byte-identical across processes.
func contributionStatActionNames() []string {
	names := make([]string, 0, len(contributionStatActions))
	for action := range contributionStatActions {
		names = append(names, action)
	}
	sort.Strings(names)
	return names
}

// =============================================================================
// Profile Endpoints
// =============================================================================

// GetPublicProfile returns a user's public profile with privacy-gated fields.
func (s *ContributorProfileService) GetPublicProfile(username string, viewer contracts.ShowViewer) (*contracts.PublicProfileResponse, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	var user authm.User
	err := s.db.Where("username = ?", username).First(&user).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to find user: %w", err)
	}

	isOwner := viewer.UserID != 0 && viewer.UserID == user.ID

	// Private profile check: only the owner can view
	if user.ProfileVisibility == "private" && !isOwner {
		return nil, nil
	}

	privacy := parsePrivacySettings(user.PrivacySettings)

	username_str := ""
	if user.Username != nil {
		username_str = *user.Username
	}

	resp := &contracts.PublicProfileResponse{
		Username:          username_str,
		Bio:               user.Bio,
		BioHTML:           renderBioHTML(user.Bio),
		AvatarURL:         user.AvatarURL,
		DisplayName:       user.DisplayName,
		FirstName:         user.FirstName,
		Location:          user.Location,
		ProfileVisibility: user.ProfileVisibility,
		UserTier:          user.UserTier,
		JoinedAt:          user.CreatedAt,
	}

	// Owner always sees everything + privacy settings for editing
	if isOwner {
		resp.PrivacySettings = &privacy
		resp.LastActive = &user.UpdatedAt

		stats, err := s.GetContributionStats(user.ID, viewer)
		if err != nil {
			return nil, fmt.Errorf("failed to get contribution stats: %w", err)
		}
		resp.Stats = stats

		sections, err := s.GetOwnSections(user.ID)
		if err != nil {
			return nil, fmt.Errorf("failed to get profile sections: %w", err)
		}
		resp.Sections = sections

		return resp, nil
	}

	// Non-owner: apply privacy gating
	switch privacy.Contributions {
	case contracts.PrivacyVisible:
		stats, err := s.GetContributionStats(user.ID, viewer)
		if err != nil {
			return nil, fmt.Errorf("failed to get contribution stats: %w", err)
		}
		resp.Stats = stats
	case contracts.PrivacyCountOnly:
		stats, err := s.GetContributionStats(user.ID, viewer)
		if err != nil {
			return nil, fmt.Errorf("failed to get contribution stats: %w", err)
		}
		resp.StatsCount = &stats.TotalContributions
	}
	// contracts.PrivacyHidden: Stats and StatsCount both remain nil

	if privacy.LastActive == contracts.PrivacyVisible {
		resp.LastActive = &user.UpdatedAt
	}

	if privacy.ProfileSections == contracts.PrivacyVisible {
		sections, err := s.GetUserSections(user.ID)
		if err != nil {
			return nil, fmt.Errorf("failed to get profile sections: %w", err)
		}
		resp.Sections = sections
	}

	return resp, nil
}

// GetOwnProfile returns the authenticated caller's own profile, bypassing
// visibility checks.
//
// Takes only the viewer: the subject and the caller are the same person here, so
// a separate id parameter would be derivable state, and the failure mode of
// letting the two disagree is serving one account's profile under another's
// clearance.
func (s *ContributorProfileService) GetOwnProfile(viewer contracts.ShowViewer) (*contracts.PublicProfileResponse, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	userID := viewer.UserID

	var user authm.User
	err := s.db.First(&user, userID).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to find user: %w", err)
	}

	stats, err := s.GetContributionStats(user.ID, viewer)
	if err != nil {
		return nil, fmt.Errorf("failed to get contribution stats: %w", err)
	}

	sections, err := s.GetOwnSections(user.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to get profile sections: %w", err)
	}

	privacy := parsePrivacySettings(user.PrivacySettings)

	username := ""
	if user.Username != nil {
		username = *user.Username
	}

	return &contracts.PublicProfileResponse{
		Username:          username,
		Bio:               user.Bio,
		BioHTML:           renderBioHTML(user.Bio),
		AvatarURL:         user.AvatarURL,
		DisplayName:       user.DisplayName,
		FirstName:         user.FirstName,
		Location:          user.Location,
		ProfileVisibility: user.ProfileVisibility,
		UserTier:          user.UserTier,
		PrivacySettings:   &privacy,
		JoinedAt:          user.CreatedAt,
		LastActive:        &user.UpdatedAt,
		Stats:             stats,
		Sections:          sections,
	}, nil
}

// =============================================================================
// Privacy Settings
// =============================================================================

// UpdatePrivacySettings validates and persists new privacy settings for a user.
func (s *ContributorProfileService) UpdatePrivacySettings(userID uint, settings contracts.PrivacySettings) (*contracts.PrivacySettings, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	if err := ValidatePrivacySettings(settings); err != nil {
		return nil, err
	}

	raw, err := json.Marshal(settings)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal privacy settings: %w", err)
	}
	rawMsg := json.RawMessage(raw)

	if err := s.db.Model(&authm.User{}).Where("id = ?", userID).Update("privacy_settings", &rawMsg).Error; err != nil {
		return nil, fmt.Errorf("failed to update privacy settings: %w", err)
	}

	return &settings, nil
}

// =============================================================================
// Contribution Stats
// =============================================================================

// GetContributionStats computes aggregate contribution counts for a user, as
// the caller in viewer is allowed to see them.
//
// EVERY count sourced from audit_logs, shows, revisions or collections is
// narrowed to what viewer may see, because each has a filtered public sibling to
// be differenced against: the audit-sourced counters (moderation_actions,
// releases_created, labels_created, festivals_created) against the contributions
// timeline, which reads the same rows for the same actor through the same
// condition; shows_submitted against that timeline too; revisions_made against
// the total GET /users/{id}/revisions reports; collection_items_added against
// the add_collection_item rows in the timeline; and collection_subscriptions
// against GET /auth/collections. A whole number differenced against a filtered
// one is a count of withheld rows published as arithmetic. The zero viewer is
// the anonymous tier.
//
// TWO gated-entity counts here are deliberately NOT narrowed: reports_filed and
// reports_resolved. Every counter's position, theirs included, is recorded in
// contributionStatDispositions in the test beside this file, and a counter added
// over a gated entity fails that test until it records one.
//
// The counts therefore differ per caller, and nothing about this response may be
// cached across credentials. An admin's totals are whole. THE PROFILE OWNER'S
// ARE NOT, quite: a moderator reading their own profile loses the approve_show
// and reject_show rows naming shows they did not submit, and any tag vote on a
// collection they can no longer see. The alternative is a per-caller special
// case on a count whose whole purpose is to be the same rule the timeline
// applies.
func (s *ContributorProfileService) GetContributionStats(userID uint, viewer contracts.ShowViewer) (*contracts.ContributionStats, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	stats := &contracts.ContributionStats{}

	// Count submissions from entity tables
	showsVisible, showsVisibleArgs := shared.VisibleShowPredicateSQL("shows", viewer)
	s.db.Model(&catalogm.Show{}).
		Where("submitted_by = ?", userID).
		Where(showsVisible, showsVisibleArgs...).
		Count(&stats.ShowsSubmitted)
	s.db.Model(&catalogm.Venue{}).Where("submitted_by = ?", userID).Count(&stats.VenuesSubmitted)
	s.db.Table("pending_entity_edits").
		Where("submitted_by = ? AND entity_type = ?", userID, adminm.PendingEditEntityVenue).
		Count(&stats.VenueEditsSubmitted)

	// Count actions from audit_log grouped by action.
	// PSY-618: edit_<type> rows live in entity_edit_audit_logs now and are
	// counted separately below so the contributor activity feed stops
	// dual-rendering trusted-user direct-edits and the stats counters read
	// from a single source of truth.
	//
	// Narrowed by the TIMELINE'S OWN condition, from the same builder the
	// timeline and the activity heatmap use, so the three surfaces cannot
	// disagree about the same rows. The condition passes every row naming an
	// entity type with no read-time rule.
	auditVisible, auditVisibleArgs := contributionVisibilitySQL("audit_logs", viewer)
	type actionCount struct {
		Action string
		Count  int64
	}
	var actionCounts []actionCount
	s.db.Model(&adminm.AuditLog{}).
		Select("action, count(*) as count").
		Where("actor_id = ?", userID).
		Where("action IN ?", contributionStatActionNames()).
		Where(auditVisible, auditVisibleArgs...).
		Group("action").
		Scan(&actionCounts)

	for _, ac := range actionCounts {
		if counter := contributionStatActions[ac.Action]; counter != nil {
			*counter(stats) += ac.Count
		}
	}

	// Count edit events from entity_edit_audit_logs (PSY-618). Edits used to
	// live in audit_logs as "edit_<type>" actions but were split out so
	// trusted-user direct-edits stop dual-rendering in the activity feed.
	//
	// entity_edit_audit_logs.entity_type is a FREE COLUMN with no allowlist
	// behind it, so which types it can carry is a property of its writers rather
	// than a constraint.
	//
	// The switch below is closed over four ungated types, so today the exclusion
	// changes NO count: it is a second lock on a door the switch already shuts.
	// It is here because three of the four counters this arm feeds are also fed
	// by the gated audit_logs scan above, and the disposition recorded for them
	// says "narrowed": adding a `case "show":` would otherwise put gated rows
	// into a counter labelled gated, with nothing failing. Same list the
	// heatmap's two undecided arms exclude.
	type entityEditCount struct {
		EntityType string
		Count      int64
	}
	var entityEditCounts []entityEditCount
	s.db.Model(&adminm.EntityEditAuditLog{}).
		Select("entity_type, count(*) as count").
		Where("actor_id = ?", userID).
		Where("entity_type NOT IN ?", heatmapGatedEntityTypes()).
		Group("entity_type").
		Scan(&entityEditCounts)

	for _, ec := range entityEditCounts {
		switch ec.EntityType {
		case "artist":
			stats.ArtistsEdited += ec.Count
		case "release":
			stats.ReleasesCreated += ec.Count
		case "label":
			stats.LabelsCreated += ec.Count
		case "festival":
			stats.FestivalsCreated += ec.Count
		}
	}

	// Revisions made, counting only revisions on shows viewer may see.
	//
	// This number and the total GET /users/{id}/revisions reports are the same
	// number, and they have to stay that way: both are public, so a difference
	// between them is a count of the author's edits on hidden shows, published
	// as arithmetic. The gate there is admin/revision_visibility.go and the rule
	// under both is services/shared/show_visibility.go.
	//
	// Non-show revisions are counted whatever their entity, matching that
	// listing: the revisions table carries no collection entity_type, so `show`
	// is the only gated type that reaches this count.
	revisionsVisible, revisionsVisibleArgs := shared.VisibleShowRevisionsSQL(shared.RevisionsTable, viewer)
	s.db.Model(&adminm.Revision{}).
		Where("user_id = ?", userID).
		Where(revisionsVisible, revisionsVisibleArgs...).
		Count(&stats.RevisionsMade)

	// Pending entity edits submitted
	s.db.Model(&adminm.PendingEntityEdit{}).Where("submitted_by = ?", userID).Count(&stats.PendingEditsSubmitted)

	// Community participation: votes.
	//
	// tag_votes is POLYMORPHIC, so a row can name a gated show or a private
	// collection, and the routes that write it are gated on exactly that
	// (handlers/catalog.VoteTagHandler). The count takes the same registry-backed
	// condition every other polymorphic surface takes, so a vote the caller could
	// not have cast is not counted for them either. catalogm.TagEntityTypes and
	// the registry hold the same seven types, so nothing is dropped for being
	// unregistered.
	//
	// The other two name no gated entity: relationship votes are artist-to-artist
	// and request votes name a community request.
	tagVotesVisible, tagVotesVisibleArgs := shared.VisibleCommentEntitySQL(
		"tag_votes.entity_type", "tag_votes.entity_id", viewer)
	s.db.Model(&catalogm.TagVote{}).
		Where("user_id = ?", userID).
		Where(tagVotesVisible, tagVotesVisibleArgs...).
		Count(&stats.TagVotesCast)
	s.db.Model(&catalogm.ArtistRelationshipVote{}).Where("user_id = ?", userID).Count(&stats.RelationshipVotesCast)
	s.db.Model(&communitym.RequestVote{}).Where("user_id = ?", userID).Count(&stats.RequestVotesCast)

	// Community participation: collections, narrowed to the collections viewer
	// may see.
	//
	// Both have a filtered public sibling on the same profile: the
	// add_collection_item rows in GetContributionHistory, and the collections
	// GET /auth/collections lists. A whole number differenced against a filtered
	// one is a count of private collections published as arithmetic, which is the
	// disclosure the timeline's gate exists to prevent.
	collectionsVisible, collectionsVisibleArgs := shared.VisibleCollectionPredicateSQL("collections", viewer)
	s.db.Model(&communitym.CollectionItem{}).
		Joins("JOIN collections ON collections.id = collection_items.collection_id").
		Where("collection_items.added_by_user_id = ?", userID).
		Where(collectionsVisible, collectionsVisibleArgs...).
		Count(&stats.CollectionItemsAdded)
	s.db.Model(&communitym.CollectionSubscriber{}).
		Joins("JOIN collections ON collections.id = collection_subscribers.collection_id").
		Where("collection_subscribers.user_id = ?", userID).
		Where(collectionsVisible, collectionsVisibleArgs...).
		Count(&stats.CollectionSubscriptions)

	// Reports filed (entity_reports + show_reports + artist_reports)
	var entityReportsFiled, showReportsFiled, artistReportsFiled int64
	s.db.Model(&communitym.EntityReport{}).Where("reported_by = ?", userID).Count(&entityReportsFiled)
	s.db.Model(&communitym.ShowReport{}).Where("reported_by = ?", userID).Count(&showReportsFiled)
	s.db.Model(&communitym.ArtistReport{}).Where("reported_by = ?", userID).Count(&artistReportsFiled)
	stats.ReportsFiled = entityReportsFiled + showReportsFiled + artistReportsFiled

	// Reports resolved (entity_reports reviewed by this user with resolved/dismissed status)
	var entityReportsResolved, showReportsResolved, artistReportsResolved int64
	s.db.Model(&communitym.EntityReport{}).Where("reviewed_by = ? AND status IN ?", userID, []string{"resolved", "dismissed"}).Count(&entityReportsResolved)
	s.db.Model(&communitym.ShowReport{}).Where("reviewed_by = ? AND status IN ?", userID, []string{"resolved", "dismissed"}).Count(&showReportsResolved)
	s.db.Model(&communitym.ArtistReport{}).Where("reviewed_by = ? AND status IN ?", userID, []string{"resolved", "dismissed"}).Count(&artistReportsResolved)
	stats.ReportsResolved = entityReportsResolved + showReportsResolved + artistReportsResolved

	// Social: followers and following via user_bookmarks with action = 'follow'
	// (PSY-1496). Followers = other users who follow this user (entity_type=user).
	// Following = catalog entities this user follows — exclude entity_type=user
	// so the count stays "entities I follow" (aligned with ProfileFollowing / Library).
	s.db.Model(&engagementm.UserBookmark{}).
		Where("user_id = ? AND action = ? AND entity_type <> ?",
			userID, engagementm.BookmarkActionFollow, svcsengagement.FollowEntityUser).
		Count(&stats.FollowingCount)
	s.db.Model(&engagementm.UserBookmark{}).
		Where("entity_type = ? AND entity_id = ? AND action = ?",
			svcsengagement.FollowEntityUser, userID, engagementm.BookmarkActionFollow).
		Count(&stats.FollowersCount)

	// Approval rate from pending_entity_edits
	var approved, rejected int64
	s.db.Model(&adminm.PendingEntityEdit{}).Where("submitted_by = ? AND status = ?", userID, adminm.PendingEditStatusApproved).Count(&approved)
	s.db.Model(&adminm.PendingEntityEdit{}).Where("submitted_by = ? AND status = ?", userID, adminm.PendingEditStatusRejected).Count(&rejected)
	if total := approved + rejected; total > 0 {
		rate := float64(approved) / float64(total)
		stats.ApprovalRate = &rate
	}

	// Total contributions: content creation + moderation + community participation
	stats.TotalContributions = stats.ShowsSubmitted + stats.VenuesSubmitted +
		stats.VenueEditsSubmitted + stats.ReleasesCreated + stats.LabelsCreated +
		stats.FestivalsCreated + stats.ArtistsEdited + stats.ModerationActions +
		stats.RevisionsMade + stats.PendingEditsSubmitted +
		stats.TagVotesCast + stats.RelationshipVotesCast + stats.RequestVotesCast +
		stats.CollectionItemsAdded + stats.ReportsFiled + stats.ReportsResolved

	return stats, nil
}

// =============================================================================
// Contribution History
// =============================================================================

// contributionShowEntityTypes are the entity_type discriminators a contribution
// row carries when it names a show: the plain type on a submission, an audit row
// and a direct-edit audit row, and the synthetic "<type>_edit" the
// pending_entity_edits union emits.
//
// Both are gated, and only the plain one is written today:
// adminm.ValidPendingEditEntityTypes admits artist, venue, festival, release and
// label, so the union emits no "show_edit". It is dispositioned anyway because
// admitting show there is a one-line edit in another package, and such a row
// resolves to no name while still publishing the show's id, which is the whole of
// what an enumeration oracle needs.
var contributionShowEntityTypes = []string{"show", "show_edit"}

// heatmapGatedEntityTypes are the entity_type values the heatmap's two
// undecided arms refuse outright: every discriminator this file gates anywhere.
//
// Derived from the two lists above rather than written again, so a type that
// gains a rule is excluded from those arms by the same edit that gates it. The
// arms cannot decide a row against its entity because they carry catalog types
// whose gate is "no rule at all"; refusing the gated ones is what keeps a writer
// that starts recording one from publishing a day.
//
// Sorted so the emitted statement is byte-identical across processes.
func heatmapGatedEntityTypes() []string {
	types := make([]string, 0, len(contributionShowEntityTypes)+1)
	types = append(types, contributionShowEntityTypes...)
	types = append(types, contributionCollectionEntityType)
	sort.Strings(types)
	return types
}

// contributionCollectionEntityType is the discriminator a contribution row
// carries when it names a collection. There is no "collection_edit" twin: the
// synthetic "<type>_edit" comes from pending_entity_edits alone, and
// adminm.ValidPendingEditEntityTypes admits artist, venue, festival, release and
// label, so the union never says collection.
const contributionCollectionEntityType = "collection"

// contributionCollectionIDKind says WHICH id an audit action stores under
// entity_type "collection".
type contributionCollectionIDKind int

const (
	// collectionIDUndecided is the ZERO VALUE and is never stored. An action
	// with no entry resolves to it, and the gate refuses that row, so a writer
	// added without a disposition withholds rows instead of judging them against
	// the wrong table.
	collectionIDUndecided contributionCollectionIDKind = iota
	// collectionIDIsCollection: entity_id is a collections row id.
	collectionIDIsCollection
	// collectionIDIsItem: entity_id is a collection_items row id.
	collectionIDIsItem
)

// contributionCollectionActions is the disposition of every audit action written
// with entity_type "collection". FOUR handlers write that discriminator:
// community/collection.go for the collection's own lifecycle,
// community/entity_report.go for the report actions, engagement/comment.go for
// create_comment and engagement/comment_subscription.go for the subscribe pair.
// The last three all store whatever entity type the caller named, so any of them
// produces a collection-typed row.
//
// AN ACTION MISSING HERE IS DROPPED FROM EVERY TIMELINE, including its own
// author's and on public collections, so the map being short is a data-loss bug
// as well as an over-withholding one.
//
// TWO KINDS OF ID UNDER ONE DISCRIMINATOR is the fact this map exists to record.
// A gate that read entity_id one way for all of them would judge the item
// actions against whatever collection happens to share the item's number, and
// every one of these rows carries the parent's slug in its metadata, so the row
// discloses the collection's identity whichever id it holds.
//
// An action missing from this map is refused, which is the same fail-closed
// default the entity registry in services/shared uses. Adding a writer is one
// line here plus the judgement it forces.
//
// A ROW WHOSE COLLECTION OR ITEM NO LONGER EXISTS is withheld from everyone,
// including its own author. Collections and collection_items are both
// hard-deleted and neither the id nor the recorded slug resolves afterwards, so
// deleting your own collection erases its rows from your own timeline. That is
// the same answer the show arm gives for a deleted show, and it is the
// recoverable direction: the alternative reads a slug out of a private
// collection.
var contributionCollectionActions = map[string]contributionCollectionIDKind{
	"create_collection":         collectionIDIsCollection,
	"update_collection":         collectionIDIsCollection,
	"delete_collection":         collectionIDIsCollection,
	contributionCloneAction:     collectionIDIsCollection,
	"set_collection_featured":   collectionIDIsCollection,
	"bulk_add_collection_items": collectionIDIsCollection,
	"add_collection_tag":        collectionIDIsCollection,
	"remove_collection_tag":     collectionIDIsCollection,

	// handlers/community/entity_report.go. Each stores the REPORTED entity's
	// type and id, so a reported collection produces a collection-typed row.
	"report_collection":     collectionIDIsCollection,
	"resolve_entity_report": collectionIDIsCollection,
	"dismiss_entity_report": collectionIDIsCollection,

	// handlers/engagement. Both families store the COMMENTED-ON or SUBSCRIBED-TO
	// entity's type and id, so a collection produces a collection-typed row.
	"create_comment":       collectionIDIsCollection,
	"subscribe_comments":   collectionIDIsCollection,
	"unsubscribe_comments": collectionIDIsCollection,

	"add_collection_item":    collectionIDIsItem,
	"update_collection_item": collectionIDIsItem,
	"remove_collection_item": collectionIDIsItem,
}

// contributionCollectionActionsOfKind is the sorted list of actions with one
// disposition, for the SQL IN-lists. Sorted so the emitted statement is
// byte-identical across processes, since Go randomises map iteration and the
// bind order has to be a property of the map rather than of the walk.
func contributionCollectionActionsOfKind(kind contributionCollectionIDKind) []string {
	if kind == collectionIDUndecided {
		// The zero value is never stored, so asking for its members would build
		// an IN-list that matches every unregistered action and hand it a branch.
		return nil
	}
	actions := make([]string, 0, len(contributionCollectionActions))
	for action, k := range contributionCollectionActions {
		if k == kind {
			actions = append(actions, action)
		}
	}
	sort.Strings(actions)
	return actions
}

// isCollectionItemAction reports whether an action's entity_id is a
// collection_items id rather than a collections id.
func isCollectionItemAction(action string) bool {
	return contributionCollectionActions[action] == collectionIDIsItem
}

// contributionCloneSourceKeys are the metadata keys clone_collection writes to
// name the collection that was forked. The clone itself is public, so the ROW
// passes the gate on its own id while these two keys can still name a source the
// viewer may not see; they are removed rather than the row being withheld.
var contributionCloneSourceKeys = []string{"source_slug", "source_id"}

// contributionCloneAction is the audit action that writes the keys above. It is
// the disposition map's key too, so a rename cannot leave the scrub matching an
// action nothing writes.
const contributionCloneAction = "clone_collection"

// contributionEntityRequestActions is every audit action whose entity_id is an
// entity_requests row id.
//
// THE ROW'S entity_type LIES ABOUT WHICH TABLE THAT ID BELONGS TO. The entity
// request writers store the REQUESTED catalog type ("show", "artist", …) so an
// admin query by entity_type reaches them, and the request's own id beside it.
// Nothing else on the row distinguishes it from an audit row that really names a
// show, so the ACTION is the only discriminator, and it is the one this map
// keys on.
//
// An action missing here is judged by the entity-type arms, which read entity_id
// as an id in the table entity_type names. For a request that is an unrelated
// record with the same number.
//
// The rescue void path writes entity_type "entity_request" when a re-read failed
// and it has no requested type to record; that row is a member of this family
// like every other row its action writes, which is why the family is keyed on
// the action alone.
var contributionEntityRequestActions = map[string]bool{
	// handlers/community/entity_request.go, create: one of three actions
	// depending on what the create did.
	"queue_entity_request":        true,
	"replace_entity_request":      true,
	"auto_approve_entity_request": true,

	// handlers/community/entity_request.go, admin decide.
	"approve_entity_request": true,
	"reject_entity_request":  true,

	// handlers/community/entity_request_rescue.go.
	"rescue_fulfill_entity_request": true,
	"rescue_void_entity_request":    true,
}

// contributionEntityRequestActionNames is the map's keys, sorted so the emitted
// statement is byte-identical across processes and the bind order is a property
// of the map rather than of Go's randomised map iteration.
func contributionEntityRequestActionNames() []string {
	names := make([]string, 0, len(contributionEntityRequestActions))
	for action := range contributionEntityRequestActions {
		names = append(names, action)
	}
	sort.Strings(names)
	return names
}

// =============================================================================
// WHAT audit_logs.metadata MAY PUBLISH
// =============================================================================

// contributionCollectionMetadataKeys are the two keys the collection arm of
// contributionVisibilitySQL reads to decide a collection-typed row.
//
// A row served by that arm was decided AGAINST THE COLLECTION THESE NAME, so
// publishing them adds nothing the served row does not already say. Any other
// key on those rows is withheld, including the entity_type and entity_id
// add_collection_item records for the item's own subject, which can name a show
// the viewer may not see.
var contributionCollectionMetadataKeys = []string{"collection_id", "slug"}

// contributionMetadataKeys is the ALLOWLIST of audit metadata keys the
// contributions timeline publishes, keyed by action. Every other key on every
// other action is dropped.
//
// audit_logs.metadata is written by ~100 call sites across nine packages and is
// served, verbatim before this map existed, by GET /users/{username}/contributions
// — optional auth, `contributions` visible by default, so an ANONYMOUS caller
// under the ACTOR's own username. Without an allowlist every writer decides by
// accident what becomes public: an admin's rejection reason, a moderation note,
// a report id, the id of a gated show a report names.
//
// AN ACTION WITH NO ENTRY PUBLISHES NO METADATA, and an entry naming no key is
// how a writer records that it publishes none. The distinction is presence, not
// emptiness: the disposition test fails for an action nobody has decided, while
// the projection treats undecided and withheld alike, so a writer that ships
// before its disposition withholds rather than publishes.
//
// THE ALLOWLIST IS NOT VIEWER-DEPENDENT. The owner reading their own timeline
// through GET /auth/profile/contributions gets the same keys an anonymous reader
// gets, because both handlers call one service and the page is public by
// default: a key safe only for the owner would be one privacy-setting flip from
// public. Admins read whole rows on GET /admin/audit-logs, which this does not
// touch.
var contributionMetadataKeys = map[string][]string{
	// The collection lifecycle and item actions, all written by
	// handlers/community/collection.go. Uniform across the family: the gate arm
	// that serves any of these rows decides it against the collection these two
	// keys name.
	"create_collection":         contributionCollectionMetadataKeys,
	"update_collection":         contributionCollectionMetadataKeys,
	"delete_collection":         contributionCollectionMetadataKeys,
	"set_collection_featured":   contributionCollectionMetadataKeys,
	"bulk_add_collection_items": contributionCollectionMetadataKeys,
	"add_collection_tag":        contributionCollectionMetadataKeys,
	"remove_collection_tag":     contributionCollectionMetadataKeys,
	"add_collection_item":       contributionCollectionMetadataKeys,
	"update_collection_item":    contributionCollectionMetadataKeys,
	"remove_collection_item":    contributionCollectionMetadataKeys,

	// The clone's forked-from attribution. The clone is public and the SOURCE
	// may not be, so these two are gated a second time, per viewer and per row,
	// by scrubCloneSourceMetadata.
	contributionCloneAction: contributionCloneSourceKeys,

	// ---------------------------------------------------------------------
	// Everything below publishes NO key. Grouped by writer, and each entry is
	// a decision about that writer's metadata rather than an omission.
	// ---------------------------------------------------------------------

	// handlers/admin. Reasons, edit ids and submitter ids are moderation
	// artifacts; a rejection reason in particular is an admin's prose about
	// another user's submission.
	"approve_show":      nil,
	"reject_show":       nil,
	"verify_venue":      nil,
	"revision_rollback": nil,

	// handlers/catalog. Names and ids the entity's own public route already
	// serves, plus batch counters nobody reads. Withheld because the timeline
	// resolves an entity's name through enrichEntityNames, which is gated,
	// while these are copies frozen at write time behind no gate at all.
	"create_artist":                     nil,
	"add_artist_alias":                  nil,
	"delete_artist_alias":               nil,
	"merge_artists":                     nil,
	"create_artist_relationship":        nil,
	"delete_artist_relationship":        nil,
	"derive_artist_relationships":       nil,
	"create_festival":                   nil,
	"delete_festival":                   nil,
	"add_festival_artist":               nil,
	"update_festival_artist":            nil,
	"remove_festival_artist":            nil,
	"add_festival_venue":                nil,
	"remove_festival_venue":             nil,
	"create_label":                      nil,
	"delete_label":                      nil,
	"add_artist_to_label":               nil,
	"add_release_to_label":              nil,
	"create_release":                    nil,
	"delete_release":                    nil,
	"add_release_link":                  nil,
	"remove_release_link":               nil,
	"create_tag":                        nil,
	"update_tag":                        nil,
	"delete_tag":                        nil,
	"create_tag_alias":                  nil,
	"delete_tag_alias":                  nil,
	"bulk_import_tag_aliases":           nil,
	"snooze_low_quality_tag":            nil,
	"bulk_low_quality_tags":             nil,
	"create_venue":                      nil,
	"create_radio_station":              nil,
	"update_radio_station":              nil,
	"delete_radio_station":              nil,
	"create_radio_show":                 nil,
	"update_radio_show":                 nil,
	"delete_radio_show":                 nil,
	"trigger_radio_station_sync":        nil,
	"trigger_radio_show_backfill":       nil,
	"link_radio_play":                   nil,
	"bulk_link_radio_plays":             nil,
	"rematch_radio_plays":               nil,
	"cancel_radio_sync_run":             nil,
	"update_streaming_discovery_status": nil,
	"accept_link_suggestion":            nil,
	"reject_link_suggestion":            nil,

	// handlers/community, reports and requests. report_id, report_type and
	// notes are the moderation queue's own fields, and show_id on the two show
	// report actions names a show that may be gated while the row itself is
	// typed "show_report" and passes every entity-type arm untouched.
	"resolve_entity_report":    nil,
	"dismiss_entity_report":    nil,
	"dismiss_report":           nil,
	"resolve_report":           nil,
	"resolve_report_with_flag": nil,
	"create_request":           nil,
	"update_request":           nil,
	"delete_request":           nil,
	"fulfill_request":          nil,
	"approve_fulfillment":      nil,
	"reject_fulfillment":       nil,
	"close_request":            nil,

	// handlers/community, entity requests. The whole family, and the reason the
	// allowlist was worth building: a replacement's row carries a digest of the
	// submission it destroyed, and the decide rows carry the requester's id.
	// The rows themselves reach only the requester and admins; the metadata
	// reaches nobody.
	"queue_entity_request":          nil,
	"replace_entity_request":        nil,
	"auto_approve_entity_request":   nil,
	"approve_entity_request":        nil,
	"reject_entity_request":         nil,
	"rescue_fulfill_entity_request": nil,
	"rescue_void_entity_request":    nil,

	// handlers/engagement. The comment actions record the commented-on entity's
	// id and the comment's own id; the entity id is the one the row's gate
	// already decided, and the comment id names a comment whose own route
	// decides who may read it.
	"create_comment":          nil,
	"edit_comment":            nil,
	"delete_comment":          nil,
	"update_reply_permission": nil,
	"hide_comment":            nil,
	"restore_comment":         nil,
	"approve_comment":         nil,
	"reject_comment":          nil,
	"subscribe_comments":      nil,
	"unsubscribe_comments":    nil,
	"create_field_note":       nil,

	// entity_edit_audit_logs, whose rows enter the union with a synthesised
	// "edit_<entity_type>" action and their metadata intact. Four writers pass
	// none; admin_scenes.go records which field it cleared.
	"edit_artist":   nil,
	"edit_festival": nil,
	"edit_label":    nil,
	"edit_release":  nil,
	"edit_scene":    nil,
}

// contributionMetadataWithheldPrefixes are the action PREFIXES of the writers
// that build an action by concatenating a literal onto an entity type, so the
// full action string exists only at run time.
//
// Every family here publishes NO metadata, which is also the projection's
// default for an action it does not recognise, so the projection never consults
// this list: it exists so the disposition test can tell a family that was
// decided from one nobody has looked at.
//
// AN ACTION REACHABLE ONLY THROUGH A PREFIX THEREFORE PUBLISHES NOTHING. To
// publish a key for one, spell the exact action into contributionMetadataKeys,
// where a reader sees which action it is rather than which letters it starts
// with.
var contributionMetadataWithheldPrefixes = []string{
	// handlers/admin/pending_edit.go: "approve_edit_" / "reject_edit_" + the
	// edit's entity type. Both carry the edit id, the submitter's user id and,
	// on a rejection, the admin's reason.
	"approve_edit_",
	"reject_edit_",

	// handlers/community/entity_report.go: "report_" + the reported entity's
	// type. Carries the report id and the report type.
	"report_",
}

// projectContributionMetadata returns the metadata one contribution row may
// publish: the allowlisted keys of its action that the row actually carries.
//
// Returns nil rather than an empty map when nothing survives, so the field is
// omitted from the response instead of rendering as `{}` — a row with a
// withheld key and a row with no metadata at all answer alike.
func projectContributionMetadata(action string, metadata map[string]interface{}) map[string]interface{} {
	keys := contributionMetadataKeys[action]
	if len(keys) == 0 || len(metadata) == 0 {
		return nil
	}
	projected := make(map[string]interface{}, len(keys))
	for _, key := range keys {
		if value, ok := metadata[key]; ok {
			projected[key] = value
		}
	}
	if len(projected) == 0 {
		return nil
	}
	return projected
}

// contributionVisibilitySQL returns the condition that decides ONE row of an
// audit-shaped result against viewer, plus its bind arguments.
//
// The timeline's gate, extracted so the ACTIVITY HEATMAP applies the same one.
// Both routes are anonymous and both read audit_logs for the same actor, so a
// heatmap that counted a day the timeline withholds would locate a private
// collection to the day it was touched, and differencing the two recovers how
// many actions were taken on it. One condition, one home, no drift.
//
// alias is the table or subquery alias holding action, entity_type, entity_id
// and metadata, and is a literal in the calling code.
//
// THE ACTION IS READ BEFORE THE ENTITY TYPE, because a row's entity_type does
// not by itself say which table its entity_id belongs to. The entity request
// writers store the REQUESTED catalog type beside a REQUEST id, so an
// outermost CASE splits those rows off by action first and decides them against
// entity_requests; only the remaining rows reach the entity-type arms, where
// entity_id does mean an id in the table entity_type names.
//
// Then one arm per entity type that has a read-time rule:
// contributionShowEntityTypes are the two discriminators a show row can carry,
// and "collection" is the third. Anything else passes.
//
// The collection arm reads entity_id TWO WAYS because the audit writers store two
// kinds of id under that discriminator, and contributionCollectionActions is the
// disposition of each. The CASE picks per row, so neither family is judged
// against the other's table, and an action with NO disposition answers FALSE
// rather than falling into whichever branch is written last.
//
// Parentheses written out rather than left to the driver: the binding this pins
// is `IF a request THEN visible ELSE (not a show OR visible) AND (not a
// collection OR visible)`, never a form where a trailing AND binds inside one of
// the ORs, which would publish every gated row.
//
// Placeholders bind by POSITION, so the arguments are appended in statement
// order.
func contributionVisibilitySQL(alias string, viewer contracts.ShowViewer) (string, []interface{}) {
	entityIDExpr := alias + ".entity_id"
	parentIDExpr := alias + ".metadata->>'collection_id'"
	legacySlugExpr := alias + ".metadata->>'slug'"

	visibleShows, visibleShowsArgs := shared.VisibleShowExistsSQL(entityIDExpr, viewer)
	visibleItemParents, visibleItemParentArgs := shared.VisibleCollectionItemExistsSQL(entityIDExpr, viewer)
	visibleByParentID, visibleByParentIDArgs := shared.VisibleCollectionTextIDExistsSQL(parentIDExpr, viewer)
	visibleByLegacySlug, visibleByLegacySlugArgs := shared.VisibleCollectionSlugExistsSQL(legacySlugExpr, viewer)
	visibleCollections, visibleCollectionArgs := shared.VisibleCollectionExistsSQL(entityIDExpr, viewer)
	visibleRequests, visibleRequestArgs := shared.VisibleEntityRequestExistsSQL(entityIDExpr, viewer)

	// THE SENTINEL COUNTS AS ABSENT on the slug arm's key test. A stored
	// `"collection_id": 0` names no collection, so the parent-id arm answers no
	// for it, and reading the key as PRESENT would leave those rows passing no
	// arm at all and withheld from their own author. The writers cannot produce
	// one any more; the rows that already carry it are what this reads.
	parentIDAbsent := "(" + parentIDExpr + " IS NULL OR " + parentIDExpr + " = '0')"

	cond := fmt.Sprintf(
		"(CASE WHEN %s.action IN ? THEN %s ELSE ("+
			"(%s.entity_type NOT IN ? OR %s)"+
			" AND (%s.entity_type <> ?"+
			" OR (CASE WHEN %s.action IN ? THEN (%s OR %s"+
			" OR (%s AND %s))"+
			" WHEN %s.action IN ? THEN %s"+
			" ELSE FALSE END))"+
			") END)",
		alias, visibleRequests,
		alias, visibleShows,
		alias,
		alias, visibleItemParents, visibleByParentID,
		parentIDAbsent, visibleByLegacySlug,
		alias, visibleCollections)

	var args []interface{}
	args = append(args, contributionEntityRequestActionNames())
	args = append(args, visibleRequestArgs...)
	args = append(args, contributionShowEntityTypes)
	args = append(args, visibleShowsArgs...)
	args = append(args, contributionCollectionEntityType)
	args = append(args, contributionCollectionActionsOfKind(collectionIDIsItem))
	args = append(args, visibleItemParentArgs...)
	args = append(args, visibleByParentIDArgs...)
	args = append(args, visibleByLegacySlugArgs...)
	args = append(args, contributionCollectionActionsOfKind(collectionIDIsCollection))
	args = append(args, visibleCollectionArgs...)
	return cond, args
}

// GetContributionHistory returns a paginated, unified contribution timeline for
// a user, containing only what the caller in viewer is allowed to see.
//
// Rows naming a show or a collection the viewer may not see are dropped, and
// dropped INSIDE the union rather than after it, so the total this reports counts
// the same rows the page contains. Filtering afterwards would return a short page
// beside a total announcing how many rows were withheld, which is the same
// disclosure stated as a number.
//
// The filter is on the unified result, not on the shows subquery alone, because
// four of the five sources can name a show: a submission names the show it
// created, and an audit row, a pending edit and an edit audit each carry a show
// entity_id.
func (s *ContributorProfileService) GetContributionHistory(userID uint, limit, offset int, entityType string, viewer contracts.ShowViewer) ([]*contracts.ContributionEntry, int64, error) {
	if s.db == nil {
		return nil, 0, fmt.Errorf("database not initialized")
	}

	if limit < 1 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}

	auditQuery := `SELECT id, action, entity_type, entity_id, metadata, created_at, 'audit_log' as source FROM audit_logs WHERE actor_id = ?`
	showQuery := `SELECT id, 'submit_show' as action, 'show' as entity_type, id as entity_id, NULL as metadata, created_at, 'submission' as source FROM shows WHERE submitted_by = ?`
	venueQuery := `SELECT id, 'submit_venue' as action, 'venue' as entity_type, id as entity_id, NULL as metadata, created_at, 'submission' as source FROM venues WHERE submitted_by = ?`
	// Suggested edits pulled from the unified pending_entity_edits table (PSY-503
	// retired the legacy pending_venue_edits queue). The source entity_type is
	// normalized back to "{type}_edit" so the activity UI keeps a distinct
	// event icon for edits vs. submissions.
	entityEditQuery := `SELECT id, 'submit_' || entity_type || '_edit' as action, entity_type || '_edit' as entity_type, entity_id as entity_id, NULL as metadata, created_at, 'submission' as source FROM pending_entity_edits WHERE submitted_by = ?`
	// Direct entity-edit audits live in their own table post-PSY-618. These
	// are the terminal "edit applied" events (formerly `edit_<type>` rows in
	// audit_logs). We synthesise the `edit_<type>` action prefix so the
	// frontend's icon/label map — which keys on action="edit_artist" et al.
	// — keeps working without churn.
	entityEditAuditQuery := `SELECT id, 'edit_' || entity_type as action, entity_type, entity_id, metadata, created_at, 'edit_audit' as source FROM entity_edit_audit_logs WHERE actor_id = ?`

	args := []interface{}{userID, userID, userID, userID, userID}

	unionSQL := fmt.Sprintf("(%s) UNION ALL (%s) UNION ALL (%s) UNION ALL (%s) UNION ALL (%s)",
		auditQuery, showQuery, venueQuery, entityEditQuery, entityEditAuditQuery)

	// The visibility gate, applied to the unified result so one condition covers
	// every source. One arm per entity type that has a read-time rule:
	// contributionShowEntityTypes are the two discriminators a show row can
	// carry, and "collection" is the third. Anything else passes.
	//
	// The collection arm reads entity_id TWO WAYS because the audit writers store
	// two kinds of id under that discriminator, and contributionCollectionActions
	// is the disposition of each. The CASE picks per row, so neither family is
	// judged against the other's table, and an action with NO disposition answers
	// FALSE rather than falling into whichever branch is written last.
	//
	// The item branch also accepts the parent named by the metadata's
	// collection_id, because collection_items are hard-deleted and a
	// remove_collection_item row names an item that no longer exists. An id is
	// never reissued, so that reference stays true.
	//
	// A THIRD ARM CARRIES THE ROWS THAT RECORD NO USABLE collection_id, and it is
	// keyed on the metadata SLUG. It selects the rows written before the writers
	// recorded an id, and the rows that recorded the 0 sentinel, which names no
	// collection and so passes the id arm no more than a missing key does.
	// Without this arm every such row whose item has since been removed, which
	// is every remove_collection_item row ever written since the item is deleted
	// by definition, is withheld from its own author on a public collection, and
	// from the total as well as the page. The slug's weakness is real (a rename
	// frees the string, so a later collection can take it) and it is confined to
	// this arm for that reason. The item writers now take the parent id from the
	// service that authorised the write, so the arm takes no new rows.
	//
	// Parentheses written out rather than left to the driver, and the entity-type
	// filter ANDed after: the binding this pins is
	// `(not a show OR visible) AND (not a collection OR visible) AND type = x`,
	// never a form where the trailing AND binds inside one of the ORs, which
	// would publish every gated row.
	//
	// Placeholders bind by POSITION, so the arguments below are appended in
	// statement order.
	filter, filterArgs := contributionVisibilitySQL("unified", viewer)
	entityFilter := " WHERE " + filter
	args = append(args, filterArgs...)
	if entityType != "" {
		entityFilter += " AND unified.entity_type = ?"
		args = append(args, entityType)
	}

	countSQL := fmt.Sprintf("SELECT COUNT(*) FROM (%s) AS unified%s", unionSQL, entityFilter)
	var total int64
	countArgs := make([]interface{}, len(args))
	copy(countArgs, args)
	if err := s.db.Raw(countSQL, countArgs...).Scan(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to count contributions: %w", err)
	}

	dataSQL := fmt.Sprintf("SELECT * FROM (%s) AS unified%s ORDER BY created_at DESC LIMIT ? OFFSET ?",
		unionSQL, entityFilter)
	args = append(args, limit, offset)

	var rows []contributionRow
	if err := s.db.Raw(dataSQL, args...).Scan(&rows).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to get contributions: %w", err)
	}

	entries := make([]*contracts.ContributionEntry, len(rows))
	for i, row := range rows {
		entry := &contracts.ContributionEntry{
			ID:         row.ID,
			Action:     row.Action,
			EntityType: row.EntityType,
			EntityID:   row.EntityID,
			CreatedAt:  row.CreatedAt,
			Source:     row.Source,
		}
		// THE ALLOWLIST IS APPLIED HERE, before anything else reads the row,
		// so no later pass can reintroduce a key by working from the stored
		// document. What survives is the intersection of the action's
		// allowlisted keys with the keys the row carries.
		if row.Metadata != nil {
			var metadata map[string]interface{}
			if err := json.Unmarshal(*row.Metadata, &metadata); err == nil {
				entry.Metadata = projectContributionMetadata(row.Action, metadata)
			}
		}
		entries[i] = entry
	}

	s.enrichEntityNames(entries, viewer)
	s.scrubCloneSourceMetadata(entries, viewer)

	return entries, total, nil
}

// scrubCloneSourceMetadata removes the forked-from keys from clone_collection
// rows whose SOURCE viewer may not see.
//
// The clone is a collection of its own and CloneCollection creates it public, so
// the row passes the gate on its own id. Its metadata names the source, which
// may since have gone private, and the timeline is anonymous-readable. The row
// stays and the two keys go, because the contribution itself is public and only
// the attribution is gated.
//
// One batched lookup for the whole page, filtered by the same predicate every
// other collection read uses.
func (s *ContributorProfileService) scrubCloneSourceMetadata(entries []*contracts.ContributionEntry, viewer contracts.ShowViewer) {
	sourceIDs := make([]uint, 0)
	for _, e := range entries {
		if e.Metadata == nil || e.Action != contributionCloneAction {
			continue
		}
		if id, ok := metadataUint(e.Metadata["source_id"]); ok {
			sourceIDs = append(sourceIDs, id)
		}
	}
	// The QUERY is skipped when no row names a source id; the delete loop below
	// is not, because a row can carry source_slug with no usable source_id and
	// the slug is the disclosure.
	visibleIDs := make(map[uint]bool)
	if len(sourceIDs) > 0 {
		visible, visibleArgs := shared.VisibleCollectionPredicateSQL("collections", viewer)
		var rows []struct{ ID uint }
		s.db.Table("collections").
			Select("id").
			Where("id IN ?", sourceIDs).
			Where(visible, visibleArgs...).
			Scan(&rows)
		for _, r := range rows {
			visibleIDs[r.ID] = true
		}
	}

	for _, e := range entries {
		// SCOPED TO THE ACTION THIS FUNCTION IS ABOUT. These two keys mean "the
		// collection that was forked" on a clone_collection row and nothing at
		// all elsewhere, so an unscoped delete would silently strip them from a
		// future writer that used either name for something else.
		if e.Metadata == nil || e.Action != contributionCloneAction {
			continue
		}
		id, ok := metadataUint(e.Metadata["source_id"])
		if ok && visibleIDs[id] {
			continue
		}
		// An unreadable source id is removed on the same terms as a missing one,
		// so a source that was deleted and one that went private answer alike.
		for _, key := range contributionCloneSourceKeys {
			delete(e.Metadata, key)
		}
	}
}

// metadataUint reads an id out of a decoded JSON metadata value. encoding/json
// decodes every number into float64, so the numeric case is the one that
// matters; the rest are refused rather than coerced.
func metadataUint(v interface{}) (uint, bool) {
	f, ok := v.(float64)
	if !ok || f <= 0 {
		return 0, false
	}
	return uint(f), true
}

// =============================================================================
// Activity Heatmap
// =============================================================================

// GetActivityHeatmap returns daily contribution counts for the last 365 days, as
// the caller in viewer is allowed to see them.
// It aggregates activity across audit_logs, shows, venues, pending_entity_edits, and revisions.
// Only days with count > 0 are returned; the frontend fills in gaps.
//
// The SHOW and REVISION arms are narrowed to what viewer may see: a per-day count
// is a public number, and a day this route counts while the contributions
// timeline does not is a hidden show located to the day it was submitted.
//
// The AUDIT_LOGS arm carries the same condition the contributions timeline
// applies, from the same builder. Both routes are anonymous and both read
// audit_logs for the same actor, so an arm that counted a day the timeline
// withholds would locate a private collection or a gated show to the day it was
// touched, and differencing the two would report how many actions were taken on
// it.
//
// The pending_entity_edits and entity_edit_audit_logs arms are not decided row by
// row. Neither table writes a gated entity type today: pending_entity_edits is
// held to adminm.ValidPendingEditEntityTypes (artist, venue, festival, release,
// label) and entity_edit_audit_logs is written with artist, release, label,
// festival and scene. But entity_edit_audit_logs.entity_type is a free column
// with no allowlist behind it, so that is a property of the writers rather than a
// constraint. Both arms therefore EXCLUDE the gated discriminators outright:
// each carries `entity_type NOT IN ?` bound to heatmapGatedEntityTypes(), so a
// writer that starts recording a show or a collection withholds those days
// instead of locating a gated entity to the day it was touched. Narrowing them to
// the real per-row gate is the follow-up; refusing is the recoverable direction
// until then.
func (s *ContributorProfileService) GetActivityHeatmap(userID uint, viewer contracts.ShowViewer) (*contracts.ActivityHeatmapResponse, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	heatmapShowsVisible, heatmapShowsArgs := shared.VisibleShowPredicateSQL("shows", viewer)
	heatmapRevisionsVisible, heatmapRevisionsArgs := shared.VisibleShowRevisionsSQL(shared.RevisionsTable, viewer)
	heatmapAuditVisible, heatmapAuditArgs := contributionVisibilitySQL("audit_logs", viewer)

	query := fmt.Sprintf(`
		SELECT activity_date, SUM(cnt) AS total_count
		FROM (
			SELECT DATE(created_at) AS activity_date, COUNT(*) AS cnt
			FROM audit_logs
			WHERE actor_id = ? AND created_at >= NOW() - INTERVAL '365 days'
			  AND %s
			GROUP BY DATE(created_at)

			UNION ALL

			SELECT DATE(created_at) AS activity_date, COUNT(*) AS cnt
			FROM shows
			WHERE submitted_by = ? AND created_at >= NOW() - INTERVAL '365 days'
			  AND %s
			GROUP BY DATE(created_at)

			UNION ALL

			SELECT DATE(created_at) AS activity_date, COUNT(*) AS cnt
			FROM venues
			WHERE submitted_by = ? AND created_at >= NOW() - INTERVAL '365 days'
			GROUP BY DATE(created_at)

			UNION ALL

			SELECT DATE(created_at) AS activity_date, COUNT(*) AS cnt
			FROM pending_entity_edits
			WHERE submitted_by = ? AND created_at >= NOW() - INTERVAL '365 days'
			  AND entity_type NOT IN ?
			GROUP BY DATE(created_at)

			UNION ALL

			SELECT DATE(created_at) AS activity_date, COUNT(*) AS cnt
			FROM revisions
			WHERE user_id = ? AND created_at >= NOW() - INTERVAL '365 days'
			  AND %s
			GROUP BY DATE(created_at)

			UNION ALL

			-- PSY-618: edit_<type> rows moved out of audit_logs into
			-- entity_edit_audit_logs. Without this UNION the heatmap
			-- under-counts trusted-user direct edits post-backfill.
			SELECT DATE(created_at) AS activity_date, COUNT(*) AS cnt
			FROM entity_edit_audit_logs
			WHERE actor_id = ? AND created_at >= NOW() - INTERVAL '365 days'
			  AND entity_type NOT IN ?
			GROUP BY DATE(created_at)
		) AS combined
		GROUP BY activity_date
		ORDER BY activity_date ASC
	`, heatmapAuditVisible, heatmapShowsVisible, heatmapRevisionsVisible)

	type dayRow struct {
		ActivityDate time.Time `gorm:"column:activity_date"`
		TotalCount   int       `gorm:"column:total_count"`
	}

	// Argument order follows the placeholders left to right through the unioned
	// arms: audit_logs (+ its visibility args), shows (+ its visibility args),
	// venues, pending_entity_edits (+ its refused types), revisions (+ its
	// visibility args), entity_edit_audit_logs (+ its refused types).
	gatedTypes := heatmapGatedEntityTypes()
	heatmapArgs := []interface{}{userID}
	heatmapArgs = append(heatmapArgs, heatmapAuditArgs...)
	heatmapArgs = append(heatmapArgs, userID)
	heatmapArgs = append(heatmapArgs, heatmapShowsArgs...)
	heatmapArgs = append(heatmapArgs, userID, userID, gatedTypes, userID)
	heatmapArgs = append(heatmapArgs, heatmapRevisionsArgs...)
	heatmapArgs = append(heatmapArgs, userID, gatedTypes)

	var rows []dayRow
	err := s.db.Raw(query, heatmapArgs...).Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("failed to get activity heatmap: %w", err)
	}

	days := make([]contracts.ActivityDay, len(rows))
	for i, row := range rows {
		days[i] = contracts.ActivityDay{
			Date:  row.ActivityDate.Format("2006-01-02"),
			Count: row.TotalCount,
		}
	}

	return &contracts.ActivityHeatmapResponse{Days: days}, nil
}

// =============================================================================
// Profile Sections
// =============================================================================

const maxProfileSections = 3

// GetUserSections returns visible profile sections for a public user.
func (s *ContributorProfileService) GetUserSections(userID uint) ([]*contracts.ProfileSectionResponse, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	var sections []authm.UserProfileSection
	err := s.db.Where("user_id = ? AND is_visible = ?", userID, true).
		Order("position ASC").
		Find(&sections).Error
	if err != nil {
		return nil, fmt.Errorf("failed to get profile sections: %w", err)
	}

	return buildSectionResponses(sections), nil
}

// GetOwnSections returns all profile sections for the authenticated user.
func (s *ContributorProfileService) GetOwnSections(userID uint) ([]*contracts.ProfileSectionResponse, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	var sections []authm.UserProfileSection
	err := s.db.Where("user_id = ?", userID).
		Order("position ASC").
		Find(&sections).Error
	if err != nil {
		return nil, fmt.Errorf("failed to get profile sections: %w", err)
	}

	return buildSectionResponses(sections), nil
}

// CreateSection creates a new profile section. Returns error if user already has max sections.
func (s *ContributorProfileService) CreateSection(userID uint, title string, content string, position int) (*contracts.ProfileSectionResponse, error) {
	if s.db == nil {
		return nil, apperrors.ErrProfileInternal(fmt.Errorf("database not initialized"))
	}

	if len(title) == 0 || len(title) > 255 {
		return nil, apperrors.ErrProfileSectionInvalid("title must be between 1 and 255 characters")
	}
	if len(content) > 10000 {
		return nil, apperrors.ErrProfileSectionInvalid("content must be at most 10000 characters")
	}
	if position < 0 || position >= maxProfileSections {
		return nil, apperrors.ErrProfileSectionInvalid(fmt.Sprintf("position must be between 0 and %d", maxProfileSections-1))
	}

	// Check section count
	var count int64
	s.db.Model(&authm.UserProfileSection{}).Where("user_id = ?", userID).Count(&count)
	if count >= int64(maxProfileSections) {
		return nil, apperrors.ErrProfileSectionInvalid(fmt.Sprintf("maximum %d profile sections allowed", maxProfileSections))
	}

	section := authm.UserProfileSection{
		UserID:    userID,
		Title:     title,
		Content:   content,
		Position:  position,
		IsVisible: true,
	}

	if err := s.db.Create(&section).Error; err != nil {
		return nil, apperrors.ErrProfileInternal(fmt.Errorf("failed to create profile section: %w", err))
	}

	return buildSectionResponse(&section), nil
}

// UpdateSection updates a profile section owned by the user.
func (s *ContributorProfileService) UpdateSection(userID uint, sectionID uint, updates map[string]interface{}) (*contracts.ProfileSectionResponse, error) {
	if s.db == nil {
		return nil, apperrors.ErrProfileInternal(fmt.Errorf("database not initialized"))
	}

	var section authm.UserProfileSection
	err := s.db.Where("id = ? AND user_id = ?", sectionID, userID).First(&section).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperrors.ErrProfileSectionNotFound()
		}
		return nil, apperrors.ErrProfileInternal(fmt.Errorf("failed to find profile section: %w", err))
	}

	// Validate updates
	if title, ok := updates["title"]; ok {
		t := title.(string)
		if len(t) == 0 || len(t) > 255 {
			return nil, apperrors.ErrProfileSectionInvalid("title must be between 1 and 255 characters")
		}
	}
	if content, ok := updates["content"]; ok {
		c := content.(string)
		if len(c) > 10000 {
			return nil, apperrors.ErrProfileSectionInvalid("content must be at most 10000 characters")
		}
	}
	if position, ok := updates["position"]; ok {
		p := position.(int)
		if p < 0 || p >= maxProfileSections {
			return nil, apperrors.ErrProfileSectionInvalid(fmt.Sprintf("position must be between 0 and %d", maxProfileSections-1))
		}
	}

	if err := s.db.Model(&section).Updates(updates).Error; err != nil {
		return nil, apperrors.ErrProfileInternal(fmt.Errorf("failed to update profile section: %w", err))
	}

	// Reload after update
	s.db.First(&section, section.ID)

	return buildSectionResponse(&section), nil
}

// DeleteSection deletes a profile section owned by the user.
func (s *ContributorProfileService) DeleteSection(userID uint, sectionID uint) error {
	if s.db == nil {
		return apperrors.ErrProfileInternal(fmt.Errorf("database not initialized"))
	}

	result := s.db.Where("id = ? AND user_id = ?", sectionID, userID).Delete(&authm.UserProfileSection{})
	if result.Error != nil {
		return apperrors.ErrProfileInternal(fmt.Errorf("failed to delete profile section: %w", result.Error))
	}
	if result.RowsAffected == 0 {
		return apperrors.ErrProfileSectionNotFound()
	}

	return nil
}

// renderBioHTML renders a user's raw bio markdown to sanitized HTML using the
// shared profileMarkdown renderer (goldmark + bluemonday), mirroring profile
// sections. Returns "" when the bio is nil or empty so the response omits
// bio_html.
func renderBioHTML(bio *string) string {
	if bio == nil {
		return ""
	}
	return profileMarkdown.Render(*bio)
}

func buildSectionResponses(sections []authm.UserProfileSection) []*contracts.ProfileSectionResponse {
	responses := make([]*contracts.ProfileSectionResponse, len(sections))
	for i := range sections {
		responses[i] = buildSectionResponse(&sections[i])
	}
	return responses
}

func buildSectionResponse(section *authm.UserProfileSection) *contracts.ProfileSectionResponse {
	return &contracts.ProfileSectionResponse{
		ID:          section.ID,
		Title:       section.Title,
		Content:     section.Content,
		ContentHTML: profileMarkdown.Render(section.Content),
		Position:    section.Position,
		IsVisible:   section.IsVisible,
		CreatedAt:   section.CreatedAt,
		UpdatedAt:   section.UpdatedAt,
	}
}

// =============================================================================
// Percentile Rankings
// =============================================================================

// percentileDimension describes a single contribution dimension for ranking.
type percentileDimension struct {
	key    string // e.g. "shows_submitted"
	label  string // human-readable label
	weight int    // weight for overall score
}

var percentileDimensions = []percentileDimension{
	{key: "shows_submitted", label: "Shows Submitted", weight: 25},
	{key: "venues_submitted", label: "Venues Submitted", weight: 15},
	{key: "tags_applied", label: "Tags Applied", weight: 10},
	{key: "edits_approved", label: "Edits Approved", weight: 25},
	{key: "requests_fulfilled", label: "Requests Fulfilled", weight: 10},
}

// GetPercentileRankings computes percentile rankings for a user across 5 contribution dimensions.
// Returns nil if fewer than 10 active users exist (rankings not meaningful).
func (s *ContributorProfileService) GetPercentileRankings(userID uint) (*contracts.PercentileRankings, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	// Check total active users
	var totalUsers int64
	if err := s.db.Model(&authm.User{}).Where("is_active = ?", true).Count(&totalUsers).Error; err != nil {
		return nil, fmt.Errorf("failed to count active users: %w", err)
	}
	if totalUsers < 10 {
		return nil, nil
	}

	// Get user's counts for each dimension
	userCounts := make(map[string]int64)

	// The public tier, expressed with binds because these are GORM Where calls
	// and can carry arguments. The inlined spellings exist for the leaderboard's
	// string builders, which cannot.
	percentileShowsVisible, percentileShowsArgs := shared.VisibleShowPredicateSQL("shows", contracts.ShowViewer{})
	percentileRevisionsVisible, percentileRevisionsArgs := shared.VisibleShowRevisionsSQL(shared.RevisionsTable, contracts.ShowViewer{})

	// shows_submitted, and edits_approved below, count the PUBLIC tier only for
	// every caller (PSY-1939). A percentile is a position in one shared cohort:
	// the leaderboard it ranks against reads approved-only, so a percentile
	// computed over a wider count would place the user against a population
	// nobody else is measured in, and the gap between the two numbers would be a
	// count of their hidden shows.
	var showCount int64
	s.db.Model(&catalogm.Show{}).
		Where("submitted_by = ?", userID).
		Where(percentileShowsVisible, percentileShowsArgs...).
		Count(&showCount)
	userCounts["shows_submitted"] = showCount

	// venues_submitted
	var venueCount int64
	s.db.Model(&catalogm.Venue{}).Where("submitted_by = ?", userID).Count(&venueCount)
	userCounts["venues_submitted"] = venueCount

	// tags_applied, PUBLIC tier like its two gated siblings above.
	//
	// entity_tags is polymorphic, so a row can name a gated show or a private
	// collection, and this count is published as a VALUE for a NAMED user on an
	// optional-auth route. The leaderboard's tags dimension already reports the
	// public-tier count for the same user through the same predicate
	// (services/user/leaderboard.go), so a whole count here would have been that
	// user's private collections and gated shows recoverable by subtracting one
	// public number from another.
	var tagCount int64
	s.db.Model(&catalogm.EntityTag{}).
		Where("added_by_user_id = ?", userID).
		Where(shared.PublicEntityTagsSQL("entity_tags")).
		Count(&tagCount)
	userCounts["tags_applied"] = tagCount

	// edits_approved: pending_entity_edits approved + revisions
	var pendingEditsApproved int64
	s.db.Model(&adminm.PendingEntityEdit{}).
		Where("submitted_by = ? AND status = ?", userID, adminm.PendingEditStatusApproved).
		Count(&pendingEditsApproved)
	var revisionCount int64
	s.db.Model(&adminm.Revision{}).
		Where("user_id = ?", userID).
		Where(percentileRevisionsVisible, percentileRevisionsArgs...).
		Count(&revisionCount)
	userCounts["edits_approved"] = pendingEditsApproved + revisionCount

	// requests_fulfilled
	var requestsFulfilledCount int64
	s.db.Model(&communitym.Request{}).Where("fulfiller_id = ?", userID).Count(&requestsFulfilledCount)
	userCounts["requests_fulfilled"] = requestsFulfilledCount

	// For each dimension, compute the percentile.
	// Use subquery pattern: count users whose contribution count for this dimension
	// is strictly less than the target user's count. Users with 0 contributions are
	// included via LEFT JOIN + COALESCE in a wrapping subquery.
	rankings := make([]contracts.PercentileRanking, 0, len(percentileDimensions))
	weightedSum := 0
	totalWeight := 0

	for _, dim := range percentileDimensions {
		userVal := userCounts[dim.key]
		var usersWithLess int64

		switch dim.key {
		case "shows_submitted":
			// The COHORT reads the same public tier the subject's own count does.
			// Comparing a filtered numerator against an unfiltered population
			// would rank a user with hidden submissions BELOW peers whose
			// unfiltered totals are counted whole, which is not a percentile of
			// anything (PSY-1939).
			s.db.Raw(`
				SELECT COUNT(*) FROM (
					SELECT u.id, COUNT(s.id) AS cnt
					FROM users u
					LEFT JOIN shows s ON s.submitted_by = u.id AND `+shared.PublicShowPredicateSQL("s")+`
					WHERE u.is_active = true
					GROUP BY u.id
				) sub WHERE sub.cnt < ?
			`, userVal).Scan(&usersWithLess)
		case "venues_submitted":
			s.db.Raw(`
				SELECT COUNT(*) FROM (
					SELECT u.id, COUNT(v.id) AS cnt
					FROM users u
					LEFT JOIN venues v ON v.submitted_by = u.id
					WHERE u.is_active = true
					GROUP BY u.id
				) sub WHERE sub.cnt < ?
			`, userVal).Scan(&usersWithLess)
		case "tags_applied":
			// The COHORT takes the same public-tier condition the user's own
			// count above takes. A narrowed numerator over a whole population
			// would place the user against a cohort measured differently from
			// them, which is the shape the shows dimension avoids by carrying
			// its predicate into the join.
			s.db.Raw(`
				SELECT COUNT(*) FROM (
					SELECT u.id, COUNT(et.id) AS cnt
					FROM users u
					LEFT JOIN entity_tags et ON et.added_by_user_id = u.id AND `+shared.PublicEntityTagsSQL("et")+`
					WHERE u.is_active = true
					GROUP BY u.id
				) sub WHERE sub.cnt < ?
			`, userVal).Scan(&usersWithLess)
		case "edits_approved":
			s.db.Raw(`
				SELECT COUNT(*) FROM (
					SELECT u.id,
						COALESCE((SELECT COUNT(*) FROM pending_entity_edits pe WHERE pe.submitted_by = u.id AND pe.status = 'approved'), 0) +
						COALESCE((SELECT COUNT(*) FROM revisions r WHERE r.user_id = u.id AND `+shared.PublicShowRevisionsSQL("r")+`), 0) AS cnt
					FROM users u
					WHERE u.is_active = true
				) sub WHERE sub.cnt < ?
			`, userVal).Scan(&usersWithLess)
		case "requests_fulfilled":
			s.db.Raw(`
				SELECT COUNT(*) FROM (
					SELECT u.id, COUNT(req.id) AS cnt
					FROM users u
					LEFT JOIN requests req ON req.fulfiller_id = u.id
					WHERE u.is_active = true
					GROUP BY u.id
				) sub WHERE sub.cnt < ?
			`, userVal).Scan(&usersWithLess)
		}

		percentile := int(float64(usersWithLess) / float64(totalUsers) * 100)
		if percentile > 100 {
			percentile = 100
		}

		rankings = append(rankings, contracts.PercentileRanking{
			Dimension:  dim.key,
			Label:      dim.label,
			Percentile: percentile,
			Value:      userVal,
		})

		weightedSum += percentile * dim.weight
		totalWeight += dim.weight
	}

	overallScore := 0
	if totalWeight > 0 {
		overallScore = weightedSum / totalWeight
	}

	return &contracts.PercentileRankings{
		Rankings:     rankings,
		OverallScore: overallScore,
	}, nil
}

// =============================================================================
// Entity Name Enrichment
// =============================================================================

// enrichEntityNames resolves each entry's display name from the entity's own
// table.
//
// ONE ARM PER DISCRIMINATOR, and the two gated arms carry the shared predicate.
// It cannot splice services/shared's fence unconditionally the way the comment
// batch loader does: five of the discriminators here are synthetic
// ("venue_edit", "request" and their siblings) and the registry does not resolve
// them, so an unconditional fence would answer FALSE and strip the names off
// every suggested edit. The arms it does fence are the arms that have a rule.
func (s *ContributorProfileService) enrichEntityNames(entries []*contracts.ContributionEntry, viewer contracts.ShowViewer) {
	// The show and collection lookups repeat the visibility gates the union
	// already applied. Deliberate belt and braces on a security boundary, and
	// they cost no extra query: a title is the payload the two detail routes
	// withhold, and these are the only places in this file that read one. A row
	// that slipped past the union keeps its id here but gains no name.
	showsVisible, showsVisibleArgs := shared.VisibleShowPredicateSQL("shows", viewer)
	collectionsVisible, collectionsVisibleArgs := shared.VisibleCollectionPredicateSQL("collections", viewer)

	idsByType := make(map[string][]uint)
	for _, e := range entries {
		if skipContributionNameLookup(e) {
			continue
		}
		idsByType[e.EntityType] = append(idsByType[e.EntityType], e.EntityID)
	}

	nameMap := make(map[string]map[uint]string)

	for entityType, ids := range idsByType {
		if len(ids) == 0 {
			continue
		}
		names := make(map[uint]string)
		// "<type>_edit" cases handle the synthetic discriminator emitted by
		// the pending_entity_edits UNION in GetContributionHistory; they
		// resolve from the same underlying table as their base type.
		switch entityType {
		case "show":
			var results []struct {
				ID    uint
				Title string
			}
			s.db.Table("shows").
				Select("id, title").
				Where("id IN ?", ids).
				Where(showsVisible, showsVisibleArgs...).
				Scan(&results)
			for _, r := range results {
				names[r.ID] = r.Title
			}
		case "venue", "venue_edit":
			var results []struct {
				ID   uint
				Name string
			}
			s.db.Table("venues").Select("id, name").Where("id IN ?", ids).Scan(&results)
			for _, r := range results {
				names[r.ID] = r.Name
			}
		case "artist", "artist_edit":
			var results []struct {
				ID   uint
				Name string
			}
			s.db.Table("artists").Select("id, name").Where("id IN ?", ids).Scan(&results)
			for _, r := range results {
				names[r.ID] = r.Name
			}
		case "release", "release_edit":
			var results []struct {
				ID    uint
				Title string
			}
			s.db.Table("releases").Select("id, title").Where("id IN ?", ids).Scan(&results)
			for _, r := range results {
				names[r.ID] = r.Title
			}
		case "label", "label_edit":
			var results []struct {
				ID   uint
				Name string
			}
			s.db.Table("labels").Select("id, name").Where("id IN ?", ids).Scan(&results)
			for _, r := range results {
				names[r.ID] = r.Name
			}
		case "festival", "festival_edit":
			var results []struct {
				ID   uint
				Name string
			}
			s.db.Table("festivals").Select("id, name").Where("id IN ?", ids).Scan(&results)
			for _, r := range results {
				names[r.ID] = r.Name
			}
		case "request":
			var results []struct {
				ID    uint
				Title string
			}
			s.db.Table("requests").Select("id, title").Where("id IN ?", ids).Scan(&results)
			for _, r := range results {
				names[r.ID] = r.Title
			}
		case "collection":
			var results []struct {
				ID    uint
				Title string
			}
			// Fenced on the same terms as the show branch above, and for the same
			// reason: a title is the payload GET /collections/{slug} withholds, and
			// this is the one place in this file that reads one.
			s.db.Table("collections").
				Select("id, title").
				Where("id IN ?", ids).
				Where(collectionsVisible, collectionsVisibleArgs...).
				Scan(&results)
			for _, r := range results {
				names[r.ID] = r.Title
			}
		}
		nameMap[entityType] = names
	}

	for _, e := range entries {
		if skipContributionNameLookup(e) {
			continue
		}
		if names, ok := nameMap[e.EntityType]; ok {
			if name, ok := names[e.EntityID]; ok {
				e.EntityName = name
			}
		}
	}
}

// skipContributionNameLookup reports whether an entry's entity_id names
// something other than a row of the table its entity_type names, so resolving it
// there would publish an unrelated record's name.
//
// Two families qualify, and they are the two the gate itself has to special-case
// for the same reason:
//
//   - a collection ITEM action's entity_id is a collection_items id, so looking
//     it up in collections resolves whichever collection shares that number;
//   - an ENTITY REQUEST action's entity_id is an entity_requests id while its
//     entity_type names the requested catalog type, so looking it up resolves
//     whichever artist, venue or show shares that number.
//
// Both carry no name at all rather than a wrong one. Neither reads the request's
// own payload for a name: what a queued request may publish about itself is a
// question for the surface that decides to show one.
//
// Both loops in enrichEntityNames consult this, so a type that is skipped in the
// gathering pass cannot be assigned a name in the writing one.
func skipContributionNameLookup(e *contracts.ContributionEntry) bool {
	if contributionEntityRequestActions[e.Action] {
		return true
	}
	return e.EntityType == contributionCollectionEntityType && isCollectionItemAction(e.Action)
}
