package catalog

import "time"

// ArtistLinkSuggestion is one pre-computed, human-reviewed music-link candidate
// in the bulk-backfill queue (PSY-1199). A sweep cmd (PSY-1206) populates rows
// from MusicBrainz-sourced Bandcamp/Spotify candidates; the admin review API
// lists pending rows and accepts (writes the link via the existing artist
// update path) or rejects them. Nothing is auto-applied — the spikes
// (PSY-1196/1197) found false matches carry real links, so every row is
// human-reviewed.
//
// The candidate columns mirror contracts.MusicLinkCandidate (the LOCKED
// discover-music wire shape) so a discovered candidate can be inserted directly.
// Platform/Confidence/Status are CHECK-constrained at the DB — keep these const
// blocks in sync with the column constraints in the PSY-1199 migration.
type ArtistLinkSuggestion struct {
	ID       uint `gorm:"primaryKey"`
	ArtistID uint `gorm:"column:artist_id;not null"`
	// Platform is the streaming platform: bandcamp or spotify. Mirrors
	// contracts.MusicPlatform* — the suggestion's accept path routes on it.
	Platform string `gorm:"column:platform;size:20;not null"`
	// URL is the candidate profile/artist URL (a Bandcamp *.bandcamp.com profile
	// root or an open.spotify.com/artist URL).
	URL string `gorm:"column:url;type:text;not null"`
	// Source is the discovery source — always "musicbrainz" today (the column
	// CHECK currently allows only that value).
	Source string `gorm:"column:source;size:20;not null;default:musicbrainz"`
	// MBArtistID / MBArtistName carry the MusicBrainz provenance the candidate
	// came from, for reviewer context.
	MBArtistID   *string `gorm:"column:mb_artist_id;type:text"`
	MBArtistName *string `gorm:"column:mb_artist_name;type:text"`
	// Confidence is the region TIER: "high" (MB geography aligned with a PH show
	// region) or "review" (mismatch / non-US / no PH region). It is NOT a gate —
	// a "review" row is still reviewable; it just sorts after "high".
	Confidence string `gorm:"column:confidence;size:10;not null"`
	// RegionMatch / Live mirror the candidate's region-alignment flag and the
	// SSRF-guarded liveness-probe result at sweep time.
	RegionMatch bool `gorm:"column:region_match;not null;default:false"`
	Live        bool `gorm:"column:live;not null;default:false"`
	// Notes is an optional reviewer note (touring-act caveat, MB disambiguation).
	Notes *string `gorm:"column:notes;type:text"`
	// Status is the review state: pending (default), accepted, or rejected.
	Status string `gorm:"column:status;size:10;not null;default:pending"`

	CreatedAt time.Time `gorm:"column:created_at;not null"`
	// ReviewedAt / ReviewedByUserID are stamped when a row is accepted or
	// rejected; nil while pending.
	ReviewedAt       *time.Time `gorm:"column:reviewed_at"`
	ReviewedByUserID *uint      `gorm:"column:reviewed_by_user_id"`
}

func (ArtistLinkSuggestion) TableName() string {
	return "artist_link_suggestions"
}

// Link-suggestion status values. CHECK-constrained at the DB (PSY-1199
// migration) — keep in sync with the column constraint.
const (
	LinkSuggestionStatusPending  = "pending"
	LinkSuggestionStatusAccepted = "accepted"
	LinkSuggestionStatusRejected = "rejected"
)

// Link-suggestion source values. CHECK-constrained at the DB — only
// "musicbrainz" today.
const (
	LinkSuggestionSourceMusicBrainz = "musicbrainz"
)
