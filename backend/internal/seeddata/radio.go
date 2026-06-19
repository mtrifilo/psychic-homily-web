// Package seeddata is the canonical source of truth for reference data
// seeded into fresh development, staging, and E2E databases.
//
// Two consumers share this package:
//
//   - cmd/seed converts these structs to GORM models and inserts them via
//     the ORM. Use this in local dev and on stage after migrations.
//   - cmd/gen-e2e-seed renders the same data as idempotent SQL for
//     frontend/e2e/setup-db.sh to pipe through psql.
//
// The goal is one source of truth for seed data so the drift that caused
// PSY-385 (radio seed split across the migration + the Go seed CLI) cannot
// repeat. New radio stations and shows are added here, not in new SQL
// migrations. See docs/runbooks/migrations.md for the full rule.
package seeddata

// RadioNetwork is the canonical description of a network (a parent brand
// grouping sibling stations under a common identity, e.g. WFMU's flagship
// 91.1 broadcast plus its stream-only sub-channels). Networks are flat —
// no hierarchy, no parent station, no nesting. RadioStations reference
// a network by slug via NetworkSlug; the seed inserter resolves the slug
// to radio_networks.id at insert time.
type RadioNetwork struct {
	Slug string
	Name string
}

// RadioStation is the canonical description of a radio station to seed.
// Zero values carry meaning: empty-string optional fields and a zero
// FrequencyMHz map to SQL NULL (see RenderRadioSeedSQL and the mapping
// in cmd/seed/main.go).
type RadioStation struct {
	Name           string
	Slug           string
	Description    string
	City           string
	State          string // empty -> NULL (e.g. UK/London has no state)
	Country        string // ISO two-letter code
	Timezone       string // IANA zone name
	StreamURL      string
	Website        string
	DonationURL    string
	BroadcastType  string  // "terrestrial" | "internet" | "both"
	FrequencyMHz   float64 // 0 -> NULL (for internet-only stations)
	PlaylistSource string  // provider tag used by the fetcher pipeline
	NetworkSlug    string  // empty -> NULL (station has no parent network)
	IsFlagship     bool    // true -> primary station of NetworkSlug; only one flagship per network
}

// RadioShow is the canonical description of a flagship show on a radio
// station. StationSlug is the FK by slug rather than by id, so show
// records can be defined without knowing the station's autoincrement id.
type RadioShow struct {
	StationSlug     string // resolved to radio_stations.id at insert time
	Name            string
	Slug            string
	HostName        string // empty -> NULL (rotating residents, etc.)
	Description     string
	ScheduleDisplay string
	ArchiveURL      string
	ExternalID      string // provider-specific (KEXP numeric id, WFMU DJ code, NTS slug)
}

// RadioNetworks is the full seed set of radio networks. Currently the WFMU
// network groups the 91.1 broadcast plus three stream-only sub-channels
// (PSY-508). KEXP and NTS don't yet have networks — single-channel brands
// don't need one. Add a network row only when a station belongs to a brand
// with multiple sibling stations.
var RadioNetworks = []RadioNetwork{
	{Slug: "wfmu", Name: "WFMU"},
}

