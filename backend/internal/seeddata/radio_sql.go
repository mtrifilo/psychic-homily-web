package seeddata

import (
	"io"
	"strconv"
	"strings"
)

// RenderRadioSeedSQL writes idempotent SQL INSERT statements for the
// RadioNetworks, RadioStations, and RadioShows seed data to w. Output is
// safe to pipe through psql: every statement uses ON CONFLICT (slug)
// DO NOTHING, so re-running against a populated database is a no-op.
//
// Networks are emitted first so the radio_stations.network_id subquery
// (resolved by network slug) can find them.
//
// Consumers:
//   - cmd/gen-e2e-seed -> frontend/e2e/setup-db.sh
//   - manually via `go run ./cmd/gen-e2e-seed > file.sql`
//
// cmd/seed writes directly via GORM and does NOT go through this path.
func RenderRadioSeedSQL(w io.Writer) error {
	var b strings.Builder

	b.WriteString("-- Radio networks (generated from backend/internal/seeddata/radio.go)\n")
	b.WriteString("INSERT INTO radio_networks (slug, name, created_at, updated_at) VALUES\n")
	for i, n := range RadioNetworks {
		b.WriteString("  (")
		b.WriteString(sqlString(n.Slug))
		b.WriteString(", ")
		b.WriteString(sqlString(n.Name))
		b.WriteString(", NOW(), NOW())")
		if i < len(RadioNetworks)-1 {
			b.WriteString(",")
		}
		b.WriteString("\n")
	}
	b.WriteString("ON CONFLICT (slug) DO NOTHING;\n\n")

	b.WriteString("-- Radio stations (generated from backend/internal/seeddata/radio.go)\n")
	b.WriteString("INSERT INTO radio_stations (name, slug, description, city, state, country, timezone, stream_url, website, donation_url, broadcast_type, frequency_mhz, playlist_source, network_id, is_flagship, is_active, created_at, updated_at) VALUES\n")
	for i, s := range RadioStations {
		b.WriteString("  (")
		b.WriteString(sqlString(s.Name))
		b.WriteString(", ")
		b.WriteString(sqlString(s.Slug))
		b.WriteString(", ")
		b.WriteString(sqlString(s.Description))
		b.WriteString(", ")
		b.WriteString(sqlString(s.City))
		b.WriteString(", ")
		b.WriteString(sqlStringOrNull(s.State))
		b.WriteString(", ")
		b.WriteString(sqlString(s.Country))
		b.WriteString(", ")
		b.WriteString(sqlString(s.Timezone))
		b.WriteString(", ")
		b.WriteString(sqlString(s.StreamURL))
		b.WriteString(", ")
		b.WriteString(sqlString(s.Website))
		b.WriteString(", ")
		b.WriteString(sqlString(s.DonationURL))
		b.WriteString(", ")
		b.WriteString(sqlString(s.BroadcastType))
		b.WriteString(", ")
		b.WriteString(sqlFloatOrNull(s.FrequencyMHz))
		b.WriteString(", ")
		b.WriteString(sqlString(s.PlaylistSource))
		b.WriteString(", ")
		b.WriteString(sqlNetworkIDFromSlug(s.NetworkSlug))
		b.WriteString(", ")
		b.WriteString(strconv.FormatBool(s.IsFlagship))
		b.WriteString(", true, NOW(), NOW())")
		if i < len(RadioStations)-1 {
			b.WriteString(",")
		}
		b.WriteString("\n")
	}
	b.WriteString("ON CONFLICT (slug) DO NOTHING;\n\n")

	b.WriteString("-- Radio shows (generated from backend/internal/seeddata/radio.go)\n")
	b.WriteString("INSERT INTO radio_shows (station_id, name, slug, host_name, description, schedule_display, archive_url, external_id, is_active, created_at, updated_at) VALUES\n")
	for i, s := range RadioShows {
		b.WriteString("  ((SELECT id FROM radio_stations WHERE slug = ")
		b.WriteString(sqlString(s.StationSlug))
		b.WriteString("), ")
		b.WriteString(sqlString(s.Name))
		b.WriteString(", ")
		b.WriteString(sqlString(s.Slug))
		b.WriteString(", ")
		b.WriteString(sqlStringOrNull(s.HostName))
		b.WriteString(", ")
		b.WriteString(sqlString(s.Description))
		b.WriteString(", ")
		b.WriteString(sqlString(s.ScheduleDisplay))
		b.WriteString(", ")
		b.WriteString(sqlString(s.ArchiveURL))
		b.WriteString(", ")
		b.WriteString(sqlString(s.ExternalID))
		b.WriteString(", true, NOW(), NOW())")
		if i < len(RadioShows)-1 {
			b.WriteString(",")
		}
		b.WriteString("\n")
	}
	b.WriteString("ON CONFLICT (slug) DO NOTHING;\n\n")

	// Radio episodes. radio_episodes has no slug; the dedup key is the
	// idx_radio_episodes_unique expression index on
	// (show_id, air_date, COALESCE(external_id, '')), which is the ON
	// CONFLICT target below so re-runs are no-ops. show_id is resolved from
	// the show slug at insert time. updated_at is omitted so its DEFAULT NOW() applies.
	b.WriteString("-- Radio episodes (generated from backend/internal/seeddata/radio.go)\n")
	b.WriteString("INSERT INTO radio_episodes (show_id, title, air_date, description, archive_url, external_id, play_count, created_at) VALUES\n")
	for i, e := range RadioEpisodes {
		b.WriteString("  ((SELECT id FROM radio_shows WHERE slug = ")
		b.WriteString(sqlString(e.ShowSlug))
		b.WriteString("), ")
		b.WriteString(sqlStringOrNull(e.Title))
		b.WriteString(", ")
		b.WriteString(sqlString(e.AirDate))
		b.WriteString(", ")
		b.WriteString(sqlStringOrNull(e.Description))
		b.WriteString(", ")
		b.WriteString(sqlStringOrNull(e.ArchiveURL))
		b.WriteString(", ")
		b.WriteString(sqlString(e.ExternalID))
		// play_count is set to the number of seeded plays on this episode so
		// the denormalized count the detail page shows matches the tracklist.
		b.WriteString(", ")
		b.WriteString(strconv.Itoa(countPlaysForEpisode(e.ShowSlug, e.AirDate)))
		b.WriteString(", NOW())")
		if i < len(RadioEpisodes)-1 {
			b.WriteString(",")
		}
		b.WriteString("\n")
	}
	b.WriteString("ON CONFLICT (show_id, air_date, COALESCE(external_id, '')) DO NOTHING;\n\n")

	// Radio plays. episode_id is resolved from the parent episode's
	// (show_id, air_date) natural key; artist_id is resolved from the
	// (optional) artist slug — empty slug -> NULL (unmatched play). The ON
	// CONFLICT target matches idx_radio_plays_dedup (episode_id, dedup_key),
	// where dedup_key is the GENERATED STORED content-hash column (PSY-1131).
	// Seeded plays set no provider_play_id, so dedup_key is the md5 hash over
	// (position, artist_name, track_title, album_title); distinct positions
	// keep these rows distinct, so re-runs are no-ops. radio_plays has no
	// updated_at column.
	b.WriteString("-- Radio plays (generated from backend/internal/seeddata/radio.go)\n")
	b.WriteString("INSERT INTO radio_plays (episode_id, position, artist_name, track_title, artist_id, air_timestamp, created_at) VALUES\n")
	for i, p := range RadioPlays {
		b.WriteString("  ((SELECT id FROM radio_episodes WHERE show_id = (SELECT id FROM radio_shows WHERE slug = ")
		b.WriteString(sqlString(p.EpisodeShowSlug))
		b.WriteString(") AND air_date = ")
		b.WriteString(sqlString(p.EpisodeAirDate))
		b.WriteString("), ")
		b.WriteString(strconv.Itoa(p.Position))
		b.WriteString(", ")
		b.WriteString(sqlString(p.ArtistName))
		b.WriteString(", ")
		b.WriteString(sqlStringOrNull(p.TrackTitle))
		b.WriteString(", ")
		b.WriteString(sqlArtistIDFromSlug(p.ArtistSlug))
		b.WriteString(", ")
		b.WriteString(sqlStringOrNull(p.AirTimestamp))
		b.WriteString(", NOW())")
		if i < len(RadioPlays)-1 {
			b.WriteString(",")
		}
		b.WriteString("\n")
	}
	b.WriteString("ON CONFLICT (episode_id, dedup_key) DO NOTHING;\n")

	_, err := io.WriteString(w, b.String())
	return err
}

