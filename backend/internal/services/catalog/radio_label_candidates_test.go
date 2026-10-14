package catalog

import (
	"math"
	"slices"
	"testing"
	"time"

	"psychic-homily-backend/internal/services/contracts"
)

func TestRadioLabelIsRising(t *testing.T) {
	cases := []struct {
		recent, prior int
		want          bool
	}{
		{3, 2, true},  // exactly 1.5x
		{2, 2, false}, // flat
		{2, 1, true},  // 2x
		{4, 3, false}, // 1.33x
		{30, 20, true},
		{29, 20, false},
		{3, 0, true}, // no prior plays: the new-play floor
		{2, 0, false},
		{0, 0, false},
		{0, 5, false},
	}
	for _, c := range cases {
		if got := RadioLabelIsRising(c.recent, c.prior); got != c.want {
			t.Errorf("rising(recent %d, prior %d) = %v, want %v", c.recent, c.prior, got, c.want)
		}
	}
}

func TestRadioLabelCandidateScore(t *testing.T) {
	cases := []struct {
		name                         string
		stationRows, plays, rec, pri int
		wantVelocity, wantScore      float64
	}{
		{"floor: 2 rows, 2 plays, 2 recent over 1 prior", 2, 2, 2, 1, 2, 2 * math.Log(2) * 2},
		{"no prior plays: velocity is the recent count", 3, 10, 4, 0, 4, 3 * math.Log(10) * 4},
		{"ratio", 5, 100, 30, 20, 1.5, 5 * math.Log(100) * 1.5},
	}
	for _, c := range cases {
		if got := RadioLabelVelocity(c.rec, c.pri); math.Abs(got-c.wantVelocity) > 1e-9 {
			t.Errorf("%s: velocity = %v, want %v", c.name, got, c.wantVelocity)
		}
		if got := RadioLabelCandidateScore(c.stationRows, c.plays, c.rec, c.pri); math.Abs(got-c.wantScore) > 1e-9 {
			t.Errorf("%s: score = %v, want %v", c.name, got, c.wantScore)
		}
	}
}

func labelFixtureRow(key string, rows, plays, recent, prior int, last string) radioLabelCandidateRow {
	day, err := time.Parse(time.DateOnly, last)
	if err != nil {
		panic(err)
	}
	return radioLabelCandidateRow{
		LabelKey: &key, PlayCount: plays, RecentPlayCount: recent, PriorPlayCount: prior,
		StationRowCount: rows, StationFamilyCount: 1, ShowCount: 1,
		FirstAiredOn: day.AddDate(0, 0, -30), LastAiredOn: day,
	}
}

func labelCandidateKeys(cs []contracts.RadioLabelCandidate) []string {
	keys := make([]string, len(cs))
	for i, c := range cs {
		keys[i] = c.LabelKey
	}
	return keys
}

func TestRankRadioLabelCandidates(t *testing.T) {
	rows := []radioLabelCandidateRow{
		// 2 x ln 200 x 2 = 21.19
		labelFixtureRow("many plays", 2, 200, 20, 10, "2026-07-01"),
		// 6 x ln 20 x 1.5 = 26.96
		labelFixtureRow("many stations", 6, 20, 6, 4, "2026-07-01"),
		// 2 x ln 12 x 12 = 59.64
		labelFixtureRow("new", 2, 12, 12, 0, "2026-07-01"),
		// Same counts as "tie newer": the more recent play ranks first.
		labelFixtureRow("tie older", 3, 10, 4, 2, "2026-06-01"),
		labelFixtureRow("tie newer", 3, 10, 4, 2, "2026-07-02"),
		// Same counts and date as "tie newer": the key decides.
		labelFixtureRow("tie newer b", 3, 10, 4, 2, "2026-07-02"),
	}
	cases := []struct {
		sort contracts.RadioLabelCandidateSort
		want []string
	}{
		{contracts.RadioLabelCandidateSortScore, []string{"new", "many stations", "many plays", "tie newer", "tie newer b", "tie older"}},
		{contracts.RadioLabelCandidateSortPlays, []string{"many plays", "many stations", "new", "tie newer", "tie newer b", "tie older"}},
		{contracts.RadioLabelCandidateSortStations, []string{"many stations", "tie newer", "tie newer b", "tie older", "new", "many plays"}},
		{contracts.RadioLabelCandidateSortVelocity, []string{"new", "many plays", "tie newer", "tie newer b", "tie older", "many stations"}},
	}
	for _, c := range cases {
		got := rankRadioLabelCandidates(rows, c.sort)
		if keys := labelCandidateKeys(got); !slices.Equal(keys, c.want) {
			t.Errorf("%s order = %v, want %v", c.sort, keys, c.want)
		}
	}

	for _, c := range rankRadioLabelCandidates(rows, contracts.RadioLabelCandidateSortScore) {
		if c.Lane != contracts.RadioCandidateLaneHuman {
			t.Errorf("%s: lane = %q, want human", c.LabelKey, c.Lane)
		}
		if c.LabelSpellings == nil || c.Stations == nil || c.Artists == nil || c.ReleaseNames == nil || c.MBReleaseIDs == nil {
			t.Errorf("%s: evidence lists must be empty arrays, not nil", c.LabelKey)
		}
		if want := RadioLabelVelocity(c.RecentPlayCount, c.PriorPlayCount); c.Velocity != want {
			t.Errorf("%s: velocity = %v, want %v", c.LabelKey, c.Velocity, want)
		}
	}
}
