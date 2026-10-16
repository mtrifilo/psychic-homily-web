package testutil

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/netip"
	"os"
	"runtime"
	"strings"
	"sync"
	"testing"
	"unicode"
	"unicode/utf8"
)

// errNonLoopbackRequest is the error a request refused by the network guard
// wraps. There is no expected refusal: any refusal fails the package run.
var errNonLoopbackRequest = errors.New("network guard: non-loopback request refused in a test")

// RefuseNonLoopbackNetwork runs the package's tests with http.DefaultTransport
// replaced by a transport that sends loopback requests on and refuses every
// other request without dialing, and returns the exit code for os.Exit. It is
// called from the package's TestMain:
//
//	func TestMain(m *testing.M) { os.Exit(testutil.RefuseNonLoopbackNetwork(m)) }
//
// Loopback means a host of localhost or an IP in 127.0.0.0/8 or ::1, which
// covers every httptest server; a loopback request can still reach a local
// service that is not the test's own. http.DefaultClient, http.Get and any
// client with a nil Transport read http.DefaultTransport per request, so they
// are guarded too. Nothing else is: a client built around its own transport
// (an injected fake, or a production client with its own dialer), a raw
// net.Dial and a DNS lookup all go around the guard.
//
// Each refusal is printed when it happens, naming the method, the URL with its
// userinfo, query and fragment omitted, and the Test function on the requesting
// goroutine's stack. A RoundTripper has no handle on the *testing.T, so the
// guard fails the package run instead of the test: it returns a non-zero code
// when any request was refused, or when a test left http.DefaultTransport or
// http.DefaultClient.Transport swapped, which would have switched the guard
// off for the tests after it. A test that swaps the default transport for its
// own duration and restores it in t.Cleanup composes with the guard.
//
// The guard stays installed after the tests finish, so a goroutine that
// outlives its test is still guarded and never races a restore.
func RefuseNonLoopbackNetwork(m *testing.M) int {
	return runGuarded(m.Run, os.Stderr)
}

// runGuarded installs a guard that logs to log, calls run and returns its exit
// code as the guard amends it.
func runGuarded(run func() int, log io.Writer) int {
	guard := installNetworkGuard(log)
	return guard.exitCode(run())
}

// refusedRequest is one request the guard refused.
type refusedRequest struct {
	method string
	url    string
	caller string
}

func (r refusedRequest) String() string {
	return fmt.Sprintf("%s %s from %s", r.method, r.url, r.caller)
}

// networkGuard is the http.RoundTripper installed as http.DefaultTransport.
type networkGuard struct {
	loopback http.RoundTripper // the transport loopback requests go through

	mu      sync.Mutex // guards log and refused
	log     io.Writer
	refused []refusedRequest
}

// installNetworkGuard makes a new guard, logging to log, http.DefaultTransport.
// Loopback requests go through the default it replaces.
func installNetworkGuard(log io.Writer) *networkGuard {
	guard := &networkGuard{loopback: http.DefaultTransport, log: log}
	http.DefaultTransport = guard
	return guard
}

func (g *networkGuard) RoundTrip(req *http.Request) (*http.Response, error) {
	if isLoopbackHost(req.URL.Hostname()) {
		return g.loopback.RoundTrip(req)
	}
	// A RoundTripper must close the request body, even on error.
	if req.Body != nil {
		_ = req.Body.Close()
	}
	refused := refusedRequest{method: req.Method, url: redactedURL(req), caller: requestingTest()}
	g.mu.Lock()
	g.refused = append(g.refused, refused)
	_, _ = fmt.Fprintf(g.log, "network guard: refused %s (requests through http.DefaultTransport in tests may reach loopback hosts only: inject a transport or use an httptest server)\n", refused)
	g.mu.Unlock()
	return nil, fmt.Errorf("%w: %s", errNonLoopbackRequest, refused)
}

// refusals returns a copy of every request refused so far.
func (g *networkGuard) refusals() []refusedRequest {
	g.mu.Lock()
	defer g.mu.Unlock()
	return append([]refusedRequest(nil), g.refused...)
}

