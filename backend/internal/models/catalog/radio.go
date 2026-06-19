package catalog

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"
)

// Broadcast type constants for radio stations
const (
	BroadcastTypeTerrestrial = "terrestrial"
	BroadcastTypeInternet    = "internet"
	BroadcastTypeBoth        = "both"
)

// Playlist source constants for radio stations
const (
	PlaylistSourceKEXP   = "kexp_api"
	PlaylistSourceNTS    = "nts_api"
	PlaylistSourceWFMU   = "wfmu_scrape"
	PlaylistSourceManual = "manual"
)

// Rotation status constants for radio plays (KEXP). Enforced by the
// radio_plays_rotation_status_check CHECK constraint (PSY-1131). NULL is also
// accepted (most providers don't supply a rotation); an unrecognized provider
// value must be normalized to NULL by the pipeline before insert.
const (
	RotationStatusHeavy          = "heavy"
	RotationStatusMedium         = "medium"
	RotationStatusLight          = "light"
	RotationStatusRecommendedNew = "recommended_new"
	RotationStatusLibrary        = "library"
)

// Station provenance constants (radio_stations.source). PSY-1131.
//   - canonical: hand-curated seed (KEXP/WFMU/NTS)
//   - discovered: created on first observed episode by the ingestion pipeline
//   - manual: added by a human via admin UI
const (
	RadioStationSourceCanonical  = "canonical"
	RadioStationSourceDiscovered = "discovered"
	RadioStationSourceManual     = "manual"
)

// Show provenance constants (radio_shows.source). PSY-1131.
//   - provider: synced from a station's provider feed (includes pre-seeded shows)
//   - manual: added by a human
const (
	RadioShowSourceProvider = "provider"
	RadioShowSourceManual   = "manual"
)

// Lifecycle-state constants shared by radio_stations and radio_shows. PSY-1131.
// Replaces bare is_active as the operational signal: active = in service;
// dormant = temporarily not airing/syncing; retired = permanently gone.
const (
	RadioLifecycleActive  = "active"
	RadioLifecycleDormant = "dormant"
	RadioLifecycleRetired = "retired"
)

// Episode status constants (radio_episodes.status). PSY-1131. Makes "live" an
// explicit stored fact rather than the implicit "a row exists for today" that
// produced the false ON-AIR bug (PSY-1128). Windowless episodes default to
// 'aired' and are never 'live'.
const (
	RadioEpisodeStatusScheduled = "scheduled"
	RadioEpisodeStatusLive      = "live"
	RadioEpisodeStatusAired     = "aired"
	RadioEpisodeStatusArchived  = "archived"
)

// Playlist-fetch lifecycle constants (radio_episodes.playlist_state). PSY-1131.
// Decoupled from episode status: an aired episode can still have a pending
// playlist fetch.
const (
	RadioPlaylistStatePending     = "pending"
	RadioPlaylistStatePartial     = "partial"
	RadioPlaylistStateComplete    = "complete"
	RadioPlaylistStateUnavailable = "unavailable"
)

// Match-state constants (radio_plays.match_state). PSY-1131. Replaces the
// implicit "artist_id IS NULL == unmatched" with an explicit state. no_match
// (matcher ran, found nothing) is distinct from unmatched (matcher not yet run).
const (
	RadioPlayMatchStateUnmatched = "unmatched"
	RadioPlayMatchStateMatched   = "matched"
	RadioPlayMatchStateAmbiguous = "ambiguous"
	RadioPlayMatchStateNoMatch   = "no_match"
)

// BroadcastTypes is the list of valid broadcast types
var BroadcastTypes = []string{
	BroadcastTypeTerrestrial,
	BroadcastTypeInternet,
	BroadcastTypeBoth,
}

// IsValidBroadcastType checks whether a string is a valid broadcast type
func IsValidBroadcastType(s string) bool {
	for _, bt := range BroadcastTypes {
		if bt == s {
			return true
		}
	}
	return false
}

