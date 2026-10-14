package catalog

import (
	"math"
	"slices"
	"testing"
	"time"

	"github.com/lib/pq"

	"psychic-homily-backend/internal/services/contracts"
)

func TestRadioArtistCandidateScore(t *testing.T) {
	cases := []struct {
		name                     string
		stationRows, plays, show int
		want                     float64
	}{
		{"floor: 2 rows, 3 plays, 1 show", 2, 3, 1, 2 * math.Log(3) * 1.1},
		{"6 rows, 499 plays, 48 shows", 6, 499, 48, 6 * math.Log(499) * 5.8},
		{"shows add a tenth each", 3, 10, 10, 3 * math.Log(10) * 2},
		{"one play scores zero", 4, 1, 1, 0},
	}
	for _, c := range cases {
		got := RadioArtistCandidateScore(c.stationRows, c.plays, c.show)
		if math.Abs(got-c.want) > 1e-9 {
			t.Errorf("%s: score = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestRadioCandidateLaneFor(t *testing.T) {
	cases := []struct {
		stationRows, mbIDs int
		want               contracts.RadioCandidateLane
	}{
		{2, 1, contracts.RadioCandidateLaneAuto},
		{5, 1, contracts.RadioCandidateLaneAuto},
		{2, 0, contracts.RadioCandidateLaneHuman},
		{2, 2, contracts.RadioCandidateLaneHuman},
		{1, 1, contracts.RadioCandidateLaneHuman},
	}
	for _, c := range cases {
		if got := RadioCandidateLaneFor(c.stationRows, c.mbIDs); got != c.want {
			t.Errorf("lane(rows=%d, ids=%d) = %q, want %q", c.stationRows, c.mbIDs, got, c.want)
		}
	}
}

func candidateFixtureRow(key string, rows, plays, shows int, last string, mbIDs ...string) radioArtistCandidateRow {
	day, err := time.Parse(time.DateOnly, last)
	if err != nil {
		panic(err)
	}
	return radioArtistCandidateRow{
		ArtistKey: key, PlayCount: plays, StationRowCount: rows, StationFamilyCount: 1, ShowCount: shows,
		FirstAiredOn: day.AddDate(0, 0, -30), LastAiredOn: day, MBArtistIDs: pq.StringArray(mbIDs),
	}
}

func candidateKeys(cs []contracts.RadioArtistCandidate) []string {
	keys := make([]string, len(cs))
	for i, c := range cs {
		keys[i] = c.ArtistKey
	}
	return keys
}

func TestRankRadioArtistCandidates(t *testing.T) {
	rows := []radioArtistCandidateRow{
		candidateFixtureRow("many plays", 2, 100, 2, "2026-07-01"),
		candidateFixtureRow("many stations", 6, 5, 2, "2026-07-01", "11111111-1111-1111-1111-111111111111"),
		// Same counts as "tie newer": the more recent play ranks first.
		candidateFixtureRow("tie older", 3, 10, 4, "2026-06-01"),
		candidateFixtureRow("tie newer", 3, 10, 4, "2026-07-02"),
		// Same counts and date as "tie newer": the key decides.
		candidateFixtureRow("tie newer b", 3, 10, 4, "2026-07-02", "a", "b"),
	}

	byScore := rankRadioArtistCandidates(rows, contracts.RadioArtistCandidateSortScore)
	// many stations 6 x ln 5 x 1.2 = 11.59; many plays 2 x ln 100 x 1.2 = 11.05;
	// the tie group 3 x ln 10 x 1.4 = 9.67.
	wantScore := []string{"many stations", "many plays", "tie newer", "tie newer b", "tie older"}
	if got := candidateKeys(byScore); !slices.Equal(got, wantScore) {
		t.Errorf("score order = %v, want %v", got, wantScore)
	}
	for i := 1; i < len(byScore); i++ {
		if byScore[i].Score > byScore[i-1].Score {
			t.Errorf("score order not descending at %d: %v > %v", i, byScore[i].Score, byScore[i-1].Score)
		}
	}

	wantPlays := []string{"many plays", "tie newer", "tie newer b", "tie older", "many stations"}
	if got := candidateKeys(rankRadioArtistCandidates(rows, contracts.RadioArtistCandidateSortPlays)); !slices.Equal(got, wantPlays) {
		t.Errorf("plays order = %v, want %v", got, wantPlays)
	}
	wantStations := []string{"many stations", "tie newer", "tie newer b", "tie older", "many plays"}
	if got := candidateKeys(rankRadioArtistCandidates(rows, contracts.RadioArtistCandidateSortStations)); !slices.Equal(got, wantStations) {
		t.Errorf("stations order = %v, want %v", got, wantStations)
	}

	for _, c := range byScore {
		switch c.ArtistKey {
		case "many stations":
			if c.Lane != contracts.RadioCandidateLaneAuto || c.MBArtistID == nil || c.MBArtistIDConflict {
				t.Errorf("single id: got lane %q id %v conflict %v, want auto with the id", c.Lane, c.MBArtistID, c.MBArtistIDConflict)
			}
		case "tie newer b":
			if c.Lane != contracts.RadioCandidateLaneHuman || c.MBArtistID != nil || !c.MBArtistIDConflict || len(c.MBArtistIDs) != 2 {
				t.Errorf("two ids: got lane %q id %v conflict %v ids %v, want human, no id, conflict, both ids", c.Lane, c.MBArtistID, c.MBArtistIDConflict, c.MBArtistIDs)
			}
		default:
			if c.Lane != contracts.RadioCandidateLaneHuman || c.MBArtistIDs == nil || len(c.MBArtistIDs) != 0 {
				t.Errorf("%s: got lane %q ids %v, want human and an empty id list", c.ArtistKey, c.Lane, c.MBArtistIDs)
			}
		}
		if c.Stations == nil || c.TopTracks == nil || c.AlbumNames == nil || c.LabelNames == nil {
			t.Errorf("%s: evidence lists must be empty arrays, not nil", c.ArtistKey)
		}
	}
}
