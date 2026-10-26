// Package indexnow tells IndexNow search engines which public pages changed:
// on a schedule it collects the shows, artists, venues and releases whose
// sitemap lastmod moved since the last successful run, and posts their URLs to
// the IndexNow endpoint.
package indexnow

import (
	"fmt"
	"net"
	"net/url"
	"regexp"
	"strings"
	"time"

	"psychic-homily-backend/internal/testenv"
)

const (
	// LoopName is the loop's name in logs and its background_service_runs
	// primary key.
	LoopName = "indexnow_pings"

	// KeyEnvVar holds the IndexNow key. Empty means the pinger does not start.
	// The frontend serves the same value at /{key}.txt, which is what proves
	// ownership of the host to the search engines.
	KeyEnvVar = "INDEXNOW_KEY"

	// DisableEnvVar set to "1" keeps the pinger from starting even with a key.
	DisableEnvVar = "DISABLE_INDEXNOW_PINGS"

	// EndpointOverrideEnvVar points the pinger at a loopback stub instead of
	// the IndexNow endpoint. It is honoured only in a test, ci or development
	// ENVIRONMENT and only for a loopback host; any other value disables the
	// pinger, so no setting can send a submission anywhere but submitEndpoint
	// or this machine, and production always posts to submitEndpoint.
	EndpointOverrideEnvVar = "INDEXNOW_ENDPOINT_OVERRIDE"

	// environmentEnvVar names the deployment; see testenv.AllowedEnvironments.
	environmentEnvVar = "ENVIRONMENT"

	// submitEndpoint is the shared IndexNow endpoint, which forwards every
	// submission to all participating search engines. It is a constant: the
	// outbound host is never built from configuration or data.
	submitEndpoint = "https://api.indexnow.org/indexnow"

	// maxURLsPerRequest is the protocol's limit on urlList in one POST.
	maxURLsPerRequest = 10_000

	// defaultInterval is the time between runs.
	defaultInterval = 10 * time.Minute
)

// keyPattern is the protocol's key format: 8 to 128 characters, each a letter,
// a digit or a dash.
var keyPattern = regexp.MustCompile(`^[A-Za-z0-9-]{8,128}$`)

// Config is what one run needs to address a submission.
type Config struct {
	// Key is the IndexNow key, in keyPattern's format.
	Key string
	// SiteOrigin is the scheme and host every submitted URL and the key file
	// live under, with no path and no trailing slash.
	SiteOrigin string
	// Host is SiteOrigin's host, the submission's `host` field.
	Host string
	// Endpoint is submitEndpoint, or a loopback override.
	Endpoint string
}

// KeyLocation is the URL the frontend serves the key file at. The file sits
// at the site root because a key file's directory bounds the URLs it may
// vouch for, and the root is the one directory that covers every page.
func (c Config) KeyLocation() string {
	return c.SiteOrigin + "/" + c.Key + ".txt"
}

// ConfigFromEnv reads the pinger's configuration. It reports enabled false,
// with the reason, when the key is unset or malformed, when frontendURL does
// not name an http(s) origin, or when the endpoint override is set outside a
// test environment or to a host that is not loopback.
func ConfigFromEnv(frontendURL string, getenv func(string) string) (cfg Config, enabled bool, reason string) {
	key := strings.TrimSpace(getenv(KeyEnvVar))
	if key == "" {
		return Config{}, false, KeyEnvVar + " is not set"
	}
	if !keyPattern.MatchString(key) {
		return Config{}, false, KeyEnvVar + " is not 8 to 128 letters, digits or dashes"
	}

	origin, host, err := siteOrigin(frontendURL)
	if err != nil {
		return Config{}, false, err.Error()
	}

	endpoint := submitEndpoint
	if override := strings.TrimSpace(getenv(EndpointOverrideEnvVar)); override != "" {
		if !testenv.IsAllowedEnvironment(getenv(environmentEnvVar)) {
			return Config{}, false, EndpointOverrideEnvVar + " is set outside a test, ci or development ENVIRONMENT"
		}
		if !isLoopbackURL(override) {
			return Config{}, false, EndpointOverrideEnvVar + " is set to a host that is not loopback"
		}
		endpoint = override
	}

	return Config{Key: key, SiteOrigin: origin, Host: host, Endpoint: endpoint}, true, ""
}

// siteOrigin is the origin the site serves its pages at, derived from the
// frontend URL. A www host is read as its apex, because the site's www host
// permanently redirects every page to the apex and the sitemap announces apex
// URLs.
func siteOrigin(frontendURL string) (origin, host string, err error) {
	u, err := url.Parse(strings.TrimSpace(frontendURL))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" {
		return "", "", fmt.Errorf("frontend URL %q is not an http(s) origin", frontendURL)
	}
	host = strings.TrimPrefix(strings.ToLower(u.Host), "www.")
	return u.Scheme + "://" + host, host, nil
}

// isLoopbackURL reports whether raw is an http(s) URL whose host is localhost
// or a loopback address.
func isLoopbackURL(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return false
	}
	host := u.Hostname()
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
