package middleware

import (
	"crypto/sha256"
	"crypto/subtle"
	"errors"
	"net/http"
	"strings"
)

// ServerReadSecretHeader is the request header a server-side caller of the API
// (the frontend's server renders, metadata, sitemap and proxy probes) carries
// the shared server-read secret in.
//
// The name contains "secret" on purpose: observability.ScrubSentryEvent redacts
// any header whose name does, on error events and on transactions.
const ServerReadSecretHeader = "X-Server-Read-Secret"

// ServerReadSecretEnvVar names the environment variable that holds the shared
// server-read secret. Unset, empty or all whitespace disables the exemption, so
// every request takes the anonymous path exactly as if the header did not
// exist.
//
// It may hold two secrets separated by a comma, which is how the secret is
// rotated without a window in which no server read is exempt: set "new,old"
// here, switch the frontend to "new", then set "new" here alone.
const ServerReadSecretEnvVar = "SERVER_READ_SECRET"

// maxServerReadSecrets is how many comma-separated secrets ServerReadSecretEnvVar
// may hold: the current one and the one a rotation is retiring.
const maxServerReadSecrets = 2

// ErrUnusableServerReadSecret reports a configured secret the exemption refuses
// to run with. The frontend sends only visible ASCII, so a secret with any
// other character could never match and would leave the exemption silently
// dead.
var ErrUnusableServerReadSecret = errors.New(
	"server-read secret must be one or two comma-separated values of visible ASCII")

// ServerReadExemption recognizes a server-side read: a GET or HEAD whose
// ServerReadSecretHeader matches a configured secret. Recognition answers one
// question, whether the request skips the ANONYMOUS public-read limiter, and
// grants nothing else. The request still reaches its handler with no identity,
// no other limiter consults this type, and writes are never exempt.
//
// Every failure fails closed: a nil exemption, another method, an absent header
// or a wrong value all leave the request on the anonymous per-IP bucket.
//
// A nil *ServerReadExemption recognizes nothing, which is what an unset or
// unusable secret builds.
type ServerReadExemption struct {
	// secretDigests are the SHA-256 digests of the configured secrets.
	// Comparing digests rather than raw values gives subtle.ConstantTimeCompare
	// two inputs of equal length, so the comparison time says nothing about a
	// secret's length or its prefix.
	secretDigests [][sha256.Size]byte
	// sample selects which exempt requests write a sample line.
	sample func() bool
}

// NewServerReadExemption builds the exemption from the configured value, read
// once at process start. It returns nil, which exempts nothing, when the value
// is blank, and nil with ErrUnusableServerReadSecret when it cannot be used.
// Each secret is trimmed of surrounding whitespace, so a value pasted into a
// dashboard with a trailing newline still matches what the frontend sends,
// which trims the same way.
func NewServerReadExemption(configured string) (*ServerReadExemption, error) {
	return newServerReadExemption(configured, sampleAt(allowedReadSampleRate))
}

func newServerReadExemption(configured string, sample func() bool) (*ServerReadExemption, error) {
	if strings.TrimSpace(configured) == "" {
		return nil, nil
	}
	parts := strings.Split(configured, ",")
	if len(parts) > maxServerReadSecrets {
		return nil, ErrUnusableServerReadSecret
	}
	e := &ServerReadExemption{sample: sample}
	for _, part := range parts {
		secret := strings.TrimSpace(part)
		if !isVisibleASCII(secret) {
			return nil, ErrUnusableServerReadSecret
		}
		e.secretDigests = append(e.secretDigests, sha256.Sum256([]byte(secret)))
	}
	return e, nil
}

// isVisibleASCII reports whether s is non-empty and every byte is in 0x21-0x7e.
func isVisibleASCII(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < 0x21 || s[i] > 0x7e {
			return false
		}
	}
	return true
}

// Enabled reports whether a secret is configured. It never reveals the secret.
func (e *ServerReadExemption) Enabled() bool {
	return e != nil
}

// Exempts reports whether r is a server-side read that skips the anonymous
// public-read limiter. Every configured secret is compared, so the time taken
// does not say which one matched.
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
	matched := 0
	for i := range e.secretDigests {
		matched |= subtle.ConstantTimeCompare(digest[:], e.secretDigests[i][:])
	}
	return matched == 1
}

// logSample writes the exempt request's sample line when the sampler selects
// it. The line names the limiter the request skipped and carries
// server_read_exempt=true; it never carries the header or its value.
func (e *ServerReadExemption) logSample(r *http.Request) {
	if e.sample() {
		publicReadAnonymousSpec.logServerReadExempt(r)
	}
}
