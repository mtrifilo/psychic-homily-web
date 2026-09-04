package utils

import (
	"encoding/json"
	"fmt"
	"os"
	"testing"
	"time"
	_ "time/tzdata" // embed the IANA database so the corpus zones load host-independently

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// dstClockCase is one wall clock the corpus makes a claim about.
type dstClockCase struct {
	Date      string `json:"date"`
	Clock     string `json:"clock"`
	Zone      string `json:"zone"`
	UTC       string `json:"utc"`
	Exists    bool   `json:"exists"`
	Ambiguous bool   `json:"ambiguous"`
	Why       string `json:"why"`
}

func (c dstClockCase) name() string {
	return fmt.Sprintf("%s %s %s", c.Date, c.Clock, c.Zone)
}

// dstClockCorpus mirrors testdata/dst_clock_corpus.json, the file this test
// shares with frontend/lib/utils/timeUtils.test.ts.
type dstClockCorpus struct {
	Cases []dstClockCase `json:"cases"`
}

func loadDSTClockCorpus(t *testing.T) dstClockCorpus {
	t.Helper()
	raw, err := os.ReadFile("testdata/dst_clock_corpus.json")
	require.NoError(t, err, "the corpus is shared with the frontend test; do not move it without updating both")

	var corpus dstClockCorpus
	require.NoError(t, json.Unmarshal(raw, &corpus))
	require.NotEmpty(t, corpus.Cases)
	return corpus
}

// parseWallClock reads a corpus row's stated day and clock as plain integers.
// Deliberately not time.Parse: a layout parse would produce a time.Time, and
// building one is the very thing under test.
func parseWallClock(t *testing.T, c dstClockCase) (year int, month time.Month, day, hour, minute int) {
	t.Helper()
	var m int
	_, err := fmt.Sscanf(c.Date, "%d-%d-%d", &year, &m, &day)
	require.NoError(t, err, "unreadable date in corpus row %s", c.name())
	_, err = fmt.Sscanf(c.Clock, "%d:%d", &hour, &minute)
	require.NoError(t, err, "unreadable clock in corpus row %s", c.name())
	return year, time.Month(m), day, hour, minute
}

// TestDSTClockCorpus holds every corpus row to time.Date, which is the function
// the Go writers compose through and the answer the frontend resolver has to
// match. A row edited by hand to a number nothing produces fails here rather
// than travelling to the other language as a claim.
func TestDSTClockCorpus(t *testing.T) {
	corpus := loadDSTClockCorpus(t)

	// A corpus that pinned only ordinary nights would pass while saying nothing
	// about the two nights this exists for.
	var gaps, overlaps int
	for _, c := range corpus.Cases {
		if !c.Exists {
			gaps++
		}
		if c.Ambiguous {
			overlaps++
		}
	}
	assert.GreaterOrEqual(t, gaps, 2, "corpus must carry spring-forward gap rows")
	assert.GreaterOrEqual(t, overlaps, 2, "corpus must carry fall-back overlap rows")

	for _, c := range corpus.Cases {
		t.Run(c.name(), func(t *testing.T) {
			loc, err := time.LoadLocation(c.Zone)
			require.NoError(t, err, "corpus names a zone this runtime cannot load")

			year, month, day, hour, minute := parseWallClock(t, c)
			got := time.Date(year, month, day, hour, minute, 0, 0, loc)
			assert.Equal(t, c.UTC, got.UTC().Format("2006-01-02T15:04:05Z"), c.Why)

			// exists means the instant reads back in the zone as the clock that
			// was asked for. Inside a spring-forward gap it cannot, which is the
			// signal a writer refuses on.
			back := got.In(loc)
			readsBack := back.Year() == year && back.Month() == month && back.Day() == day &&
				back.Hour() == hour && back.Minute() == minute
			assert.Equal(t, c.Exists, readsBack, "exists flag disagrees with what the instant reads back as")

			// ambiguous means some other instant reads back as the same clock.
			// The candidates are one transition step away in either direction,
			// and a step is an hour in most zones and half an hour in a few.
			ambiguous := false
			for _, delta := range []time.Duration{
				-2 * time.Hour, -time.Hour, -30 * time.Minute,
				30 * time.Minute, time.Hour, 2 * time.Hour,
			} {
				other := got.Add(delta).In(loc)
				if other.Year() == year && other.Month() == month && other.Day() == day &&
					other.Hour() == hour && other.Minute() == minute {
					ambiguous = true
					break
				}
			}
			assert.Equal(t, c.Ambiguous, ambiguous, "ambiguous flag disagrees with the zone's own transitions")
		})
	}
}
