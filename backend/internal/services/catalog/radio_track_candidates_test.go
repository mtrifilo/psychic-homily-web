package catalog

import (
	"math"
	"slices"
	"testing"
	"time"

	"github.com/lib/pq"

	"psychic-homily-backend/internal/services/contracts"
)

func TestRadioTrackCandidateScore(t *testing.T) {
	cases := []struct {
		name               string
		stationRows, plays int
		want               float64
	}{
		{"floor: 2 rows, 3 plays", 2, 3, 2 * math.Log(3)},
		{"4 rows, 35 plays", 4, 35, 4 * math.Log(35)},
		{"one play scores zero", 3, 1, 0},
	}
	for _, c := range cases {
		got := RadioTrackCandidateScore(c.stationRows, c.plays)
		if math.Abs(got-c.want) > 1e-9 {
			t.Errorf("%s: score = %v, want %v", c.name, got, c.want)
		}
	}
}

func trackFixtureRow(id uint, rows, plays int, last string, mbIDs ...string) radioTrackCandidateRow {
	day, err := time.Parse(time.DateOnly, last)
	if err != nil {
		panic(err)
	}
	return radioTrackCandidateRow{
		TrackID: &id, PlayCount: plays, StationRowCount: rows, StationFamilyCount: 1, ShowCount: 1,
		FirstAiredOn: day.AddDate(0, 0, -30), LastAiredOn: day, MBReleaseIDs: pq.StringArray(mbIDs),
		ArtistKeys: pq.StringArray{"artist"},
	}
}

func trackIDs(cs []contracts.RadioTrackCandidate) []uint {
	ids := make([]uint, len(cs))
	for i, c := range cs {
		ids[i] = c.TrackID
	}
	return ids
}

func TestInRadioTrackCandidateView(t *testing.T) {
	candidates := map[string]bool{"new band": true, "new band two": true}
	unlinked, unmatched := contracts.RadioTrackCandidateViewUnlinked, contracts.RadioTrackCandidateViewUnmatched
	cases := []struct {
		name          string
		keys          []string
		artists       []int64
		release       bool
		wantUnlinked  bool
		wantUnmatched bool
	}{
		{"linked artist, no release", []string{"known"}, []int64{7}, false, true, false},
		{"linked artist with a release link", []string{"known"}, []int64{7}, true, false, false},
		{"one linked play among unlinked ones", []string{"known", "new band"}, []int64{7}, false, true, false},
		{"unlinked, artist candidate", []string{"new band"}, nil, false, false, true},
		{"unlinked, a release link does not matter", []string{"new band"}, nil, true, false, true},
		{"unlinked, every key a candidate", []string{"new band", "new band two"}, nil, false, false, true},
		{"unlinked, one key a candidate", []string{"new band", "catalog name"}, nil, false, false, true},
		{"unlinked, no key a candidate", []string{"catalog name"}, nil, false, false, false},
		{"no keys", nil, nil, false, false, false},
	}
	for _, c := range cases {
		row := radioTrackCandidateRow{ArtistKeys: c.keys, ArtistIDs: c.artists, ReleaseLinked: c.release}
		if got := inRadioTrackCandidateView(row, unlinked, candidates); got != c.wantUnlinked {
			t.Errorf("%s: unlinked = %v, want %v", c.name, got, c.wantUnlinked)
		}
		if got := inRadioTrackCandidateView(row, unmatched, candidates); got != c.wantUnmatched {
			t.Errorf("%s: unmatched = %v, want %v", c.name, got, c.wantUnmatched)
		}
	}
	if inRadioTrackCandidateView(radioTrackCandidateRow{ArtistKeys: []string{"new band"}}, unmatched, nil) {
		t.Error("without the artist candidate keys no unlinked-artist track is in the unmatched view")
	}
	if got := rankRadioTrackCandidates([]radioTrackCandidateRow{trackFixtureRow(1, 2, 3, "2026-05-01")}, nil, contracts.RadioCandidateSortScore); got[0].ArtistCandidateKeys == nil || len(got[0].ArtistCandidateKeys) != 0 {
		t.Errorf("without candidate keys: got %v, want an empty list", got[0].ArtistCandidateKeys)
	}
	if inRadioTrackCandidateView(radioTrackCandidateRow{ArtistIDs: []int64{7}}, "bogus", candidates) {
		t.Error("an unknown view holds no track")
	}
}

