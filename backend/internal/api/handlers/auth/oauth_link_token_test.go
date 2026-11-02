package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"psychic-homily-backend/internal/api/handlers/shared/testhelpers"
	authm "psychic-homily-backend/internal/models/auth"
	"psychic-homily-backend/internal/testlog"
)

const testTokenSecret = "test-secret-at-least-32-characters-long"

func TestOAuthLinkToken_RoundTrip(t *testing.T) {
	token, err := mintOAuthLinkToken(testTokenSecret, 7)
	if err != nil {
		t.Fatalf("mint: %v", err)
	}
	if !consumeOAuthLinkToken(t.Context(), testTokenSecret, token, 7) {
		t.Fatal("a freshly minted token must spend")
	}
}

// Single use, which is the property that makes a leaked start URL worth
// nothing on a second click.
func TestOAuthLinkToken_SpendsOnce(t *testing.T) {
	token, err := mintOAuthLinkToken(testTokenSecret, 7)
	if err != nil {
		t.Fatalf("mint: %v", err)
	}
	if !consumeOAuthLinkToken(t.Context(), testTokenSecret, token, 7) {
		t.Fatal("first spend must succeed")
	}
	if consumeOAuthLinkToken(t.Context(), testTokenSecret, token, 7) {
		t.Error("a token must not spend twice")
	}
}

// Every binding is load-bearing: drop any one and a token minted in one
// situation becomes spendable in another.
func TestOAuthLinkToken_Bindings(t *testing.T) {
	cases := []struct {
		name          string
		spendSecret   string
		spendUser     uint
		mutateToken   func(string) string
		wantSpendable bool
	}{
		{"correct", testTokenSecret, 7, nil, true},
		{"another account", testTokenSecret, 8, nil, false},
		{"another secret", "a-different-secret-of-sufficient-length", 7, nil, false},
		{
			name: "tampered payload", spendSecret: testTokenSecret, spendUser: 8,
			// Rewrite the subject and keep the old signature: the signature is
			// the only thing stopping a caller minting their own claims.
			mutateToken: func(tok string) string {
				payload, sig, _ := strings.Cut(tok, ".")
				return "8" + payload[1:] + "." + sig
			},
			wantSpendable: false,
		},
		{
			name: "signature only", spendSecret: testTokenSecret, spendUser: 7,
			mutateToken:   func(tok string) string { _, sig, _ := strings.Cut(tok, "."); return sig },
			wantSpendable: false,
		},
		{
			name: "empty", spendSecret: testTokenSecret, spendUser: 7,
			mutateToken:   func(string) string { return "" },
			wantSpendable: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			token, err := mintOAuthLinkToken(testTokenSecret, 7)
			if err != nil {
				t.Fatalf("mint: %v", err)
			}
			if tc.mutateToken != nil {
				token = tc.mutateToken(token)
			}
			if got := consumeOAuthLinkToken(t.Context(), tc.spendSecret, token, tc.spendUser); got != tc.wantSpendable {
				t.Errorf("spendable = %v, want %v", got, tc.wantSpendable)
			}
		})
	}
}

// The MAC is domain-separated, so a value the same secret signed for some
// other purpose cannot be presented here as a link token.
func TestOAuthLinkToken_MACIsDomainSeparated(t *testing.T) {
	claims := oauthLinkTokenClaims{
		userID: 7,
		expiry: time.Now().Add(time.Minute),
		nonce:  "undomained-nonce",
	}
	payload := encodeOAuthLinkTokenPayload(claims)

	mac := hmac.New(sha256.New, []byte(testTokenSecret))
	mac.Write([]byte(payload)) // no domain prefix
	undomained := payload + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))

	if consumeOAuthLinkToken(t.Context(), testTokenSecret, undomained, 7) {
		t.Error("a MAC computed without the domain prefix must not spend")
	}
}

func TestOAuthLinkToken_Expires(t *testing.T) {
	claims := oauthLinkTokenClaims{
		userID: 7,
		expiry: time.Now().Add(-time.Second),
		nonce:  "expired-nonce",
	}
	payload := encodeOAuthLinkTokenPayload(claims)
	token := payload + "." + signOAuthLinkToken(testTokenSecret, payload)

	if consumeOAuthLinkToken(t.Context(), testTokenSecret, token, 7) {
		t.Error("an expired token must not spend")
	}
}

