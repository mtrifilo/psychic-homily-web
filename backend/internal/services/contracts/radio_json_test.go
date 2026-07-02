package contracts

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestRadioWindowJSONTags pins the wire names of the frozen-air-window fields
// (PSY-1298/PSY-1306). The frontend types are hand-maintained string matches
// of these tags with no generated bridge — a typo'd or renamed tag would
// compile, pass every struct-field test, and ship as a silent null that
// quietly reverts every viewer-local surface to station-dated rendering.
func TestRadioWindowJSONTags(t *testing.T) {
	cases := []struct {
		name string
		v    any
		keys []string
	}{
		{"RadioStationEpisodeRow", RadioStationEpisodeRow{}, []string{`"starts_at"`, `"ends_at"`}},
		{"RadioShowListResponse", RadioShowListResponse{}, []string{`"latest_air_date"`, `"latest_starts_at"`, `"latest_ends_at"`}},
		{"RadioNowPlayingResponse", RadioNowPlayingResponse{}, []string{`"episode_air_date"`, `"episode_starts_at"`, `"episode_ends_at"`}},
		{"RadioEpisodeDetailResponse", RadioEpisodeDetailResponse{}, []string{`"starts_at"`, `"ends_at"`, `"station_timezone"`}},
	}
	for _, tc := range cases {
		b, err := json.Marshal(tc.v)
		if err != nil {
			t.Fatalf("%s: marshal: %v", tc.name, err)
		}
		for _, k := range tc.keys {
			if !strings.Contains(string(b), k) {
				t.Errorf("%s: serialized JSON missing key %s (fields are nil-able but never omitempty)", tc.name, k)
			}
		}
	}
}