func TestRankRadioTrackCandidates(t *testing.T) {
	const release = "11111111-1111-4111-8111-111111111111"
	nonLatin := trackFixtureRow(6, 3, 10, "2026-07-02", release)
	nonLatin.NonLatinSpelling = true
	variant := trackFixtureRow(7, 2, 3, "2026-05-01", release)
	variant.VariantReleaseID = true
	rows := []radioTrackCandidateRow{
		trackFixtureRow(1, 2, 100, "2026-07-01"),
		trackFixtureRow(2, 6, 5, "2026-07-01", release),
		// Same counts as track 4: the more recent play ranks first.
		trackFixtureRow(3, 3, 10, "2026-06-01"),
		trackFixtureRow(4, 3, 10, "2026-07-02"),
		// Same counts and date as track 4: the lower id ranks first.
		trackFixtureRow(5, 3, 10, "2026-07-02", "a", "b"),
		nonLatin,
		variant,
	}
	keys := map[string]bool{"artist": true}

	byScore := rankRadioTrackCandidates(rows, keys, contracts.RadioCandidateSortScore)
	// track 1: 2 x ln 100 = 9.21; track 2: 6 x ln 5 = 9.66; the tie group 3 x ln 10 = 6.91; track 7: 2 x ln 3 = 2.20.
	if got, want := trackIDs(byScore), []uint{2, 1, 4, 5, 6, 3, 7}; !slices.Equal(got, want) {
		t.Errorf("score order = %v, want %v", got, want)
	}
	if got, want := trackIDs(rankRadioTrackCandidates(rows, keys, contracts.RadioCandidateSortPlays)), []uint{1, 4, 5, 6, 3, 2, 7}; !slices.Equal(got, want) {
		t.Errorf("plays order = %v, want %v", got, want)
	}
	if got, want := trackIDs(rankRadioTrackCandidates(rows, keys, contracts.RadioCandidateSortStations)), []uint{2, 4, 5, 6, 3, 1, 7}; !slices.Equal(got, want) {
		t.Errorf("stations order = %v, want %v", got, want)
	}

	for _, c := range byScore {
		switch c.TrackID {
		case 2:
			if c.Lane != contracts.RadioCandidateLaneAuto || c.MBReleaseID == nil || *c.MBReleaseID != release {
				t.Errorf("single release id: got lane %q id %v, want auto with the id", c.Lane, c.MBReleaseID)
			}
		case 5:
			if c.Lane != contracts.RadioCandidateLaneHuman || c.MBReleaseID != nil || len(c.MBReleaseIDs) != 2 {
				t.Errorf("two release ids: got lane %q id %v ids %v, want human, no id, both ids", c.Lane, c.MBReleaseID, c.MBReleaseIDs)
			}
		case 6:
			if c.Lane != contracts.RadioCandidateLaneHuman || c.MBReleaseID == nil || !c.NonLatinSpelling {
				t.Errorf("non-Latin spelling: got lane %q id %v flag %v, want human with the id and the flag", c.Lane, c.MBReleaseID, c.NonLatinSpelling)
			}
		case 7:
			if c.Lane != contracts.RadioCandidateLaneHuman || c.MBReleaseID == nil || !c.VariantReleaseID {
				t.Errorf("variant release id: got lane %q id %v flag %v, want human with the id and the flag", c.Lane, c.MBReleaseID, c.VariantReleaseID)
			}
		default:
			if c.Lane != contracts.RadioCandidateLaneHuman || c.MBReleaseIDs == nil || len(c.MBReleaseIDs) != 0 {
				t.Errorf("track %d: got lane %q ids %v, want human and an empty id list", c.TrackID, c.Lane, c.MBReleaseIDs)
			}
		}
		if c.TitleSpellings == nil || c.ArtistKeys == nil || c.Artists == nil || c.Stations == nil || c.AlbumNames == nil {
			t.Errorf("track %d: evidence lists must be empty arrays, not nil", c.TrackID)
		}
		if !slices.Equal(c.ArtistCandidateKeys, []string{"artist"}) {
			t.Errorf("track %d: artist candidate keys = %v, want [artist]", c.TrackID, c.ArtistCandidateKeys)
		}
	}
}
