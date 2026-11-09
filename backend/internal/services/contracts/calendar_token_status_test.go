package contracts

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	engagementm "psychic-homily-backend/internal/models/engagement"
)

// WithoutFeedURLs keeps only whether a token exists and when it was minted:
// the observed reads and the link-reset marker are withheld along with the
// URLs, by construction, so none reaches the JSON an API-token caller gets.
func TestCalendarTokenStatusResponse_WithoutFeedURLsWithholdsReadsAndReset(t *testing.T) {
	now := time.Date(2026, 11, 9, 12, 0, 0, 0, time.UTC)
	reason := engagementm.CalendarLinkResetPasswordReset
	full := CalendarTokenStatusResponse{
		HasToken:       true,
		CreatedAt:      &now,
		FeedURL:        "https://api.example.test/feeds/phcal_x/saved-shows.ics",
		FollowsFeedURL: "https://api.example.test/feeds/phcal_x/follows.atom",
		URLRecoverable: true,
		Reads: []CalendarFeedReadResponse{
			{Client: engagementm.CalendarFeedClientGoogle, FirstReadAt: now, LastReadAt: now},
		},
		LinkResetAt:     &now,
		LinkResetReason: &reason,
	}

	withheld := full.WithoutFeedURLs()

	if !withheld.HasToken || withheld.CreatedAt == nil || !withheld.CreatedAt.Equal(now) {
		t.Errorf("has_token and created_at must survive, got %+v", withheld)
	}
	raw, err := json.Marshal(withheld)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var fields map[string]any
	if err := json.Unmarshal(raw, &fields); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	for _, key := range []string{"reads", "link_reset_at", "link_reset_reason", "feed_url", "follows_feed_url"} {
		if _, present := fields[key]; present {
			t.Errorf("%s must be absent for a caller that may not receive the feed URLs, got %s", key, raw)
		}
	}
	if fields["url_recoverable"] != false {
		t.Errorf("url_recoverable = %v, want false", fields["url_recoverable"])
	}
}

// FeedStatus copies every field CalendarFeedStatusResponse declares from its
// same-named twin. Every field of the source is set to a non-zero value, so a
// field declared on both types but left out of the copy reads as zero here
// and fails.
func TestCalendarTokenStatusResponse_FeedStatusCopiesEveryDeclaredField(t *testing.T) {
	var full CalendarTokenStatusResponse
	fillNonZero(t, reflect.ValueOf(&full).Elem())

	got := reflect.ValueOf(full.FeedStatus())
	source := reflect.ValueOf(full)
	for i := 0; i < got.NumField(); i++ {
		name := got.Type().Field(i).Name
		if want := source.FieldByName(name); !reflect.DeepEqual(got.Field(i).Interface(), want.Interface()) {
			t.Errorf("FeedStatus().%s = %v, want %v copied from the token status", name, got.Field(i).Interface(), want.Interface())
		}
	}
}

// fillNonZero sets v, and everything it holds, to a value that is not its
// type's zero value.
func fillNonZero(t *testing.T, v reflect.Value) {
	t.Helper()
	if v.Type() == reflect.TypeOf(time.Time{}) {
		v.Set(reflect.ValueOf(time.Date(2026, 11, 9, 12, 0, 0, 0, time.UTC)))
		return
	}
	switch v.Kind() {
	case reflect.Bool:
		v.SetBool(true)
	case reflect.String:
		v.SetString("x")
	case reflect.Pointer:
		v.Set(reflect.New(v.Type().Elem()))
		fillNonZero(t, v.Elem())
	case reflect.Slice:
		v.Set(reflect.MakeSlice(v.Type(), 1, 1))
		fillNonZero(t, v.Index(0))
	case reflect.Struct:
		for i := 0; i < v.NumField(); i++ {
			fillNonZero(t, v.Field(i))
		}
	default:
		t.Fatalf("fillNonZero: add a case for %s", v.Type())
	}
}

// Every field of the address-free status is the same-named field of the
// token status, with the same type and tags, so the two publish one contract
// for what they share. None of the token status's address fields is among
// them.
func TestCalendarFeedStatusResponse_FieldsMirrorTheTokenStatus(t *testing.T) {
	feed := reflect.TypeOf(CalendarFeedStatusResponse{})
	token := reflect.TypeOf(CalendarTokenStatusResponse{})
	addresses := map[string]bool{"FeedURL": true, "FollowsFeedURL": true, "URLRecoverable": true}

	for i := 0; i < feed.NumField(); i++ {
		f := feed.Field(i)
		if addresses[f.Name] {
			t.Errorf("CalendarFeedStatusResponse has address field %s", f.Name)
			continue
		}
		twin, ok := token.FieldByName(f.Name)
		if !ok {
			t.Errorf("CalendarFeedStatusResponse.%s has no twin on CalendarTokenStatusResponse", f.Name)
			continue
		}
		if f.Type != twin.Type || f.Tag != twin.Tag {
			t.Errorf("CalendarFeedStatusResponse.%s = %s `%s`, token status has %s `%s`",
				f.Name, f.Type, f.Tag, twin.Type, twin.Tag)
		}
	}
}

// The API enum tags list exactly the model's vocabularies, in order, so the
// published schema and the generated frontend types cannot drift from the
// values the server writes.
func TestCalendarStatusEnumTagsMatchTheModelVocabularies(t *testing.T) {
	var clients []string
	for _, c := range engagementm.CalendarFeedClients {
		clients = append(clients, string(c))
	}
	var reasons []string
	for _, r := range engagementm.CalendarLinkResetReasons {
		reasons = append(reasons, string(r))
	}

	cases := []struct {
		structType reflect.Type
		field      string
		want       string
	}{
		{reflect.TypeOf(CalendarFeedReadResponse{}), "Client", strings.Join(clients, ",")},
		{reflect.TypeOf(CalendarTokenStatusResponse{}), "LinkResetReason", strings.Join(reasons, ",")},
	}
	for _, c := range cases {
		field, ok := c.structType.FieldByName(c.field)
		if !ok {
			t.Fatalf("%s has no field %s", c.structType.Name(), c.field)
		}
		if got := field.Tag.Get("enum"); got != c.want {
			t.Errorf("%s.%s enum tag = %q, want %q", c.structType.Name(), c.field, got, c.want)
		}
	}
}