// countPlaysForEpisode returns how many seeded RadioPlays belong to the
// episode identified by (showSlug, airDate). Used to set radio_episodes
// .play_count so the denormalized count matches the seeded tracklist.
func countPlaysForEpisode(showSlug, airDate string) int {
	n := 0
	for _, p := range RadioPlays {
		if p.EpisodeShowSlug == showSlug && p.EpisodeAirDate == airDate {
			n++
		}
	}
	return n
}

// sqlArtistIDFromSlug returns NULL for an unmatched play (empty slug), else
// a subquery resolving the artist slug to artists.id at insert time. This
// is what populates radio_plays.artist_id — the column GetAsHeardOnForArtist
// joins on — so the artist "As Heard On" cross-link renders.
func sqlArtistIDFromSlug(slug string) string {
	if slug == "" {
		return "NULL"
	}
	return "(SELECT id FROM artists WHERE slug = " + sqlString(slug) + ")"
}

// sqlString quotes a value as an SQL string literal, escaping any embedded
// single-quote characters via the Postgres-standard doubling (`'` -> `”`).
func sqlString(v string) string {
	return "'" + strings.ReplaceAll(v, "'", "''") + "'"
}

// sqlStringOrNull returns the SQL literal NULL for an empty string, else
// the quoted string. Use for columns where empty-string has no meaning.
func sqlStringOrNull(v string) string {
	if v == "" {
		return "NULL"
	}
	return sqlString(v)
}

// sqlFloatOrNull returns NULL for a zero value, else the float formatted
// without the locale's thousands separator.
func sqlFloatOrNull(v float64) string {
	if v == 0 {
		return "NULL"
	}
	return strconv.FormatFloat(v, 'f', -1, 64)
}

// sqlNetworkIDFromSlug returns NULL for stations not assigned to a network,
// else a subquery that resolves the network slug to radio_networks.id at
// insert time. Avoids hardcoding numeric IDs in seed SQL.
func sqlNetworkIDFromSlug(slug string) string {
	if slug == "" {
		return "NULL"
	}
	return "(SELECT id FROM radio_networks WHERE slug = " + sqlString(slug) + ")"
}
