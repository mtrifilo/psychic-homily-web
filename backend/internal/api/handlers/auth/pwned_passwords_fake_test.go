package auth

import (
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"regexp"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"
)

// pwnedPasswordsRangePath is the only path the breach check requests: the first
// five hex characters of the password's SHA-1, never the whole hash.
var pwnedPasswordsRangePath = regexp.MustCompile(`^/range/[0-9A-F]{5}$`)

// pwnedPasswordsPaddingLine is a range entry no password in these tests hashes
// to. Every answer carries it, so "suffix absent" is decided by reading a
// non-empty range rather than an empty one.
const pwnedPasswordsPaddingLine = "0000000000000000000000000000000000A:0\r\n"

// fakePwnedPasswords answers Pwned Passwords range requests in-process, the way
// api.pwnedpasswords.com does, so no test depends on a live third-party
// response. It fails the test on a request the breach check would never make,
// and counts the requests it answers.
type fakePwnedPasswords struct {
	t       *testing.T
	respond func(prefix string) (*http.Response, error)
	calls   atomic.Int32
}

func (f *fakePwnedPasswords) RoundTrip(r *http.Request) (*http.Response, error) {
	f.calls.Add(1)
	if r.Method != http.MethodGet || r.URL.Host != "api.pwnedpasswords.com" || !pwnedPasswordsRangePath.MatchString(r.URL.Path) {
		f.t.Errorf("unexpected Pwned Passwords request: %s %s", r.Method, r.URL)
		return nil, errors.New("unexpected Pwned Passwords request")
	}
	return f.respond(strings.TrimPrefix(r.URL.Path, "/range/"))
}

// pwnedPasswordsListing returns a fake whose ranges list the SHA-1 suffix of
// each breached password, so the check finds exactly those passwords and no
// other.
func pwnedPasswordsListing(t *testing.T, breached ...string) *fakePwnedPasswords {
	return &fakePwnedPasswords{t: t, respond: func(prefix string) (*http.Response, error) {
		var body strings.Builder
		for _, password := range breached {
			sum := sha1.Sum([]byte(password))
			hash := strings.ToUpper(hex.EncodeToString(sum[:]))
			if hash[:5] == prefix {
				fmt.Fprintf(&body, "%s:%d\r\n", hash[5:], 42)
			}
		}
		body.WriteString(pwnedPasswordsPaddingLine)
		return &http.Response{
			StatusCode: http.StatusOK,
			Status:     "200 OK",
			Header:     http.Header{"Content-Type": []string{"text/plain"}},
			Body:       io.NopCloser(strings.NewReader(body.String())),
		}, nil
	}}
}

// pwnedPasswordsUnreachable returns a fake whose every request fails the way a
// refused TCP dial fails, before any HTTP response exists.
func pwnedPasswordsUnreachable(t *testing.T) *fakePwnedPasswords {
	return &fakePwnedPasswords{t: t, respond: func(string) (*http.Response, error) {
		return nil, &net.OpError{Op: "dial", Net: "tcp", Err: syscall.ECONNREFUSED}
	}}
}

// pwnedPasswordsForbidden returns a fake that fails the test on any request,
// for a handler that must not reach the breach check.
func pwnedPasswordsForbidden(t *testing.T) *fakePwnedPasswords {
	return &fakePwnedPasswords{t: t, respond: func(prefix string) (*http.Response, error) {
		t.Errorf("unexpected breach check for range %s", prefix)
		return nil, errors.New("unexpected breach check")
	}}
}
