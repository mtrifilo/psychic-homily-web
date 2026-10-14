package contracts

import "time"

// RadioCandidateLane names how an enrichment candidate may enter the catalog.
type RadioCandidateLane string

const (
	// RadioCandidateLaneAuto marks a candidate eligible for automatic
	// creation: its plays carry exactly one MusicBrainz id and it clears the
	// station floor.
	RadioCandidateLaneAuto RadioCandidateLane = "auto"
	// RadioCandidateLaneHuman marks a candidate that a person verifies before
	// anything is created.
	RadioCandidateLaneHuman RadioCandidateLane = "human"
)

// RadioCandidateLaneFilter selects which lanes a candidate listing returns.
type RadioCandidateLaneFilter string

const (
	RadioCandidateLaneFilterAll   RadioCandidateLaneFilter = "all"
	RadioCandidateLaneFilterAuto  RadioCandidateLaneFilter = "auto"
	RadioCandidateLaneFilterHuman RadioCandidateLaneFilter = "human"
)

// RadioArtistCandidateSort names the order of an artist candidate listing.
// Every order is descending and breaks ties by score, then by the most recent
// play, then by artist key.
type RadioArtistCandidateSort string

const (
	// RadioArtistCandidateSortScore ranks by score.
	RadioArtistCandidateSortScore RadioArtistCandidateSort = "score"
	// RadioArtistCandidateSortPlays ranks by plays in the window.
	RadioArtistCandidateSortPlays RadioArtistCandidateSort = "plays"
	// RadioArtistCandidateSortStations ranks by distinct station rows.
	RadioArtistCandidateSortStations RadioArtistCandidateSort = "stations"
)

// RadioArtistCandidatesQuery selects an artist candidate listing.
type RadioArtistCandidatesQuery struct {
	Lane  RadioCandidateLaneFilter
	Sort  RadioArtistCandidateSort
	Limit int
}

// RadioCandidateStation is one station row a candidate's plays aired on.
type RadioCandidateStation struct {
	StationID uint   `json:"station_id"`
	Slug      string `json:"slug"`
	Name      string `json:"name"`
	PlayCount int    `json:"play_count"`
}

// RadioCandidateTrack is one radio_tracks row among a candidate's plays.
type RadioCandidateTrack struct {
	TrackID   uint   `json:"track_id"`
	Title     string `json:"title" doc:"The track's most common raw title spelling"`
	PlayCount int    `json:"play_count" doc:"The candidate's counted plays of this track"`
}

// RadioCandidateName is one album or label name seen on a candidate's plays.
type RadioCandidateName struct {
	Name      string `json:"name" doc:"Most common raw spelling among the plays that carry it"`
	PlayCount int    `json:"play_count"`
}

// RadioArtistCandidate is one unmatched artist name with the evidence for
// adding it to the catalog. Every count covers the candidate's counted plays:
// plays in the window, not excluded for the artist ranking, not ambiguous.
type RadioArtistCandidate struct {
	ArtistKey          string                  `json:"artist_key" doc:"radio_normalize_name of the artist name; the candidate's identity"`
	DisplayName        string                  `json:"display_name" doc:"Most common raw artist spelling among the counted plays"`
	Lane               RadioCandidateLane      `json:"lane" enum:"auto,human"`
	Score              float64                 `json:"score" doc:"station rows x ln(plays) x (1 + shows / 10)"`
	PlayCount          int                     `json:"play_count"`
	StationRowCount    int                     `json:"station_row_count" doc:"Distinct radio_stations rows (each WFMU stream counts)"`
	StationFamilyCount int                     `json:"station_family_count" doc:"Distinct broadcasters (a network counts once)"`
	ShowCount          int                     `json:"show_count"`
	FirstAiredOn       string                  `json:"first_aired_on" doc:"Earliest counted play's episode air date (YYYY-MM-DD)"`
	LastAiredOn        string                  `json:"last_aired_on" doc:"Latest counted play's episode air date (YYYY-MM-DD)"`
	Stations           []RadioCandidateStation `json:"stations" doc:"Station rows by play count, descending"`
	TopTracks          []RadioCandidateTrack   `json:"top_tracks" doc:"Up to three tracks by play count, over the plays that also count toward the track ranking and whose key radio_track_keys maps"`
	AlbumNames         []RadioCandidateName    `json:"album_names" doc:"Up to five album names by play count"`
	AlbumNameCount     int                     `json:"album_name_count" doc:"Distinct normalized album names"`
	LabelNames         []RadioCandidateName    `json:"label_names" doc:"Up to five label names by play count, over the plays that also count toward the label ranking"`
	LabelNameCount     int                     `json:"label_name_count" doc:"Distinct normalized label names"`
	MBArtistID         *string                 `json:"mb_artist_id,omitempty" doc:"Set when the counted plays carry exactly one MusicBrainz artist id"`
	MBArtistIDs        []string                `json:"mb_artist_ids" doc:"Every distinct MusicBrainz artist id the counted plays carry, sorted"`
	MBArtistIDConflict bool                    `json:"mb_artist_id_conflict" doc:"True when the counted plays carry more than one MusicBrainz artist id"`
}

// RadioArtistCandidateListing is one snapshot of the artist candidate ranking.
type RadioArtistCandidateListing struct {
	Candidates []RadioArtistCandidate `json:"candidates"`
	// Total counts the candidates in the requested lanes; AutoTotal and
	// HumanTotal count each lane whatever the filter.
	Total          int    `json:"total" doc:"Candidates in the requested lanes"`
	AutoTotal      int    `json:"auto_total"`
	HumanTotal     int    `json:"human_total"`
	AsOf           string `json:"as_of" doc:"The UTC date the window ends on (YYYY-MM-DD)"`
	WindowStart    string `json:"window_start" doc:"The earliest episode air date counted (YYYY-MM-DD)"`
	WindowDays     int    `json:"window_days"`
	MinStationRows int    `json:"min_station_rows"`
	MinPlays       int    `json:"min_plays"`
	// TracksRebuiltAt is nil until the radio_tracks rebuild loop has succeeded
	// once; top_tracks reflect that rebuild.
	TracksRebuiltAt *time.Time `json:"tracks_rebuilt_at,omitempty" doc:"When the radio_tracks rebuild loop last completed successfully"`
}
