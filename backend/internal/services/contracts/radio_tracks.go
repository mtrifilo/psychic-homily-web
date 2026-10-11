package contracts

import "time"

// RadioTrackSort names the column a top-tracks listing ranks by, descending.
type RadioTrackSort string

const (
	// RadioTrackSortPlays ranks by play_count.
	RadioTrackSortPlays RadioTrackSort = "plays"
	// RadioTrackSortStationRows ranks by station_row_count.
	RadioTrackSortStationRows RadioTrackSort = "station_rows"
	// RadioTrackSortStationFamilies ranks by station_family_count.
	RadioTrackSortStationFamilies RadioTrackSort = "station_families"
)

// RadioTrackSummary is one radio_tracks row as the internal listing returns it.
type RadioTrackSummary struct {
	ID                    uint      `json:"id"`
	ArtistKey             string    `json:"artist_key" doc:"Normalized artist half of the track key"`
	TitleKey              string    `json:"title_key" doc:"Normalized, suffix-stripped title half of the track key"`
	DisplayArtist         string    `json:"display_artist" doc:"Most common raw artist spelling among the track's plays"`
	DisplayTitle          string    `json:"display_title" doc:"Most common raw title spelling among the track's plays"`
	PlayCount             int       `json:"play_count"`
	StationRowCount       int       `json:"station_row_count" doc:"Distinct radio_stations rows (each WFMU stream counts)"`
	StationFamilyCount    int       `json:"station_family_count" doc:"Distinct broadcasters (a network counts once)"`
	ShowCount             int       `json:"show_count"`
	FirstAiredOn          string    `json:"first_aired_on" doc:"Earliest episode air date (YYYY-MM-DD)"`
	LastAiredOn           string    `json:"last_aired_on" doc:"Latest episode air date (YYYY-MM-DD)"`
	MBRecordingID         *string   `json:"mb_recording_id,omitempty" doc:"Set when every MusicBrainz-tagged play carries the same recording id"`
	MBRecordingIDConflict bool      `json:"mb_recording_id_conflict" doc:"True when the track's plays carry more than one recording id"`
	KeyCount              int       `json:"key_count" doc:"Track keys mapped to this track (more than one when a MusicBrainz recording id joined keys)"`
	AlbumPlayCount        int       `json:"album_play_count"`
	LabelPlayCount        int       `json:"label_play_count"`
	RebuiltAt             time.Time `json:"rebuilt_at" doc:"The rebuild that last changed this row"`
}

// RadioTrackListing is a top-tracks listing plus the table-wide counts a
// reader needs to interpret it.
type RadioTrackListing struct {
	Tracks       []RadioTrackSummary `json:"tracks"`
	TotalTracks  int64               `json:"total_tracks" doc:"Keyed tracks in radio_tracks (the unkeyed marker row excluded)"`
	UnkeyedPlays int64               `json:"unkeyed_plays" doc:"Plays whose artist or title normalizes to empty"`
}
