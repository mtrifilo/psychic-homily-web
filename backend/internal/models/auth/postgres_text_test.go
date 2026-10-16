package auth

import (
	"testing"

	"psychic-homily-backend/internal/testutil"
)

var postgresRejectsTextCases = []struct {
	name  string
	value string
	want  bool
}{
	{"empty", "", false},
	{"plain username", "nobody", false},
	{"other control byte", "no\x01body", false},
	{"non-ASCII", "nöbody", false},
	{"noncharacter U+FFFE", "no￾body", false},
	{"NUL", "no\x00body", true},
	{"invalid byte", "no\xffbody", true},
	{"truncated sequence", "no\xc3", true},
	{"overlong encoding", "no\xc0\xaebody", true},
	{"UTF-16 surrogate", "no\xed\xa0\x80body", true},
}

func TestPostgresRejectsText(t *testing.T) {
	for _, tc := range postgresRejectsTextCases {
		t.Run(tc.name, func(t *testing.T) {
			if got := PostgresRejectsText(tc.value); got != tc.want {
				t.Errorf("PostgresRejectsText(%q) = %v, want %v", tc.value, got, tc.want)
			}
		})
	}
}

// TestPostgresRejectsText_AgreesWithPostgres binds every case as a text
// parameter against a real database: Postgres refuses exactly the values the
// predicate reports.
func TestPostgresRejectsText_AgreesWithPostgres(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	td := testutil.SetupTestPostgres(t)
	defer td.Cleanup()

	for _, tc := range postgresRejectsTextCases {
		t.Run(tc.name, func(t *testing.T) {
			var echoed string
			err := td.DB.Raw("SELECT ?::text", tc.value).Scan(&echoed).Error
			if refused := err != nil; refused != tc.want {
				t.Errorf("Postgres refused %q = %v (err %v), want %v", tc.value, refused, err, tc.want)
			}
		})
	}
}
