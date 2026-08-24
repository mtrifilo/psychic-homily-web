package shared

import (
	"errors"
	"strings"
	"testing"

	"gorm.io/gorm"

	catalogm "psychic-homily-backend/internal/models/catalog"
	"psychic-homily-backend/internal/testutil"
)

// NormalizeIANATimezone is validated against a REAL Postgres rather than a
// stub, because the entire point of the function is that Postgres' zone catalog
// and Go's differ. A fake would agree with whatever the test author assumed and
// prove nothing.
func TestNormalizeIANATimezone(t *testing.T) {
	db := testutil.SetupTestPostgres(t).DB

	str := func(s string) *string { return &s }

	cases := []struct {
		name    string
		in      *string
		want    *string // nil means "expect NULL"
		wantErr bool
	}{
		// Absent / empty: store NULL and let the caller's fallback chain decide.
		{name: "nil pointer", in: nil, want: nil},
		{name: "empty string", in: str(""), want: nil},
		{name: "whitespace only", in: str("   "), want: nil},
		{name: "tabs and newlines only", in: str("\t\n "), want: nil},

		// Canonical names pass through unchanged.
		{name: "canonical US zone", in: str("America/Phoenix"), want: str("America/Phoenix")},
		{name: "canonical three-part zone", in: str("America/Indiana/Indianapolis"), want: str("America/Indiana/Indianapolis")},
		{name: "canonical non-US zone", in: str("Europe/Ljubljana"), want: str("Europe/Ljubljana")},
		{name: "canonical UTC", in: str("UTC"), want: str("UTC")},

		// Non-canonical spelling resolves to the catalog's own spelling, which
		// is what gets persisted -- that is the "normalize" half of the name.
		{name: "lowercased", in: str("america/phoenix"), want: str("America/Phoenix")},
		{name: "uppercased", in: str("AMERICA/PHOENIX"), want: str("America/Phoenix")},
		{name: "surrounded by spaces", in: str("  America/Phoenix  "), want: str("America/Phoenix")},
		{name: "surrounded by tabs and newlines", in: str("\tAmerica/Phoenix\n"), want: str("America/Phoenix")},

		// Junk is rejected rather than silently stored.
		{name: "obvious junk", in: str("Not/AZone"), wantErr: true},
		{name: "sql-ish junk", in: str("'; DROP TABLE venues; --"), wantErr: true},
		{name: "empty-ish path", in: str("/"), wantErr: true},
		{name: "plain word", in: str("Phoenix"), wantErr: true},
		{name: "numeric", in: str("-07:00"), wantErr: true},

		// "Local" is a Go alias with no pg_timezone_names entry on any build.
		{name: "Go alias Local", in: str("Local"), wantErr: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := NormalizeIANATimezone(db, tc.in)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected an error for %q, got %v", derefForTest(tc.in), derefForTest(got))
				}
				if got != nil {
					t.Errorf("a rejected value must not also return a zone, got %q", *got)
				}
				// The message has to name the offending value or an operator
				// reading the log cannot tell which venue to fix.
				if !strings.Contains(err.Error(), strings.TrimSpace(derefForTest(tc.in))) {
					t.Errorf("error %q does not name the rejected value", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error for %q: %v", derefForTest(tc.in), err)
			}
			switch {
			case tc.want == nil && got != nil:
				t.Errorf("expected NULL, got %q", *got)
			case tc.want != nil && got == nil:
				t.Errorf("expected %q, got NULL", *tc.want)
			case tc.want != nil && got != nil && *got != *tc.want:
				t.Errorf("expected %q, got %q", *tc.want, *got)
			}
		})
	}
}

