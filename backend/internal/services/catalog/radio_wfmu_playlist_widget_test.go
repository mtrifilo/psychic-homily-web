package catalog

import (
	"os"
	"strings"
	"testing"
)

// PSY-1327: WFMU's playlist song cells embed a comment-thread widget — a
// hidden "→" jump button plus a hidden `"Title" by "Artist"` summary span —
// that the old whole-cell text flatten concatenated into every stored track
// title (`Mr. Giant Man → "Mr. Giant Man" by "James Last"`; 117k stage rows).
// Fixture: the real wfmu.org/playlists/shows/165980 page (Strength Through
// Failure, 2026-07-02), captured 2026-07-02 — 20 summary spans present.
func TestParseWFMUPlaylistPage_SkipsHiddenCommentWidget(t *testing.T) {
	body, err := os.ReadFile("testdata/wfmu_playlist_comment_widget.html")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	plays, err := parseWFMUPlaylistPage(body)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(plays) == 0 {
		t.Fatal("no plays parsed from the fixture")
	}

	for _, p := range plays {
		if p.TrackTitle == nil {
			continue
		}
		if strings.Contains(*p.TrackTitle, "→") || strings.Contains(*p.TrackTitle, `" by "`) {
			t.Errorf("track title carries hidden-widget text: %q (artist %q)", *p.TrackTitle, p.ArtistName)
		}
	}

	// The user-reported row, exactly as WFMU displays it.
	found := false
	for _, p := range plays {
		if p.ArtistName == "James Last" {
			found = true
			if p.TrackTitle == nil || *p.TrackTitle != "Mr. Giant Man" {
				t.Errorf("James Last track = %v, want bare \"Mr. Giant Man\"", p.TrackTitle)
			}
			if p.AlbumTitle == nil || *p.AlbumTitle != "Voodoo Party" {
				t.Errorf("James Last album = %v, want \"Voodoo Party\"", p.AlbumTitle)
			}
		}
	}
	if !found {
		t.Error("fixture's first row (James Last) not parsed")
	}
}
