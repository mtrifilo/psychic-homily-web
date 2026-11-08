package auth

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"
)

// Calendar sync choice: what the user decided the first time they were asked
// to put saved shows on a calendar. Stored in user_preferences.calendar_sync
// as a nullable JSONB document; NULL means they have not decided yet, the only
// state in which the client asks.
//
// Lives in models/auth, beside HomeLayout and for the same reason: contracts
// imports this package, so the service that writes the document, the handler
// that rejects a bad one and the interface between them name one type.

// CalendarSyncVersion is the only document version this server accepts. A
// newer client's document may carry meaning in fields this build does not
// model, so storing it after a partial read would silently discard them.
const CalendarSyncVersion = 1

// States. These are a STORAGE CONTRACT shared with the frontend, so they are
// renamed only alongside a migration that rewrites stored documents.
const (
	CalendarSyncConnected = "connected"
	CalendarSyncNotNow    = "not_now"
	CalendarSyncNever     = "never"
)

// Providers: how a connected user put shows on a calendar. Same storage
// contract as the states. A provider is recorded when the user taps it; the
// web cannot observe whether a subscription actually took.
const (
	CalendarProviderAppleSubscribe  = "apple_subscribe"
	CalendarProviderGoogleSubscribe = "google_subscribe"
	CalendarProviderGooglePerShow   = "google_per_show"
	CalendarProviderApplePerShow    = "apple_per_show"
	CalendarProviderCopied          = "copied"
	CalendarProviderShared          = "shared"
	CalendarProviderDownloaded      = "downloaded"
)

// CalendarSyncMaxDecline bounds how far past decided_at a not_now may run.
// The client sends decided_at + 30 days; the extra day absorbs a client that
// adds 30 calendar days across a daylight-saving change. A longer decline is
// "never" in disguise, and that has its own state.
const CalendarSyncMaxDecline = 31 * 24 * time.Hour

// knownCalendarSyncStates and knownCalendarProviders are the whitelists. The
// enum tags on CalendarSync must spell them literally (struct tags must be
// constant), and TestCalendarSyncEnumTagsMatchVocabulary keeps them in step.
var (
	knownCalendarSyncStates = []string{
		CalendarSyncConnected,
		CalendarSyncNotNow,
		CalendarSyncNever,
	}
	knownCalendarProviders = []string{
		CalendarProviderAppleSubscribe,
		CalendarProviderGoogleSubscribe,
		CalendarProviderGooglePerShow,
		CalendarProviderApplePerShow,
		CalendarProviderCopied,
		CalendarProviderShared,
		CalendarProviderDownloaded,
	}
)

// ErrInvalidCalendarSync marks every rejection below, so one errors.Is at the
// transport boundary separates "the client sent a bad document" (422) from
// "the write failed" (500). The wrapped text says which rule was broken.
var ErrInvalidCalendarSync = errors.New("invalid calendar sync choice")

// calendarSyncVocabularyCSV renders a whitelist in the CSV form an enum tag
// spells. Test-only; see knownCalendarSyncStates.
func calendarSyncVocabularyCSV(values []string) string {
	return strings.Join(values, ",")
}

// CalendarSync is the whole stored document. Replace semantics: a write
// carries the complete choice, including every provider the user has
// connected, and the stored value is exactly what the next read returns.
//
// The schema is nullable so that a response field holding it can say "no
// decision yet" as null; huma cannot mark a $ref FIELD nullable, only the
// referenced schema.
//
// An expired not_now is NOT rewritten on read: the stored document keeps
// saying not_now, and a client compares declined_until with its own clock.
type CalendarSync struct {
	_             struct{}   `nullable:"true"`
	Version       int        `json:"version" doc:"Document version; must be 1"`
	State         string     `json:"state" enum:"connected,not_now,never" doc:"connected, not_now (declined until declined_until) or never"`
	Providers     []string   `json:"providers,omitempty" enum:"apple_subscribe,google_subscribe,google_per_show,apple_per_show,copied,shared,downloaded" minItems:"1" maxItems:"7" uniqueItems:"true" doc:"Every way the user connected a calendar; required, non-empty and distinct with connected, absent otherwise"`
	DecidedAt     time.Time  `json:"decided_at" doc:"When the user made this choice (RFC 3339)"`
	DeclinedUntil *time.Time `json:"declined_until,omitempty" doc:"When a not_now lapses; required with not_now, absent otherwise. Later than decided_at by at most 31 days"`
	Declines      *int       `json:"declines,omitempty" minimum:"0" maximum:"2" doc:"How many times the user has said not now, counted by the client; required with not_now (0 to 2), absent otherwise"`
}

