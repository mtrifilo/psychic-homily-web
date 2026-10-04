package venueaddress

import (
	"context"
	"crypto/x509"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"psychic-homily-backend/internal/utils/urlguard"
)

// testFetcher talks to httptest servers on loopback, which the production
// dial guard refuses, so it uses a plain client.
func testFetcher() *Fetcher {
	return newFetcher(&http.Client{Timeout: 5 * time.Second}, time.Millisecond)
}

func TestFetch_ReadsPageWithIdentifyingUserAgent(t *testing.T) {
	var ua string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/robots.txt" {
			http.NotFound(w, r)
			return
		}
		ua = r.Header.Get("User-Agent")
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte("<p>2424 N Lincoln Ave</p>"))
	}))
	defer srv.Close()

	page, err := testFetcher().Fetch(context.Background(), srv.URL+"/")
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if !strings.Contains(string(page.HTML), "2424 N Lincoln Ave") {
		t.Fatalf("body = %q", page.HTML)
	}
	if !strings.HasPrefix(ua, "PsychicHomily/1.0 (venue-address-backfill;") {
		t.Fatalf("User-Agent = %q", ua)
	}
}

func TestFetch_Classification(t *testing.T) {
	tests := []struct {
		name        string
		robots      string
		status      int
		contentType string
		unavailable bool // definitive
		transient   bool
	}{
		{name: "robots disallow", robots: "User-agent: *\nDisallow: /", status: 200, contentType: "text/html", unavailable: true},
		{name: "robots names us", robots: "User-agent: *\nAllow: /\n\nUser-agent: PsychicHomily\nDisallow: /", status: 200, contentType: "text/html", unavailable: true},
		{name: "403 is definitive", status: 403, contentType: "text/html", unavailable: true},
		{name: "404 is definitive", status: 404, contentType: "text/html", unavailable: true},
		{name: "non-HTML is definitive", status: 200, contentType: "application/pdf", unavailable: true},
		{name: "500 is transient", status: 500, contentType: "text/html", transient: true},
		{name: "429 is transient", status: 429, contentType: "text/html", transient: true},
		{name: "ok", status: 200, contentType: "text/html"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/robots.txt" {
					if tt.robots == "" {
						http.NotFound(w, r)
						return
					}
					_, _ = w.Write([]byte(tt.robots))
					return
				}
				w.Header().Set("Content-Type", tt.contentType)
				w.WriteHeader(tt.status)
				_, _ = w.Write([]byte("<p>x</p>"))
			}))
			defer srv.Close()

			_, err := testFetcher().Fetch(context.Background(), srv.URL+"/page")
			switch {
			case tt.unavailable:
				if !errors.Is(err, ErrUnavailable) {
					t.Fatalf("err = %v, want ErrUnavailable", err)
				}
			case tt.transient:
				if err == nil || errors.Is(err, ErrUnavailable) {
					t.Fatalf("err = %v, want a transient error", err)
				}
			default:
				if err != nil {
					t.Fatalf("err = %v", err)
				}
			}
		})
	}
}

func TestFetch_RobotsServerErrorIsTransient(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()
	_, err := testFetcher().Fetch(context.Background(), srv.URL+"/")
	if err == nil || errors.Is(err, ErrUnavailable) {
		t.Fatalf("err = %v, want transient", err)
	}
}

func TestFetch_SpacesRequestsToOneHost(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte("<p>x</p>"))
	}))
	defer srv.Close()

	f := newFetcher(&http.Client{Timeout: 5 * time.Second}, 150*time.Millisecond)
	start := time.Now()
	for i := 0; i < 2; i++ {
		if _, err := f.Fetch(context.Background(), srv.URL+"/"); err != nil {
			t.Fatal(err)
		}
	}
	// robots.txt + page + page: three requests, two waits.
	if elapsed := time.Since(start); elapsed < 300*time.Millisecond {
		t.Fatalf("three requests to one host took %v, want at least two intervals", elapsed)
	}
	if hits != 3 {
		t.Fatalf("hits = %d, want 3 (robots.txt cached after the first)", hits)
	}
}

