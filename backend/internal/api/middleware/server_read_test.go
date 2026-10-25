package middleware

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/getsentry/sentry-go"
	"github.com/go-chi/httprate"

	authm "psychic-homily-backend/internal/models/auth"
	"psychic-homily-backend/internal/observability"
)

const testServerReadSecret = "server-read-secret-for-tests-0123456789"

// serverReadMW builds the auth-state router with a 1-request anonymous budget,
// so the second metered request from one address is a 429.
func serverReadMW(serverRead *ServerReadExemption) func(http.Handler) http.Handler {
	anon := httprate.Limit(1, time.Minute, httprate.WithKeyFuncs(httprate.KeyByIP),
		httprate.WithLimitHandler(testRejection))
	perUser := httprate.Limit(1, time.Minute, httprate.WithKeyFuncs(rateLimitUserKeyFunc),
		httprate.WithLimitHandler(testRejection))
	ipCeiling := httprate.Limit(1000, time.Minute, httprate.WithKeyFuncs(httprate.KeyByIP),
		httprate.WithLimitHandler(testRejection))
	return RateLimitPublicReadsByAuthState(newTestJWTService(), nil, serverRead, anon, perUser, ipCeiling)
}

func serverReadReq(method, remoteAddr string, header *string) *http.Request {
	req := httptest.NewRequest(method, "/artists/some-artist", nil)
	req.RemoteAddr = remoteAddr
	if header != nil {
		req.Header.Set(ServerReadSecretHeader, *header)
	}
	return req
}

func strRef(s string) *string { return &s }

// mustServerRead builds an exemption from a configuration the test knows is
// usable.
func mustServerRead(configured string, sample func() bool) *ServerReadExemption {
	e, err := newServerReadExemption(configured, sample)
	if err != nil {
		panic(err)
	}
	return e
}

func TestServerReadExemption_RightSecretSkipsAnonymousLimiterOnGetAndHead(t *testing.T) {
	for _, method := range []string{http.MethodGet, http.MethodHead} {
		t.Run(method, func(t *testing.T) {
			handler := serverReadMW(mustServerRead(testServerReadSecret, never))(okHandler())
			const addr = "10.20.30.40:1000"
			for i := 0; i < 5; i++ {
				if rr := serve(handler, serverReadReq(method, addr, strRef(testServerReadSecret))); rr.Code != http.StatusOK {
					t.Fatalf("server read %d: status = %d, want 200", i, rr.Code)
				}
			}
			// The exempt reads never touched the bucket: the address still has
			// its whole anonymous budget, and is metered past it.
			if rr := serve(handler, serverReadReq(method, addr, nil)); rr.Code != http.StatusOK {
				t.Fatalf("first anonymous read after server reads: status = %d, want 200", rr.Code)
			}
			if rr := serve(handler, serverReadReq(method, addr, nil)); rr.Code != http.StatusTooManyRequests {
				t.Errorf("second anonymous read: status = %d, want 429", rr.Code)
			}
		})
	}
}

func TestServerReadExemption_WrongEmptyOrAbsentSecretIsMetered(t *testing.T) {
	for name, header := range map[string]*string{
		"wrong":         strRef("not-the-secret"),
		"prefix":        strRef(testServerReadSecret[:10]),
		"secret plus":   strRef(testServerReadSecret + "x"),
		"empty":         strRef(""),
		"absent":        nil,
		"case mismatch": strRef(strings.ToUpper(testServerReadSecret)),
	} {
		t.Run(name, func(t *testing.T) {
			handler := serverReadMW(mustServerRead(testServerReadSecret, never))(okHandler())
			const addr = "10.20.30.41:1000"
			if rr := serve(handler, serverReadReq(http.MethodGet, addr, header)); rr.Code != http.StatusOK {
				t.Fatalf("first read: status = %d, want 200", rr.Code)
			}
			if rr := serve(handler, serverReadReq(http.MethodGet, addr, header)); rr.Code != http.StatusTooManyRequests {
				t.Errorf("second read: status = %d, want 429 (no exemption)", rr.Code)
			}
		})
	}
}

// The exemption is for reads only. A POST that reaches this limiter (the
// read-via-POST batch endpoints do) is metered even with the right secret.
func TestServerReadExemption_PostWithRightSecretIsMetered(t *testing.T) {
	handler := serverReadMW(mustServerRead(testServerReadSecret, never))(okHandler())
	const addr = "10.20.30.42:1000"
	if rr := serve(handler, serverReadReq(http.MethodPost, addr, strRef(testServerReadSecret))); rr.Code != http.StatusOK {
		t.Fatalf("first POST: status = %d, want 200", rr.Code)
	}
	if rr := serve(handler, serverReadReq(http.MethodPost, addr, strRef(testServerReadSecret))); rr.Code != http.StatusTooManyRequests {
		t.Errorf("second POST: status = %d, want 429 (writes are never exempt)", rr.Code)
	}
}

