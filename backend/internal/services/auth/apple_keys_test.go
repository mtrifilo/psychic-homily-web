package auth

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"fmt"
	"log/slog"
	"math/big"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"psychic-homily-backend/internal/config"
)

const keysTestBundleID = "com.psychichomily.ios"

// keyServer is an httptest stand-in for Apple's key endpoint that counts
// requests and serves whatever response the test last set.
type keyServer struct {
	*httptest.Server
	fetches atomic.Int32

	mu     sync.Mutex
	status int
	body   []byte
}

func newKeyServer(t *testing.T, body []byte) *keyServer {
	t.Helper()
	ks := &keyServer{status: http.StatusOK, body: body}
	ks.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ks.fetches.Add(1)
		ks.mu.Lock()
		status, body := ks.status, ks.body
		ks.mu.Unlock()
		w.WriteHeader(status)
		_, _ = w.Write(body)
	}))
	t.Cleanup(ks.Close)
	return ks
}

func (ks *keyServer) serve(status int, body []byte) {
	ks.mu.Lock()
	ks.status, ks.body = status, body
	ks.mu.Unlock()
}

func (ks *keyServer) count() int { return int(ks.fetches.Load()) }

// fakeClock is a settable clock for the key cache.
type fakeClock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *fakeClock) advance(d time.Duration) {
	c.mu.Lock()
	c.now = c.now.Add(d)
	c.mu.Unlock()
}

// recordCounter is a slog handler that counts records by message.
type recordCounter struct {
	mu     sync.Mutex
	counts map[string]int
}

func (h *recordCounter) Enabled(context.Context, slog.Level) bool { return true }
func (h *recordCounter) Handle(_ context.Context, r slog.Record) error {
	h.mu.Lock()
	h.counts[r.Message]++
	h.mu.Unlock()
	return nil
}
func (h *recordCounter) WithAttrs([]slog.Attr) slog.Handler { return h }
func (h *recordCounter) WithGroup(string) slog.Handler      { return h }

func (h *recordCounter) count(msg string) int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.counts[msg]
}

func newRSAKey(t *testing.T) *rsa.PrivateKey {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	return key
}

// jwks renders a JWK set holding key under each of kids.
func jwks(key *rsa.PrivateKey, kids ...string) []byte {
	n := base64.RawURLEncoding.EncodeToString(key.N.Bytes())
	e := base64.RawURLEncoding.EncodeToString(big.NewInt(int64(key.E)).Bytes())
	out := `{"keys":[`
	for i, kid := range kids {
		if i > 0 {
			out += ","
		}
		out += `{"kty":"RSA","kid":"` + kid + `","use":"sig","alg":"RS256","n":"` + n + `","e":"` + e + `"}`
	}
	return []byte(out + `]}`)
}

func newKeysTestService(url string) (*AppleAuthService, *fakeClock, *recordCounter) {
	cfg := &config.Config{
		Apple: config.AppleConfig{BundleID: keysTestBundleID},
		JWT:   config.JWTConfig{SecretKey: "test-secret", Expiry: 24},
	}
	svc := createTestAppleService(cfg, url)
	clock := &fakeClock{now: time.Date(2026, 10, 14, 12, 0, 0, 0, time.UTC)}
	logs := &recordCounter{counts: map[string]int{}}
	svc.keysNow = clock.Now
	svc.keysLogger = slog.New(logs)
	return svc, clock, logs
}

func signedToken(t *testing.T, key *rsa.PrivateKey, kid string) string {
	t.Helper()
	return createAppleIdentityToken(t, key, kid, keysTestBundleID, "apple-sub", "keys@example.com", time.Now().Add(time.Hour))
}

// Tokens naming key ids the fresh set lacks, sent concurrently, cause one
// forced refresh between them, and every one is refused.
func TestAppleKeys_ConcurrentUnknownKidsFetchOnce(t *testing.T) {
	key := newRSAKey(t)
	ks := newKeyServer(t, jwks(key, "known-kid"))
	svc, _, _ := newKeysTestService(ks.URL)

	_, err := svc.getApplePublicKey("known-kid")
	require.NoError(t, err)
	require.Equal(t, 1, ks.count())

	const n = 20
	errs := make([]error, n)
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, errs[i] = svc.ValidateIdentityToken(signedToken(t, key, fmt.Sprintf("made-up-%d", i)))
		}(i)
	}
	wg.Wait()

	assert.Equal(t, 2, ks.count(), "one priming fetch plus one forced refresh")
	for i, err := range errs {
		require.Error(t, err, "token %d", i)
		assert.Contains(t, err.Error(), "apple public key not found")
	}
}