// The origin gate, which both legs of the link run.
//
// The configured frontend is the primary test. Fetch metadata answers "same
// site as this API", and on stage and every preview the frontend is on
// *.vercel.app while the API is on Railway: genuinely cross-site, so a check
// that stopped at the metadata would refuse every Connect outside production.
func TestRequestIsFromOurFrontend(t *testing.T) {
	const frontend = "https://psychichomily.com"

	cases := []struct {
		name     string
		fetch    string
		origin   string
		referer  string
		wantPass bool
	}{
		// The case that was refused before: a cross-site deployment naming us.
		{"cross-site metadata but our Origin", "cross-site", frontend, "", true},
		{"cross-site metadata but our Referer", "cross-site", "", frontend + "/profile?tab=settings", true},
		{"cross-site metadata and another Origin", "cross-site", "https://evil.example", "", false},
		{"cross-site metadata and nothing else", "cross-site", "", "", false},

		{"our Origin, no metadata", "", frontend, "", true},
		{"our Referer, no metadata", "", "", frontend + "/profile", true},
		{"another Origin, no metadata", "", "https://evil.example", "", false},
		{"another Referer, no metadata", "", "", "https://evil.example/x", false},
		{"nothing at all", "", "", "", false},

		// Same-site deployments, where Referer may be stripped entirely.
		{"same-origin metadata", "same-origin", "", "", true},
		{"same-site metadata", "same-site", "", "", true},
		{"user-initiated navigation", "none", "", "", true},

		{"opaque origin", "", "null", "", false},
		{"opaque origin in capitals", "", "NULL", "", false},
		{"opaque origin in capitals with our Referer", "", "NULL", frontend + "/profile", true},
		{"scheme mismatch", "", "http://psychichomily.com", "", false},
		// A host that merely ends with ours is a different host.
		{"suffix host", "", "https://notpsychichomily.com", "", false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/auth/link/google", nil)
			if tc.fetch != "" {
				req.Header.Set("Sec-Fetch-Site", tc.fetch)
			}
			if tc.origin != "" {
				req.Header.Set("Origin", tc.origin)
			}
			if tc.referer != "" {
				req.Header.Set("Referer", tc.referer)
			}
			if got := requestIsFromOurFrontend(req, frontend); got != tc.wantPass {
				t.Errorf("requestIsFromOurFrontend = %v, want %v", got, tc.wantPass)
			}
		})
	}
}

func TestOriginFromHeaders(t *testing.T) {
	const ours = "http://localhost:3000"

	cases := []struct {
		name    string
		origin  string
		referer string
		want    string
	}{
		{"no Origin and no Referer", "", "", ""},
		{"Origin alone", ours, "", ours},
		{"Referer alone, trimmed to its origin", "", ours + "/profile?tab=settings", ours},
		{"opaque Origin falls back to Referer", "null", ours + "/profile", ours},
		{"another Origin with our Referer", "https://evil.example", ours + "/profile", "https://evil.example"},
		{"opaque Origin and nothing else", "null", "", ""},
		{"opaque Origin in capitals falls back to Referer", "NULL", ours + "/profile", ours},
		{"opaque Origin in mixed case and nothing else", "Null", "", ""},
		{"Referer with surrounding whitespace, trimmed to its origin", "", " " + ours + "/profile ", ours},
		{"Origin with a path, returned as sent", ours + "/", "", ours + "/"},
		{"Referer that is not an absolute URL", "", "/profile", ""},
		{"Referer that is not a URL", "", "not a url", ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := originFromHeaders(tc.origin, tc.referer); got != tc.want {
				t.Errorf("originFromHeaders(%q, %q) = %q, want %q", tc.origin, tc.referer, got, tc.want)
			}
		})
	}
}

// namesFrontend is the one frontend test both legs of the link run.
func TestNamesFrontend(t *testing.T) {
	const ours = "https://www.psychichomily.com"

	cases := []struct {
		name    string
		origin  string
		referer string
		want    bool
	}{
		{"our Origin", ours, "", true},
		{"our Origin in another case", "HTTPS://WWW.PSYCHICHOMILY.COM", "", true},
		{"our Referer alone", "", ours + "/profile", true},
		{"opaque Origin in capitals with our Referer", "NULL", ours + "/profile", true},
		{"another Origin with our Referer", "https://evil.example", ours + "/profile", false},
		{"nothing at all", "", "", false},
		// The www frontend redirects pages to the apex, so a browser on the
		// production site reports the apex origin.
		{"the apex Origin of the same deployment", "https://psychichomily.com", "", true},
		{"the apex Referer of the same deployment", "", "https://psychichomily.com/profile", true},
		{"the stage Origin is another deployment", "https://stage.psychichomily.com", "", false},
		{"a lookalike of the apex", "https://psychichomily.com.evil.example", "", false},
		{"the apex over http", "http://psychichomily.com", "", false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := namesFrontend(tc.origin, tc.referer, ours); got != tc.want {
				t.Errorf("namesFrontend(%q, %q, %q) = %v, want %v", tc.origin, tc.referer, ours, got, tc.want)
			}
		})
	}
}

