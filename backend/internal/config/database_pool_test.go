package config

import (
	"math"
	"strconv"
	"testing"
	"time"
)

// The pool bounds are the one config this service has that fails SILENTLY when
// wrong: every clamped value is legal to database/sql and simply means something
// other than what the operator asked for. So each clamp is pinned, including the
// ones whose whole job is to refuse a value that would disable the bound.
func TestDatabaseConfigFromEnv(t *testing.T) {
	maxLifetimeMinutes := int(math.MaxInt64 / int64(time.Minute))

	cases := []struct {
		name            string
		env             map[string]string
		wantMaxOpen     int
		wantMaxIdle     int
		wantConnMaxLife time.Duration
	}{
		{
			name:            "unset uses the defaults",
			wantMaxOpen:     DefaultDBMaxOpenConns,
			wantMaxIdle:     DefaultDBMaxIdleConns,
			wantConnMaxLife: DefaultDBConnMaxLifetimeMinutes * time.Minute,
		},
		{
			name: "set values are taken as given",
			env: map[string]string{
				EnvDBMaxOpenConns:           "40",
				EnvDBMaxIdleConns:           "12",
				EnvDBConnMaxLifetimeMinutes: "5",
			},
			wantMaxOpen:     40,
			wantMaxIdle:     12,
			wantConnMaxLife: 5 * time.Minute,
		},
		{
			// 0 is database/sql for "unlimited", which is the state the whole
			// setting exists to leave.
			name:            "zero max-open falls back rather than meaning unlimited",
			env:             map[string]string{EnvDBMaxOpenConns: "0"},
			wantMaxOpen:     DefaultDBMaxOpenConns,
			wantMaxIdle:     DefaultDBMaxIdleConns,
			wantConnMaxLife: DefaultDBConnMaxLifetimeMinutes * time.Minute,
		},
		{
			name:            "negative max-open falls back",
			env:             map[string]string{EnvDBMaxOpenConns: "-5"},
			wantMaxOpen:     DefaultDBMaxOpenConns,
			wantMaxIdle:     DefaultDBMaxIdleConns,
			wantConnMaxLife: DefaultDBConnMaxLifetimeMinutes * time.Minute,
		},
		{
			// database/sql reduces this silently; doing it here keeps the logged
			// shape and the effective shape the same.
			name:            "idle above open is reduced to open",
			env:             map[string]string{EnvDBMaxOpenConns: "5", EnvDBMaxIdleConns: "50"},
			wantMaxOpen:     5,
			wantMaxIdle:     5,
			wantConnMaxLife: DefaultDBConnMaxLifetimeMinutes * time.Minute,
		},
		{
			name:            "negative idle clamps to zero",
			env:             map[string]string{EnvDBMaxIdleConns: "-1"},
			wantMaxOpen:     DefaultDBMaxOpenConns,
			wantMaxIdle:     0,
			wantConnMaxLife: DefaultDBConnMaxLifetimeMinutes * time.Minute,
		},
		{
			// Zero idle is a legitimate ask, unlike zero max-open.
			name:            "zero idle is honoured",
			env:             map[string]string{EnvDBMaxIdleConns: "0"},
			wantMaxOpen:     DefaultDBMaxOpenConns,
			wantMaxIdle:     0,
			wantConnMaxLife: DefaultDBConnMaxLifetimeMinutes * time.Minute,
		},
		{
			// SetConnMaxLifetime(<=0) is "never retire", the failure the setting
			// exists to prevent, so a non-positive value falls back.
			name:            "zero lifetime falls back rather than meaning never retire",
			env:             map[string]string{EnvDBConnMaxLifetimeMinutes: "0"},
			wantMaxOpen:     DefaultDBMaxOpenConns,
			wantMaxIdle:     DefaultDBMaxIdleConns,
			wantConnMaxLife: DefaultDBConnMaxLifetimeMinutes * time.Minute,
		},
		{
			name:            "negative lifetime falls back",
			env:             map[string]string{EnvDBConnMaxLifetimeMinutes: "-30"},
			wantMaxOpen:     DefaultDBMaxOpenConns,
			wantMaxIdle:     DefaultDBMaxIdleConns,
			wantConnMaxLife: DefaultDBConnMaxLifetimeMinutes * time.Minute,
		},
		{
			// Minutes are multiplied into a Duration; past this the product wraps
			// negative and lands on "never retire" from the other side.
			name:            "a lifetime that would overflow the duration falls back",
			env:             map[string]string{EnvDBConnMaxLifetimeMinutes: strconv.Itoa(maxLifetimeMinutes + 1)},
			wantMaxOpen:     DefaultDBMaxOpenConns,
			wantMaxIdle:     DefaultDBMaxIdleConns,
			wantConnMaxLife: DefaultDBConnMaxLifetimeMinutes * time.Minute,
		},
		{
			name:            "an unparseable value falls back",
			env:             map[string]string{EnvDBMaxOpenConns: "twenty"},
			wantMaxOpen:     DefaultDBMaxOpenConns,
			wantMaxIdle:     DefaultDBMaxIdleConns,
			wantConnMaxLife: DefaultDBConnMaxLifetimeMinutes * time.Minute,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			for k, v := range tc.env {
				t.Setenv(k, v)
			}

			got := databaseConfigFromEnv()

			if got.MaxOpenConns != tc.wantMaxOpen {
				t.Errorf("MaxOpenConns = %d, want %d", got.MaxOpenConns, tc.wantMaxOpen)
			}
			if got.MaxIdleConns != tc.wantMaxIdle {
				t.Errorf("MaxIdleConns = %d, want %d", got.MaxIdleConns, tc.wantMaxIdle)
			}
			if got.ConnMaxLifetime != tc.wantConnMaxLife {
				t.Errorf("ConnMaxLifetime = %s, want %s", got.ConnMaxLifetime, tc.wantConnMaxLife)
			}
			if got.ConnMaxLifetime <= 0 {
				t.Errorf("ConnMaxLifetime = %s, which database/sql reads as never retiring a connection",
					got.ConnMaxLifetime)
			}
			if got.MaxOpenConns < 1 {
				t.Errorf("MaxOpenConns = %d, which database/sql reads as an unlimited pool", got.MaxOpenConns)
			}
			if got.MaxIdleConns > got.MaxOpenConns {
				t.Errorf("MaxIdleConns %d exceeds MaxOpenConns %d", got.MaxIdleConns, got.MaxOpenConns)
			}
		})
	}
}