// Lookups that miss while a fetch is in flight wait for it instead of starting
// their own, so a cold cache under concurrent load fetches once.
func TestAppleKeys_ConcurrentColdLookupsShareOneFetch(t *testing.T) {
	key := newRSAKey(t)
	release := make(chan struct{})
	arrived := make(chan struct{}, 64)
	var fetches atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fetches.Add(1)
		arrived <- struct{}{}
		<-release
		_, _ = w.Write(jwks(key, "kid-a"))
	}))
	t.Cleanup(srv.Close)
	svc, _, _ := newKeysTestService(srv.URL)

	const n = 10
	errs := make([]error, n)
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, errs[i] = svc.getApplePublicKey("kid-a")
		}(i)
	}
	<-arrived
	// Give the other lookups time to reach the in-flight fetch before it ends.
	time.Sleep(100 * time.Millisecond)
	close(release)
	wg.Wait()

	assert.Equal(t, int32(1), fetches.Load())
	for i, err := range errs {
		assert.NoError(t, err, "lookup %d", i)
	}
}

// A key server that never answers fails validation within the client timeout,
// for the lookup that started the fetch and for one waiting on it.
func TestAppleKeys_HangingServerFailsWithinTimeout(t *testing.T) {
	release := make(chan struct{})
	arrived := make(chan struct{}, 8)
	var fetches atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fetches.Add(1)
		arrived <- struct{}{}
		select {
		case <-release:
		case <-r.Context().Done():
		}
	}))
	t.Cleanup(srv.Close)
	t.Cleanup(func() { close(release) })

	svc, _, logs := newKeysTestService(srv.URL)
	svc.keysHTTPClient = &http.Client{Timeout: 500 * time.Millisecond}
	key := newRSAKey(t)
	token := signedToken(t, key, "any-kid")

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	done := make(chan error, 2)
	validate := func() {
		_, err := svc.ValidateIdentityToken(token)
		done <- err
	}
	go validate()
	<-arrived
	// The fetch is now in flight and stays there until the client timeout, so
	// this lookup finds it running and waits on it.
	go validate()
	for i := 0; i < 2; i++ {
		select {
		case err := <-done:
			require.Error(t, err)
			assert.Contains(t, err.Error(), "failed to fetch Apple keys")
		case <-ctx.Done():
			t.Fatal("validation still blocked on the key server after 3s")
		}
	}
	assert.Equal(t, int32(1), fetches.Load(), "the second lookup waited on the first fetch")
	assert.Equal(t, 1, logs.count("apple_keys_fetch_failed"))
}

func TestAppleKeys_DefaultClientIsBounded(t *testing.T) {
	assert.Equal(t, 5*time.Second, appleKeysHTTPClient.Timeout)
}

// A second unknown key id inside the cooldown is refused without a fetch; the
// first one after the cooldown fetches again.
func TestAppleKeys_UnknownKidInsideCooldownSkipsFetch(t *testing.T) {
	key := newRSAKey(t)
	ks := newKeyServer(t, jwks(key, "known-kid"))
	svc, clock, _ := newKeysTestService(ks.URL)

	_, err := svc.getApplePublicKey("known-kid")
	require.NoError(t, err)
	require.Equal(t, 1, ks.count())

	_, err = svc.getApplePublicKey("unknown-1")
	require.Error(t, err)
	require.Equal(t, 2, ks.count(), "the first unknown kid forces a refresh")

	clock.advance(30 * time.Second)
	_, err = svc.ValidateIdentityToken(signedToken(t, key, "unknown-2"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "apple public key not found for kid: unknown-2 (refresh cooling down, not fetched)")
	assert.Equal(t, 2, ks.count(), "inside the cooldown there is no fetch")

	clock.advance(30 * time.Second)
	_, err = svc.getApplePublicKey("unknown-3")
	require.Error(t, err)
	assert.Equal(t, 3, ks.count(), "the cooldown has passed")
}

// A rotated key whose id arrives after the cooldown fetches once and
// validates, then answers from the cache.
func TestAppleKeys_RotationAfterCooldownFetchesOnceAndValidates(t *testing.T) {
	key := newRSAKey(t)
	ks := newKeyServer(t, jwks(key, "old-kid"))
	svc, clock, _ := newKeysTestService(ks.URL)

	_, err := svc.getApplePublicKey("old-kid")
	require.NoError(t, err)
	// A made-up kid spends the forced refresh for this window.
	_, err = svc.getApplePublicKey("made-up")
	require.Error(t, err)
	require.Equal(t, 2, ks.count())

	ks.serve(http.StatusOK, jwks(key, "old-kid", "new-kid"))

	clock.advance(10 * time.Second)
	_, err = svc.ValidateIdentityToken(signedToken(t, key, "new-kid"))
	require.Error(t, err, "inside the cooldown the new kid is refused")
	require.Equal(t, 2, ks.count())

	clock.advance(appleKeysForcedRefreshCooldown)
	claims, err := svc.ValidateIdentityToken(signedToken(t, key, "new-kid"))
	require.NoError(t, err)
	assert.Equal(t, "apple-sub", claims.Subject)
	assert.Equal(t, 3, ks.count(), "the rotation fetched once")

	_, err = svc.ValidateIdentityToken(signedToken(t, key, "new-kid"))
	require.NoError(t, err)
	assert.Equal(t, 3, ks.count(), "the rotated key answers from the cache")
}

// A failed forced refresh keeps the cached set, so known kids still validate
// without a fetch, and the failure is logged once per log interval.
func TestAppleKeys_FailedRefreshKeepsCachedSet(t *testing.T) {
	key := newRSAKey(t)
	ks := newKeyServer(t, jwks(key, "known-kid"))
	svc, clock, logs := newKeysTestService(ks.URL)

	_, err := svc.getApplePublicKey("known-kid")
	require.NoError(t, err)

	ks.serve(http.StatusServiceUnavailable, nil)
	_, err = svc.getApplePublicKey("unknown-1")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "status 503")
	require.Equal(t, 2, ks.count())
	assert.Equal(t, 1, logs.count("apple_keys_fetch_failed"))

	_, err = svc.ValidateIdentityToken(signedToken(t, key, "known-kid"))
	require.NoError(t, err, "the cached set still validates known kids")
	assert.Equal(t, 2, ks.count())

	// Once the cooldown, the backoff and the log interval have all passed, the
	// next forced refresh fetches, fails too and logs again.
	clock.advance(max(appleKeysForcedRefreshCooldown, appleKeysFailureBackoff, appleKeysFailureLogInterval))
	_, err = svc.getApplePublicKey("unknown-2")
	require.Error(t, err)
	assert.Equal(t, 3, ks.count())
	assert.Equal(t, 2, logs.count("apple_keys_fetch_failed"))
}

