package models

import (
	"encoding/json"
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
	PlaylistSourceKEXP      = "kexp_api"
	PlaylistSourceNTS       = "nts_api"
	PlaylistSourceWFMU      = "wfmu_scrape"
	PlaylistSourceManual    = "manual"
)

// Rotation status constants for radio plays (KEXP)
const (
	RotationStatusHeavy          = "heavy"
	RotationStatusMedium         = "medium"
	RotationStatusLight          = "light"
	RotationStatusRecommendedNew = "recommended_new"
	RotationStatusLibrary        = "library"
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
	IsActive            bool             `gorm:"column:is_active;not null;default:true"`
	CreatedAt           time.Time        `gorm:"not null"`
	UpdatedAt           time.Time        `gorm:"not null"`

	// Relationships
	Shows []RadioShow `gorm:"foreignKey:StationID"`
}

// TableName specifies the table name for RadioStation
func (RadioStation) TableName() string {
	return "radio_stations"
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
	IsActive        bool             `gorm:"column:is_active;not null;default:true"`
	CreatedAt       time.Time        `gorm:"not null"`
	UpdatedAt       time.Time        `gorm:"not null"`

	// Relationships
	Station  RadioStation  `gorm:"foreignKey:StationID"`
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
	CreatedAt       time.Time        `gorm:"not null"`

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
	ID       uint `gorm:"primaryKey"`
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

// RadioArtistAffinity represents co-occurrence of two artists across radio playlists.
// The composite primary key is (artist_a_id, artist_b_id).
// A CHECK constraint ensures artist_a_id < artist_b_id (canonical ordering).
type RadioArtistAffinity struct {
	ArtistAID         uint       `gorm:"column:artist_a_id;primaryKey"`
	ArtistBID         uint       `gorm:"column:artist_b_id;primaryKey"`
	CoOccurrenceCount int        `gorm:"column:co_occurrence_count;not null;default:0"`
	ShowCount         int        `gorm:"column:show_count;not null;default:0"`
	StationCount      int        `gorm:"column:station_count;not null;default:0"`
	LastCoOccurrence  *string    `gorm:"column:last_co_occurrence;type:date"`
	UpdatedAt         time.Time  `gorm:"not null"`

	// Relationships
	ArtistA Artist `gorm:"foreignKey:ArtistAID"`
	ArtistB Artist `gorm:"foreignKey:ArtistBID"`
}

// TableName specifies the table name for RadioArtistAffinity
func (RadioArtistAffinity) TableName() string {
	return "radio_artist_affinity"
}