// Fixed-offset abbreviations and deprecated aliases are the interesting class,
// and their presence in pg_timezone_names depends on the SERVER'S TZDATA
// PACKAGING, not on Postgres: postgres:18 (Debian) ships 487 zones without EST
// or Asia/Calcutta because Debian splits the `backward` links into
// tzdata-legacy, while postgres:16-alpine ships 599 with them. So the
// expectation is derived from the live catalog instead of hard-coded -- a
// hard-coded one turns an image bump into a mystifying failure in this package.
func TestNormalizeIANATimezone_TracksTheServersCatalog(t *testing.T) {
	db := testutil.SetupTestPostgres(t).DB

	for _, name := range []string{"EST", "MST", "US/Arizona", "US/Eastern", "Asia/Calcutta", "Navajo"} {
		var canonical string
		if err := db.Raw(
			"SELECT COALESCE((SELECT n.name FROM pg_timezone_names n WHERE lower(n.name) = lower(?) LIMIT 1), '')", name,
		).Scan(&canonical).Error; err != nil {
			t.Fatal(err)
		}
		got, err := NormalizeIANATimezone(db, &name)
		if canonical == "" {
			if err == nil {
				t.Errorf("%s: absent from this server's catalog but was accepted", name)
			} else if !errors.Is(err, ErrUnknownTimezone) {
				t.Errorf("%s: expected ErrUnknownTimezone, got %v", name, err)
			}
			continue
		}
		if err != nil {
			t.Errorf("%s: present in this server's catalog but was rejected: %v", name, err)
		} else if got == nil || *got != canonical {
			t.Errorf("%s: expected canonical %q, got %v", name, canonical, got)
		}
	}
}

// A database failure must NOT look like a bad zone: the derived value survives.
// Otherwise one pool timeout silently blanks a venue's timezone forever.
func TestNormalizedGeocodedTimezoneOrNull_KeepsValueWhenValidationFails(t *testing.T) {
	db := testutil.SetupTestPostgres(t).DB
	good := "America/Phoenix"

	// Valid zone survives; junk is nulled.
	if got := NormalizedGeocodedTimezoneOrNull(db, &good); got == nil || *got != good {
		t.Errorf("valid zone should pass through, got %v", got)
	}
	junk := "Not/AZone"
	if got := NormalizedGeocodedTimezoneOrNull(db, &junk); got != nil {
		t.Errorf("unknown zone should be nulled, got %q", *got)
	}

	// A closed pool is a DB failure, not a verdict on the zone.
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	broken, err := gorm.Open(db.Dialector, &gorm.Config{})
	if err != nil {
		t.Skipf("cannot build a second handle to simulate a DB failure: %v", err)
	}
	if inner, err := broken.DB(); err == nil {
		_ = inner.Close()
	}
	_ = sqlDB
	if got := NormalizedGeocodedTimezoneOrNull(broken, &good); got == nil || *got != good {
		t.Errorf("a DB failure must keep the derived value, got %v", got)
	}
}

// A nil DB must be an error from the validator itself, not a panic and not a
// silent pass. (Its log-and-NULL wrapper deliberately differs -- see
// NormalizedGeocodedTimezoneOrNull.)
func TestNormalizeIANATimezone_NilDBIsAnError(t *testing.T) {
	value := "America/Phoenix"
	got, err := NormalizeIANATimezone(nil, &value)
	if err == nil {
		t.Fatalf("expected an error with a nil DB, got %v", derefForTest(got))
	}
	if got != nil {
		t.Errorf("expected no zone with a nil DB, got %q", *got)
	}
}

// ...but a nil/blank input short-circuits BEFORE the DB is consulted, so it
// stays usable on a code path that has no handle.
func TestNormalizeIANATimezone_NilInputSkipsTheDB(t *testing.T) {
	got, err := NormalizeIANATimezone(nil, nil)
	if err != nil || got != nil {
		t.Fatalf("nil input must return (nil, nil) without touching the DB, got (%v, %v)", derefForTest(got), err)
	}
	blank := "  "
	got, err = NormalizeIANATimezone(nil, &blank)
	if err != nil || got != nil {
		t.Fatalf("blank input must return (nil, nil) without touching the DB, got (%v, %v)", derefForTest(got), err)
	}
}