// A failed fetch holds off every fetch path, cold and expired included, for
// the backoff; lookups inside it get the failure without a fetch. An expired
// set never answers a lookup itself. Failures inside one log interval log
// once.
func TestAppleKeys_FailedFetchBacksOffOnEveryPath(t *testing.T) {
	cases := []struct {
		name string
		// prime leaves the service in the state under test, with the key
		// server answering 500.
		prime func(t *testing.T, svc *AppleAuthService, ks *keyServer, clock *fakeClock)
	}{
		{
			name: "cold",
			prime: func(t *testing.T, svc *AppleAuthService, ks *keyServer, clock *fakeClock) {
				ks.serve(http.StatusInternalServerError, nil)
			},
		},
		{
			name: "expired",
			prime: func(t *testing.T, svc *AppleAuthService, ks *keyServer, clock *fakeClock) {
				_, err := svc.getApplePublicKey("known-kid")
				require.NoError(t, err)
				clock.advance(appleKeysTTL + time.Second)
				ks.serve(http.StatusInternalServerError, nil)
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			key := newRSAKey(t)
			ks := newKeyServer(t, jwks(key, "known-kid"))
			svc, clock, logs := newKeysTestService(ks.URL)
			tc.prime(t, svc, ks, clock)
			base := ks.count()

			_, err := svc.getApplePublicKey("known-kid")
			require.Error(t, err)
			assert.Contains(t, err.Error(), "status 500")
			require.Equal(t, base+1, ks.count())

			clock.advance(appleKeysFailureBackoff - time.Second)
			for _, kid := range []string{"known-kid", "made-up"} {
				_, err = svc.ValidateIdentityToken(signedToken(t, key, kid))
				require.Error(t, err)
				assert.Contains(t, err.Error(), "status 500 (backing off after a failed fetch, not fetched)")
			}
			assert.Equal(t, base+1, ks.count(), "inside the backoff there is no fetch")

			clock.advance(time.Second)
			_, err = svc.getApplePublicKey("known-kid")
			require.Error(t, err)
			assert.Equal(t, base+2, ks.count(), "the backoff has passed")
			assert.Equal(t, 1, logs.count("apple_keys_fetch_failed"), "failures inside one log interval log once")

			ks.serve(http.StatusOK, jwks(key, "known-kid"))
			clock.advance(appleKeysFailureBackoff)
			_, err = svc.ValidateIdentityToken(signedToken(t, key, "known-kid"))
			require.NoError(t, err)
			assert.Equal(t, base+3, ks.count())
		})
	}
}

// A 200 response with no usable key is a failed fetch: the cached set stays.
func TestAppleKeys_EmptySetKeepsCachedSet(t *testing.T) {
	key := newRSAKey(t)
	ks := newKeyServer(t, jwks(key, "known-kid"))
	svc, _, _ := newKeysTestService(ks.URL)

	_, err := svc.getApplePublicKey("known-kid")
	require.NoError(t, err)

	ks.serve(http.StatusOK, []byte(`{"keys":[]}`))
	_, err = svc.getApplePublicKey("unknown-kid")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "contained no RSA keys")
	require.Equal(t, 2, ks.count())

	_, err = svc.ValidateIdentityToken(signedToken(t, key, "known-kid"))
	require.NoError(t, err)
	assert.Equal(t, 2, ks.count())
}
