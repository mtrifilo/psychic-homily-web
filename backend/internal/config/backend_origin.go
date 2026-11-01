package config

import (
	"fmt"
	"net"
	"net/url"
	"os"
	"strings"
)

// deployedBackendOrigins maps each deployed frontend origin to the public
// origin of the API that serves it. The production frontend is reachable at
// both its apex and www forms, and FRONTEND_URL may name either.
var deployedBackendOrigins = map[string]string{
	"https://psychichomily.com":       "https://api.psychichomily.com",
	"https://www.psychichomily.com":   "https://api.psychichomily.com",
	"https://stage.psychichomily.com": "https://api-stage.psychichomily.com",
}

// recognisedFrontendForms lists, for error messages, the FRONTEND_URL forms
// ResolveBackendOrigin recognises.
const recognisedFrontendForms = "https://psychichomily.com, https://www.psychichomily.com, " +
	"https://stage.psychichomily.com, or an http(s) origin on localhost, 127.0.0.1 or [::1]"

// localBackendPort is the port a local backend's origin names when the server
// address carries none.
const localBackendPort = "8080"

// ResolveBackendOrigin returns the public origin of this backend, the base of
// every backend-hosted URL it hands out (email unsubscribe links, the calendar
// feed, OAuth callbacks), and reports whether frontendURL is a recognised form.
//
// A non-blank backendPublicURL wins. Otherwise a deployed frontend maps to its
// API host, and anything else resolves to http://localhost on serverAddr's
// port (8080 when it names none). A frontend URL is recognised when it is a
// deployed frontend or an http(s) origin on a loopback host; surrounding
// whitespace and trailing slashes are ignored, and a path, query, fragment or
// userinfo makes it unrecognised.
func ResolveBackendOrigin(frontendURL, backendPublicURL, serverAddr string) (origin string, recognised bool) {
	deployedOrigin, isDeployed := deployedBackendOrigins[normalizeURL(frontendURL)]
	recognised = isDeployed || isLoopbackOrigin(frontendURL)

	switch override := normalizeURL(backendPublicURL); {
	case override != "":
		return override, recognised
	case isDeployed:
		return deployedOrigin, recognised
	default:
		return localBackendOrigin(serverAddr), recognised
	}
}

// ValidateBackendOrigin returns an error when frontendURL is not a form
// ResolveBackendOrigin recognises and backendPublicURL is blank: the one
// configuration in which every backend-hosted URL would name a local backend
// that no recipient can reach. The error names the frontend URL's host, never
// the whole value.
func ValidateBackendOrigin(frontendURL, backendPublicURL string) error {
	if _, recognised := ResolveBackendOrigin(frontendURL, backendPublicURL, ""); recognised || normalizeURL(backendPublicURL) != "" {
		return nil
	}
	return fmt.Errorf(
		"%s (%s) is not a recognised frontend, so backend-hosted links (email unsubscribe links, the calendar feed, OAuth callbacks) "+
			"cannot name this backend's public origin; set %s to one of %s, or set %s to this backend's public origin",
		EnvFrontendURL, describeHost(frontendURL), EnvFrontendURL, recognisedFrontendForms, EnvBackendPublicURL,
	)
}

// BackendOriginFromEnv is ResolveBackendOrigin's origin for frontendURL with
// BACKEND_PUBLIC_URL and API_ADDR read from the environment, the same inputs
// Load reads, for callers that hold a frontend URL but not a Config.
func BackendOriginFromEnv(frontendURL string) string {
	origin, _ := ResolveBackendOrigin(frontendURL, os.Getenv(EnvBackendPublicURL), serverAddrFromEnv())
	return origin
}

// BackendOrigin is ResolveBackendOrigin's origin for this configuration.
func (c *Config) BackendOrigin() string {
	origin, _ := ResolveBackendOrigin(c.Email.FrontendURL, c.Server.BackendPublicURL, c.Server.Addr)
	return origin
}

// serverAddrFromEnv returns API_ADDR, or the default listen address when it
// is unset.
func serverAddrFromEnv() string {
	return GetEnv(EnvAPIAddr, "localhost:"+localBackendPort)
}

// normalizeURL trims surrounding whitespace and trailing slashes.
func normalizeURL(raw string) string {
	return strings.TrimRight(strings.TrimSpace(raw), "/")
}

// isLoopbackOrigin reports whether raw, normalized, is a bare http(s) origin
// whose host is localhost or a loopback IP address.
func isLoopbackOrigin(raw string) bool {
	u, err := url.Parse(normalizeURL(raw))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil ||
		u.Path != "" || u.RawQuery != "" || u.Fragment != "" || u.Opaque != "" {
		return false
	}
	host := u.Hostname()
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// localBackendOrigin returns http://localhost on serverAddr's port, or on
// localBackendPort when serverAddr names none.
func localBackendOrigin(serverAddr string) string {
	port := localBackendPort
	if _, p, err := net.SplitHostPort(strings.TrimSpace(serverAddr)); err == nil && p != "" {
		port = p
	}
	return "http://localhost:" + port
}

// describeHost describes raw by its host alone, for an error message that
// must not echo userinfo, a path or a query.
func describeHost(raw string) string {
	if strings.TrimSpace(raw) == "" {
		return "empty"
	}
	u, err := url.Parse(normalizeURL(raw))
	if err != nil || u.Host == "" {
		return "no parseable host"
	}
	return fmt.Sprintf("host %q", u.Host)
}
