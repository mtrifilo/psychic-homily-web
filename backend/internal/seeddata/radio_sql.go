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
	b.WriteString("INSERT INTO radio_stations (name, slug, description, city, state, country, timezone, stream_url, website, donation_url, broadcast_type, frequency_mhz, playlist_source, network_id, is_active, created_at, updated_at) VALUES\n")
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
	b.WriteString("ON CONFLICT (slug) DO NOTHING;\n")

	_, err := io.WriteString(w, b.String())
	return err
}

// sqlString quotes a value as an SQL string literal, escaping any embedded
// single-quote characters via the Postgres-standard doubling (`'` -> `''`).
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
