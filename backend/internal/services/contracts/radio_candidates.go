package contracts

import "time"

// RadioCandidateLane names how an enrichment candidate may enter the catalog.
type RadioCandidateLane string

const (
	// RadioCandidateLaneAuto marks a candidate eligible for automatic
	// creation: its plays carry exactly one MusicBrainz id for the entity it
	// would create, it clears the station floor, and none of the human-lane
	// conditions catalog.RadioCandidateLaneFor states holds.
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

// Valid reports whether f is one of the declared filters.
func (f RadioCandidateLaneFilter) Valid() bool {
	switch f {
	case RadioCandidateLaneFilterAll, RadioCandidateLaneFilterAuto, RadioCandidateLaneFilterHuman:
		return true
	}
	return false
}

// Includes reports whether a candidate in the given lane passes the filter.
func (f RadioCandidateLaneFilter) Includes(lane RadioCandidateLane) bool {
	return f == RadioCandidateLaneFilterAll || string(f) == string(lane)
}

// RadioCandidateSort names the order of a candidate listing. Every order is
// descending and breaks ties by score, then by the most recent play, then by
// the candidate's identity (artist key or track id).
type RadioCandidateSort string

const (
	// RadioCandidateSortScore ranks by score.
	RadioCandidateSortScore RadioCandidateSort = "score"
	// RadioCandidateSortPlays ranks by plays in the window.
	RadioCandidateSortPlays RadioCandidateSort = "plays"
	// RadioCandidateSortStations ranks by distinct station rows.
	RadioCandidateSortStations RadioCandidateSort = "stations"
)

// RadioTrackCandidateView names which tracks a track candidate listing holds.
type RadioTrackCandidateView string

const (
	// RadioTrackCandidateViewUnlinked holds the tracks of catalog artists that
	// have no release link: a counted play is linked to an artist, and no play
	// of the track, at any time, is linked to a release credited to that
	// play's linked artist.
	RadioTrackCandidateViewUnlinked RadioTrackCandidateView = "unlinked"
	// RadioTrackCandidateViewUnmatched holds the tracks of artists the
	// catalog does not have: no counted play is linked to an artist, and at
	// least one artist key among the counted plays is an artist candidate's
	// key.
	RadioTrackCandidateViewUnmatched RadioTrackCandidateView = "unmatched"
)

// Valid reports whether v is one of the declared views.
func (v RadioTrackCandidateView) Valid() bool {
	return v == RadioTrackCandidateViewUnlinked || v == RadioTrackCandidateViewUnmatched
}

// RadioArtistCandidatesQuery selects an artist candidate listing.
type RadioArtistCandidatesQuery struct {
	Lane  RadioCandidateLaneFilter
	Sort  RadioCandidateSort
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

// RadioCandidateName is a name seen on a candidate's plays with its play
// count; the list that holds it says how names are grouped.
type RadioCandidateName struct {
	Name      string `json:"name"`
	PlayCount int    `json:"play_count"`
}

// RadioArtistCandidate is one unmatched artist name with the evidence for
// adding it to the catalog. Every count covers the candidate's counted plays:
// plays in the window, not excluded for the artist ranking, not ambiguous.
type RadioArtistCandidate struct {
	ArtistKey           string                  `json:"artist_key" doc:"radio_normalize_name of the artist name; the candidate's identity"`
	DisplayName         string                  `json:"display_name" doc:"Most common raw artist spelling among the counted plays"`
	ArtistSpellings     []RadioCandidateName    `json:"artist_spellings" doc:"Up to five exact raw artist spellings (case and spaces kept), each with the plays carrying exactly that spelling; the first is display_name"`
	ArtistSpellingCount int                     `json:"artist_spelling_count" doc:"Distinct raw artist spellings"`
	Lane                RadioCandidateLane      `json:"lane" enum:"auto,human"`
	Score               float64                 `json:"score" doc:"station rows x ln(plays) x (1 + shows / 10)"`
	PlayCount           int                     `json:"play_count"`
	StationRowCount     int                     `json:"station_row_count" doc:"Distinct radio_stations rows (each WFMU stream counts)"`
	StationFamilyCount  int                     `json:"station_family_count" doc:"Distinct broadcasters (a network counts once)"`
	ShowCount           int                     `json:"show_count"`
	FirstAiredOn        string                  `json:"first_aired_on" doc:"Earliest counted play's episode air date (YYYY-MM-DD)"`
	LastAiredOn         string                  `json:"last_aired_on" doc:"Latest counted play's episode air date (YYYY-MM-DD)"`
	Stations            []RadioCandidateStation `json:"stations" doc:"Station rows by play count, descending"`
	TopTracks           []RadioCandidateTrack   `json:"top_tracks" doc:"Up to three tracks by play count, over the plays that also count toward the track ranking and whose key radio_track_keys maps"`
	AlbumNames          []RadioCandidateName    `json:"album_names" doc:"Up to five album names by play count; names that normalize alike are one entry, shown in their most common trimmed spelling, counting all their plays"`
	AlbumNameCount      int                     `json:"album_name_count" doc:"Distinct normalized album names"`
	LabelNames          []RadioCandidateName    `json:"label_names" doc:"Up to five label names by play count, over the plays that also count toward the label ranking; grouped and shown like album_names"`
	LabelNameCount      int                     `json:"label_name_count" doc:"Distinct normalized label names"`
	MBArtistID          *string                 `json:"mb_artist_id,omitempty" doc:"Set when the counted plays carry exactly one MusicBrainz artist id"`
	MBArtistIDs         []string                `json:"mb_artist_ids" doc:"Every distinct MusicBrainz artist id the counted plays carry, sorted"`
	MBArtistIDConflict  bool                    `json:"mb_artist_id_conflict" doc:"True when the counted plays carry more than one MusicBrainz artist id"`
	NonLatinSpelling    bool                    `json:"non_latin_spelling" doc:"True when a counted play's artist name carries a character normalization may not fold to a-z, 0-9 or punctuation (usually a non-Latin script, also some symbols such as a registered sign); the key may then be a fragment of that name, and the candidate is in the human lane"`
	SharedMBArtistKeys  []string                `json:"shared_mb_artist_keys" doc:"Other candidates' artist keys whose plays carry one of this candidate's MusicBrainz artist ids, sorted; a candidate that has any is in the human lane"`
}

// RadioTrackCandidatesQuery selects a track candidate listing.
type RadioTrackCandidatesQuery struct {
	View  RadioTrackCandidateView
	Lane  RadioCandidateLaneFilter
	Sort  RadioCandidateSort
	Limit int
}

// RadioCandidateArtist is one catalog artist a candidate's plays are linked to.
type RadioCandidateArtist struct {
	ArtistID  uint   `json:"artist_id"`
	Name      string `json:"name"`
	Slug      string `json:"slug"`
	PlayCount int    `json:"play_count" doc:"The candidate's counted plays linked to this artist"`
}

// RadioTrackCandidate is one radio_tracks row with the evidence for enriching
// it. Every count covers the track's counted plays: plays in the window, not
// excluded for the track ranking, not ambiguous, whose track key
// radio_track_keys maps to this track.
type RadioTrackCandidate struct {
	TrackID             uint                    `json:"track_id" doc:"The radio_tracks row; the candidate's identity"`
	DisplayTitle        string                  `json:"display_title" doc:"Most common raw title spelling among the counted plays"`
	TitleSpellings      []RadioCandidateName    `json:"title_spellings" doc:"Up to five exact raw title spellings, each with the plays carrying exactly that spelling; the first is display_title"`
	TitleSpellingCount  int                     `json:"title_spelling_count" doc:"Distinct raw title spellings"`
	DisplayArtist       string                  `json:"display_artist" doc:"Most common raw artist spelling among the counted plays"`
	ArtistKeys          []RadioCandidateName    `json:"artist_keys" doc:"Every distinct artist key (radio_normalize_name of the artist name) among the counted plays, by play count"`
	ArtistCandidateKeys []string                `json:"artist_candidate_keys" doc:"The artist_keys that are qualifying artist candidates' artist_key, in artist_keys order (by play count); in the unmatched view at least one, in the unlinked view always empty (not computed)"`
	Artists             []RadioCandidateArtist  `json:"artists" doc:"Catalog artists the counted plays are linked to, by play count; empty in the unmatched view"`
	Lane                RadioCandidateLane      `json:"lane" enum:"auto,human" doc:"auto when the counted plays carry exactly one MusicBrainz release id, it is not on a variant title (variant_release_id), no spelling is non-Latin (non_latin_spelling) and the station floor is met; human otherwise. It describes the release only: an unmatched track can be auto while the artist candidate it joins is human"`
	Score               float64                 `json:"score" doc:"station rows x ln(plays)"`
	PlayCount           int                     `json:"play_count"`
	StationRowCount     int                     `json:"station_row_count" doc:"Distinct radio_stations rows (each WFMU stream counts)"`
	StationFamilyCount  int                     `json:"station_family_count" doc:"Distinct broadcasters (a network counts once)"`
	ShowCount           int                     `json:"show_count"`
	FirstAiredOn        string                  `json:"first_aired_on" doc:"Earliest counted play's episode air date (YYYY-MM-DD)"`
	LastAiredOn         string                  `json:"last_aired_on" doc:"Latest counted play's episode air date (YYYY-MM-DD)"`
	Stations            []RadioCandidateStation `json:"stations" doc:"Station rows by play count, descending"`
	AlbumNames          []RadioCandidateName    `json:"album_names" doc:"Up to five album names by play count; names that normalize alike are one entry, shown in their most common trimmed spelling, counting all their plays"`
	AlbumNameCount      int                     `json:"album_name_count" doc:"Distinct normalized album names"`
	MBReleaseID         *string                 `json:"mb_release_id,omitempty" doc:"Set when the counted plays carry exactly one MusicBrainz release id"`
	MBReleaseIDs        []string                `json:"mb_release_ids" doc:"Every distinct MusicBrainz release id the counted plays carry, lower-cased and sorted"`
	NonLatinSpelling    bool                    `json:"non_latin_spelling" doc:"True when a counted play's artist name or title carries a character normalization may not fold to a-z, 0-9 or punctuation (usually a non-Latin script); a key half may then be a fragment of that name, and the candidate is in the human lane"`
	VariantReleaseID    bool                    `json:"variant_release_id" doc:"True when every counted play carrying a MusicBrainz release id has a title radio_track_title_key strips (a featured credit, a trailing bracket group, or a dash tail naming live, remix, edit, version, mono or stereo), so the id may be that version's release; the flag follows what the key function strips, which can include a title that is not a version. The candidate is in the human lane"`
}

// RadioTrackCandidateListing is one snapshot of a track candidate view.
type RadioTrackCandidateListing struct {
	View             RadioTrackCandidateView `json:"view" enum:"unlinked,unmatched"`
	Candidates       []RadioTrackCandidate   `json:"candidates"`
	Total            int                     `json:"total" doc:"Candidates in the view and the requested lanes"`
	AutoTotal        int                     `json:"auto_total" doc:"Candidates in the view and the auto lane, whatever the lane filter"`
	HumanTotal       int                     `json:"human_total" doc:"Candidates in the view and the human lane, whatever the lane filter"`
	QualifyingTracks int                     `json:"qualifying_tracks" doc:"Tracks that clear the floors, in either view or in neither (a catalog artist's track with a credited release link, or a track with no artist-linked counted play none of whose artist keys is an artist candidate)"`
	UntrackedPlays   int64                   `json:"untracked_plays" doc:"Plays in the window, not excluded and not ambiguous, that count toward no track: a key half normalizes to empty, or radio_track_keys has no row for the key yet (the play arrived after the last rebuild)"`
	AsOf             string                  `json:"as_of" doc:"The UTC date the window ends on (YYYY-MM-DD)"`
	WindowStart      string                  `json:"window_start" doc:"The earliest episode air date counted (YYYY-MM-DD)"`
	WindowDays       int                     `json:"window_days"`
	MinStationRows   int                     `json:"min_station_rows"`
	MinPlays         int                     `json:"min_plays"`
	// TracksRebuiltAt is nil until the radio_tracks rebuild loop has succeeded
	// once; the tracks reflect that rebuild.
	TracksRebuiltAt *time.Time `json:"tracks_rebuilt_at,omitempty" doc:"When the radio_tracks rebuild loop last completed successfully"`
}

// RadioArtistCandidateListing is one snapshot of the artist candidate ranking.
type RadioArtistCandidateListing struct {
	Candidates     []RadioArtistCandidate `json:"candidates"`
	Total          int                    `json:"total" doc:"Candidates in the requested lanes"`
	AutoTotal      int                    `json:"auto_total" doc:"Candidates in the auto lane, whatever the lane filter"`
	HumanTotal     int                    `json:"human_total" doc:"Candidates in the human lane, whatever the lane filter"`
	UnkeyablePlays int64                  `json:"unkeyable_plays" doc:"Plays in the window, not excluded, whose artist name normalizes to empty (for example a name written only in a non-Latin script), and so count toward no candidate"`
	AsOf           string                 `json:"as_of" doc:"The UTC date the window ends on (YYYY-MM-DD)"`
	WindowStart    string                 `json:"window_start" doc:"The earliest episode air date counted (YYYY-MM-DD)"`
	WindowDays     int                    `json:"window_days"`
	MinStationRows int                    `json:"min_station_rows"`
	MinPlays       int                    `json:"min_plays"`
	// TracksRebuiltAt is nil until the radio_tracks rebuild loop has succeeded
	// once; top_tracks reflect that rebuild.
	TracksRebuiltAt *time.Time `json:"tracks_rebuilt_at,omitempty" doc:"When the radio_tracks rebuild loop last completed successfully"`
}

// RadioLabelCandidateSort names the order of a label candidate listing. Every
// order is descending and breaks ties by score, then by the most recent play,
// then by label key.
type RadioLabelCandidateSort string

const (
	// RadioLabelCandidateSortScore ranks by score.
	RadioLabelCandidateSortScore RadioLabelCandidateSort = "score"
	// RadioLabelCandidateSortPlays ranks by plays in the window.
	RadioLabelCandidateSortPlays RadioLabelCandidateSort = "plays"
	// RadioLabelCandidateSortStations ranks by distinct station rows.
	RadioLabelCandidateSortStations RadioLabelCandidateSort = "stations"
	// RadioLabelCandidateSortVelocity ranks by velocity.
	RadioLabelCandidateSortVelocity RadioLabelCandidateSort = "velocity"
)

// RadioLabelCandidatesQuery selects a label candidate listing.
type RadioLabelCandidatesQuery struct {
	Lane  RadioCandidateLaneFilter
	Sort  RadioLabelCandidateSort
	Limit int
}

// RadioLabelCandidateArtist is one artist seen on a label candidate's plays.
type RadioLabelCandidateArtist struct {
	Name      string `json:"name"`
	PlayCount int    `json:"play_count"`
	ArtistIDs []uint `json:"artist_ids" doc:"Distinct catalog artist ids these plays are linked to, ascending; empty when none is"`
}

// RadioLabelCandidate is one unlinked label name with the evidence for adding
// it to the catalog. Every count covers the candidate's counted plays: plays
// in the window that are not excluded for the label ranking.
type RadioLabelCandidate struct {
	LabelKey           string                      `json:"label_key" doc:"radio_normalize_name of the label name; the candidate's identity"`
	DisplayName        string                      `json:"display_name" doc:"Most common trimmed label spelling among the counted plays"`
	LabelSpellings     []RadioCandidateName        `json:"label_spellings" doc:"Up to five trimmed label spellings (case kept), each with the plays carrying it; the first is display_name"`
	LabelSpellingCount int                         `json:"label_spelling_count" doc:"Distinct trimmed label spellings"`
	Lane               RadioCandidateLane          `json:"lane" enum:"auto,human" doc:"Plays carry no MusicBrainz label id, so every label candidate is in the human lane"`
	Score              float64                     `json:"score" doc:"station rows x ln(plays) x velocity"`
	Velocity           float64                     `json:"velocity" doc:"recent_play_count / prior_play_count, or recent_play_count when prior_play_count is zero"`
	PlayCount          int                         `json:"play_count" doc:"Plays in the window"`
	RecentPlayCount    int                         `json:"recent_play_count" doc:"Plays aired on or after the listing's recent_start"`
	PriorPlayCount     int                         `json:"prior_play_count" doc:"Plays aired on or after the listing's prior_start and before its recent_start"`
	StationRowCount    int                         `json:"station_row_count" doc:"Distinct radio_stations rows (each WFMU stream counts)"`
	StationFamilyCount int                         `json:"station_family_count" doc:"Distinct broadcasters (a network counts once)"`
	ShowCount          int                         `json:"show_count"`
	FirstAiredOn       string                      `json:"first_aired_on" doc:"Earliest counted play's episode air date (YYYY-MM-DD)"`
	LastAiredOn        string                      `json:"last_aired_on" doc:"Latest counted play's episode air date (YYYY-MM-DD)"`
	Stations           []RadioCandidateStation     `json:"stations" doc:"Station rows by play count, descending"`
	Artists            []RadioLabelCandidateArtist `json:"artists" doc:"Up to five artists by play count, over the plays not excluded for the artist ranking (placeholder artists such as Various Artists are skipped); names that normalize alike are one entry, shown in their most common trimmed spelling"`
	ArtistCount        int                         `json:"artist_count" doc:"Distinct normalized artist names, over the same plays as artists"`
	ReleaseNames       []RadioCandidateName        `json:"release_names" doc:"Up to five release (album) titles by play count; titles that normalize alike are one entry, shown in their most common trimmed spelling, counting all their plays"`
	ReleaseNameCount   int                         `json:"release_name_count" doc:"Distinct normalized release titles"`
	MBReleaseIDs       []string                    `json:"mb_release_ids" doc:"Up to five distinct MusicBrainz release ids by play count, lower case"`
	MBReleaseIDCount   int                         `json:"mb_release_id_count" doc:"Distinct MusicBrainz release ids"`
	NonLatinSpelling   bool                        `json:"non_latin_spelling" doc:"True when a counted play's label name carries a character normalization may not fold to a-z, 0-9 or punctuation; the key may then be a fragment of that name"`
}

// RadioLabelCandidateListing is one snapshot of the label candidate ranking.
type RadioLabelCandidateListing struct {
	Candidates     []RadioLabelCandidate `json:"candidates"`
	Total          int                   `json:"total" doc:"Candidates in the requested lanes"`
	AutoTotal      int                   `json:"auto_total" doc:"Candidates in the auto lane, whatever the lane filter"`
	HumanTotal     int                   `json:"human_total" doc:"Candidates in the human lane, whatever the lane filter"`
	UnkeyablePlays int64                 `json:"unkeyable_plays" doc:"Plays in the window, not excluded, whose label name normalizes to empty (for example a name written only in a non-Latin script), and so count toward no candidate"`
	AsOf           string                `json:"as_of" doc:"The UTC date the windows end on (YYYY-MM-DD)"`
	WindowStart    string                `json:"window_start" doc:"The earliest episode air date counted (YYYY-MM-DD)"`
	WindowDays     int                   `json:"window_days"`
	RecentStart    string                `json:"recent_start" doc:"The earliest air date of the recent velocity window (YYYY-MM-DD)"`
	PriorStart     string                `json:"prior_start" doc:"The earliest air date of the prior velocity window, which ends the day before recent_start (YYYY-MM-DD)"`
	VelocityDays   int                   `json:"velocity_days" doc:"Length of each velocity window in days"`
	MinStationRows int                   `json:"min_station_rows"`
	MinVelocity    float64               `json:"min_velocity" doc:"A candidate with prior plays needs recent_play_count at least this multiple of prior_play_count"`
	MinNewPlays    int                   `json:"min_new_plays" doc:"A candidate with no prior plays needs at least this many recent plays"`
}