// PlaylistSources is the list of valid playlist sources. getProvider dispatches
// the three scraper/API sources (kexp_api, nts_api, wfmu_scrape) to a provider;
// "manual" is a valid value meaning hand-curated playlists with no automated
// provider. The empty string is also accepted by IsValidPlaylistSource and
// likewise means "no automated provider" (a link-only station not auto-imported).
var PlaylistSources = []string{
	PlaylistSourceKEXP,
	PlaylistSourceNTS,
	PlaylistSourceWFMU,
	PlaylistSourceManual,
}

// IsValidPlaylistSource reports whether s is an accepted playlist_source. The
// empty string is valid (no automated provider / link-only). Rejecting anything
// else stops invalid values like "wfmu_html" from being persisted and silently
// breaking all playlist import for the station. (PSY-927)
func IsValidPlaylistSource(s string) bool {
	if s == "" {
		return true
	}
	for _, ps := range PlaylistSources {
		if ps == s {
			return true
		}
	}
	return false
}

// IsValidRotationStatus reports whether s is an accepted rotation_status. The
// empty string is valid (no rotation supplied — the common case for non-KEXP
// providers); it maps to a NULL column. Any other unrecognized value is invalid
// and must be normalized to "" by the pipeline before insert, or the
// radio_plays_rotation_status_check CHECK will reject the row (PSY-1131).
func IsValidRotationStatus(s string) bool {
	switch s {
	case "", RotationStatusHeavy, RotationStatusMedium, RotationStatusLight,
		RotationStatusRecommendedNew, RotationStatusLibrary:
		return true
	default:
		return false
	}
}

// IsValidRadioStationSource reports whether s is an accepted station source.
func IsValidRadioStationSource(s string) bool {
	switch s {
	case RadioStationSourceCanonical, RadioStationSourceDiscovered, RadioStationSourceManual:
		return true
	default:
		return false
	}
}

// IsValidRadioShowSource reports whether s is an accepted show source.
func IsValidRadioShowSource(s string) bool {
	switch s {
	case RadioShowSourceProvider, RadioShowSourceManual:
		return true
	default:
		return false
	}
}

// IsValidRadioLifecycleState reports whether s is an accepted lifecycle_state
// for a station or show.
func IsValidRadioLifecycleState(s string) bool {
	switch s {
	case RadioLifecycleActive, RadioLifecycleDormant, RadioLifecycleRetired:
		return true
	default:
		return false
	}
}

// IsValidRadioEpisodeStatus reports whether s is an accepted episode status.
func IsValidRadioEpisodeStatus(s string) bool {
	switch s {
	case RadioEpisodeStatusScheduled, RadioEpisodeStatusLive,
		RadioEpisodeStatusAired, RadioEpisodeStatusArchived:
		return true
	default:
		return false
	}
}

// IsValidRadioPlaylistState reports whether s is an accepted playlist_state.
func IsValidRadioPlaylistState(s string) bool {
	switch s {
	case RadioPlaylistStatePending, RadioPlaylistStatePartial,
		RadioPlaylistStateComplete, RadioPlaylistStateUnavailable:
		return true
	default:
		return false
	}
}

// IsValidRadioPlayMatchState reports whether s is an accepted play match_state.
func IsValidRadioPlayMatchState(s string) bool {
	switch s {
	case RadioPlayMatchStateUnmatched, RadioPlayMatchStateMatched,
		RadioPlayMatchStateAmbiguous, RadioPlayMatchStateNoMatch:
		return true
	default:
		return false
	}
}

// RadioScheduleSlot is one recurring weekly air slot in a RadioSchedule.
// DayOfWeek is 0=Sunday..6=Saturday. Start/End are "HH:MM" 24-hour local times
// in the parent RadioSchedule's Timezone. An End <= Start denotes a slot that
// wraps past midnight (e.g. 23:00–01:00).
type RadioScheduleSlot struct {
	DayOfWeek int    `json:"day_of_week"`
	Start     string `json:"start"`
	End       string `json:"end"`
}