// RadioStations is the full seed set, matching what's currently in prod
// after migrations 000068 / 000076 / 000077 + the WFMU sub-stream
// migrations from PSY-508.
var RadioStations = []RadioStation{
	{
		Name:           "KEXP",
		Slug:           "kexp",
		Description:    "KEXP is a listener-supported, non-commercial radio station in Seattle, Washington, known for championing independent and emerging artists across all genres.",
		City:           "Seattle",
		State:          "WA",
		Country:        "US",
		Timezone:       "America/Los_Angeles",
		StreamURL:      "https://kexp.streamguys1.com/kexp160.aac",
		Website:        "https://www.kexp.org",
		DonationURL:    "https://www.kexp.org/donate/",
		BroadcastType:  "both",
		FrequencyMHz:   90.3,
		PlaylistSource: "kexp_api",
	},
	{
		Name:           "WFMU",
		Slug:           "wfmu",
		Description:    "WFMU is the longest-running freeform radio station in the United States, broadcasting from Jersey City, New Jersey. Known for its eclectic, unconstrained programming.",
		City:           "Jersey City",
		State:          "NJ",
		Country:        "US",
		Timezone:       "America/New_York",
		StreamURL:      "https://stream0.wfmu.org/freeform-128k",
		Website:        "https://wfmu.org",
		DonationURL:    "https://wfmu.org/marathon/",
		BroadcastType:  "both",
		FrequencyMHz:   91.1,
		PlaylistSource: "wfmu_scrape",
		NetworkSlug:    "wfmu",
		IsFlagship:     true,
	},
	{
		// PSY-508: WFMU stream-only 24/7 sub-channel curated by Doug
		// Schulkind. Distinct from his weekly 91.1 show "Give The Drummer
		// Some". No discrete shows — the stream is the program.
		Name:           "Give the Drummer Radio",
		Slug:           "wfmu-drummer",
		Description:    "WFMU stream-only 24/7 channel curated by Doug Schulkind. Eclectic blends of soul, jazz, gospel, country, and global grooves.",
		City:           "Jersey City",
		State:          "NJ",
		Country:        "US",
		Timezone:       "America/New_York",
		StreamURL:      "https://wfmu.org/drummer",
		Website:        "https://wfmu.org/drummer",
		DonationURL:    "",
		BroadcastType:  "internet",
		FrequencyMHz:   0,
		PlaylistSource: "wfmu_scrape",
		NetworkSlug:    "wfmu",
	},
	{
		// PSY-508: WFMU stream-only 24/7 sub-channel.
		Name:           "Rock'n'Soul Radio",
		Slug:           "wfmu-rocknsoulradio",
		Description:    "WFMU stream-only 24/7 channel programming rock and soul.",
		City:           "Jersey City",
		State:          "NJ",
		Country:        "US",
		Timezone:       "America/New_York",
		StreamURL:      "https://wfmu.org/rocknsoulradio",
		Website:        "https://wfmu.org/rocknsoulradio",
		DonationURL:    "",
		BroadcastType:  "internet",
		FrequencyMHz:   0,
		PlaylistSource: "wfmu_scrape",
		NetworkSlug:    "wfmu",
	},
	{
		// PSY-508: WFMU stream-only 24/7 sub-channel.
		Name:           "Sheena's Jungle Room",
		Slug:           "wfmu-sheena",
		Description:    "WFMU stream-only 24/7 channel.",
		City:           "Jersey City",
		State:          "NJ",
		Country:        "US",
		Timezone:       "America/New_York",
		StreamURL:      "https://wfmu.org/sheena",
		Website:        "https://wfmu.org/sheena",
		DonationURL:    "",
		BroadcastType:  "internet",
		FrequencyMHz:   0,
		PlaylistSource: "wfmu_scrape",
		NetworkSlug:    "wfmu",
	},
	{
		Name:           "NTS Radio",
		Slug:           "nts-radio",
		Description:    "NTS is an online radio station based in London, broadcasting 24/7 across two channels with shows from over 500 residents worldwide.",
		City:           "London",
		State:          "",
		Country:        "GB",
		Timezone:       "Europe/London",
		StreamURL:      "https://stream-relay-geo.ntslive.net/stream",
		Website:        "https://www.nts.live",
		DonationURL:    "https://www.nts.live/supporters",
		BroadcastType:  "internet",
		FrequencyMHz:   0,
		PlaylistSource: "nts_api",
	},
}