// VenueTimezoneByNameCity is the lookup every name-keyed WRITER uses to find
// the zone a show's wall-clock time should be anchored in, so its three
// outcomes have to stay distinguishable: a zone, "no zone known" (fall back to
// the state map), and a query failure (which must never be read as "no zone").
func TestVenueTimezoneByNameCity(t *testing.T) {
	db := testutil.SetupTestPostgres(t).DB

	london := "Europe/London"
	seed := []catalogm.Venue{
		{Name: "Boom Leeds", City: "Leeds", State: "England", Timezone: &london},
		{Name: "Zoneless Hall", City: "Phoenix", State: "AZ"},
	}
	for i := range seed {
		if err := db.Create(&seed[i]).Error; err != nil {
			t.Fatalf("seed venue %q: %v", seed[i].Name, err)
		}
	}

	t.Run("returns the stored zone", func(t *testing.T) {
		got, err := VenueTimezoneByNameCity(db, "Boom Leeds", "Leeds")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got == nil || *got != london {
			t.Errorf("got %q, want %q", derefForTest(got), london)
		}
	})

	// The venues uniqueness index is on LOWER(name), LOWER(city), so this
	// lookup has to match it or a writer misses the row it is about to link to.
	t.Run("matches case-insensitively on name and city", func(t *testing.T) {
		got, err := VenueTimezoneByNameCity(db, "boom leeds", "LEEDS")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got == nil || *got != london {
			t.Errorf("got %q, want %q", derefForTest(got), london)
		}
	})

	// Both "no such venue" and "venue has no zone" mean the caller keeps its
	// state-map fallback, and neither is an error.
	t.Run("nil for a venue with no stored zone", func(t *testing.T) {
		got, err := VenueTimezoneByNameCity(db, "Zoneless Hall", "Phoenix")
		if err != nil || got != nil {
			t.Errorf("got (%q, %v), want (nil, nil)", derefForTest(got), err)
		}
	})

	t.Run("nil for a venue that does not exist yet", func(t *testing.T) {
		got, err := VenueTimezoneByNameCity(db, "Not A Venue", "Nowhere")
		if err != nil || got != nil {
			t.Errorf("got (%q, %v), want (nil, nil)", derefForTest(got), err)
		}
	})

	// A different city is a different venue, even under the same name.
	t.Run("does not match a same-named venue in another city", func(t *testing.T) {
		got, err := VenueTimezoneByNameCity(db, "Boom Leeds", "Phoenix")
		if err != nil || got != nil {
			t.Errorf("got (%q, %v), want (nil, nil)", derefForTest(got), err)
		}
	})

	// No handle means nothing is being persisted, so there is nothing to look
	// up and the caller keeps its fallback.
	t.Run("nil DB is not an error", func(t *testing.T) {
		got, err := VenueTimezoneByNameCity(nil, "Boom Leeds", "Leeds")
		if err != nil || got != nil {
			t.Errorf("got (%q, %v), want (nil, nil)", derefForTest(got), err)
		}
	})
}

// A QUERY failure must surface rather than degrade to "no zone": inside a
// transaction it also poisons the statements after it, so a swallowed error
// reappears as an unrelated abort further along.
func TestVenueTimezoneByNameCity_QueryFailureIsAnError(t *testing.T) {
	db := testutil.SetupTestPostgres(t).DB
	if err := db.Exec("ALTER TABLE venues RENAME TO venues_hidden").Error; err != nil {
		t.Fatalf("rename venues: %v", err)
	}
	t.Cleanup(func() {
		_ = db.Exec("ALTER TABLE venues_hidden RENAME TO venues").Error
	})

	got, err := VenueTimezoneByNameCity(db, "Boom Leeds", "Leeds")
	if err == nil {
		t.Fatalf("expected an error when the query fails, got zone %q", derefForTest(got))
	}
	if got != nil {
		t.Errorf("a failed query must not return a zone, got %q", *got)
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		t.Error("a query failure must not be reported as record-not-found")
	}
}

func derefForTest(p *string) string {
	if p == nil {
		return "<nil>"
	}
	return *p
}