// Production runs with FRONTEND_URL=https://www.psychichomily.com and serves
// its pages at the apex, so both the mint and the spend must accept either
// production origin, and still refuse a foreign or opaque one.
func TestLinkLegsAcceptEveryProductionFrontendOrigin(t *testing.T) {
	const configured = "https://www.psychichomily.com"

	cases := []struct {
		name   string
		origin string
		want   bool
	}{
		{"apex Origin", "https://psychichomily.com", true},
		{"www Origin", "https://www.psychichomily.com", true},
		{"foreign Origin", "https://evil.example", false},
		{"opaque Origin", "null", false},
	}

	for _, tc := range cases {
		t.Run("mint: "+tc.name, func(t *testing.T) {
			h := NewOAuthAccountHandler(nil, "test-secret", configured)
			user := &authm.User{ID: 7}
			resp, err := h.StartOAuthLinkHandler(testhelpers.CtxWithUser(user), &StartOAuthLinkRequest{Origin: tc.origin})
			if !tc.want {
				testhelpers.AssertHumaErrorWithDetail(t, err, 403, "Start the connection from Settings")
				return
			}
			if err != nil || resp == nil || resp.Body.Token == "" {
				t.Fatalf("mint = (%v, %v), want a token", resp, err)
			}
		})

		t.Run("spend: "+tc.name, func(t *testing.T) {
			// No fetch metadata, so only the named origin decides.
			req := httptest.NewRequest(http.MethodGet, "/auth/link/google", nil)
			req.Header.Set("Origin", tc.origin)
			if got := requestIsFromOurFrontend(req, configured); got != tc.want {
				t.Errorf("requestIsFromOurFrontend with Origin %q = %v, want %v", tc.origin, got, tc.want)
			}
		})
	}
}

// useSpentNonceSet swaps the process-wide spent-nonce set for set until the
// test ends.
func useSpentNonceSet(t *testing.T, set *spentNonceSet) {
	t.Helper()
	previous := spentOAuthLinkTokens
	spentOAuthLinkTokens = set
	t.Cleanup(func() { spentOAuthLinkTokens = previous })
}

func (s *spentNonceSet) size() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.spent) != len(s.byExpiry) {
		panic(fmt.Sprintf("spentNonceSet: %d nonces in spent, %d in byExpiry", len(s.spent), len(s.byExpiry)))
	}
	return len(s.spent)
}

func TestSpentNonceSet_RefusesCapacityBelowOne(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("expected a capacity of 0 to panic")
		}
	}()
	newSpentNonceSet(0, time.Now)
}

// A spent nonce stays refused for as long as its token is live, however many
// other spends prune around it, and its token is refused as expired after.
func TestSpentNonceSet_RefusesAReplayWhileTheTokenIsLive(t *testing.T) {
	clock := newFakeClock()
	set := newSpentNonceSet(4, clock.Now)
	expiry := clock.Now().Add(oauthLinkTokenTTL)

	if ok, _ := set.spend("n", expiry); !ok {
		t.Fatal("first spend must succeed")
	}
	clock.Advance(oauthLinkTokenTTL)
	if ok, _ := set.spend("n", expiry); ok {
		t.Fatal("a replay at exactly the token's expiry must be refused")
	}
	if ok, _ := set.spend("m", clock.Now().Add(oauthLinkTokenTTL)); !ok {
		t.Fatal("another token must still spend")
	}
	if ok, _ := set.spend("n", expiry); ok {
		t.Fatal("a spend that pruned must not have dropped a live nonce")
	}

	clock.Advance(time.Nanosecond)
	if ok, _ := set.spend("n", expiry); ok {
		t.Fatal("a token past its expiry must be refused, pruned or not")
	}
	if ok, _ := set.spend("o", clock.Now().Add(oauthLinkTokenTTL)); !ok {
		t.Fatal("another token must still spend")
	}
	if size := set.size(); size != 2 {
		t.Errorf("expected the expired nonce pruned and the two live ones kept, size %d", size)
	}
}

