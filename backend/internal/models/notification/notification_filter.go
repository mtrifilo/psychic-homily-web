package notification

import (
	"encoding/json"
	"time"

	"github.com/lib/pq"
)

// Filter ownership sources (PSY-1467). Quick entity toggles only create/delete
// "managed" rows; settings-authored filters are "user" and must never be
// mutated by the NotifyMeButton lifecycle.
const (
	FilterSourceUser    = "user"
	FilterSourceManaged = "managed"
)

// NotificationFilter represents a user-created filter for automatic show notifications.
// When a show is approved, all active filters are evaluated against it.
type NotificationFilter struct {
	ID     uint   `gorm:"primaryKey" json:"id"`
	UserID uint   `gorm:"not null" json:"user_id"`
	Name   string `gorm:"size:128;not null" json:"name"`

	// Source is "user" (settings-authored) or "managed" (entity-page quick toggle).
	// Default "user". PSY-1467.
	Source string `gorm:"size:16;not null;default:user" json:"source"`

	// IsActive allows pausing without deleting. Default true.
	IsActive bool `gorm:"not null;default:true" json:"is_active"`

	// Match criteria — all nullable, NULL means "any".
	// OR logic within a criteria type, AND logic across types.
	ArtistIDs     pq.Int64Array    `gorm:"type:bigint[]" json:"artist_ids,omitempty"`
	VenueIDs      pq.Int64Array    `gorm:"type:bigint[]" json:"venue_ids,omitempty"`
	LabelIDs      pq.Int64Array    `gorm:"type:bigint[]" json:"label_ids,omitempty"`
	TagIDs        pq.Int64Array    `gorm:"type:bigint[]" json:"tag_ids,omitempty"`
	ExcludeTagIDs pq.Int64Array    `gorm:"type:bigint[];column:exclude_tag_ids" json:"exclude_tag_ids,omitempty"`
	Cities        *json.RawMessage `gorm:"type:jsonb" json:"cities,omitempty"`
	PriceMaxCents *int             `gorm:"column:price_max_cents" json:"price_max_cents,omitempty"`

	// Delivery preferences
	NotifyEmail bool `gorm:"not null;default:true" json:"notify_email"`
	NotifyInApp bool `gorm:"not null;default:true" json:"notify_in_app"`
	NotifyPush  bool `gorm:"not null;default:false" json:"notify_push"`

	// Metadata
	LastMatchedAt *time.Time `json:"last_matched_at,omitempty"`
	MatchCount    int        `gorm:"not null;default:0" json:"match_count"`
	CreatedAt     time.Time  `gorm:"not null" json:"created_at"`
	UpdatedAt     time.Time  `gorm:"not null" json:"updated_at"`
}

// TableName specifies the table name for NotificationFilter.
func (NotificationFilter) TableName() string {
	return "notification_filters"
}

// NotificationLog records every notification sent, for deduplication and user history.
type NotificationLog struct {
	ID         uint       `gorm:"primaryKey" json:"id"`
	UserID     uint       `gorm:"not null" json:"user_id"`
	FilterID   *uint      `json:"filter_id,omitempty"`
	EntityType string     `gorm:"size:50;not null" json:"entity_type"`
	EntityID   uint       `gorm:"column:entity_id;not null" json:"entity_id"`
	Channel    string     `gorm:"size:20;not null" json:"channel"`
	SentAt     time.Time  `gorm:"not null" json:"sent_at"`
	ReadAt     *time.Time `json:"read_at,omitempty"`

	// SubjectEntityID is the FOLLOWED entity whose subscription produced this
	// row, where (EntityType, EntityID) is what the row is ABOUT (PSY-1896).
	// For an artist show alert that split is artist vs show: the inbox line
	// reads "<artist> announced a show" while the row points at the show.
	//
	// The subject's type is implied by EntityType rather than stored beside it,
	// so the two can never disagree. NULL for every writer that has no followed
	// subject: filter matches, scene follows, comment replies, mentions, and
	// request fulfillments.
	SubjectEntityID *uint `gorm:"column:subject_entity_id" json:"subject_entity_id,omitempty"`

	// AlertBucket is the calendar day a COALESCED alert covers, and it is part
	// of that alert's identity rather than metadata about it (PSY-1895).
	//
	// Venue show alerts are one row per (user, venue, DAY): tomorrow's alert for
	// the same venue is a different notification, not a duplicate of today's.
	// (EntityType, EntityID) is ('venue_show_alert', venue id) and has no room
	// for the day, so it is stored here and included in
	// uq_notification_log_venue_show_alert.
	//
	// A time.Time on a DATE column: only the calendar fields are meaningful, and
	// the driver reads the value back at midnight UTC regardless of the zone the
	// day was computed in. Compare and write it as a day, never as an instant.
	//
	// Two writers use it, with different widths: venue show alerts bucket by
	// venue-local DAY, and artist release digests (PSY-1897) by the Monday of an
	// ISO week in UTC. Both store a calendar day here; only the stride differs.
	//
	// NULL for every per-event writer. A database CHECK per coalesced
	// discriminator forbids NULL on its own rows, because a NULL there would make
	// that discriminator's unique index inert (NULLs compare distinct) and
	// re-send the alert on every flush. A new coalesced type needs its OWN CHECK:
	// the existing ones name their entity_type explicitly and cover nothing else.
	AlertBucket *time.Time `gorm:"column:alert_bucket" json:"alert_bucket,omitempty"`
}