func TestServerReadExemption_EmptySecretDisablesIt(t *testing.T) {
	for name, secret := range map[string]string{"empty": "", "whitespace": " \t\n"} {
		t.Run(name, func(t *testing.T) {
			exemption, err := NewServerReadExemption(secret)
			if err != nil || exemption != nil || exemption.Enabled() {
				t.Fatalf("NewServerReadExemption(%q) = %v, %v; want a disabled (nil) exemption and no error", secret, exemption, err)
			}
			handler := serverReadMW(exemption)(okHandler())
			const addr = "10.20.30.43:1000"
			// Whatever the header says, including the empty string the
			// unset secret would otherwise equal.
			for _, header := range []*string{strRef(""), strRef(secret), strRef(testServerReadSecret)} {
				req := serverReadReq(http.MethodGet, addr, header)
				if exemption.Exempts(req) {
					t.Errorf("disabled exemption exempts header %q", *header)
				}
			}
			if rr := serve(handler, serverReadReq(http.MethodGet, addr, strRef(secret))); rr.Code != http.StatusOK {
				t.Fatalf("first read: status = %d, want 200", rr.Code)
			}
			if rr := serve(handler, serverReadReq(http.MethodGet, addr, strRef(secret))); rr.Code != http.StatusTooManyRequests {
				t.Errorf("second read: status = %d, want 429 (disabled exemption)", rr.Code)
			}
		})
	}
}

// A configured secret with surrounding whitespace matches the trimmed value,
// which is what the frontend sends.
func TestServerReadExemption_TrimsConfiguredSecret(t *testing.T) {
	exemption := mustServerRead("  "+testServerReadSecret+"\n", never)
	if !exemption.Exempts(serverReadReq(http.MethodGet, "10.20.30.44:1", strRef(testServerReadSecret))) {
		t.Error("trimmed secret does not match the configured value with surrounding whitespace")
	}
}

// The exemption grants no identity: the handler behind it sees the same
// context an anonymous request gets, with no per-user key.
func TestServerReadExemption_GrantsNoIdentity(t *testing.T) {
	var sawUser bool
	handler := serverReadMW(mustServerRead(testServerReadSecret, never))(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, sawUser = r.Context().Value(rateLimitUserIDKey{}).(uint)
		w.WriteHeader(http.StatusOK)
	}))
	if rr := serve(handler, serverReadReq(http.MethodGet, "10.20.30.45:1", strRef(testServerReadSecret))); rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
	if sawUser {
		t.Error("an exempt server read reached the handler carrying a user id")
	}
}

// A server render that forwards a viewer's session is metered on that viewer's
// per-user bucket even when it also carries the secret.
func TestServerReadExemption_SessionStillMeteredPerUser(t *testing.T) {
	jwtService := newTestJWTService()
	user := &authm.User{Email: strPtr("viewer@example.com")}
	user.ID = 77
	token, err := jwtService.CreateToken(user)
	if err != nil {
		t.Fatalf("CreateToken: %v", err)
	}
	anon := httprate.Limit(1, time.Minute, httprate.WithKeyFuncs(httprate.KeyByIP),
		httprate.WithLimitHandler(testRejection))
	perUser := httprate.Limit(1, time.Minute, httprate.WithKeyFuncs(rateLimitUserKeyFunc),
		httprate.WithLimitHandler(testRejection))
	ipCeiling := httprate.Limit(1000, time.Minute, httprate.WithKeyFuncs(httprate.KeyByIP),
		httprate.WithLimitHandler(testRejection))
	handler := RateLimitPublicReadsByAuthState(jwtService, nil, mustServerRead(testServerReadSecret, never),
		anon, perUser, ipCeiling)(okHandler())

	req := func() *http.Request {
		r := readReq("10.20.30.46:1", token)
		r.Header.Set(ServerReadSecretHeader, testServerReadSecret)
		return r
	}
	if rr := serve(handler, req()); rr.Code != http.StatusOK {
		t.Fatalf("first: status = %d, want 200", rr.Code)
	}
	if rr := serve(handler, req()); rr.Code != http.StatusTooManyRequests {
		t.Errorf("second: status = %d, want 429 (session wins; per-user cap applies)", rr.Code)
	}
}