func TestFetch_ProductionDialerRefusesLoopback(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("a loopback server must never be reached")
	}))
	defer srv.Close()
	_, err := NewFetcher().Fetch(context.Background(), srv.URL+"/")
	if err == nil || !strings.Contains(err.Error(), "ssrf guard") {
		t.Fatalf("err = %v, want an ssrf guard refusal", err)
	}
	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("a non-public host is permanent, so the refusal must be definitive: %v", err)
	}
}

func TestFetch_FollowsRedirectsThroughTheSameChecks(t *testing.T) {
	var hops int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/robots.txt":
			_, _ = w.Write([]byte("User-agent: *\nDisallow: /private"))
		case "/start":
			atomic.AddInt32(&hops, 1)
			http.Redirect(w, r, "/final", http.StatusFound)
		case "/final":
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write([]byte("<p>ok</p>"))
		case "/to-private":
			http.Redirect(w, r, "/private/page", http.StatusMovedPermanently)
		case "/private/page":
			t.Error("a robots-disallowed redirect target must never be fetched")
		case "/to-instagram":
			http.Redirect(w, r, "https://www.instagram.com/somevenue", http.StatusFound)
		case "/loop":
			http.Redirect(w, r, "/loop", http.StatusFound)
		}
	}))
	defer srv.Close()
	f := testFetcher()

	page, err := f.Fetch(context.Background(), srv.URL+"/start")
	if err != nil || !strings.HasSuffix(page.URL, "/final") || hops != 1 {
		t.Fatalf("page=%+v err=%v hops=%d", page, err, hops)
	}
	for _, path := range []string{"/to-private", "/to-instagram", "/loop"} {
		if _, err := f.Fetch(context.Background(), srv.URL+path); !errors.Is(err, ErrUnavailable) {
			t.Errorf("%s: err = %v, want ErrUnavailable", path, err)
		}
	}
}

func TestDefinitiveTransportError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"unresolvable name", &net.DNSError{Err: "no such host", Name: "gone.example", IsNotFound: true}, true},
		{"resolver timeout", &net.DNSError{Err: "timeout", Name: "slow.example", IsTimeout: true}, false},
		{"dial guard", fmt.Errorf("dial: %w", urlguard.ErrNonPublicAddress), true},
		{"bad certificate", x509.HostnameError{Host: "wrong.example", Certificate: &x509.Certificate{}}, true},
		{"refused", errors.New("connect: connection refused"), false},
	}
	for _, tt := range tests {
		if got := definitiveTransportError(tt.err); got != tt.want {
			t.Errorf("%s: definitive = %v, want %v", tt.name, got, tt.want)
		}
	}
}

func TestLoginWalled(t *testing.T) {
	for host, want := range map[string]bool{
		"www.instagram.com": true, "m.facebook.com": true, "x.com": true,
		"tickets.example": false, "notinstagram.com": false,
	} {
		if got := LoginWalled(host); got != want {
			t.Errorf("LoginWalled(%q) = %v, want %v", host, got, want)
		}
	}
}

func TestFetch_RejectsNonHTTPURL(t *testing.T) {
	_, err := testFetcher().Fetch(context.Background(), "file:///etc/passwd")
	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("err = %v, want ErrUnavailable", err)
	}
}

func TestRobots(t *testing.T) {
	body := `
# comment
User-agent: Googlebot
Disallow: /

User-agent: *
Disallow: /private
Allow: /private/ok
Disallow: /*.pdf$
`
	r := parseRobots(body, "psychichomily")
	tests := []struct {
		path string
		want bool
	}{
		{"/", true},
		{"/private", false},
		{"/private/page", false},
		{"/private/ok/page", true},
		{"/files/a.pdf", false},
		{"/files/a.pdf?x=1", true},
	}
	for _, tt := range tests {
		if got := r.allows(tt.path); got != tt.want {
			t.Errorf("allows(%q) = %v, want %v", tt.path, got, tt.want)
		}
	}
	if parseRobots("User-agent: *\nDisallow:", "psychichomily").allows("/x") != true {
		t.Error("an empty Disallow allows everything")
	}
	var none *robotsRules
	if !none.allows("/anything") {
		t.Error("no robots.txt allows everything")
	}
}
