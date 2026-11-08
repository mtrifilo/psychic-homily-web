package auth

import (
	"encoding/json"
	"errors"
	"reflect"
	"strconv"
	"testing"
	"time"
)

var calendarSyncDecidedAt = time.Date(2026, 11, 8, 18, 30, 0, 0, time.UTC)

func timePtr(t time.Time) *time.Time { return &t }

func connectedCalendarSync(providers ...string) *CalendarSync {
	return &CalendarSync{
		Version:   CalendarSyncVersion,
		State:     CalendarSyncConnected,
		Providers: providers,
		DecidedAt: calendarSyncDecidedAt,
	}
}

func intPtr(n int) *int { return &n }

func notNowCalendarSync(until time.Duration) *CalendarSync {
	return &CalendarSync{
		Version:       CalendarSyncVersion,
		State:         CalendarSyncNotNow,
		DecidedAt:     calendarSyncDecidedAt,
		DeclinedUntil: timePtr(calendarSyncDecidedAt.Add(until)),
		Declines:      intPtr(1),
	}
}

func notNowWithDeclines(declines *int) *CalendarSync {
	doc := notNowCalendarSync(30 * 24 * time.Hour)
	doc.Declines = declines
	return doc
}

func neverCalendarSync() *CalendarSync {
	return &CalendarSync{
		Version:   CalendarSyncVersion,
		State:     CalendarSyncNever,
		DecidedAt: calendarSyncDecidedAt,
	}
}

func TestCalendarSyncValidate_AcceptsEachState(t *testing.T) {
	cases := []struct {
		name string
		doc  *CalendarSync
	}{
		{"connected with one provider", connectedCalendarSync(CalendarProviderGoogleSubscribe)},
		{"connected with two providers", connectedCalendarSync(
			CalendarProviderGoogleSubscribe, CalendarProviderAppleSubscribe)},
		{"connected with every provider", connectedCalendarSync(knownCalendarProviders...)},
		{"not_now for 30 days", notNowCalendarSync(30 * 24 * time.Hour)},
		{"not_now at the 31-day cap", notNowCalendarSync(CalendarSyncMaxDecline)},
		{"not_now for one second", notNowCalendarSync(time.Second)},
		{"not_now with zero declines", notNowWithDeclines(intPtr(0))},
		{"not_now with the second decline", notNowWithDeclines(intPtr(CalendarSyncMaxDeclines))},
		{"never", neverCalendarSync()},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.doc.Validate(); err != nil {
				t.Errorf("expected a valid document, got %v", err)
			}
		})
	}
}

func TestCalendarSyncValidate_Rejects(t *testing.T) {
	withVersion := func(v int) *CalendarSync {
		doc := neverCalendarSync()
		doc.Version = v
		return doc
	}
	withoutDecidedAt := neverCalendarSync()
	withoutDecidedAt.DecidedAt = time.Time{}

	notNowWithoutUntil := notNowCalendarSync(time.Hour)
	notNowWithoutUntil.DeclinedUntil = nil

	notNowWithProviders := notNowCalendarSync(30 * 24 * time.Hour)
	notNowWithProviders.Providers = []string{CalendarProviderCopied}

	notNowWithEmptyProviders := notNowCalendarSync(30 * 24 * time.Hour)
	notNowWithEmptyProviders.Providers = []string{}

	connectedWithUntil := connectedCalendarSync(CalendarProviderCopied)
	connectedWithUntil.DeclinedUntil = timePtr(calendarSyncDecidedAt.Add(time.Hour))

	neverWithProviders := neverCalendarSync()
	neverWithProviders.Providers = []string{CalendarProviderShared}

	neverWithUntil := neverCalendarSync()
	neverWithUntil.DeclinedUntil = timePtr(calendarSyncDecidedAt.Add(time.Hour))

	unknownState := neverCalendarSync()
	unknownState.State = "maybe"

	connectedWithDeclines := connectedCalendarSync(CalendarProviderCopied)
	connectedWithDeclines.Declines = intPtr(0)

	neverWithDeclines := neverCalendarSync()
	neverWithDeclines.Declines = intPtr(2)

	cases := []struct {
		name string
		doc  *CalendarSync
	}{
		{"nil document", nil},
		{"missing version", withVersion(0)},
		{"unsupported version", withVersion(2)},
		{"missing decided_at", withoutDecidedAt},
		{"unknown state", unknownState},
		{"empty state", &CalendarSync{Version: CalendarSyncVersion, DecidedAt: calendarSyncDecidedAt}},
		{"connected without providers", connectedCalendarSync()},
		{"connected with an empty provider list", connectedCalendarSync([]string{}...)},
		{"connected with an unknown provider", connectedCalendarSync("outlook")},
		{"connected with a provider needing repair", connectedCalendarSync(" google_subscribe")},
		{"connected with a duplicate provider", connectedCalendarSync(
			CalendarProviderAppleSubscribe, CalendarProviderGoogleSubscribe, CalendarProviderAppleSubscribe)},
		{"connected with declined_until", connectedWithUntil},
		{"not_now without declined_until", notNowWithoutUntil},
		{"not_now with declined_until equal to decided_at", notNowCalendarSync(0)},
		{"not_now with declined_until before decided_at", notNowCalendarSync(-time.Hour)},
		{"not_now past the 31-day cap", notNowCalendarSync(CalendarSyncMaxDecline + time.Second)},
		{"not_now with providers", notNowWithProviders},
		{"not_now with an empty provider list", notNowWithEmptyProviders},
		{"never with providers", neverWithProviders},
		{"never with declined_until", neverWithUntil},
		{"not_now without declines", notNowWithDeclines(nil)},
		{"not_now with negative declines", notNowWithDeclines(intPtr(-1))},
		{"not_now past the declines cap", notNowWithDeclines(intPtr(CalendarSyncMaxDeclines + 1))},
		{"connected with declines", connectedWithDeclines},
		{"never with declines", neverWithDeclines},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.doc.Validate()
			if err == nil {
				t.Fatalf("expected a rejection")
			}
			// The wrap is what the handler keys its 422 off, so it is part of
			// the contract, not an implementation detail.
			if !errors.Is(err, ErrInvalidCalendarSync) {
				t.Errorf("rejection must wrap ErrInvalidCalendarSync, got %v", err)
			}
		})
	}
}

