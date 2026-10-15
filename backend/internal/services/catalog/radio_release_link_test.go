package catalog

import "testing"

func TestPickReleaseLink(t *testing.T) {
	uintPtr := func(v uint) *uint { return &v }
	credit := func(id uint, name string) releaseCredit { return releaseCredit{ArtistID: id, ArtistName: name} }
	release := func(id uint, credits ...releaseCredit) releaseLinkCandidate {
		return releaseLinkCandidate{ReleaseID: id, Credits: credits}
	}

	tests := []struct {
		name       string
		artistID   *uint
		artistName string
		candidates []releaseLinkCandidate
		want       *uint
	}{
		{"linked: no candidates", uintPtr(1), "a", nil, nil},
		{"linked: credited release", uintPtr(1), "a", []releaseLinkCandidate{release(10, credit(1, "a"))}, uintPtr(10)},
		{"linked: only another artist's release", uintPtr(1), "a", []releaseLinkCandidate{release(10, credit(2, "b"))}, nil},
		{"linked: skips the lower-id release of another artist", uintPtr(1), "a",
			[]releaseLinkCandidate{release(10, credit(2, "b")), release(11, credit(1, "a"))}, uintPtr(11)},
		{"linked: two credited releases take the lower id", uintPtr(1), "a",
			[]releaseLinkCandidate{release(10, credit(1, "a")), release(11, credit(1, "a"))}, uintPtr(10)},
		{"linked: co-credited release", uintPtr(1), "a",
			[]releaseLinkCandidate{release(10, credit(2, "b"), credit(1, "a"))}, uintPtr(10)},
		{"linked: release without credits", uintPtr(1), "a", []releaseLinkCandidate{release(10)}, nil},
		{"linked: a matching name alone does not count", uintPtr(1), "b", []releaseLinkCandidate{release(10, credit(2, "b"))}, nil},

		{"unlinked: unique title, sole credit, same name", nil, "a", []releaseLinkCandidate{release(10, credit(1, "a"))}, uintPtr(10)},
		{"unlinked: names equal under normalizeName", nil, "NICE NICE!", []releaseLinkCandidate{release(10, credit(1, "Nice Nice"))}, uintPtr(10)},
		{"unlinked: sole credit with another name", nil, "a", []releaseLinkCandidate{release(10, credit(2, "b"))}, nil},
		{"unlinked: empty artist name", nil, "", []releaseLinkCandidate{release(10, credit(1, ""))}, nil},
		{"unlinked: shared title", nil, "a",
			[]releaseLinkCandidate{release(10, credit(1, "a")), release(11, credit(2, "b"))}, nil},
		{"unlinked: two credited artists", nil, "a", []releaseLinkCandidate{release(10, credit(1, "a"), credit(2, "b"))}, nil},
		{"unlinked: release without credits", nil, "a", []releaseLinkCandidate{release(10)}, nil},
		{"unlinked: no candidates", nil, "a", nil, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := pickReleaseLink(tt.artistID, tt.artistName, tt.candidates)
			switch {
			case tt.want == nil && got != nil:
				t.Fatalf("got release %d, want none", *got)
			case tt.want != nil && got == nil:
				t.Fatalf("got none, want release %d", *tt.want)
			case tt.want != nil && *got != *tt.want:
				t.Fatalf("got release %d, want %d", *got, *tt.want)
			}
		})
	}
}