// RadioSchedule is the validated JSONB shape stored in radio_shows.schedule
// (PSY-1131). It is the basis for the air-window / "live" computation consumed
// in P4. The column itself is a plain JSONB; this struct + Validate is the
// single place the shape is enforced (the app boundary), so the rule lives in
// one place rather than being duplicated in a brittle JSONB CHECK constraint.
//
//	{ "timezone": "America/Los_Angeles",
//	  "slots": [ { "day_of_week": 1, "start": "06:00", "end": "10:00" } ] }
type RadioSchedule struct {
	Timezone string              `json:"timezone"`
	Slots    []RadioScheduleSlot `json:"slots"`
}

// hhmmPattern matches an "HH:MM" 24-hour time string (00:00–23:59).
var hhmmPattern = regexp.MustCompile(`^([01][0-9]|2[0-3]):[0-5][0-9]$`)

// Validate checks that a RadioSchedule is well-formed: a non-empty IANA
// timezone that the standard library can load, and each slot a valid weekday
// (0–6) with "HH:MM" start/end times. It does NOT reject End <= Start (that is
// the legitimate midnight-wrap encoding). Returns the first violation found.
func (s RadioSchedule) Validate() error {
	if strings.TrimSpace(s.Timezone) == "" {
		return fmt.Errorf("radio schedule: timezone is required")
	}
	if _, err := time.LoadLocation(s.Timezone); err != nil {
		return fmt.Errorf("radio schedule: invalid timezone %q: %w", s.Timezone, err)
	}
	for i, slot := range s.Slots {
		if slot.DayOfWeek < 0 || slot.DayOfWeek > 6 {
			return fmt.Errorf("radio schedule: slot %d: day_of_week %d out of range 0–6", i, slot.DayOfWeek)
		}
		if !hhmmPattern.MatchString(slot.Start) {
			return fmt.Errorf("radio schedule: slot %d: start %q is not HH:MM", i, slot.Start)
		}
		if !hhmmPattern.MatchString(slot.End) {
			return fmt.Errorf("radio schedule: slot %d: end %q is not HH:MM", i, slot.End)
		}
	}
	return nil
}

// ParseRadioSchedule decodes and validates a radio_shows.schedule JSONB value.
// A nil/empty raw message is treated as "no schedule" (nil, nil) — a show is
// not required to have a structured schedule. Use this anywhere the stored
// schedule is read so the validated shape is the only one callers ever see.
func ParseRadioSchedule(raw *json.RawMessage) (*RadioSchedule, error) {
	if raw == nil || len(*raw) == 0 || string(*raw) == "null" {
		return nil, nil
	}
	var sched RadioSchedule
	if err := json.Unmarshal(*raw, &sched); err != nil {
		return nil, fmt.Errorf("radio schedule: invalid JSON: %w", err)
	}
	if err := sched.Validate(); err != nil {
		return nil, err
	}
	return &sched, nil
}