// CalendarSyncMaxDeclines is the highest declines count. The product asks at
// most twice before it stops, so a count above this means the client is still
// asking after the point the product promised to stop. The maximum tag on
// CalendarSync.Declines spells the same number; the model test joins them.
const CalendarSyncMaxDeclines = 2

// Validate reports whether the document may be stored. Every rejection wraps
// ErrInvalidCalendarSync.
//
// The enum tags reject an unknown state or provider at the transport layer
// before this runs. These rules are still the authority: they are what a
// non-HTTP caller of the service gets, and they cover the per-state field
// rules no schema tag can express.
func (c *CalendarSync) Validate() error {
	if c == nil {
		return fmt.Errorf("%w: document is missing", ErrInvalidCalendarSync)
	}
	if c.Version != CalendarSyncVersion {
		return fmt.Errorf("%w: unsupported version %d, expected %d",
			ErrInvalidCalendarSync, c.Version, CalendarSyncVersion)
	}
	if c.DecidedAt.IsZero() {
		return fmt.Errorf("%w: decided_at is required", ErrInvalidCalendarSync)
	}

	// Exact match throughout: no trimming, no case folding. The values are
	// generated by the client, so one needing repair is a client defect, and
	// repairing it here would let two spellings of one value look valid.
	if !slices.Contains(knownCalendarSyncStates, c.State) {
		return fmt.Errorf("%w: unknown state %q", ErrInvalidCalendarSync, c.State)
	}

	// Each optional field belongs to exactly one state. A field carried by
	// another state is rejected, not dropped, so a stored document never
	// holds a value no reader of its state looks at.
	if c.State != CalendarSyncConnected && c.Providers != nil {
		return fmt.Errorf("%w: providers are only allowed with %s",
			ErrInvalidCalendarSync, CalendarSyncConnected)
	}
	if c.State != CalendarSyncNotNow && c.DeclinedUntil != nil {
		return fmt.Errorf("%w: declined_until is only allowed with %s",
			ErrInvalidCalendarSync, CalendarSyncNotNow)
	}
	if c.State != CalendarSyncNotNow && c.Declines != nil {
		return fmt.Errorf("%w: declines is only allowed with %s",
			ErrInvalidCalendarSync, CalendarSyncNotNow)
	}

	switch c.State {
	case CalendarSyncConnected:
		return validateCalendarProviders(c.Providers)
	case CalendarSyncNotNow:
		return c.validateNotNow()
	}
	return nil
}

// validateNotNow enforces the fields a not_now requires: a declined_until
// after decided_at by at most CalendarSyncMaxDecline, and a declines count
// from 0 to CalendarSyncMaxDeclines.
func (c *CalendarSync) validateNotNow() error {
	if c.DeclinedUntil == nil {
		return fmt.Errorf("%w: declined_until is required with %s",
			ErrInvalidCalendarSync, CalendarSyncNotNow)
	}
	if !c.DeclinedUntil.After(c.DecidedAt) {
		return fmt.Errorf("%w: declined_until must be after decided_at", ErrInvalidCalendarSync)
	}
	if c.DeclinedUntil.Sub(c.DecidedAt) > CalendarSyncMaxDecline {
		return fmt.Errorf("%w: declined_until must be at most 31 days after decided_at",
			ErrInvalidCalendarSync)
	}
	if c.Declines == nil {
		return fmt.Errorf("%w: declines is required with %s",
			ErrInvalidCalendarSync, CalendarSyncNotNow)
	}
	if *c.Declines < 0 || *c.Declines > CalendarSyncMaxDeclines {
		return fmt.Errorf("%w: declines must be from 0 to %d, got %d",
			ErrInvalidCalendarSync, CalendarSyncMaxDeclines, *c.Declines)
	}
	return nil
}

// validateCalendarProviders enforces the connected-state list: at least one
// provider, each known, none repeated. A repeat is rejected rather than
// collapsed because the client sends the merged list, so a duplicate means
// its merge is wrong.
func validateCalendarProviders(providers []string) error {
	if len(providers) == 0 {
		return fmt.Errorf("%w: providers are required with %s",
			ErrInvalidCalendarSync, CalendarSyncConnected)
	}
	seen := make(map[string]struct{}, len(providers))
	for _, provider := range providers {
		if !slices.Contains(knownCalendarProviders, provider) {
			return fmt.Errorf("%w: unknown provider %q", ErrInvalidCalendarSync, provider)
		}
		if _, duplicate := seen[provider]; duplicate {
			return fmt.Errorf("%w: duplicate provider %q", ErrInvalidCalendarSync, provider)
		}
		seen[provider] = struct{}{}
	}
	return nil
}
