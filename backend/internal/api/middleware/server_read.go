package middleware

import (
	"crypto/sha256"
	"crypto/subtle"
	"net/http"
	"strings"
)

// ServerReadSecretHeader is the request header a server-side caller of the API
// (the frontend's server renders, metadata, sitemap and proxy probes) carries
// the shared server-read secret in.
//
// The name contains "secret" on purpose: observability.ScrubSentryEvent redacts
// any header whose name does, so a Sentry event raised during such a request
// never carries the value.
const ServerReadSecretHeader = "X-Server-Read-Secret"

// ServerReadSecretEnvVar names the environment variable that holds the shared
// server-read secret. Unset, empty or all whitespace disables the exemption, so
// every request takes the anonymous path exactly as if the header did not
// exist.
const ServerReadSecretEnvVar = "SERVER_READ_SECRET"

// ServerReadExemption recognizes a server-side read: a GET or HEAD whose
// ServerReadSecretHeader matches the configured secret. Recognition answers one
// question, whether the request skips the ANONYMOUS public-read limiter, and
// grants nothing else. The request still reaches its handler with no identity,
// no other limiter consults this type, and writes are never exempt.
//
// Every failure fails closed: a nil exemption, another method, an absent header
// or a wrong value all leave the request on the anonymous per-IP bucket.
//
// A nil *ServerReadExemption recognizes nothing, which is what an unset secret
// builds.
type ServerReadExemption struct {
	// secretDigest is the SHA-256 of the configured secret. Comparing digests
	// rather than raw values gives subtle.ConstantTimeCompare two inputs of
	// equal length, so the comparison time says nothing about the secret's
	// length or its prefix.
	secretDigest [sha256.Size]byte
	// sample selects which exempt requests write a sample line.
	sample func() bool
}

// NewServerReadExemption builds the exemption for secret, read once at process
// start. It returns nil, which exempts nothing, when the trimmed secret is
// empty. Surrounding whitespace is trimmed so a value pasted into a dashboard
// with a trailing newline still matches what the frontend sends, which trims
// the same way.
func NewServerReadExemption(secret string) *ServerReadExemption {
	return newServerReadExemption(secret, sampleAt(allowedReadSampleRate))
}

func newServerReadExemption(secret string, sample func() bool) *ServerReadExemption {
	secret = strings.TrimSpace(secret)
	if secret == "" {
		return nil
	}
	return &ServerReadExemption{secretDigest: sha256.Sum256([]byte(secret)), sample: sample}
}

// Enabled reports whether a secret is configured. It never reveals the secret.
func (e *ServerReadExemption) Enabled() bool {
	return e != nil
}

// Exempts reports whether r is a server-side read that skips the anonymous
// public-read limiter.
func (e *ServerReadExemption) Exempts(r *http.Request) bool {
	if e == nil {
		return false
	}
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		return false
	}
	presented := r.Header.Get(ServerReadSecretHeader)
	if presented == "" {
		return false
	}
	digest := sha256.Sum256([]byte(presented))
	return subtle.ConstantTimeCompare(digest[:], e.secretDigest[:]) == 1
}

// logSample writes the exempt request's sample line when the sampler selects
// it. The line names the limiter the request skipped and carries
// server_read_exempt=true; it never carries the header or its value.
func (e *ServerReadExemption) logSample(r *http.Request) {
	if e.sample() {
		publicReadAnonymousSpec.logServerReadExempt(r)
	}
}