// RadioStation represents a radio station entity in the knowledge graph
type RadioStation struct {
	ID                  uint             `gorm:"primaryKey"`
	Name                string           `gorm:"not null"`
	Slug                string           `gorm:"not null;uniqueIndex"`
	Description         *string          `gorm:"column:description"`
	City                *string          `gorm:"column:city"`
	State               *string          `gorm:"column:state"`
	Country             *string          `gorm:"column:country;default:'US'"`
	Timezone            *string          `gorm:"column:timezone"`
	StreamURL           *string          `gorm:"column:stream_url"`
	StreamURLs          *json.RawMessage `gorm:"column:stream_urls;type:jsonb;default:'{}'"`
	Website             *string          `gorm:"column:website"`
	DonationURL         *string          `gorm:"column:donation_url"`
	DonationEmbedURL    *string          `gorm:"column:donation_embed_url"`
	LogoURL             *string          `gorm:"column:logo_url"`
	Social              *json.RawMessage `gorm:"column:social;type:jsonb;default:'{}'"`
	BroadcastType       string           `gorm:"column:broadcast_type;not null;default:'both'"`
	FrequencyMHz        *float64         `gorm:"column:frequency_mhz;type:decimal(5,1)"`
	PlaylistSource      *string          `gorm:"column:playlist_source"`
	PlaylistConfig      *json.RawMessage `gorm:"column:playlist_config;type:jsonb"`
	LastPlaylistFetchAt *time.Time       `gorm:"column:last_playlist_fetch_at"`
	// IsActive is retained for backward compatibility with existing read paths
	// (idx_radio_shows_active, GORM model default). LifecycleState is the new
	// operational signal (PSY-1131); reads should migrate to it over the P2/P4
	// pipeline rebuild.
	IsActive       bool      `gorm:"column:is_active;not null;default:true"`
	Source         string    `gorm:"column:source;not null;default:canonical"`
	LifecycleState string    `gorm:"column:lifecycle_state;not null;default:active"`
	NetworkID      *uint     `gorm:"column:network_id"`
	IsFlagship     bool      `gorm:"column:is_flagship;not null;default:false"`
	CreatedAt      time.Time `gorm:"not null"`
	UpdatedAt      time.Time `gorm:"not null"`

	// Relationships
	Shows   []RadioShow   `gorm:"foreignKey:StationID"`
	Network *RadioNetwork `gorm:"foreignKey:NetworkID"`
}

// TableName specifies the table name for RadioStation
func (RadioStation) TableName() string {
	return "radio_stations"
}

// RadioNetwork represents a parent brand grouping sibling radio_stations.
// Example: WFMU's 91.1 broadcast plus three stream-only sub-channels are
// all siblings under the WFMU network. Networks are flat (no hierarchy);
// stations link to networks via radio_stations.network_id.
type RadioNetwork struct {
	ID        uint      `gorm:"primaryKey"`
	Slug      string    `gorm:"not null;uniqueIndex"`
	Name      string    `gorm:"not null"`
	CreatedAt time.Time `gorm:"not null"`
	UpdatedAt time.Time `gorm:"not null"`

	// Relationships
	Stations []RadioStation `gorm:"foreignKey:NetworkID"`
}

// TableName specifies the table name for RadioNetwork
func (RadioNetwork) TableName() string {
	return "radio_networks"
}

// RadioShow represents a recurring radio program on a station
type RadioShow struct {
	ID              uint             `gorm:"primaryKey"`
	StationID       uint             `gorm:"column:station_id;not null"`
	Name            string           `gorm:"not null"`
	Slug            string           `gorm:"not null;uniqueIndex"`
	HostName        *string          `gorm:"column:host_name"`
	Description     *string          `gorm:"column:description"`
	ScheduleDisplay *string          `gorm:"column:schedule_display"`
	Schedule        *json.RawMessage `gorm:"column:schedule;type:jsonb"`
	GenreTags       *json.RawMessage `gorm:"column:genre_tags;type:jsonb;default:'[]'"`
	ArchiveURL      *string          `gorm:"column:archive_url"`
	ImageURL        *string          `gorm:"column:image_url"`
	ExternalID      *string          `gorm:"column:external_id"`
	// IsActive retained for backward compatibility; LifecycleState is the new
	// operational signal (PSY-1131).
	IsActive       bool      `gorm:"column:is_active;not null;default:true"`
	Source         string    `gorm:"column:source;not null;default:provider"`
	LifecycleState string    `gorm:"column:lifecycle_state;not null;default:active"`
	CreatedAt      time.Time `gorm:"not null"`
	UpdatedAt      time.Time `gorm:"not null"`

	// Relationships
	Station  RadioStation   `gorm:"foreignKey:StationID"`
	Episodes []RadioEpisode `gorm:"foreignKey:ShowID"`
}

// TableName specifies the table name for RadioShow
func (RadioShow) TableName() string {
	return "radio_shows"
}