// The JSON keys are a storage contract shared with the frontend and with every
// document already written, so they are pinned against an accidental rename.
// The fields a state does not use are absent, not null.
func TestCalendarSyncJSONKeys(t *testing.T) {
	cases := []struct {
		name string
		doc  *CalendarSync
		want string
	}{
		{"connected", connectedCalendarSync(CalendarProviderGoogleSubscribe, CalendarProviderAppleSubscribe),
			`{"version":1,"state":"connected","providers":["google_subscribe","apple_subscribe"],"decided_at":"2026-11-08T18:30:00Z"}`},
		{"not_now", notNowCalendarSync(30 * 24 * time.Hour),
			`{"version":1,"state":"not_now","decided_at":"2026-11-08T18:30:00Z","declined_until":"2026-12-08T18:30:00Z","declines":1}`},
		{"not_now with zero declines keeps the key", notNowWithDeclines(intPtr(0)),
			`{"version":1,"state":"not_now","decided_at":"2026-11-08T18:30:00Z","declined_until":"2026-12-08T18:30:00Z","declines":0}`},
		{"never", neverCalendarSync(),
			`{"version":1,"state":"never","decided_at":"2026-11-08T18:30:00Z"}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			encoded, err := json.Marshal(tc.doc)
			if err != nil {
				t.Fatalf("marshal failed: %v", err)
			}
			if string(encoded) != tc.want {
				t.Errorf("document shape drifted\n got: %s\nwant: %s", encoded, tc.want)
			}
		})
	}
}

// Struct tags must be constant literals, so the OpenAPI enums cannot be built
// from the whitelists. This test is the join: widening a whitelist without
// widening its tag would leave the API rejecting a value the service accepts,
// and the regenerated frontend types would disagree with the server.
func TestCalendarSyncEnumTagsMatchVocabulary(t *testing.T) {
	cases := []struct {
		field string
		want  []string
	}{
		{"State", knownCalendarSyncStates},
		{"Providers", knownCalendarProviders},
	}
	for _, tc := range cases {
		field, ok := reflect.TypeOf(CalendarSync{}).FieldByName(tc.field)
		if !ok {
			t.Fatalf("CalendarSync.%s must exist", tc.field)
		}
		if got := field.Tag.Get("enum"); got != calendarSyncVocabularyCSV(tc.want) {
			t.Errorf("%s enum tag must list exactly the whitelist, in order\n got: %s\nwant: %s",
				tc.field, got, calendarSyncVocabularyCSV(tc.want))
		}
	}

	providers, _ := reflect.TypeOf(CalendarSync{}).FieldByName("Providers")
	if got := providers.Tag.Get("maxItems"); got != strconv.Itoa(len(knownCalendarProviders)) {
		t.Errorf("schema maxItems is %q but the whitelist holds %d providers",
			got, len(knownCalendarProviders))
	}

	declines, _ := reflect.TypeOf(CalendarSync{}).FieldByName("Declines")
	if got := declines.Tag.Get("maximum"); got != strconv.Itoa(CalendarSyncMaxDeclines) {
		t.Errorf("schema maximum is %q but CalendarSyncMaxDeclines is %d",
			got, CalendarSyncMaxDeclines)
	}
}
