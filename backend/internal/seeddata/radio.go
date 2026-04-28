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

// RadioStation is the canonical description of a radio station to seed.
// Zero values carry meaning: empty-string optional fields and a zero
// FrequencyMHz map to SQL NULL (see RenderRadioSeedSQL and the mapping
// in cmd/seed/main.go).
type RadioStation struct {
	Name           string
	Slug           string
	Description    string
	City           string
	State          string  // empty -> NULL (e.g. UK/London has no state)
	Country        string  // ISO two-letter code
	Timezone       string  // IANA zone name
	StreamURL      string
	Website        string
	DonationURL    string
	BroadcastType  string  // "fm" | "internet" | "both"
	FrequencyMHz   float64 // 0 -> NULL (for internet-only stations)
	PlaylistSource string  // provider tag used by the fetcher pipeline
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

// RadioStations is the full seed set, matching what's currently in prod
// after migrations 000068 / 000076 / 000077.
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
		StationSlug:     "nts-radio",
		Name:            "Floating Points",
		Slug:            "floating-points-nts",
		HostName:        "Floating Points",
		Description:     "Eclectic selections spanning jazz, electronic, ambient, and world music from producer Floating Points.",
		ScheduleDisplay: "Monthly",
		ArchiveURL:      "https://www.nts.live/shows/floating-points",
		ExternalID:      "floating-points",
	},
	{
		StationSlug: "nts-radio",
		Name:        "The Do!! You!!! Breakfast Show w/ Charlie Bones",
		Slug:        "charlie-bones-nts",
		HostName:    "Charlie Bones",
		Description: "An eclectic morning show blending jazz, soul, funk, and left-field selections.",
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
		StationSlug:     "nts-radio",
		Name:            "Anu",
		Slug:            "anu-nts",
		HostName:        "Anu",
		Description:     "A mix of left-field club, electronic, and experimental sounds.",
		ScheduleDisplay: "Monthly",
		ArchiveURL:      "https://www.nts.live/shows/anu",
		ExternalID:      "anu",
	},
}