// A full set refuses the next spend instead of evicting: an evicted nonce
// would make its token spendable again.
func TestSpentNonceSet_FullSetRefusesRatherThanEvicts(t *testing.T) {
	const capacity = 3
	clock := newFakeClock()
	set := newSpentNonceSet(capacity, clock.Now)
	expiry := clock.Now().Add(oauthLinkTokenTTL)

	for i := range capacity {
		if ok, reported := set.spend(fmt.Sprintf("n%d", i), expiry); !ok || reported != 0 {
			t.Fatalf("spend %d below capacity = (%v, %d), want (true, 0)", i, ok, reported)
		}
	}
	if ok, reported := set.spend("over", expiry); ok || reported != 1 {
		t.Fatalf("spend at capacity = (%v, %d), want (false, 1)", ok, reported)
	}
	for i := range capacity {
		if ok, _ := set.spend(fmt.Sprintf("n%d", i), expiry); ok {
			t.Fatalf("nonce n%d was evicted to make room and spent again", i)
		}
	}
	if size := set.size(); size != capacity {
		t.Errorf("expected the set to hold capacity %d, got %d", capacity, size)
	}

	// Once the spent tokens expire, their room comes back.
	clock.Advance(oauthLinkTokenTTL + time.Nanosecond)
	if ok, _ := set.spend("over", clock.Now().Add(oauthLinkTokenTTL)); !ok {
		t.Fatal("expected expired nonces to make room for a new spend")
	}
	if size := set.size(); size != 1 {
		t.Errorf("expected the expired nonces pruned, size %d", size)
	}
}

// Tokens are spent in no order of expiry. A nonce spent later can expire
// sooner, and it frees its room at its own expiry, not at the later one's.
func TestSpentNonceSet_PrunesBySoonestExpiryWhateverTheSpendOrder(t *testing.T) {
	clock := newFakeClock()
	set := newSpentNonceSet(2, clock.Now)
	start := clock.Now()

	if ok, _ := set.spend("late", start.Add(oauthLinkTokenTTL)); !ok {
		t.Fatal("spend late")
	}
	if ok, _ := set.spend("early", start.Add(time.Minute)); !ok {
		t.Fatal("spend early")
	}

	clock.Advance(time.Minute + time.Nanosecond)
	if ok, _ := set.spend("next", clock.Now().Add(oauthLinkTokenTTL)); !ok {
		t.Fatal("expected the expired early nonce to make room")
	}
	if ok, _ := set.spend("late", start.Add(oauthLinkTokenTTL)); ok {
		t.Fatal("the live late nonce must still be refused")
	}
	if size := set.size(); size != 2 {
		t.Errorf("expected late and next held, size %d", size)
	}
}

// The set holds exactly the spent tokens that are still unexpired, so it never
// holds more than the tokens issued in one token lifetime. Tokens are spent at
// a varying delay after their mint, so spend order is not expiry order.
func TestSpentNonceSet_HoldsOnlyTokensStillInTheirLifetime(t *testing.T) {
	const (
		step   = 10 * time.Second
		spends = 4 * int(oauthLinkTokenTTL/step)
		delays = int(oauthLinkTokenTTL / step)
	)
	clock := newFakeClock()
	set := newSpentNonceSet(spends, clock.Now)

	expiries := make([]time.Time, 0, spends)
	for i := range spends {
		clock.Advance(step)
		delay := time.Duration((i*7)%delays) * step
		expiry := clock.Now().Add(-delay).Add(oauthLinkTokenTTL)
		if ok, _ := set.spend(fmt.Sprintf("n%d", i), expiry); !ok {
			t.Fatalf("spend %d of a live token refused", i)
		}
		expiries = append(expiries, expiry)

		live := 0
		for _, e := range expiries {
			if !clock.Now().After(e) {
				live++
			}
		}
		if size := set.size(); size != live {
			t.Fatalf("after spend %d the set holds %d nonces, want the %d still in their lifetime", i, size, live)
		}
	}
}

// A capacity refusal is reported at once, then at most once per interval with
// the count of every refusal held back in between.
func TestSpentNonceSet_RefusalsReportedAtMostOncePerInterval(t *testing.T) {
	clock := newFakeClock()
	set := newSpentNonceSet(1, clock.Now)
	expiry := clock.Now().Add(oauthLinkTokenTTL)

	if _, reported := set.spend("a", expiry); reported != 0 {
		t.Fatalf("no refusal yet, got report %d", reported)
	}
	if _, reported := set.spend("b", expiry); reported != 1 {
		t.Fatalf("expected the first refusal reported at once, got %d", reported)
	}
	for _, nonce := range []string{"c", "d"} {
		clock.Advance(capacityReportInterval / 4)
		if _, reported := set.spend(nonce, expiry); reported != 0 {
			t.Fatalf("expected no report inside the interval, got %d", reported)
		}
	}
	clock.Advance(capacityReportInterval / 2)
	if _, reported := set.spend("e", expiry); reported != 3 {
		t.Fatalf("expected the held-back refusals plus this one (3), got %d", reported)
	}
	if _, reported := set.spend("a", expiry); reported != 0 {
		t.Fatalf("a replay is not a capacity refusal, got report %d", reported)
	}
}