// RadioEpisode represents a single broadcast of a radio show
type RadioEpisode struct {
	ID              uint             `gorm:"primaryKey"`
	ShowID          uint             `gorm:"column:show_id;not null"`
	Title           *string          `gorm:"column:title"`
	AirDate         string           `gorm:"column:air_date;type:date;not null"`
	AirTime         *string          `gorm:"column:air_time;type:time"`
	DurationMinutes *int             `gorm:"column:duration_minutes"`
	Description     *string          `gorm:"column:description"`
	ArchiveURL      *string          `gorm:"column:archive_url"`
	MixcloudURL     *string          `gorm:"column:mixcloud_url"`
	ExternalID      *string          `gorm:"column:external_id"`
	GenreTags       *json.RawMessage `gorm:"column:genre_tags;type:jsonb"`
	MoodTags        *json.RawMessage `gorm:"column:mood_tags;type:jsonb"`
	PlayCount       int              `gorm:"column:play_count;not null;default:0"`
	// Status makes "live" an explicit stored fact (PSY-1131); windowless
	// episodes default to 'aired' and are never 'live'.
	Status string `gorm:"column:status;not null;default:aired"`
	// StartsAt/EndsAt are the real air window (timezone-aware), NULL when the
	// provider supplies no time. The basis for the air-window "live" computation.
	StartsAt *time.Time `gorm:"column:starts_at"`
	EndsAt   *time.Time `gorm:"column:ends_at"`
	// PlaylistState/PlaylistFetchedAt track the playlist-fetch lifecycle,
	// decoupled from episode Status.
	PlaylistState     string     `gorm:"column:playlist_state;not null;default:pending"`
	PlaylistFetchedAt *time.Time `gorm:"column:playlist_fetched_at"`
	CreatedAt         time.Time  `gorm:"not null"`
	UpdatedAt         time.Time  `gorm:"column:updated_at;not null"`

	// Relationships
	Show  RadioShow   `gorm:"foreignKey:ShowID"`
	Plays []RadioPlay `gorm:"foreignKey:EpisodeID"`
}

// TableName specifies the table name for RadioEpisode
func (RadioEpisode) TableName() string {
	return "radio_episodes"
}

// RadioPlay represents a single track played in a radio episode
type RadioPlay struct {
	ID        uint `gorm:"primaryKey"`
	EpisodeID uint `gorm:"column:episode_id;not null"`
	Position  int  `gorm:"column:position;not null;default:0"`

	// Raw metadata from source (always stored, never overwritten)
	ArtistName  string  `gorm:"column:artist_name;not null"`
	TrackTitle  *string `gorm:"column:track_title"`
	AlbumTitle  *string `gorm:"column:album_title"`
	LabelName   *string `gorm:"column:label_name"`
	ReleaseYear *int    `gorm:"column:release_year"`

	// Curation signals
	IsNew             bool    `gorm:"column:is_new;not null;default:false"`
	RotationStatus    *string `gorm:"column:rotation_status"`
	DJComment         *string `gorm:"column:dj_comment"`
	IsLivePerformance bool    `gorm:"column:is_live_performance;not null;default:false"`
	IsRequest         bool    `gorm:"column:is_request;not null;default:false"`

	// MatchState is the explicit matching lifecycle (PSY-1131), replacing the
	// implicit "ArtistID IS NULL == unmatched". Defaults to 'unmatched'.
	MatchState string `gorm:"column:match_state;not null;default:unmatched"`
	// ProviderPlayID is a stable provider-supplied play id (e.g. KEXP) used as
	// the dedup key when present; NULL falls back to the content hash.
	ProviderPlayID *string `gorm:"column:provider_play_id"`
	// DedupKey is a GENERATED STORED column (provider_play_id, else a content
	// hash). Read-only from Go ("->" tag): Postgres computes it, GORM never
	// writes it. Backs the (episode_id, dedup_key) unique index.
	DedupKey string `gorm:"->;column:dedup_key"`

	// Linked to our knowledge graph (populated by matching engine, nullable)
	ArtistID  *uint `gorm:"column:artist_id"`
	ReleaseID *uint `gorm:"column:release_id"`
	LabelID   *uint `gorm:"column:label_id"`

	// External IDs for cross-referencing and deduplication
	MusicBrainzRecordingID *string `gorm:"column:musicbrainz_recording_id"`
	MusicBrainzArtistID    *string `gorm:"column:musicbrainz_artist_id"`
	MusicBrainzReleaseID   *string `gorm:"column:musicbrainz_release_id"`

	// Timing
	AirTimestamp *time.Time `gorm:"column:air_timestamp"`
	CreatedAt    time.Time  `gorm:"not null"`

	// Relationships
	Episode RadioEpisode `gorm:"foreignKey:EpisodeID"`
	Artist  *Artist      `gorm:"foreignKey:ArtistID"`
	Release *Release     `gorm:"foreignKey:ReleaseID"`
	Label   *Label       `gorm:"foreignKey:LabelID"`
}

