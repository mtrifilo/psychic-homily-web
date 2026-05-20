package utils

import (
	"errors"
	"fmt"
	"net/url"
)

// RedactErrorURL strips the path and query from any *url.Error embedded in err,
// returning an error whose message no longer exposes URL-borne secrets.
//
// Go's net/http surfaces transport failures as *url.Error, and its Error()
// string embeds the full request URL. When that URL carries a secret in its
// path or query — a Discord webhook token, a signed callback, an API key query
// param — wrapping the error and forwarding it to Sentry persists the secret in
// searchable, long-retention storage. Redacting before capture keeps the host
// (useful for debugging which endpoint failed) while dropping the secret-bearing
// path and query.
//
// Errors that do not wrap a *url.Error are returned unchanged. The redacted
// copy preserves Op and the underlying Err, so error-chain inspection
// (errors.Is / errors.As against the transport cause) keeps working.
func RedactErrorURL(err error) error {
	if err == nil {
		return nil
	}

	var urlErr *url.Error
	if !errors.As(err, &urlErr) {
		return err
	}

	return &url.Error{
		Op:  urlErr.Op,
		URL: redactURL(urlErr.URL),
		Err: urlErr.Err,
	}
}

// redactURL keeps a parseable URL's scheme and host and replaces the rest with
// a placeholder. Inputs that fail to parse, or that lack a host, collapse to a
// bare placeholder so nothing recognizable survives.
//
// The result is assembled by hand rather than via url.URL.String(): the latter
// percent-encodes the placeholder ("[redacted]" -> "%5Bredacted%5D"), which is
// harder to recognize at a glance in a Sentry message.
func redactURL(raw string) string {
	const placeholder = "[redacted]"

	u, parseErr := url.Parse(raw)
	if parseErr != nil || u.Host == "" {
		return placeholder
	}
	if u.Scheme == "" {
		return u.Host + "/" + placeholder
	}
	return fmt.Sprintf("%s://%s/%s", u.Scheme, u.Host, placeholder)
}
