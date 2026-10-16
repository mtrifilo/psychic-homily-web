package testutil

import (
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"runtime"
	"strings"
	"sync"
	"testing"
)

// ErrNonLoopbackRequest is the error a request refused by the network guard
// wraps.
var ErrNonLoopbackRequest = errors.New("network guard: non-loopback request refused in a test")

// RefuseNonLoopbackNetwork runs the package's tests with http.DefaultTransport
// replaced by a transport that sends loopback requests on and refuses every
// other request without dialing, and returns the exit code for os.Exit. It is
// called from the package's TestMain:
//
//	func TestMain(m *testing.M) { os.Exit(testutil.RefuseNonLoopbackNetwork(m)) }
//
// Loopback means a host of localhost or an IP in 127.0.0.0/8 or ::1, which
// covers every httptest server. http.DefaultClient, http.Get and any client
// with a nil Transport read http.DefaultTransport per request, so they are
// guarded too. A client built around its own transport is not: the guard
// covers the default only, so an explicitly injected fake answers as usual.
//
// Each refusal is printed when it happens, naming the method, the URL with its
// query omitted, and the Test function on the requesting goroutine's stack. A
// RoundTripper has no handle on the *testing.T, so the guard fails the package
// run instead of the test: it returns a non-zero code when any request was
// refused, or when a test left http.DefaultTransport or
// http.DefaultClient.Transport swapped, which would have switched the guard
// off for the tests after it. A test that swaps the default transport for its
// own duration and restores it in t.Cleanup composes with the guard.
func RefuseNonLoopbackNetwork(m *testing.M) int {
	guard, restore := installNetworkGuard(os.Stderr)
	code := m.Run()
	code = guard.exitCode(code)
	restore()
	return code
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
	log      io.Writer

	mu      sync.Mutex
	refused []refusedRequest
}

// installNetworkGuard makes a new guard http.DefaultTransport and returns it
// with the function that puts the previous default back.
func installNetworkGuard(log io.Writer) (*networkGuard, func()) {
	previous := http.DefaultTransport
	guard := &networkGuard{loopback: previous, log: log}
	http.DefaultTransport = guard
	return guard, func() { http.DefaultTransport = previous }
}

func (g *networkGuard) RoundTrip(req *http.Request) (*http.Response, error) {
	if isLoopbackHost(req.URL.Hostname()) {
		return g.loopback.RoundTrip(req)
	}
	// A RoundTripper must close the request body, even on error.
	if req.Body != nil {
		_ = req.Body.Close()
	}
	refused := refusedRequest{method: req.Method, url: urlWithoutQuery(req), caller: requestingTest()}
	g.mu.Lock()
	g.refused = append(g.refused, refused)
	g.mu.Unlock()
	_, _ = fmt.Fprintf(g.log, "network guard: refused %s (tests may reach loopback hosts only: inject a transport or use an httptest server)\n", refused)
	return nil, fmt.Errorf("%w: %s", ErrNonLoopbackRequest, refused)
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
	failed := false
	if refused := g.refusals(); len(refused) > 0 {
		failed = true
		_, _ = fmt.Fprintf(g.log, "network guard: %d non-loopback request(s) refused; failing the package run:\n", len(refused))
		for _, r := range refused {
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
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// urlWithoutQuery is the request URL with any userinfo password redacted and
// the query left out, since a query can carry a credential.
func urlWithoutQuery(req *http.Request) string {
	u := *req.URL
	hadQuery := u.RawQuery != "" || u.ForceQuery
	u.RawQuery, u.ForceQuery, u.Fragment, u.RawFragment = "", false, "", ""
	s := u.Redacted()
	if hadQuery {
		s += "?(query omitted)"
	}
	return s
}

// requestingTest names the innermost Test function on the calling goroutine's
// stack, or, for a goroutine a test started, the function the goroutine began
// in.
func requestingTest() string {
	pcs := make([]uintptr, 128)
	frames := runtime.CallersFrames(pcs[:runtime.Callers(2, pcs)])
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
		if strings.HasPrefix(p, "Test") || strings.HasPrefix(p, "Fuzz") || strings.HasPrefix(p, "Benchmark") {
			return true
		}
	}
	return false
}
