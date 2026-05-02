package catalog

import (
	"fmt"
	"time"

	"gorm.io/gorm"

	apperrors "psychic-homily-backend/internal/errors"
	catalogm "psychic-homily-backend/internal/models/catalog"
	"psychic-homily-backend/internal/services/contracts"
)

// Low-quality tag review queue (PSY-310).
//
// A tag is "low quality" if it is not official AND at least one of:
//   - orphaned: usage_count = 0
//   - aging unused: usage_count < 3 AND created_at < now() - 7 days
//   - downvoted: aggregate tag_votes has more -1 than +1 across all applications
//   - short_name: LENGTH(name) < 3
//   - long_name: LENGTH(name) > 40
//
// Snoozed tags (reviewed_at within the last 30 days) are excluded, so admins
// can clear the queue of known-good-enough tags without deleting them. After
// 30 days a snoozed tag reappears so community drift gets re-evaluated.

const (
	lowQualityAgeDays        = 7
	lowQualityUnusedUsageMin = 3
	lowQualitySnoozeDays     = 30
	lowQualityShortNameMax   = 3  // LENGTH(name) < 3 → flagged
	lowQualityLongNameMin    = 40 // LENGTH(name) > 40 → flagged

	// BulkActionMaxTagIDs caps how many tag IDs the bulk-action endpoint will
	// accept in one call. Keeps a single accidental "select all" from blocking
	// the writer for minutes; admins can page through anything larger.
	BulkActionMaxTagIDs = 200

	// Reason identifiers returned to the frontend so the UI can render
	// matching pill labels. Keep in sync with the frontend `LOW_QUALITY_REASONS`.
	LowQualityReasonOrphaned    = "orphaned"
	LowQualityReasonAgingUnused = "aging_unused"
	LowQualityReasonDownvoted   = "downvoted"
	LowQualityReasonShortName   = "short_name"
	LowQualityReasonLongName    = "long_name"

	// Bulk-action verbs accepted by BulkActionLowQualityTags. Plain strings so
	// the handler can forward the action to the audit log without translation.
	BulkActionSnooze       = "snooze"
	BulkActionDelete       = "delete"
	BulkActionMarkOfficial = "mark_official"

	// Audit actions
	AuditActionSnoozeLowQualityTag = "snooze_low_quality_tag"
	AuditActionBulkLowQualityTags  = "bulk_low_quality_tags"
)

// GetLowQualityTagQueue returns non-official tags flagged by at least one of
// the low-quality criteria, excluding those snoozed within the last 30 days.
// Orders newest first so recent community activity surfaces quickly.
func (s *TagService) GetLowQualityTagQueue(limit, offset int) (*contracts.LowQualityTagQueueResponse, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}

	now := time.Now().UTC()
	agingCutoff := now.Add(-time.Duration(lowQualityAgeDays) * 24 * time.Hour)
	snoozeCutoff := now.Add(-time.Duration(lowQualitySnoozeDays) * 24 * time.Hour)

	// Build the candidate set. A single composite WHERE keeps pagination honest
	// (count and page reflect the same filter) and avoids fetching all tags and
	// filtering in Go.
	candidateQuery := s.db.Model(&catalogm.Tag{}).
		Where("is_official = ?", false).
		Where("(reviewed_at IS NULL OR reviewed_at <= ?)", snoozeCutoff).
		Where(s.db.
			Where("usage_count = 0").
			Or("(usage_count < ? AND created_at < ?)", lowQualityUnusedUsageMin, agingCutoff).
			Or("LENGTH(name) < ?", lowQualityShortNameMax).
			Or("LENGTH(name) > ?", lowQualityLongNameMin).
			Or("id IN (?)", downvotedTagIDsSubquery(s.db)),
		)

	var total int64
	if err := candidateQuery.Count(&total).Error; err != nil {
		return nil, fmt.Errorf("failed to count low-quality tags: %w", err)
	}

	var tags []catalogm.Tag
	if err := candidateQuery.Order("created_at DESC").Limit(limit).Offset(offset).Find(&tags).Error; err != nil {
		return nil, fmt.Errorf("failed to list low-quality tags: %w", err)
	}

	// Per-tag vote aggregates for the displayed rows — one query for the page.
	// We still need this for the Reasons computation (downvoted is a threshold
	// on the aggregate, not just presence in the subquery).
	votes, err := s.aggregateTagVotes(tags)
	if err != nil {
		return nil, err
	}

	items := make([]contracts.LowQualityTagQueueItem, len(tags))
	for i, t := range tags {
		agg := votes[t.ID]
		items[i] = contracts.LowQualityTagQueueItem{
			TagListItem: contracts.TagListItem{
				ID:         t.ID,
				Name:       t.Name,
				Slug:       t.Slug,
				Category:   t.Category,
				IsOfficial: t.IsOfficial,
				UsageCount: t.UsageCount,
				CreatedAt:  t.CreatedAt,
			},
			Upvotes:   agg.upvotes,
			Downvotes: agg.downvotes,
			Reasons:   lowQualityReasons(t, agingCutoff, agg),
		}
	}

	return &contracts.LowQualityTagQueueResponse{
		Tags:  items,
		Total: total,
	}, nil
}