// TableName specifies the table name for RadioPlay
func (RadioPlay) TableName() string {
	return "radio_plays"
}

// Import job status constants
const (
	RadioImportJobStatusPending   = "pending"
	RadioImportJobStatusRunning   = "running"
	RadioImportJobStatusCompleted = "completed"
	RadioImportJobStatusFailed    = "failed"
	RadioImportJobStatusCancelled = "cancelled"
)

// RadioImportJob represents an async import job for a radio show's episodes.
type RadioImportJob struct {
	ID                 uint         `gorm:"primaryKey" json:"id"`
	ShowID             uint         `gorm:"not null" json:"show_id"`
	Show               RadioShow    `gorm:"foreignKey:ShowID" json:"-"`
	StationID          uint         `gorm:"not null" json:"station_id"`
	Station            RadioStation `gorm:"foreignKey:StationID" json:"-"`
	Since              string       `gorm:"type:date;not null" json:"since"`
	Until              string       `gorm:"type:date;not null" json:"until"`
	Status             string       `gorm:"type:varchar(20);not null;default:pending" json:"status"`
	EpisodesFound      int          `gorm:"not null;default:0" json:"episodes_found"`
	EpisodesImported   int          `gorm:"not null;default:0" json:"episodes_imported"`
	PlaysImported      int          `gorm:"not null;default:0" json:"plays_imported"`
	PlaysMatched       int          `gorm:"not null;default:0" json:"plays_matched"`
	CurrentEpisodeDate *string      `json:"current_episode_date,omitempty"`
	ErrorLog           *string      `gorm:"type:text" json:"error_log,omitempty"`
	StartedAt          *time.Time   `json:"started_at,omitempty"`
	CompletedAt        *time.Time   `json:"completed_at,omitempty"`
	CreatedAt          time.Time    `json:"created_at"`
	UpdatedAt          time.Time    `json:"updated_at"`
}

// TableName specifies the table name for RadioImportJob
func (RadioImportJob) TableName() string { return "radio_import_jobs" }

// RadioArtistAffinity represents co-occurrence of two artists across radio playlists.
// The composite primary key is (artist_a_id, artist_b_id).
// A CHECK constraint ensures artist_a_id < artist_b_id (canonical ordering).
type RadioArtistAffinity struct {
	ArtistAID         uint      `gorm:"column:artist_a_id;primaryKey"`
	ArtistBID         uint      `gorm:"column:artist_b_id;primaryKey"`
	CoOccurrenceCount int       `gorm:"column:co_occurrence_count;not null;default:0"`
	ShowCount         int       `gorm:"column:show_count;not null;default:0"`
	StationCount      int       `gorm:"column:station_count;not null;default:0"`
	LastCoOccurrence  *string   `gorm:"column:last_co_occurrence;type:date"`
	UpdatedAt         time.Time `gorm:"not null"`

	// Relationships
	ArtistA Artist `gorm:"foreignKey:ArtistAID"`
	ArtistB Artist `gorm:"foreignKey:ArtistBID"`
}

// TableName specifies the table name for RadioArtistAffinity
func (RadioArtistAffinity) TableName() string {
	return "radio_artist_affinity"
}