// exitCode is code, or 1 when code is 0 and the run refused a request or left
// the default transport swapped. Each reason is printed.
func (g *networkGuard) exitCode(code int) int {
	g.mu.Lock()
	defer g.mu.Unlock()
	failed := false
	// The summary repeats the refusals printed as they happened, because under
	// -v those lines are interleaved with every test's output.
	if len(g.refused) > 0 {
		failed = true
		_, _ = fmt.Fprintf(g.log, "network guard: %d non-loopback request(s) refused; failing the package run (no test is marked FAIL for them):\n", len(g.refused))
		for _, r := range g.refused {
			_, _ = fmt.Fprintf(g.log, "\t%s\n", r)
		}
	}
	if http.DefaultTransport != http.RoundTripper(g) {
		failed = true
		_, _ = fmt.Fprintf(g.log, "network guard: a test left http.DefaultTransport replaced with %T; failing the package run\n", http.DefaultTransport)
	}
	if http.DefaultClient.Transport != nil {
		failed = true
		_, _ = fmt.Fprintf(g.log, "network guard: a test left http.DefaultClient.Transport set to %T; failing the package run\n", http.DefaultClient.Transport)
	}
	if failed && code == 0 {
		return 1
	}
	return code
}

// isLoopbackHost reports whether host, as returned by url.URL.Hostname, names
// the loopback interface.
func isLoopbackHost(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	addr, err := netip.ParseAddr(host)
	return err == nil && addr.WithZone("").Unmap().IsLoopback()
}

// redactedURL is the request URL without its userinfo, query and fragment,
// any of which can carry a credential, marking what was left out.
func redactedURL(req *http.Request) string {
	u := *req.URL
	var omitted []string
	if u.User != nil {
		omitted = append(omitted, "userinfo")
	}
	if u.RawQuery != "" || u.ForceQuery {
		omitted = append(omitted, "query")
	}
	if u.Fragment != "" {
		omitted = append(omitted, "fragment")
	}
	u.User, u.RawQuery, u.ForceQuery, u.Fragment, u.RawFragment = nil, "", false, "", ""
	if len(omitted) == 0 {
		return u.String()
	}
	return u.String() + " (" + strings.Join(omitted, ", ") + " omitted)"
}

// requestingTest names the innermost Test function on the calling goroutine's
// stack, or, for a goroutine a test started, the function the goroutine began
// in.
func requestingTest() string {
	pcs := make([]uintptr, 256)
	n := runtime.Callers(2, pcs)
	for n == len(pcs) {
		pcs = make([]uintptr, 2*len(pcs))
		n = runtime.Callers(2, pcs)
	}
	frames := runtime.CallersFrames(pcs[:n])
	outermost := ""
	for {
		frame, more := frames.Next()
		name := shortFuncName(frame.Function)
		if isTestFunc(name) {
			return name
		}
		if name != "" && !strings.HasPrefix(frame.Function, "runtime.") {
			outermost = name
		}
		if !more {
			break
		}
	}
	if outermost == "" {
		return "an unknown goroutine (no Test function on its stack)"
	}
	return "a goroutine with no Test function on its stack, started in " + outermost
}

// shortFuncName trims the import path from a runtime function name, leaving
// pkg.Func, pkg.(*Type).Method or pkg.Func.func1.
func shortFuncName(full string) string {
	return full[strings.LastIndex(full, "/")+1:]
}

// isTestFunc reports whether name, as returned by shortFuncName, is a test
// function, a test suite method or a closure inside one, outside the testing
// package itself.
func isTestFunc(name string) bool {
	parts := strings.Split(name, ".")
	if len(parts) < 2 || parts[0] == "testing" {
		return false
	}
	for _, p := range parts[1:] {
		if hasTestPrefix(p, "Test") || hasTestPrefix(p, "Fuzz") || hasTestPrefix(p, "Benchmark") {
			return true
		}
	}
	return false
}

// hasTestPrefix applies go test's naming rule: name is prefix alone, or prefix
// followed by a character that is not a lower-case letter, so TestFoo and
// Test_foo count and Testdata does not.
func hasTestPrefix(name, prefix string) bool {
	if !strings.HasPrefix(name, prefix) {
		return false
	}
	if len(name) == len(prefix) {
		return true
	}
	r, _ := utf8.DecodeRuneInString(name[len(prefix):])
	return !unicode.IsLower(r)
}
