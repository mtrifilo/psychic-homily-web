package testutil

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

// restoreDefaultTransportAfter puts back the http.DefaultTransport in place
// when the test started.
func restoreDefaultTransportAfter(t *testing.T) {
	t.Helper()
	previous := http.DefaultTransport
	t.Cleanup(func() { http.DefaultTransport = previous })
}

// withGuard installs a guard for one test and returns it with its log.
func withGuard(t *testing.T) (*networkGuard, *bytes.Buffer) {
	t.Helper()
	restoreDefaultTransportAfter(t)
	var log bytes.Buffer
	return installNetworkGuard(&log), &log
}

// countingTransport answers every request with 204 and counts them.
type countingTransport struct{ calls atomic.Int32 }

func (c *countingTransport) RoundTrip(*http.Request) (*http.Response, error) {
	c.calls.Add(1)
	return &http.Response{StatusCode: http.StatusNoContent, Body: http.NoBody, Header: http.Header{}}, nil
}

// closeRecordingBody is a request body that records whether it was closed.
type closeRecordingBody struct {
	io.Reader
	closed atomic.Bool
}

func (b *closeRecordingBody) Close() error {
	b.closed.Store(true)
	return nil
}

func TestNetworkGuard_RefusesNonLoopbackRequestNamingIt(t *testing.T) {
	guard, log := withGuard(t)
	// The default transport underneath must never be reached for this host.
	underneath := &countingTransport{}
	guard.loopback = underneath

	resp, err := http.Get("https://api.example.test/v1/thing?token=secret-value")
	if resp != nil {
		_ = resp.Body.Close()
	}
	if !errors.Is(err, errNonLoopbackRequest) {
		t.Fatalf("err = %v, want errNonLoopbackRequest", err)
	}
	if n := underneath.calls.Load(); n != 0 {
		t.Fatalf("the refused request reached the transport underneath %d time(s)", n)
	}

	refused := guard.refusals()
	if len(refused) != 1 {
		t.Fatalf("refusals = %v, want exactly one", refused)
	}
	want := "GET https://api.example.test/v1/thing (query omitted) from testutil.TestNetworkGuard_RefusesNonLoopbackRequestNamingIt"
	if got := refused[0].String(); got != want {
		t.Fatalf("refusal = %q, want %q", got, want)
	}
	if !strings.Contains(log.String(), want) {
		t.Fatalf("log %q does not name the refused request", log.String())
	}
	if strings.Contains(log.String(), "secret-value") {
		t.Fatal("the query, which can carry a credential, must not be printed by the guard")
	}
	if code := guard.exitCode(0); code != 1 {
		t.Fatalf("exitCode(0) = %d after a refusal, want 1", code)
	}
	if code := guard.exitCode(2); code != 2 {
		t.Fatalf("exitCode(2) = %d after a refusal, want the failing code kept", code)
	}
}

func TestNetworkGuard_NamesTheTestFromASubtestAndSuiteStyleMethod(t *testing.T) {
	guard, log := withGuard(t)
	body := &closeRecordingBody{Reader: strings.NewReader("body")}
	t.Run("subtest", func(t *testing.T) {
		_, _ = http.Post("http://192.0.2.1:1/x", "text/plain", body)
	})
	(&fakeSuite{}).TestSomething()

	refused := guard.refusals()
	if len(refused) != 2 {
		t.Fatalf("refusals = %v, want two", refused)
	}
	if got := refused[0].caller; got != "testutil.TestNetworkGuard_NamesTheTestFromASubtestAndSuiteStyleMethod.func1" {
		t.Errorf("subtest caller = %q", got)
	}
	if !body.closed.Load() {
		t.Error("the refused request's body was not closed")
	}
	if got := refused[1].caller; got != "testutil.(*fakeSuite).TestSomething" {
		t.Errorf("suite method caller = %q", got)
	}
	if got := refused[1].url; got != "https://[2001:db8::1]/y (userinfo, fragment omitted)" {
		t.Errorf("url = %q, want the userinfo and fragment left out", got)
	}
	if strings.Contains(log.String(), "token-user") || strings.Contains(log.String(), "pw") {
		t.Errorf("the userinfo must not be printed: %q", log.String())
	}
}

type fakeSuite struct{}

func (*fakeSuite) TestSomething() {
	req, _ := http.NewRequest(http.MethodDelete, "https://token-user:pw@[2001:db8::1]/y#frag", nil)
	_, _ = http.DefaultClient.Do(req)
}

