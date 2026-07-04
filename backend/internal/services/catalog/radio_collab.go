package catalog

import (
	"strings"
	"unicode"
)

// minCollabPartRunes is the minimum length (in runes) for each segment of a
// collab artist_name before we attempt a split match. Filters noise like "X, Y".
const minCollabPartRunes = 3

// collabSeparators are tried in order; the first separator that yields ≥2 valid
// parts wins. Interior punctuation in a band name like "AC/DC" is untouched
// because none of these separators appear.
var collabSeparators = []string{", ", " & ", " and "}

// splitCollabArtistName splits a WFMU-style collab credit into per-artist
// segments. Returns nil when the string is not a splittable collab (too few
// parts, or any part shorter than minCollabPartRunes).
func splitCollabArtistName(name string) []string {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return nil
	}

	// "Earth, Wind & Fire" contains both comma and ampersand — treat as a single
	// billed act; exact/alias match is the right path when the entity exists.
	if strings.Contains(trimmed, ",") && strings.Contains(trimmed, "&") {
		return nil
	}

	lower := strings.ToLower(trimmed)
	for _, sep := range collabSeparators {
		var parts []string
		if sep == " and " {
			if !strings.Contains(lower, sep) {
				continue
			}
			// Case-insensitive split on " and " preserving original casing in parts.
			parts = splitFoldSeparator(trimmed, sep)
		} else if strings.Contains(trimmed, sep) {
			parts = strings.Split(trimmed, sep)
		} else {
			continue
		}

		if out := normalizeCollabParts(parts); out != nil {
			return out
		}
	}
	return nil
}

// splitFoldSeparator splits s on separator (ASCII, lower-case) case-insensitively.
func splitFoldSeparator(s, sep string) []string {
	lower := strings.ToLower(s)
	sep = strings.ToLower(sep)
	var parts []string
	for {
		idx := strings.Index(lower, sep)
		if idx < 0 {
			parts = append(parts, s)
			break
		}
		parts = append(parts, s[:idx])
		s = s[idx+len(sep):]
		lower = lower[idx+len(sep):]
	}
	return parts
}

func normalizeCollabParts(parts []string) []string {
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		// Trim trailing punctuation noise from WFMU credits ("feat.", etc.) at
		// segment boundaries only — interior punctuation stays (P.I.L. etc.).
		p = strings.TrimFunc(p, func(r rune) bool {
			return !unicode.IsLetter(r) && !unicode.IsDigit(r)
		})
		if p == "" {
			return nil
		}
		if len([]rune(p)) < minCollabPartRunes {
			return nil
		}
		out = append(out, p)
	}
	if len(out) < 2 {
		return nil
	}
	return out
}

// collabPlayMentionsArtist reports whether a collab-split of playArtistName
// includes a segment whose normalized form equals normalizedTarget.
func collabPlayMentionsArtist(playArtistName, normalizedTarget string) bool {
	if normalizedTarget == "" {
		return false
	}
	for _, part := range splitCollabArtistName(playArtistName) {
		if normalizeName(part) == normalizedTarget {
			return true
		}
	}
	return false
}
