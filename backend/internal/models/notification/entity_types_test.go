package notification

import (
	"reflect"
	"strings"
	"testing"
)

func TestParseNotificationEntityTypes(t *testing.T) {
	cases := []struct {
		name    string
		csv     string
		want    []string
		wantErr string
	}{
		{name: "empty means no narrowing", csv: "", want: nil},
		{name: "blank means no narrowing", csv: "  ", want: nil},
		{name: "one type", csv: "venue_show_alert", want: []string{"venue_show_alert"}},
		{
			name: "several types keep first-seen order",
			csv:  "venue_show_alert,artist_show_alert",
			want: []string{"venue_show_alert", "artist_show_alert"},
		},
		{
			name: "whitespace is trimmed and duplicates collapse",
			csv:  " artist_show_alert , artist_show_alert,venue_show_alert",
			want: []string{"artist_show_alert", "venue_show_alert"},
		},
		{name: "unknown type is rejected", csv: "artist_show_alert,bogus", wantErr: `unknown notification type "bogus"`},
		{name: "case is not folded", csv: "Artist_Show_Alert", wantErr: "unknown notification type"},
		{name: "trailing comma is rejected", csv: "artist_show_alert,", wantErr: "empty value"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ParseNotificationEntityTypes(tc.csv)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("err = %v, want one containing %q", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("got %#v, want %#v", got, tc.want)
			}
		})
	}
}
