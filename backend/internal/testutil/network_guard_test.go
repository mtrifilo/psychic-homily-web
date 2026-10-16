package testutil

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

// withGuard installs a guard for one test and returns it with its log.
func withGuard(t *testing.T) (*networkGuard, *bytes.Buffer) {
	t.Helper()
	var log bytes.Buffer
	guard, restore := installNetworkGuard(&log)
	t.Cleanup(restore)
	return guard, &log
}

// countingTransport answers every request with 204 and counts them.
type countingTransport struct{ calls atomic.Int32 }

func (c *countingTransport) RoundTrip(*http.Request) (*http.Response, error) {
	c.calls.Add(1)
	return &http.Response{StatusCode: http.StatusNoContent, Body: http.NoBody, Header: http.Header{}}, nil
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
	if !errors.Is(err, ErrNonLoopbackRequest) {
		t.Fatalf("err = %v, want ErrNonLoopbackRequest", err)
	}
	if n := underneath.calls.Load(); n != 0 {
		t.Fatalf("the refused request reached the transport underneath %d time(s)", n)
	}

	refused := guard.refusals()
	if len(refused) != 1 {
		t.Fatalf("refusals = %v, want exactly one", refused)
	}
	want := "GET https://api.example.test/v1/thing?(query omitted) from testutil.TestNetworkGuard_RefusesNonLoopbackRequestNamingIt"
	if got := refused[0].String(); got != want {
		t.Fatalf("refusal = %q, want %q", got, want)
	}
	if !strings.Contains(log.String(), want) {
		t.Fatalf("log %q does not name the refused request", log.String())
	}
	if strings.Contains(log.String()+refused[0].String(), "secret-value") {
		t.Fatal("the query, which can carry a credential, must not be printed by the guard")
	}
	if code := guard.exitCode(0); code != 1 {
		t.Fatalf("exitCode(0) = %d after a refusal, want 1", code)
	}
}

func TestNetworkGuard_NamesTheTestFromASubtestAndSuiteStyleMethod(t *testing.T) {
	guard, _ := withGuard(t)
	t.Run("subtest", func(t *testing.T) {
		_, _ = http.Post("http://192.0.2.1:1/x", "text/plain", strings.NewReader("body"))
	})
	(&fakeSuite{}).TestSomething()

	refused := guard.refusals()
	if len(refused) != 2 {
		t.Fatalf("refusals = %v, want two", refused)
	}
	if got := refused[0].caller; got != "testutil.TestNetworkGuard_NamesTheTestFromASubtestAndSuiteStyleMethod.func1" {
		t.Errorf("subtest caller = %q", got)
	}
	if got := refused[1].caller; got != "testutil.(*fakeSuite).TestSomething" {
		t.Errorf("suite method caller = %q", got)
	}
}

type fakeSuite struct{}

func (*fakeSuite) TestSomething() {
	req, _ := http.NewRequest(http.MethodDelete, "https://user:pw@[2001:db8::1]/y", nil)
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
	for _, rawURL := range []string{"http://localhost:1/", "http://LOCALHOST:1/", "http://127.0.0.2:1/", "http://[::1]:1/"} {
		resp, err := http.Get(rawURL)
		if err != nil {
			t.Fatalf("GET %s: %v", rawURL, err)
		}
		_ = resp.Body.Close()
	}
	if n := underneath.calls.Load(); n != 4 {
		t.Fatalf("loopback requests reaching the transport underneath = %d, want 4", n)
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
	guard.loopback = &countingTransport{}
	for _, rawURL := range []string{
		"http://localhost.example.test/",
		"http://notlocalhost/",
		"http://0.0.0.0:1/",
		"http://10.0.0.1/",
		"http://[::ffff:8.8.8.8]/",
	} {
		resp, err := http.Get(rawURL)
		if resp != nil {
			_ = resp.Body.Close()
		}
		if !errors.Is(err, ErrNonLoopbackRequest) {
			t.Errorf("GET %s: err = %v, want ErrNonLoopbackRequest", rawURL, err)
		}
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
	if !errors.Is(err, ErrNonLoopbackRequest) {
		t.Fatalf("after the swap is restored, err = %v, want ErrNonLoopbackRequest", err)
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

func TestNetworkGuard_KeepsAFailingCode(t *testing.T) {
	guard, _ := withGuard(t)
	if code := guard.exitCode(2); code != 2 {
		t.Fatalf("exitCode(2) = %d, want 2", code)
	}
}

func TestNetworkGuard_RestoreReinstatesThePreviousDefault(t *testing.T) {
	previous := http.DefaultTransport
	_, restore := installNetworkGuard(io.Discard)
	if http.DefaultTransport == previous {
		t.Fatal("install did not replace http.DefaultTransport")
	}
	restore()
	if http.DefaultTransport != previous {
		t.Fatal("restore did not put the previous http.DefaultTransport back")
	}
}
