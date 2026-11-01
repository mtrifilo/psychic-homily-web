package config

import (
	"fmt"
	"maps"
	"net"
	"net/url"
	"slices"
	"strings"
)

// deployedBackendOrigins maps each deployed frontend origin to the public
// origin of the API that serves it. The production frontend is reachable at
// both its apex and www forms, and FRONTEND_URL may name either.
var deployedBackendOrigins = map[string]string{
	"https://psychichomily.com":       "https://api.psychichomily.com",
	"https://www.psychichomily.com":   "https://api.psychichomily.com",
	"https://stage.psychichomily.com": "https://stage.api.psychichomily.com",
}

// loopbackFrontendForm describes, for error messages, the loopback frontends
// isRecognisedFrontend accepts.
const loopbackFrontendForm = "(outside production and stage) an http(s) origin on localhost, 127.0.0.1 or [::1]"

// localBackendPort is the port a local backend's origin names when the server
// address carries none.
const localBackendPort = "8080"

// ResolveBackendOrigin returns the public origin of this backend, the base of
// every backend-hosted URL it hands out (email unsubscribe links, the calendar
// feed, OAuth callbacks). A non-blank backendPublicURL wins. Otherwise a
// deployed frontend maps to its API host, and anything else resolves to
// http://localhost on serverAddr's port (8080 when it names none).
func ResolveBackendOrigin(frontendURL, backendPublicURL, serverAddr string) string {
	if override := normalizeURL(backendPublicURL); override != "" {
		return override
	}
	if deployedOrigin, ok := deployedBackendOrigins[normalizeURL(frontendURL)]; ok {
		return deployedOrigin
	}
	return localBackendOrigin(serverAddr)
}

// ValidateBackendOrigin returns an error when the inputs to
// ResolveBackendOrigin would mint backend-hosted URLs that no recipient can
// use:
//   - a non-blank backendPublicURL that is not a bare http(s) origin, or that
//     is a loopback or wildcard (0.0.0.0, ::) origin while environment is
//     production or stage;
//   - a blank backendPublicURL with a frontendURL isRecognisedFrontend
//     refuses for environment.
//
// Errors name a URL's host, never the whole value.
func ValidateBackendOrigin(environment, frontendURL, backendPublicURL string) error {
	deployedEnvironment := environment == EnvProduction || environment == EnvStage

	if normalizeURL(backendPublicURL) != "" {
		host, ok := bareOriginHost(backendPublicURL)
		switch {
		case !ok:
			return fmt.Errorf(
				"%s (%s) is not a bare http(s) origin (scheme://host[:port], no path, query, fragment or userinfo), so every backend-hosted link "+
					"would be malformed; set it to this backend's public origin or unset it",
				EnvBackendPublicURL, describeHost(backendPublicURL))
		case deployedEnvironment && (isLoopbackHost(host) || isUnspecifiedHost(host)):
			return fmt.Errorf(
				"%s (%s) is a loopback or wildcard origin in %s=%s, so every backend-hosted link would name a local backend; "+
					"set it to this backend's public origin or unset it",
				EnvBackendPublicURL, describeHost(backendPublicURL), EnvEnvironment, environment)
		}
		return nil
	}

	if isRecognisedFrontend(frontendURL, deployedEnvironment) {
		return nil
	}
	return fmt.Errorf(
		"%s (%s) is not a recognised frontend, so backend-hosted links (email unsubscribe links, the calendar feed, OAuth callbacks) "+
			"cannot name this backend's public origin; set %s to one of %s, or %s, or set %s to this backend's public origin",
		EnvFrontendURL, describeHost(frontendURL), EnvFrontendURL,
		strings.Join(slices.Sorted(maps.Keys(deployedBackendOrigins)), ", "), loopbackFrontendForm, EnvBackendPublicURL,
	)
}

// isRecognisedFrontend reports whether frontendURL is a deployed frontend or,
// unless deployedEnvironment, an http(s) origin on a loopback host.
// Surrounding whitespace and trailing slashes are ignored; a path, query,
// fragment or userinfo makes it unrecognised.
func isRecognisedFrontend(frontendURL string, deployedEnvironment bool) bool {
	if _, isDeployed := deployedBackendOrigins[normalizeURL(frontendURL)]; isDeployed {
		return true
	}
	host, ok := bareOriginHost(frontendURL)
	return ok && !deployedEnvironment && isLoopbackHost(host)
}

// BackendOrigin is ResolveBackendOrigin for this configuration.
func (c *Config) BackendOrigin() string {
	return ResolveBackendOrigin(c.Email.FrontendURL, c.Server.BackendPublicURL, c.Server.Addr)
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

// bareOriginHost returns the hostname of raw, normalized, when it is a bare
// http(s) origin: a scheme and a host, with no userinfo, path, query or
// fragment, not even an empty "?" or "#".
func bareOriginHost(raw string) (host string, ok bool) {
	normalized := normalizeURL(raw)
	u, err := url.Parse(normalized)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil ||
		u.Path != "" || u.RawQuery != "" || u.ForceQuery || strings.Contains(normalized, "#") || u.Opaque != "" {
		return "", false
	}
	return u.Hostname(), true
}

// canonicalHost lowercases host and drops its trailing dots, the forms a
// resolver treats as the same name.
func canonicalHost(host string) string {
	return strings.ToLower(strings.TrimRight(host, "."))
}

// isLoopbackHost reports whether host, ignoring case and trailing dots, is
// localhost, a name under .localhost, or a loopback IP address.
func isLoopbackHost(host string) bool {
	name := canonicalHost(host)
	if name == "localhost" || strings.HasSuffix(name, ".localhost") {
		return true
	}
	ip := net.ParseIP(name)
	return ip != nil && ip.IsLoopback()
}

// isUnspecifiedHost reports whether host, ignoring trailing dots, is the IPv4
// or IPv6 wildcard address, which a server listens on but no client can reach.
func isUnspecifiedHost(host string) bool {
	ip := net.ParseIP(canonicalHost(host))
	return ip != nil && ip.IsUnspecified()
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