func TestNetworkGuard_NamesTheGoroutineWhenNoTestIsOnTheStack(t *testing.T) {
	guard, _ := withGuard(t)
	done := make(chan struct{})
	go backgroundFetch(done)
	<-done

	refused := guard.refusals()
	if len(refused) != 1 {
		t.Fatalf("refusals = %v, want one", refused)
	}
	want := "a goroutine with no Test function on its stack, started in testutil.backgroundFetch"
	if refused[0].caller != want {
		t.Fatalf("caller = %q, want %q", refused[0].caller, want)
	}
}

func backgroundFetch(done chan<- struct{}) {
	defer close(done)
	_, _ = http.Get("https://musicbrainz.example.test/ws/2/artist")
}

func TestIsTestFunc_FollowsGoTestNaming(t *testing.T) {
	for name, want := range map[string]bool{
		"pkg.TestFoo":             true,
		"pkg.Test_foo":            true,
		"pkg.Test":                true,
		"pkg.TestFoo.func1":       true,
		"pkg.(*Suite).TestBar":    true,
		"pkg.FuzzParse":           true,
		"pkg.BenchmarkRender":     true,
		"pkg.Testdata":            false,
		"pkg.(*server).Testimony": false,
		"pkg.fetchTest":           false,
		"testing.tRunner":         false,
		"testing.TestMainHelper":  false,
		"TestNoPackage":           false,
	} {
		if got := isTestFunc(name); got != want {
			t.Errorf("isTestFunc(%q) = %v, want %v", name, got, want)
		}
	}
}

func TestNetworkGuard_PassesLoopbackRequests(t *testing.T) {
	guard, log := withGuard(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "loopback")
	}))
	defer srv.Close()

	resp, err := http.Get(srv.URL)
	if err != nil {
		t.Fatalf("GET %s: %v", srv.URL, err)
	}
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if string(body) != "loopback" {
		t.Fatalf("body = %q", body)
	}

	underneath := &countingTransport{}
	guard.loopback = underneath
	loopbackURLs := []string{
		"http://localhost:1/",
		"http://LOCALHOST:1/",
		"http://127.0.0.2:1/",
		"http://[::1]:1/",
		"http://[::1%25lo0]:1/",
		"http://[::ffff:127.0.0.1]:1/",
	}
	for _, rawURL := range loopbackURLs {
		resp, err := http.Get(rawURL)
		if err != nil {
			t.Fatalf("GET %s: %v", rawURL, err)
		}
		_ = resp.Body.Close()
	}
	if n := underneath.calls.Load(); int(n) != len(loopbackURLs) {
		t.Fatalf("loopback requests reaching the transport underneath = %d, want %d", n, len(loopbackURLs))
	}
	if refused := guard.refusals(); len(refused) != 0 || log.Len() != 0 {
		t.Fatalf("loopback requests were refused: %v %q", refused, log.String())
	}
	if code := guard.exitCode(0); code != 0 {
		t.Fatalf("exitCode(0) = %d with nothing refused, want 0", code)
	}
}

func TestNetworkGuard_RefusesHostsThatOnlyLookLocal(t *testing.T) {
	guard, _ := withGuard(t)
	underneath := &countingTransport{}
	guard.loopback = underneath
	for _, rawURL := range []string{
		"http://localhost.example.test/",
		"http://localhost./",
		"http://notlocalhost/",
		"http://127.0.0.1.example.test/",
		"http://0.0.0.0:1/",
		"http://10.0.0.1/",
		"http://[::ffff:8.8.8.8]/",
	} {
		resp, err := http.Get(rawURL)
		if resp != nil {
			_ = resp.Body.Close()
		}
		if !errors.Is(err, errNonLoopbackRequest) {
			t.Errorf("GET %s: err = %v, want errNonLoopbackRequest", rawURL, err)
		}
	}
	if n := underneath.calls.Load(); n != 0 {
		t.Fatalf("%d look-alike request(s) reached the transport underneath", n)
	}
}

func TestNetworkGuard_RecordsConcurrentRefusals(t *testing.T) {
	guard, _ := withGuard(t)
	const n = 16
	var wg sync.WaitGroup
	for range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = http.Get("https://api.example.test/")
		}()
	}
	wg.Wait()
	if got := len(guard.refusals()); got != n {
		t.Fatalf("refusals = %d, want %d", got, n)
	}
}

