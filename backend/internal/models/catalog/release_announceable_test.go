package catalog

import (
	"testing"
	"time"
)

func TestReleaseAnnounceable(t *testing.T) {
	now := time.Date(2026, 11, 3, 15, 0, 0, 0, time.UTC)
	day := func(d time.Time) *string { s := d.Format("2006-01-02"); return &s }
	year := func(y int) *int { return &y }
	str := func(s string) *string { return &s }

	cases := []struct {
		name       string
		release    *Release
		want       bool
		wantReason string
	}{
		{"nil release", nil, false, NotAnnounceableGone},
		{"dated today", &Release{ReleaseDate: day(now)}, true, ""},
		{"dated inside the recency window",
			&Release{ReleaseDate: day(now.Add(-ReleaseRecencyWindow + 24*time.Hour))}, true, ""},
		// The window is whole UTC days: the oldest day counts at any hour.
		{"the oldest day of the window is announceable",
			&Release{ReleaseDate: day(now.Add(-ReleaseRecencyWindow))}, true, ""},
		{"dated past the recency window is back catalogue",
			&Release{ReleaseDate: day(now.Add(-ReleaseRecencyWindow - 24*time.Hour))}, false, NotAnnounceableBackCatalogue},
		{"a 1998 record imported today is back catalogue",
			&Release{ReleaseDate: str("1998-03-10"), ReleaseYear: year(1998)}, false, NotAnnounceableBackCatalogue},
		{"pre-order inside the future window",
			&Release{ReleaseDate: day(now.Add(90 * 24 * time.Hour))}, true, ""},
		{"date past the future window is implausible",
			&Release{ReleaseDate: day(now.Add(ReleaseFutureWindow + 48*time.Hour))}, false, NotAnnounceableImplausibleDate},
		{"year only, current year", &Release{ReleaseYear: year(2026)}, true, ""},
		{"year only, next year", &Release{ReleaseYear: year(2027)}, true, ""},
		{"year only, last year is back catalogue", &Release{ReleaseYear: year(2025)}, false, NotAnnounceableBackCatalogue},
		{"year only, past the future window is implausible", &Release{ReleaseYear: year(2062)}, false, NotAnnounceableImplausibleDate},
		{"undated", &Release{}, false, NotAnnounceableUndated},
		{"empty date string is undated", &Release{ReleaseDate: str("")}, false, NotAnnounceableUndated},
		{"an unparseable date falls through to the year",
			&Release{ReleaseDate: str("not-a-date"), ReleaseYear: year(2026)}, true, ""},
		{"an unparseable date with no year is undated",
			&Release{ReleaseDate: str("not-a-date")}, false, NotAnnounceableUndated},
		// The date wins over the year: a stale date with a current year stays
		// back catalogue rather than slipping through on the coarser field.
		{"a dated release ignores its year",
			&Release{ReleaseDate: str("2001-01-01"), ReleaseYear: year(2026)}, false, NotAnnounceableBackCatalogue},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, reason := ReleaseAnnounceable(tc.release, now)
			if got != tc.want || reason != tc.wantReason {
				t.Errorf("ReleaseAnnounceable = (%v, %q), want (%v, %q)", got, reason, tc.want, tc.wantReason)
			}
		})
	}
}