// An exempt request writes a sample line that says so and carries no remaining
// budget; a metered one says it was not exempt. Neither line carries the secret.
func TestServerReadExemption_SampleLinesNameTheExemption(t *testing.T) {
	logs := newLogCapture(t)
	anon := publicReadAnonymousLimiter(always)
	perUser := httprate.Limit(1000, time.Minute, httprate.WithKeyFuncs(rateLimitUserKeyFunc))
	ipCeiling := httprate.Limit(1000, time.Minute, httprate.WithKeyFuncs(httprate.KeyByIP))
	handler := RateLimitPublicReadsByAuthState(nil, nil, mustServerRead(testServerReadSecret, always),
		anon, perUser, ipCeiling)(okHandler())

	exempt := serverReadReq(http.MethodGet, "10.20.30.47:1", strRef(testServerReadSecret))
	serve(handler, logs.attach(exempt))
	wrong := serverReadReq(http.MethodGet, "10.20.30.47:1", strRef("wrong-"+testServerReadSecret))
	serve(handler, logs.attach(wrong))

	if strings.Contains(logs.buf.String(), testServerReadSecret) {
		t.Fatalf("a log line carries the server-read secret:\n%s", logs.buf.String())
	}
	samples := logs.events(rateLimitAllowedSampleEvent)
	if len(samples) != 2 {
		t.Fatalf("got %d sample lines, want 2:\n%s", len(samples), logs.buf.String())
	}
	exemptLine, meteredLine := samples[0], samples[1]
	for field, want := range map[string]any{
		"limiter":            string(LimiterPublicReadAnonymous),
		"auth_state":         authStateAnonymous,
		"path_family":        pathFamilyArtist,
		serverReadExemptAttr: true,
	} {
		if exemptLine[field] != want {
			t.Errorf("exempt line: %s = %v, want %v", field, exemptLine[field], want)
		}
	}
	if _, ok := exemptLine["remaining"]; ok {
		t.Errorf("exempt line carries remaining=%v, want none (the bucket was not consulted)", exemptLine["remaining"])
	}
	if meteredLine[serverReadExemptAttr] != false {
		t.Errorf("metered line: %s = %v, want false", serverReadExemptAttr, meteredLine[serverReadExemptAttr])
	}
	if _, ok := meteredLine["remaining"]; !ok {
		t.Error("metered line carries no remaining budget")
	}
}

// The sampler decides whether an exempt request writes a line.
func TestServerReadExemption_SamplerGatesItsLine(t *testing.T) {
	logs := newLogCapture(t)
	handler := serverReadMW(mustServerRead(testServerReadSecret, never))(okHandler())
	serve(handler, logs.attach(serverReadReq(http.MethodGet, "10.20.30.48:1", strRef(testServerReadSecret))))
	if got := len(logs.events(rateLimitAllowedSampleEvent)); got != 0 {
		t.Errorf("got %d sample lines with a sampler that selects nothing, want 0", got)
	}
}

// The header's name is one the Sentry scrubber redacts, so an event raised
// during a server read never carries the secret.
func TestServerReadSecretHeader_IsRedactedFromSentryEvents(t *testing.T) {
	event := &sentry.Event{Request: &sentry.Request{Headers: map[string]string{
		ServerReadSecretHeader: testServerReadSecret,
	}}}
	scrubbed := observability.ScrubSentryEvent(event, nil)
	if got := scrubbed.Request.Headers[ServerReadSecretHeader]; got == testServerReadSecret {
		t.Errorf("Sentry event header %s = %q, want it redacted", ServerReadSecretHeader, got)
	}
}

func TestServerReadLogValuesAreStable(t *testing.T) {
	for got, want := range map[string]string{
		serverReadExemptAttr:   "server_read_exempt",
		ServerReadSecretHeader: "X-Server-Read-Secret",
		ServerReadSecretEnvVar: "SERVER_READ_SECRET",
	} {
		if got != want {
			t.Errorf("value %q, want %q", got, want)
		}
	}
}

// A rotation configures the new and the retiring secret together; both are
// accepted until the retiring one is removed.
func TestServerReadExemption_AcceptsARotationPair(t *testing.T) {
	const next = "server-read-secret-next-9876543210"
	exemption := mustServerRead(next+", "+testServerReadSecret, never)
	for _, presented := range []string{next, testServerReadSecret} {
		if !exemption.Exempts(serverReadReq(http.MethodGet, "10.20.30.49:1", strRef(presented))) {
			t.Errorf("rotation pair does not accept %q", presented)
		}
	}
	if exemption.Exempts(serverReadReq(http.MethodGet, "10.20.30.49:1", strRef(next+","+testServerReadSecret))) {
		t.Error("the configured value as a whole was accepted as a secret")
	}
}

// A configuration the frontend could never match is refused at start, with an
// error to log, rather than running as an exemption nothing can use.
func TestServerReadExemption_RefusesAnUnusableSecret(t *testing.T) {
	for name, configured := range map[string]string{
		"inner space":   "has space inside a secret long enough to pass",
		"non-ascii":     "n\u00e9e-secret-padded-to-thirty-two-characters",
		"control char":  "tab\there-padded-to-thirty-two-characters",
		"empty part":    testServerReadSecret + ", ",
		"three secrets": "a-secret,b-secret,c-secret",
		"too short":     "only-thirty-one-characters-long",
		"leading comma": "," + testServerReadSecret,
	} {
		t.Run(name, func(t *testing.T) {
			exemption, err := NewServerReadExemption(configured)
			if !errors.Is(err, ErrUnusableServerReadSecret) {
				t.Errorf("err = %v, want ErrUnusableServerReadSecret", err)
			}
			if exemption != nil {
				t.Errorf("exemption = %v, want nil (exempts nothing)", exemption)
			}
		})
	}
}
