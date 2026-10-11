package catalog

import "time"

// RadioTrack is one row of radio_tracks: a track as broadcast, derived from
// radio_plays by catalog.RebuildRadioTracks. Every column is derived; nothing
// writes this table except the rebuild. The row whose ArtistKey and TitleKey
// are both "" (Unkeyed) aggregates every play whose artist key or title key is
// empty or NULL; it exists only while at least one such play does.
type RadioTrack struct {
	ID                    uint      `gorm:"primaryKey"`
	ArtistKey             string    `gorm:"column:artist_key;not null"`
	TitleKey              string    `gorm:"column:title_key;not null"`
	Unkeyed               bool      `gorm:"->;column:unkeyed"`
	DisplayArtist         string    `gorm:"column:display_artist;not null"`
	DisplayTitle          string    `gorm:"column:display_title;not null"`
	PlayCount             int       `gorm:"column:play_count;not null"`
	StationRowCount       int       `gorm:"column:station_row_count;not null"`
	StationFamilyCount    int       `gorm:"column:station_family_count;not null"`
	ShowCount             int       `gorm:"column:show_count;not null"`
	FirstAiredOn          time.Time `gorm:"column:first_aired_on;type:date;not null"`
	LastAiredOn           time.Time `gorm:"column:last_aired_on;type:date;not null"`
	MBRecordingID         *string   `gorm:"column:mb_recording_id"`
	MBRecordingIDConflict bool      `gorm:"column:mb_recording_id_conflict;not null"`
	AlbumPlayCount        int       `gorm:"column:album_play_count;not null"`
	LabelPlayCount        int       `gorm:"column:label_play_count;not null"`
	RebuiltAt             time.Time `gorm:"column:rebuilt_at;not null"`
}

// TableName specifies the table name for RadioTrack.
func (RadioTrack) TableName() string {
	return "radio_tracks"
}

// RadioTrackKey is one row of radio_track_keys: a track key seen in
// radio_plays and the track it belongs to.
type RadioTrackKey struct {
	ArtistKey string `gorm:"column:artist_key;primaryKey"`
	TitleKey  string `gorm:"column:title_key;primaryKey"`
	TrackID   uint   `gorm:"column:track_id;not null"`
}

// TableName specifies the table name for RadioTrackKey.
func (RadioTrackKey) TableName() string {
	return "radio_track_keys"
}
