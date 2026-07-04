package catalog

import "testing"

func TestSplitCollabArtistName(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want []string
	}{
		{"comma collab", "Astrid Sonne, Smerz", []string{"Astrid Sonne", "Smerz"}},
		{"ampersand collab", "Hooky & Winter", []string{"Hooky", "Winter"}},
		{"and collab", "Artist One and Artist Two", []string{"Artist One", "Artist Two"}},
		{"single artist", "Boy Harsher", nil},
		{"short part rejected", "AB, CD", nil},
		{"comma plus ampersand single act", "Earth, Wind & Fire", nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := splitCollabArtistName(tt.in)
			if tt.want == nil {
				if got != nil {
					t.Fatalf("splitCollabArtistName(%q) = %v, want nil", tt.in, got)
				}
				return
			}
			if len(got) != len(tt.want) {
				t.Fatalf("splitCollabArtistName(%q) = %v, want %v", tt.in, got, tt.want)
			}
			for i := range tt.want {
				if got[i] != tt.want[i] {
					t.Fatalf("part %d: got %q want %q (full %v)", i, got[i], tt.want[i], got)
				}
			}
		})
	}
}

func TestCollabPlayMentionsArtist(t *testing.T) {
	if !collabPlayMentionsArtist("zzzahara, Winter", normalizeName("Winter")) {
		t.Fatal("expected Winter mention in collab play")
	}
	if collabPlayMentionsArtist("Boy Harsher", normalizeName("Winter")) {
		t.Fatal("single artist play should not mention Winter")
	}
}

func TestNormalizeName_EarthWindFireNotCollidingWithCollabSplit(t *testing.T) {
	if parts := splitCollabArtistName("Earth, Wind & Fire"); parts != nil {
		t.Fatalf("comma+ampersand act must not collab-split, got %v", parts)
	}
}