// RadioShows is the full seed set across KEXP / WFMU / NTS, reflecting the
// state after migrations 000068, 000076 (data repair), and 000077
// (Three Chord Monte + NTS Breakfast Show additions).
var RadioShows = []RadioShow{
	// KEXP
	{
		StationSlug:     "kexp",
		Name:            "The Morning Show",
		Slug:            "the-morning-show",
		HostName:        "John Richards",
		Description:     "KEXP's flagship morning program featuring a hand-picked mix of new and classic tracks.",
		ScheduleDisplay: "Weekdays 6-10 AM PT",
		ArchiveURL:      "https://www.kexp.org/shows/the-morning-show/",
		ExternalID:      "16",
	},
	{
		StationSlug:     "kexp",
		Name:            "The Midday Show",
		Slug:            "the-midday-show",
		HostName:        "Cheryl Waters",
		Description:     "A mid-day mix of new music discoveries and deep cuts.",
		ScheduleDisplay: "Weekdays 10 AM-2 PM PT",
		ArchiveURL:      "https://www.kexp.org/shows/the-midday-show/",
		ExternalID:      "15",
	},
	{
		StationSlug:     "kexp",
		Name:            "The Afternoon Show",
		Slug:            "the-afternoon-show",
		HostName:        "Larry Mizell, Jr.",
		Description:     "KEXP afternoon programming with a mix of established and emerging artists.",
		ScheduleDisplay: "Weekdays 2-6 PM PT",
		ArchiveURL:      "https://www.kexp.org/shows/the-afternoon-show/",
		ExternalID:      "14",
	},
	{
		StationSlug:     "kexp",
		Name:            "Audioasis",
		Slug:            "audioasis",
		HostName:        "Kennady Quille",
		Description:     "KEXP's long-running Northwest music show spotlighting artists from the Pacific Northwest.",
		ScheduleDisplay: "Saturdays 6-9 PM PT",
		ArchiveURL:      "https://www.kexp.org/shows/Audioasis/",
		ExternalID:      "1",
	},
	{
		StationSlug:     "kexp",
		Name:            "El Sonido",
		Slug:            "el-sonido",
		HostName:        "Albina Cabrera, Goyri",
		Description:     "A trip around the diverse world of Latin music and culture.",
		ScheduleDisplay: "Saturdays 9 PM-12 AM PT",
		ArchiveURL:      "https://www.kexp.org/shows/El-Sonido/",
		ExternalID:      "2",
	},
	{
		StationSlug:     "kexp",
		Name:            "Midnight in a Perfect World",
		Slug:            "midnight-in-a-perfect-world",
		HostName:        "",
		Description:     "Late-night electronic music exploring ambient, house, techno, and experimental sounds.",
		ScheduleDisplay: "Saturdays 12-3 AM PT",
		ArchiveURL:      "https://www.kexp.org/shows/Midnight-in-a-Perfect-World/",
		ExternalID:      "5",
	},

	// WFMU
	{
		StationSlug:     "wfmu",
		Name:            "Give The Drummer Some",
		Slug:            "give-the-drummer-some-wfmu",
		HostName:        "Doug Schulkind",
		Description:     "Freeform radio spanning jazz, soul, gospel, country, and global grooves, from longtime WFMU DJ Doug Schulkind.",
		ScheduleDisplay: "Fridays 9 AM-Noon ET",
		ArchiveURL:      "https://wfmu.org/playlists/DS",
		ExternalID:      "DS",
	},
	{
		StationSlug:     "wfmu",
		Name:            "Bodega Pop",
		Slug:            "bodega-pop-wfmu",
		HostName:        "Gary Sullivan",
		Description:     "Global pop, regional hits, and bodega-aisle rarities.",
		ScheduleDisplay: "Wednesdays (weekly)",
		ArchiveURL:      "https://wfmu.org/playlists/PG",
		ExternalID:      "PG",
	},
	{
		StationSlug:     "wfmu",
		Name:            "Downtown Soulville",
		Slug:            "downtown-soulville-wfmu",
		HostName:        "Mr. Fine Wine",
		Description:     "Deep soul, Northern soul, sweet soul, and classic R&B from the 1950s through 1970s.",
		ScheduleDisplay: "Saturdays 6-9 PM ET",
		ArchiveURL:      "https://wfmu.org/playlists/SV",
		ExternalID:      "SV",
	},
	{
		StationSlug:     "wfmu",
		Name:            "Three Chord Monte",
		Slug:            "three-chord-monte-wfmu",
		HostName:        "Joe Belock",
		Description:     "Garage, punk, and power pop from longtime WFMU DJ Joe Belock.",
		ScheduleDisplay: "Mondays 12 PM-3 PM ET",
		ArchiveURL:      "https://wfmu.org/playlists/TM",
		ExternalID:      "TM",
	},

	// NTS
	{
		StationSlug: "nts-radio",
		Name:        "Floating Points",
		Slug:        "floating-points-nts",
		// HostName intentionally empty: NTS residencies are host-named, so
		// repeating the show name as the host renders "Floating Points w/
		// Floating Points" on now-playing surfaces (PSY-1077). Leave NULL
		// when host would equal the show name.
		HostName:        "",
		Description:     "Eclectic selections spanning jazz, electronic, ambient, and world music from producer Floating Points.",
		ScheduleDisplay: "Monthly",
		ArchiveURL:      "https://www.nts.live/shows/floating-points",
		ExternalID:      "floating-points",
	},
	{
		StationSlug:     "nts-radio",
		Name:            "The Do!! You!!! Breakfast Show w/ Charlie Bones",
		Slug:            "charlie-bones-nts",
		HostName:        "Charlie Bones",
		Description:     "An eclectic morning show blending jazz, soul, funk, and left-field selections.",
		ScheduleDisplay: "Weekdays 10 AM-1 PM GMT",
		ArchiveURL:      "https://www.nts.live/shows/the-do-you-breakfast-show",
		ExternalID:      "the-do-you-breakfast-show",
	},
	{
		StationSlug: "nts-radio",
		Name:        "The NTS Breakfast Show",
		Slug:        "breakfast-show-nts",
		// HostName intentionally empty: rotating residents (Louise Chen,
		// Flo, Zakia, Coco María, and others). Empty string -> NULL in
		// both the SQL generator and the GORM mapping.
		HostName:        "",
		Description:     "Daily morning show on NTS, rotating through residents including Louise Chen, Flo, Zakia, Coco María, and others.",
		ScheduleDisplay: "Weekdays, mornings GMT",
		ArchiveURL:      "https://www.nts.live/shows/breakfast",
		ExternalID:      "breakfast",
	},
	{
		StationSlug: "nts-radio",
		Name:        "Anu",
		Slug:        "anu-nts",
		// HostName intentionally empty: host-named residency (see the
		// Floating Points entry above; PSY-1077).
		HostName:        "",
		Description:     "A mix of left-field club, electronic, and experimental sounds.",
		ScheduleDisplay: "Monthly",
		ArchiveURL:      "https://www.nts.live/shows/anu",
		ExternalID:      "anu",
	},
}