// BulkActionLowQualityTags applies one of the supported bulk verbs (snooze,
// delete, mark_official) to a list of tag IDs in a single statement per verb.
// Returns per-action counters so the UI can surface partial outcomes ("Snoozed
// 18, 2 not found") and so the audit log can record real impact.
//
// Behavior:
//   - snooze: sets reviewed_at = now() on every tag whose ID is in the list.
//   - delete: deletes the tags. FK cascades (entity_tags, tag_votes,
//     tag_aliases) handle the dependent rows.
//   - mark_official: sets is_official = true on every tag in the list.
//
// IDs that don't exist are silently skipped and counted in NotFound. This
// lets the UI keep moving even if a row was deleted out from under the admin
// (concurrent admin actions, or a snooze that already cleared the row).
//
// actorUserID is forwarded to the audit log by the caller (handler); the
// service is decoupled from the audit log service to keep its dependency
// surface small.
func (s *TagService) BulkActionLowQualityTags(action string, tagIDs []uint) (*contracts.BulkLowQualityTagActionResult, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}
	if len(tagIDs) == 0 {
		return nil, apperrors.ErrTagBulkActionInvalid("tag_ids must not be empty")
	}
	if len(tagIDs) > BulkActionMaxTagIDs {
		return nil, apperrors.ErrTagBulkActionInvalid(
			fmt.Sprintf("tag_ids exceeds max of %d", BulkActionMaxTagIDs),
		)
	}

	// Dedupe — admins may accidentally double-select if the queue re-renders
	// mid-selection. Cheap and avoids confusing counters.
	seen := make(map[uint]struct{}, len(tagIDs))
	unique := make([]uint, 0, len(tagIDs))
	for _, id := range tagIDs {
		if id == 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		unique = append(unique, id)
	}
	if len(unique) == 0 {
		return nil, apperrors.ErrTagBulkActionInvalid("tag_ids must contain at least one non-zero ID")
	}

	result := &contracts.BulkLowQualityTagActionResult{
		Action:    action,
		Requested: int64(len(unique)),
	}

	// Resolve which IDs actually exist before mutating, so we can populate
	// NotFound accurately even when the underlying op (Update / Delete) is
	// idempotent for missing rows.
	var existingIDs []uint
	if err := s.db.Model(&catalogm.Tag{}).
		Where("id IN ?", unique).
		Pluck("id", &existingIDs).Error; err != nil {
		return nil, fmt.Errorf("failed to resolve tag IDs: %w", err)
	}
	result.NotFound = result.Requested - int64(len(existingIDs))
	if len(existingIDs) == 0 {
		return result, nil
	}

	switch action {
	case BulkActionSnooze:
		now := time.Now().UTC()
		res := s.db.Model(&catalogm.Tag{}).Where("id IN ?", existingIDs).Update("reviewed_at", now)
		if res.Error != nil {
			return nil, fmt.Errorf("failed to bulk snooze tags: %w", res.Error)
		}
		result.Affected = res.RowsAffected
	case BulkActionDelete:
		res := s.db.Where("id IN ?", existingIDs).Delete(&catalogm.Tag{})
		if res.Error != nil {
			return nil, fmt.Errorf("failed to bulk delete tags: %w", res.Error)
		}
		result.Affected = res.RowsAffected
	case BulkActionMarkOfficial:
		res := s.db.Model(&catalogm.Tag{}).Where("id IN ?", existingIDs).Update("is_official", true)
		if res.Error != nil {
			return nil, fmt.Errorf("failed to bulk mark tags official: %w", res.Error)
		}
		result.Affected = res.RowsAffected
	default:
		return nil, apperrors.ErrTagBulkActionInvalid(
			fmt.Sprintf("unknown action %q (allowed: snooze, delete, mark_official)", action),
		)
	}

	return result, nil
}

