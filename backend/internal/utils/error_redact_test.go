package utils

import (
	"errors"
	"fmt"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRedactErrorURL(t *testing.T) {
	const secret = "SECRET-TOKEN-abc"

	t.Run("redacts url.Error path and query", func(t *testing.T) {
		in := &url.Error{
			Op:  "Post",
			URL: "https://discord.com/api/webhooks/123456789/" + secret + "?wait=true",
			Err: errors.New("dial tcp: connection refused"),
		}

		got := RedactErrorURL(in)
		msg := got.Error()

		assert.NotContains(t, msg, secret, "secret token must not survive redaction")
		assert.NotContains(t, msg, "webhooks", "secret-bearing path must be stripped")
		assert.NotContains(t, msg, "wait=true", "query must be stripped")
		assert.Contains(t, msg, "discord.com", "host is preserved for debugging")
		assert.Contains(t, msg, "[redacted]", "placeholder marks the stripped segment")
		assert.Contains(t, msg, "dial tcp: connection refused", "underlying cause is preserved")
	})

	t.Run("non-url.Error passes through unchanged", func(t *testing.T) {
		in := errors.New("plain failure")
		got := RedactErrorURL(in)

		assert.Same(t, in, got, "non-url errors are returned as-is")
		assert.Equal(t, "plain failure", got.Error())
	})

	t.Run("nil passes through", func(t *testing.T) {
		assert.NoError(t, RedactErrorURL(nil))
	})

	t.Run("redacts url.Error wrapped by fmt.Errorf", func(t *testing.T) {
		inner := &url.Error{
			Op:  "Post",
			URL: "https://discord.com/api/webhooks/9/" + secret,
			Err: errors.New("timeout"),
		}
		wrapped := fmt.Errorf("discord webhook failed: %w", inner)

		got := RedactErrorURL(wrapped)
		msg := got.Error()

		assert.NotContains(t, msg, secret, "secret in a wrapped url.Error must be stripped")
		assert.Contains(t, msg, "discord.com", "host is preserved")
	})

	t.Run("redacted error preserves chain for errors.As", func(t *testing.T) {
		in := &url.Error{
			Op:  "Get",
			URL: "https://example.com/secret-path",
			Err: errors.New("boom"),
		}

		got := RedactErrorURL(in)

		var asURLErr *url.Error
		assert.True(t, errors.As(got, &asURLErr), "result is still a *url.Error")
		assert.NotContains(t, asURLErr.URL, "secret-path", "URL field itself is redacted")
		assert.Equal(t, "Get", asURLErr.Op, "Op is preserved")
	})

	t.Run("unparseable or hostless URL collapses to placeholder", func(t *testing.T) {
		in := &url.Error{
			Op:  "Get",
			URL: "://" + secret, // missing scheme -> parse error
			Err: errors.New("bad"),
		}

		got := RedactErrorURL(in)
		assert.NotContains(t, got.Error(), secret, "no recognizable URL survives a parse failure")
		assert.True(t, strings.Contains(got.Error(), "[redacted]"))
	})
}