// RadioEpisode is the canonical description of a single broadcast of a
// RadioShow to seed. ShowSlug is the FK by slug (resolved to
// radio_shows.id at insert time) so episodes can be defined without
// knowing the show's autoincrement id.
//
// An episode is addressed in the public UI by its AirDate — the
// /radio/{station}/{show}/{date} route segment is the air_date string and
// GetEpisodeByShowAndDate looks it up via (show_id, air_date). There is no
// separate episode slug column. AirDate must be a Postgres DATE literal
// (YYYY-MM-DD).
//
// ExternalID participates in the episode dedup unique index
// (idx_radio_episodes_unique on show_id, air_date, COALESCE(external_id,
// ”)), so it carries a deterministic value here rather than NULL to give
// the ON CONFLICT target a stable expression.
type RadioEpisode struct {
	ShowSlug    string // resolved to radio_shows.id at insert time
	Title       string // empty -> NULL
	AirDate     string // Postgres DATE literal, e.g. "2025-01-15"
	Description string // empty -> NULL
	ArchiveURL  string // empty -> NULL
	ExternalID  string // provider-specific episode id
}

// RadioPlay is the canonical description of a single track played in a
// seeded RadioEpisode. EpisodeShowSlug + EpisodeAirDate together identify
// the parent episode by its (show_id, air_date) natural key, mirroring how
// the episode itself is addressed.
//
// ArtistSlug, when non-empty, resolves to artists.id and populates the
// play's artist_id FK — this is what GetAsHeardOnForArtist joins on
// (WHERE rp.artist_id = ?), so a seeded play MUST set ArtistSlug to a
// real seeded artist for the artist "As Heard On" cross-link to render.
// The raw ArtistName is always stored regardless (it's the never-
// overwritten source metadata); leaving ArtistSlug empty models the
// common "unmatched play" case where the matching engine found no
// knowledge-graph artist (artist_id NULL).
//
// (EpisodeShowSlug, EpisodeAirDate, Position, ArtistName, TrackTitle,
// AirTimestamp) keep each row distinct under the radio_plays dedup unique
// index (idx_radio_plays_unique). Distinct Position values are sufficient
// on their own.
type RadioPlay struct {
	EpisodeShowSlug string // parent episode's show slug
	EpisodeAirDate  string // parent episode's air_date
	Position        int    // 1-based play order within the episode
	ArtistName      string // raw source metadata, always stored
	TrackTitle      string // empty -> NULL
	ArtistSlug      string // empty -> artist_id NULL (unmatched play)
	AirTimestamp    string // Postgres TIMESTAMPTZ literal; empty -> NULL
}

// RadioEpisodes is the seed set of radio episodes. Currently a single
// KEXP "The Morning Show" episode exists so the deep radio browse chain
// (episode detail + tracklist) is E2E-reachable (PSY-899). Production
// episodes are produced by the live fetch/import pipeline, not seeded;
// this fixture exists solely to give the read-only browse flow a row to
// click into.
var RadioEpisodes = []RadioEpisode{
	{
		ShowSlug:    "the-morning-show",
		Title:       "The Morning Show — January 15, 2025",
		AirDate:     "2025-01-15",
		Description: "Seeded fixture episode for E2E coverage of the radio episode detail + tracklist flow.",
		ArchiveURL:  "https://www.kexp.org/shows/the-morning-show/",
		ExternalID:  "e2e-the-morning-show-2025-01-15",
	},
}

// RadioPlays is the seed set of radio plays. The first play links to the
// seeded "Calexico" artist (ArtistSlug "calexico") so the artist "As Heard
// On" cross-link (GetAsHeardOnForArtist, which joins on artist_id) renders
// for that artist; the second is an intentionally unmatched play
// (artist_id NULL) covering the common source-metadata-only case (PSY-899).
var RadioPlays = []RadioPlay{
	{
		EpisodeShowSlug: "the-morning-show",
		EpisodeAirDate:  "2025-01-15",
		Position:        1,
		ArtistName:      "Calexico",
		TrackTitle:      "Crystal Frontier",
		ArtistSlug:      "calexico", // matches seeded artist -> artist_id populated -> AsHeardOn renders
		AirTimestamp:    "2025-01-15 06:05:00+00",
	},
	{
		EpisodeShowSlug: "the-morning-show",
		EpisodeAirDate:  "2025-01-15",
		Position:        2,
		ArtistName:      "Beach House",
		TrackTitle:      "Space Song",
		ArtistSlug:      "", // unmatched: no seeded artist -> artist_id NULL
		AirTimestamp:    "2025-01-15 06:09:00+00",
	},
}
