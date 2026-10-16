package auth

import (
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"reflect"
	"regexp"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"
)

// pwnedPasswordsRangePath is the only path the breach check requests: /range/
// and the first five hex characters of the password's SHA-1.
var pwnedPasswordsRangePath = regexp.MustCompile(`^/range/[0-9A-F]{5}$`)

// pwnedPasswordsOtherLines end every range answer: a breached suffix and a
// zero-count padding entry, neither of which any password in these tests hashes
// to. "Suffix absent" is therefore decided against a range that lists other
// breached suffixes, the shape a real range has.
const pwnedPasswordsOtherLines = "0000000000000000000000000000000000A:3\r\n" +
	"0000000000000000000000000000000000B:0\r\n"

// fakePwnedPasswords answers Pwned Passwords range requests in-process, the way
// api.pwnedpasswords.com does, so no test depends on a live third-party
// response. It counts the requests it receives and fails the test on any
// request the breach check would never make, including one that carries
// anything beyond the five-character prefix: a query, a body, or headers other
// than pwnedPasswordsRequestHeaders.
type fakePwnedPasswords struct {
	t       *testing.T
	respond func(prefix string) (*http.Response, error)
	calls   atomic.Int32
}

func (f *fakePwnedPasswords) RoundTrip(r *http.Request) (*http.Response, error) {
	f.calls.Add(1)
	if !isPwnedPasswordsRangeRequest(r) {
		f.t.Errorf("unexpected Pwned Passwords request: %s %s %v", r.Method, r.URL, r.Header)
		return nil, errors.New("unexpected Pwned Passwords request")
	}
	return f.respond(strings.TrimPrefix(r.URL.Path, "/range/"))
}

// pwnedPasswordsRequestHeaders is the exact header set of the range request.
var pwnedPasswordsRequestHeaders = http.Header{
	"User-Agent":  {"PsychicHomily-PasswordCheck"},
	"Add-Padding": {"true"},
}

// isPwnedPasswordsRangeRequest reports whether r is exactly the k-anonymity
// range request: a GET of the prefix path with exactly
// pwnedPasswordsRequestHeaders, and nothing else that could carry the hash.
func isPwnedPasswordsRangeRequest(r *http.Request) bool {
	if r.Method != http.MethodGet || r.URL.Host != "api.pwnedpasswords.com" ||
		!pwnedPasswordsRangePath.MatchString(r.URL.Path) || r.URL.RawQuery != "" {
		return false
	}
	if r.Body != nil && r.Body != http.NoBody {
		return false
	}
	return reflect.DeepEqual(r.Header, pwnedPasswordsRequestHeaders)
}

// dialRefused returns the error a refused TCP dial fails with, before any HTTP
// response exists.
func dialRefused() error {
	return &net.OpError{Op: "dial", Net: "tcp", Err: syscall.ECONNREFUSED}
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
		body.WriteString(pwnedPasswordsOtherLines)
		return &http.Response{
			StatusCode: http.StatusOK,
			Status:     "200 OK",
			Header:     http.Header{"Content-Type": []string{"text/plain"}},
			Body:       io.NopCloser(strings.NewReader(body.String())),
		}, nil
	}}
}

// pwnedPasswordsUnreachable returns a fake whose every request fails the way a
// refused TCP dial fails.
func pwnedPasswordsUnreachable(t *testing.T) *fakePwnedPasswords {
	return &fakePwnedPasswords{t: t, respond: func(string) (*http.Response, error) {
		return nil, dialRefused()
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
