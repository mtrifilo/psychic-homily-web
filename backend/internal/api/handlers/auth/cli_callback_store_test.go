package auth

import (
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"psychic-homily-backend/internal/testlog"
)

func cleanCLICallbackStore() {
	cliCallbackStore.reset()
}

// getCLICallback reads the process-wide store without consuming, for tests
// that check what the handlers left behind.
func getCLICallback(id string) (string, bool) {
	return cliCallbackStore.get(id)
}

func deleteCLICallback(id string) {
	cliCallbackStore.remove(id)
}

// storeExpiredCLICallback plants an already-expired entry in the process-wide
// store. It goes to the front of order, which keeps order sorted by expiry.
func storeExpiredCLICallback(id, callbackURL string) {
	cliCallbackStore.mu.Lock()
	defer cliCallbackStore.mu.Unlock()
	cliCallbackStore.removeLocked(id)
	cliCallbackStore.entries[id] = cliCallbackStore.order.PushFront(&registryEntry[string]{
		id:        id,
		value:     callbackURL,
		expiresAt: time.Now().Add(-time.Minute),
	})
}

func TestCLICallbackStore_StoreAndRetrieve(t *testing.T) {
	defer cleanCLICallbackStore()

	storeCLICallback("test-id-1", "http://localhost:9999/callback")

	url, ok := getCLICallback("test-id-1")
	if !ok {
		t.Fatal("expected callback to be found")
	}
	if url != "http://localhost:9999/callback" {
		t.Errorf("expected callback URL, got %q", url)
	}
}

func TestCLICallbackStore_NotFound(t *testing.T) {
	defer cleanCLICallbackStore()

	_, ok := getCLICallback("nonexistent")
	if ok {
		t.Error("expected not found for nonexistent key")
	}
}

func TestCLICallbackStore_Expired(t *testing.T) {
	defer cleanCLICallbackStore()

	storeExpiredCLICallback("expired-id", "http://expired.example.com")

	_, ok := getCLICallback("expired-id")
	if ok {
		t.Error("expected expired callback to not be found")
	}
}

func TestCLICallbackStore_Delete(t *testing.T) {
	defer cleanCLICallbackStore()

	storeCLICallback("del-id", "http://localhost/cb")

	_, ok := getCLICallback("del-id")
	if !ok {
		t.Fatal("expected callback to exist before delete")
	}

	deleteCLICallback("del-id")

	_, ok = getCLICallback("del-id")
	if ok {
		t.Error("expected callback to be gone after delete")
	}
	assertRegistryConsistent(t, cliCallbackStore)
}

// The callback consumes its entry in one step: a second holder of the same
// cli_callback_id, arriving after the first, finds nothing.
func TestCLICallbackStore_TakeIsSingleUse(t *testing.T) {
	defer cleanCLICallbackStore()

	storeCLICallback("take-id", "http://localhost:8888/cb")

	first, firstOK := takeCLICallback("take-id")
	second, secondOK := takeCLICallback("take-id")
	if !firstOK || first != "http://localhost:8888/cb" {
		t.Fatalf("expected the first take to get the callback, got %q, %v", first, firstOK)
	}
	if secondOK {
		t.Fatalf("expected the second take to find nothing, got %q", second)
	}
	assertRegistryConsistent(t, cliCallbackStore)
}

func TestCLICallbackStore_CleansExpiredOnStore(t *testing.T) {
	defer cleanCLICallbackStore()

	storeExpiredCLICallback("old-id", "http://old.example.com")

	storeCLICallback("new-id", "http://new.example.com")

	cliCallbackStore.mu.RLock()
	_, oldExists := cliCallbackStore.entries["old-id"]
	_, newExists := cliCallbackStore.entries["new-id"]
	cliCallbackStore.mu.RUnlock()

	if oldExists {
		t.Error("expected expired entry to be cleaned up")
	}
	if !newExists {
		t.Error("expected new entry to exist")
	}
	assertRegistryConsistent(t, cliCallbackStore)
}

// TestOAuthLoginHTTPHandler_CLICallbackAtCapacityEvictsOldestAndLogsOnce drives
// the login handler against a full process-wide store: the start succeeds, the
// oldest pending callback makes room, and the eviction is logged for the first
// start and not again inside the report interval.
func TestOAuthLoginHTTPHandler_CLICallbackAtCapacityEvictsOldestAndLogsOnce(t *testing.T) {
	cleanCLICallbackStore()
	t.Cleanup(cleanCLICallbackStore)

	for i := range cliCallbackCapacity {
		storeCLICallback(fmt.Sprintf("fill-%04d", i), "http://localhost:8888/fill")
	}

	handler := NewOAuthHTTPHandler(nil, nil, nil)
	start := func() (*httptest.ResponseRecorder, string) {
		w, req := oauthLoginRequest("google")
		req.URL.RawQuery = "cli_callback=http://localhost:8888/cli-cb"
		output := testlog.Capture(t, func() {
			handler.OAuthLoginHTTPHandler(w, testlog.Request(t, req))
		})
		return w, output
	}

	w, output := start()
	callbackID := cliCallbackID(t, w)
	if url, ok := getCLICallback(callbackID); !ok || url != "http://localhost:8888/cli-cb" {
		t.Fatalf("expected the new start's callback to be stored, got %q, %v", url, ok)
	}
	if _, ok := getCLICallback("fill-0000"); ok {
		t.Error("expected the oldest pending callback to be evicted")
	}
	if _, ok := getCLICallback("fill-0001"); !ok {
		t.Error("expected only the oldest pending callback to be evicted")
	}
	if size := cliCallbackStore.size(); size != cliCallbackCapacity {
		t.Errorf("expected the store to stay at capacity %d, got %d", cliCallbackCapacity, size)
	}
	for _, want := range []string{
		"msg=oauth_cli_callback_store_evicting",
		"evicted=1",
		fmt.Sprintf("capacity=%d", cliCallbackCapacity),
	} {
		if !strings.Contains(output, want) {
			t.Errorf("expected %q in the log; captured log:\n%s", want, output)
		}
	}
	if strings.Contains(output, callbackID) {
		t.Errorf("the login handler logged the minted callback id; captured log:\n%s", output)
	}

	var cookieMaxAge int
	for _, c := range w.Result().Cookies() {
		if c.Name == "cli_callback_id" {
			cookieMaxAge = c.MaxAge
		}
	}
	if want := int(cliCallbackTTL / time.Second); cookieMaxAge != want {
		t.Errorf("expected the cli_callback_id cookie MaxAge %d, got %d", want, cookieMaxAge)
	}

	_, output = start()
	if !strings.Contains(output, "msg=oauth_cli_callback_stored") {
		t.Fatalf("expected the second start to log its stored callback; captured log:\n%s", output)
	}
	if strings.Contains(output, "msg=oauth_cli_callback_store_evicting") {
		t.Errorf("expected no second eviction log inside the report interval; captured log:\n%s", output)
	}
}