// TableName specifies the table name for NotificationLog.
func (NotificationLog) TableName() string {
	return "notification_log"
}

// In-app notification_log discriminators shared across domain services that
// write rows and the notification read service that enriches them. Kept here
// (next to the NotificationLog model) so a domain service can mint a row
// without importing another service package. The comment-driven channel +
// entity-type constants predate this and live in the engagement package
// (engagement.NotificationChannelInApp / NotificationEntityCommentReply); the
// channel value is identical by design.
const (
	// NotificationChannelInApp is the channel value for in-app notification_log
	// rows surfaced by the bell/inbox (vs the "email" channel used by the
	// show-filter matcher).
	NotificationChannelInApp = "in_app"

	// NotificationChannelEmail is the channel value for rows that stand for an
	// email. It is also what the per-user daily email budget counts
	// (maxFilterEmailsPerDay), which is the reason a row exists for the email
	// lane at all.
	//
	// Read the column honestly: it says WHICH LANE a row belongs to, not which
	// lanes the user has enabled. The bell renders rows of either channel today
	// (the show-filter and scene-follow writers stamp "email" on rows that are
	// the user's only in-app record), so it is NOT a visibility switch for those
	// writers. PSY-1896's rows are the exception and say so at their own read
	// site: they use one row per lane, and the email lane is not a bell entry.
	NotificationChannelEmail = "email"

	// NotificationEntityShow is the entity_type the show-filter matcher and the
	// scene-follow fanout write. Spelled once here so a cross-system dedup can
	// name it without a literal.
	NotificationEntityShow = "show"

	// NotificationEntityArtistShowAlert marks a notification_log row created
	// because a show was announced by an ARTIST the user follows (PSY-1896).
	// entity_id holds the show id and subject_entity_id holds the artist id.
	//
	// A discriminator of its own rather than reusing NotificationEntityShow,
	// because the read path has to tell the three follow-driven show rows apart:
	// a filter match names its filter, a scene follow is labelled from the
	// show's venues, and this one names the artist. Sharing "show" would have
	// made this row inherit the scene label, which is the wrong sentence.
	NotificationEntityArtistShowAlert = "artist_show_alert"

	// NotificationEntityVenueShowAlert marks a notification_log row created
	// because a VENUE the user follows announced new shows (PSY-1895).
	//
	// Its ids do not line up with its artist sibling, and the difference is the
	// single most important thing to know about this discriminator:
	//
	//	artist_show_alert:  entity_id = SHOW id,  subject_entity_id = artist id
	//	venue_show_alert:   entity_id = VENUE id, subject_entity_id = NULL
	//
	// A venue alert is COALESCED — one row stands for every show announced at
	// that venue on one venue-local day — so there is no single show for
	// entity_id to hold. It holds the venue, the day lives in alert_bucket, and
	// the shows are looked up from venue_show_alert_batch at read time (which is
	// what lets a show announced later the same day join a row already
	// delivered). subject_entity_id is NULL because the followed entity and the
	// row's subject are the SAME venue; storing it twice would only create two
	// values that can disagree.
	//
	// The consequence that matters at every query site: this entity_id is NOT a
	// show id. It must never join showAlertEntityTypes in the notification
	// service, or the shared "already told about this show" predicate would
	// compare a venue id against a show id and silence unrelated notifications.
	NotificationEntityVenueShowAlert = "venue_show_alert"

	// NotificationEntityArtistReleaseDigest marks a notification_log row created
	// because artists the user follows put out new records (PSY-1897).
	//
	// Its ids line up with NEITHER show sibling, and that is the single most
	// important thing to know about this discriminator:
	//
	//	artist_show_alert:      entity_id = SHOW id,  subject_entity_id = artist id
	//	venue_show_alert:       entity_id = VENUE id, subject_entity_id = NULL
	//	artist_release_digest:  entity_id = USER id,  subject_entity_id = NULL
	//
	// A release digest is coalesced over the reader's WHOLE FOLLOW SET for one
	// week — six bands, nine records, one row — so there is no artist and no
	// release for entity_id to name without lying about the rest. The user id is
	// the one value that is true of the row; the week lives in alert_bucket, and
	// the releases are looked up from artist_release_alert_batch at read time
	// (which is what lets a release accrued later in the week join a row that has
	// already been delivered). subject_entity_id is NULL because there is no
	// single followed subject either.
	//
	// Two consequences at query sites, both of which fail silently if missed:
	//
	//   - This entity_id is NOT a show id. It must never join
	//     showAlertEntityTypes, or the shared "already told about this show"
	//     predicate would read user 42 as show 42.
	//   - This entity_id is NOT an artist id, and subject_entity_id is not one
	//     either. It must never join artistSubjectAlertTypes in
	//     services/catalog, or an artist merge would rewrite a reader's user id.
	NotificationEntityArtistReleaseDigest = "artist_release_digest"

	// NotificationEntityRequestFulfillmentProposed marks a notification_log row
	// created when someone proposes a fulfillment for a community request (the
	// request enters pending_fulfillment). entity_id holds the request_id; the
	// requester is notified so they can approve or reject. PSY-890.
	NotificationEntityRequestFulfillmentProposed = "request_fulfillment_proposed"
)