func TestNetworkGuard_LeavesAnInjectedTransportAlone(t *testing.T) {
	guard, _ := withGuard(t)
	injected := &countingTransport{}
	client := &http.Client{Transport: injected}

	resp, err := client.Get("https://api.example.test/")
	if err != nil {
		t.Fatalf("injected transport: %v", err)
	}
	_ = resp.Body.Close()
	if injected.calls.Load() != 1 {
		t.Fatalf("injected transport calls = %d, want 1", injected.calls.Load())
	}
	if refused := guard.refusals(); len(refused) != 0 {
		t.Fatalf("the guard saw a request sent through an injected transport: %v", refused)
	}
}

func TestNetworkGuard_ATestScopedSwapWinsAndTheGuardReturnsAfterIt(t *testing.T) {
	guard, _ := withGuard(t)
	swapped := &countingTransport{}

	t.Run("swaps the default for its duration", func(t *testing.T) {
		orig := http.DefaultTransport
		http.DefaultTransport = swapped
		t.Cleanup(func() { http.DefaultTransport = orig })

		resp, err := http.Get("https://appleid.example.test/auth/keys")
		if err != nil {
			t.Fatalf("swapped transport: %v", err)
		}
		_ = resp.Body.Close()
	})

	if swapped.calls.Load() != 1 {
		t.Fatalf("swapped transport calls = %d, want 1", swapped.calls.Load())
	}
	resp, err := http.Get("https://appleid.example.test/auth/keys")
	if resp != nil {
		_ = resp.Body.Close()
	}
	if !errors.Is(err, errNonLoopbackRequest) {
		t.Fatalf("after the swap is restored, err = %v, want errNonLoopbackRequest", err)
	}
	if code := guard.exitCode(0); code != 1 {
		t.Fatalf("exitCode(0) = %d, want 1", code)
	}
}

func TestNetworkGuard_FailsTheRunWhenATestLeavesTheDefaultSwapped(t *testing.T) {
	t.Run("http.DefaultTransport", func(t *testing.T) {
		guard, log := withGuard(t)
		http.DefaultTransport = &countingTransport{}
		if code := guard.exitCode(0); code != 1 {
			t.Fatalf("exitCode(0) = %d, want 1", code)
		}
		if !strings.Contains(log.String(), "left http.DefaultTransport replaced with *testutil.countingTransport") {
			t.Fatalf("log = %q", log.String())
		}
	})
	t.Run("http.DefaultClient.Transport", func(t *testing.T) {
		guard, log := withGuard(t)
		http.DefaultClient.Transport = &countingTransport{}
		t.Cleanup(func() { http.DefaultClient.Transport = nil })
		if code := guard.exitCode(0); code != 1 {
			t.Fatalf("exitCode(0) = %d, want 1", code)
		}
		if !strings.Contains(log.String(), "left http.DefaultClient.Transport set to *testutil.countingTransport") {
			t.Fatalf("log = %q", log.String())
		}
	})
}

func TestRunGuarded_GuardsTheRunAndAmendsItsExitCode(t *testing.T) {
	t.Run("a refused request fails a passing run", func(t *testing.T) {
		restoreDefaultTransportAfter(t)
		var log bytes.Buffer
		code := runGuarded(func() int {
			if _, ok := http.DefaultTransport.(*networkGuard); !ok {
				t.Errorf("during the run http.DefaultTransport is %T, want the guard", http.DefaultTransport)
			}
			resp, err := http.Get("https://api.example.test/")
			if resp != nil {
				_ = resp.Body.Close()
			}
			if !errors.Is(err, errNonLoopbackRequest) {
				t.Errorf("err = %v, want errNonLoopbackRequest", err)
			}
			return 0
		}, &log)
		if code != 1 {
			t.Fatalf("code = %d, want 1", code)
		}
		if !strings.Contains(log.String(), "1 non-loopback request(s) refused; failing the package run") {
			t.Fatalf("log = %q, want the summary", log.String())
		}
	})
	t.Run("a clean run keeps its code", func(t *testing.T) {
		restoreDefaultTransportAfter(t)
		var log bytes.Buffer
		if code := runGuarded(func() int { return 0 }, &log); code != 0 {
			t.Fatalf("code = %d, want 0 (log %q)", code, log.String())
		}
		if code := runGuarded(func() int { return 3 }, &log); code != 3 {
			t.Fatalf("code = %d, want 3", code)
		}
	})
}