// TestSpentNonceSet_SpendCostDoesNotScaleWithSize spends 10,000 distinct live
// nonces, with expiries out of spend order, into a set that holds all of them
// and into one at the production capacity. A spend that walked the set would
// make the last spends orders of magnitude slower than the first; a heap
// insert grows with the logarithm of the size. Medians keep a GC pause in
// either window from deciding the result.
func TestSpentNonceSet_SpendCostDoesNotScaleWithSize(t *testing.T) {
	const (
		spends      = 10000
		window      = 100
		maxSlowdown = 10
		spendBound  = time.Second
	)

	for _, capacity := range []int{spends, oauthLinkSpentNonceCapacity} {
		t.Run(fmt.Sprintf("capacity_%d", capacity), func(t *testing.T) {
			set := newSpentNonceSet(capacity, time.Now)
			base := time.Now().Add(time.Hour)
			nonces := make([]string, spends)
			expiries := make([]time.Time, spends)
			for i := range nonces {
				nonces[i] = fmt.Sprintf("nonce-%05d", i)
				expiries[i] = base.Add(time.Duration((i*7919)%spends) * time.Millisecond)
			}

			durations := make([]time.Duration, spends)
			start := time.Now()
			for i, nonce := range nonces {
				spendStart := time.Now()
				set.spend(nonce, expiries[i])
				durations[i] = time.Since(spendStart)
			}
			elapsed := time.Since(start)

			firstMedian := medianDuration(durations[:window])
			lastMedian := medianDuration(durations[spends-window:])
			t.Logf("%d spends in %v; median spend %v over the first %d, %v over the last %d; size %d",
				spends, elapsed, firstMedian, window, lastMedian, window, set.size())

			if elapsed > spendBound {
				t.Errorf("%d spends took %v, over the %v bound", spends, elapsed, spendBound)
			}
			if lastMedian > maxSlowdown*max(firstMedian, time.Microsecond) {
				t.Errorf("median spend grew from %v to %v as the set filled", firstMedian, lastMedian)
			}
			if want := min(spends, capacity); set.size() != want {
				t.Errorf("expected size %d, got %d", want, set.size())
			}
		})
	}
}

// Of many concurrent spends of one nonce, exactly one succeeds.
func TestSpentNonceSet_ConcurrentSpendsHaveOneWinner(t *testing.T) {
	const spenders = 32
	for round := range 50 {
		set := newSpentNonceSet(4, time.Now)
		expiry := time.Now().Add(oauthLinkTokenTTL)

		var wg sync.WaitGroup
		var winners atomic.Int32
		start := make(chan struct{})
		for range spenders {
			wg.Add(1)
			go func() {
				defer wg.Done()
				<-start
				if ok, _ := set.spend("nonce", expiry); ok {
					winners.Add(1)
				}
			}()
		}
		close(start)
		wg.Wait()

		if got := winners.Load(); got != 1 {
			t.Fatalf("round %d: %d spends succeeded, want exactly 1", round, got)
		}
	}
}

// A link token refused because the spent set is full is refused like any
// other, and the refusal is logged with the set's capacity.
func TestOAuthLinkToken_FullSpentSetRefusesAndLogs(t *testing.T) {
	useSpentNonceSet(t, newSpentNonceSet(1, time.Now))

	first, err := mintOAuthLinkToken(testTokenSecret, 7)
	if err != nil {
		t.Fatalf("mint: %v", err)
	}
	second, err := mintOAuthLinkToken(testTokenSecret, 7)
	if err != nil {
		t.Fatalf("mint: %v", err)
	}

	var firstSpent, secondSpent bool
	output := testlog.Capture(t, func() {
		ctx := testlog.Context(t, t.Context())
		firstSpent = consumeOAuthLinkToken(ctx, testTokenSecret, first, 7)
		secondSpent = consumeOAuthLinkToken(ctx, testTokenSecret, second, 7)
	})

	if !firstSpent {
		t.Fatal("the first token must spend into an empty set")
	}
	if secondSpent {
		t.Error("a token must not spend into a full set")
	}
	for _, want := range []string{"msg=oauth_link_spent_nonce_set_full", "refused=1", "capacity=1"} {
		if !strings.Contains(output, want) {
			t.Errorf("expected %q in the log, got %q", want, output)
		}
	}
	if strings.Contains(output, second) {
		t.Error("the token is never logged")
	}
}