// SnoozeLowQualityTag marks a tag as reviewed-now so it drops out of the queue
// for the snooze window. Writes a fire-and-forget audit log entry via the
// tags_audit indirection pattern — callers (handlers) record the audit log
// themselves since the service layer has no access to the audit log service.
func (s *TagService) SnoozeLowQualityTag(tagID uint, actorUserID uint) error {
	if s.db == nil {
		return fmt.Errorf("database not initialized")
	}

	var tag catalogm.Tag
	if err := s.db.First(&tag, tagID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return apperrors.ErrTagNotFound(tagID)
		}
		return fmt.Errorf("failed to get tag: %w", err)
	}

	now := time.Now().UTC()
	if err := s.db.Model(&tag).Update("reviewed_at", now).Error; err != nil {
		return fmt.Errorf("failed to snooze tag: %w", err)
	}

	return nil
}

// downvotedTagIDsSubquery builds a subquery returning the IDs of tags whose
// aggregate vote total across all (entity_type, entity_id) applications has
// more -1 than +1. Matches the shape PruneDownvotedTags uses on the full
// tag_votes table but aggregates at the tag level (not per-application) so a
// broadly downvoted tag surfaces once.
func downvotedTagIDsSubquery(db *gorm.DB) *gorm.DB {
	return db.Table("tag_votes").
		Select("tag_id").
		Group("tag_id").
		Having("SUM(CASE WHEN vote = -1 THEN 1 ELSE 0 END) > SUM(CASE WHEN vote = 1 THEN 1 ELSE 0 END)")
}

type tagVoteAggregate struct {
	upvotes   int64
	downvotes int64
}

// aggregateTagVotes fetches per-tag upvote/downvote counts across all
// applications of the tag. Empty input → empty map.
func (s *TagService) aggregateTagVotes(tags []catalogm.Tag) (map[uint]tagVoteAggregate, error) {
	out := make(map[uint]tagVoteAggregate, len(tags))
	if len(tags) == 0 {
		return out, nil
	}

	ids := make([]uint, len(tags))
	for i, t := range tags {
		ids[i] = t.ID
	}

	type row struct {
		TagID     uint
		Upvotes   int64
		Downvotes int64
	}
	var rows []row
	err := s.db.Table("tag_votes").
		Select("tag_id, SUM(CASE WHEN vote = 1 THEN 1 ELSE 0 END) AS upvotes, SUM(CASE WHEN vote = -1 THEN 1 ELSE 0 END) AS downvotes").
		Where("tag_id IN ?", ids).
		Group("tag_id").
		Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("failed to aggregate tag votes: %w", err)
	}

	for _, r := range rows {
		out[r.TagID] = tagVoteAggregate{upvotes: r.Upvotes, downvotes: r.Downvotes}
	}
	return out, nil
}

// lowQualityReasons returns the human-readable identifiers for every criterion
// the tag triggers. A tag always has at least one reason (otherwise the SQL
// filter wouldn't have returned it), but we re-evaluate rather than trust the
// filter so the UI reflects the actual data the admin is looking at.
func lowQualityReasons(t catalogm.Tag, agingCutoff time.Time, agg tagVoteAggregate) []string {
	reasons := make([]string, 0, 4)

	if t.UsageCount == 0 {
		reasons = append(reasons, LowQualityReasonOrphaned)
	} else if t.UsageCount < lowQualityUnusedUsageMin && t.CreatedAt.Before(agingCutoff) {
		reasons = append(reasons, LowQualityReasonAgingUnused)
	}

	if agg.downvotes > agg.upvotes {
		reasons = append(reasons, LowQualityReasonDownvoted)
	}

	nameLen := len(t.Name)
	if nameLen < lowQualityShortNameMax {
		reasons = append(reasons, LowQualityReasonShortName)
	} else if nameLen > lowQualityLongNameMin {
		reasons = append(reasons, LowQualityReasonLongName)
	}

	return reasons
}
