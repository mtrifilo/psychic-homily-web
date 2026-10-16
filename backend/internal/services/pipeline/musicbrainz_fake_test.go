package pipeline

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"time"
)

// mbSearchNoExactMatch is an artist search answer whose only candidate is a
// fuzzy hit: a top score under a name no test fixture uses, so SearchArtist
// finds no match whatever artist is queried.
const mbSearchNoExactMatch = `{"count":1,"offset":0,"artists":[{"id":"65f4f0c5-ef9e-490c-aee3-909e7ae6b2ab","type":"Group","score":100,"name":"Not The Queried Artist","sort-name":"Queried Artist, Not The"}]}`

// fakeMusicBrainz answers MusicBrainz artist searches in-process with
// mbSearchNoExactMatch, so no test depends on a live third-party response. It
// counts the searches it answers and the requests it refuses; a refused request
// is one SearchArtist would never make.
type fakeMusicBrainz struct {
	server     *httptest.Server
	searches   atomic.Int32
	unexpected atomic.Int32
}

func newFakeMusicBrainz() *fakeMusicBrainz {
	f := &fakeMusicBrainz{}
	f.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if r.Method != http.MethodGet || r.URL.Path != "/artist/" ||
			!strings.HasPrefix(q.Get("query"), "artist:") || q.Get("fmt") != "json" ||
			r.Header.Get("User-Agent") != mbUserAgent {
			f.unexpected.Add(1)
			http.Error(w, "unexpected request", http.StatusBadRequest)
			return
		}
		f.searches.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(mbSearchNoExactMatch))
	}))
	return f
}

// client returns a MusicBrainzClient whose requests reach this fake.
func (f *fakeMusicBrainz) client() *MusicBrainzClient {
	return newMusicBrainzTestClient(f.server.URL)
}

// newMusicBrainzTestClient points a client at an httptest server with a fast
// throttle, so a test needs no network and no ~1s wait between requests.
func newMusicBrainzTestClient(baseURL string) *MusicBrainzClient {
	c := NewMusicBrainzClient()
	c.baseURL = baseURL
	c.rateLimit = time.Millisecond
	return c
}
