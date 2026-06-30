package utils

import "regexp"

// mbidPattern matches a canonical MusicBrainz identifier: a hex UUID (8-4-4-4-12).
var mbidPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

// IsValidMBID reports whether s is a canonical 36-char MusicBrainz UUID. It is the
// trust-boundary check before an MB-API-supplied id is written to a VARCHAR(36)
// identity column (PSY-1249): an oversized value would otherwise abort the whole
// write, and a malformed-but-short value would enter a column downstream passes trust
// as identity. MBIDs are always UUIDs, so a value that fails this is an upstream
// anomaly we decline to store rather than guess at.
//
// Single source of truth (PSY-1281): both the pipeline (artist enrichment) and
// catalog (release dedup) layers validate MB ids here — keep it that way, since a
// drifted copy of a trust-boundary check is a silent security gap.
func IsValidMBID(s string) bool {
	return mbidPattern.MatchString(s)
}
