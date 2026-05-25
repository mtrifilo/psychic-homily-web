package shared

import (
	"context"
	"encoding/json"
	"io"

	"psychic-homily-backend/internal/logger"
)

// SafeWrite writes data to w. Logs at debug level on failure but does not
// return any error.
//
// Use this on HTTP response write paths where the request is already over
// and the caller has no meaningful action to take on a write failure
// (client disconnect, network timeout, full buffer). Centralizing the
// drop here also gives us one place to hang future observability hooks
// (e.g. a client-disconnect rate metric) without touching every handler.
//
// Do NOT use SafeWrite for writes to non-HTTP io.Writers (hash.Hash,
// bytes.Buffer, files) — those have their own error semantics and a
// debug log polluted with hash-write entries is noise. Use an explicit
//
//	_, _ = ...
//
// drop with a brief comment in those cases.
func SafeWrite(ctx context.Context, w io.Writer, data []byte) {
	if _, err := w.Write(data); err != nil {
		logger.FromContext(ctx).Debug("response write failed", "err", err)
	}
}

// SafeEncode JSON-encodes v to w. Logs at debug level on failure but does
// not return any error — same rationale as SafeWrite. Used for the
//
//	json.NewEncoder(w).Encode(v)
//
// drop pattern that errcheck flags on HTTP response paths.
func SafeEncode(ctx context.Context, w io.Writer, v any) {
	if err := json.NewEncoder(w).Encode(v); err != nil {
		logger.FromContext(ctx).Debug("response encode failed", "err", err)
	}
}
