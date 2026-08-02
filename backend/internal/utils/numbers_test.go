package utils

import (
	"math"
	"testing"
)

func TestWholeNumber_AcceptsIntAndIntegralFloat(t *testing.T) {
	cases := []struct {
		name  string
		value any
		want  int
	}{
		{"int", 550, 550},
		{"negative int", -3, -3},
		{"zero", 0, 0},
		// float64 is what encoding/json produces for every JSON number
		// decoded into an interface{}, which is how the pending-edit pipeline
		// hands values back.
		{"integral float64", float64(550), 550},
		{"negative integral float64", float64(-550), -550},
		{"float64 zero", float64(0), 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, ok := WholeNumber(c.value)
			if !ok {
				t.Fatalf("WholeNumber(%v) rejected a whole number", c.value)
			}
			if got != c.want {
				t.Errorf("WholeNumber(%v) = %d, want %d", c.value, got, c.want)
			}
		})
	}
}

func TestWholeNumber_RejectsNonWholeNumbers(t *testing.T) {
	// A fraction must not be accepted. Measured through the driver, an
	// unnarrowed 550.7 is stored as 550 with no error raised anywhere, so
	// without this gate the column silently gets a value nobody typed. See the
	// WholeNumber doc comment for the full measurement table.
	cases := []struct {
		name  string
		value any
	}{
		{"fraction", 550.7},
		{"tiny fraction", 1.0000001},
		{"negative fraction", -0.5},
		{"NaN", math.NaN()},
		{"positive infinity", math.Inf(1)},
		{"negative infinity", math.Inf(-1)},
		// Out of int range: converting these to int is implementation-defined
		// in Go, so the range check has to happen before the conversion.
		{"above int range", math.MaxFloat64},
		{"below int range", -math.MaxFloat64},
		{"exactly 2^63 (not a representable int)", float64(1) * (1 << 63)},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got, ok := WholeNumber(c.value); ok {
				t.Errorf("WholeNumber(%v) = (%d, true), want rejected", c.value, got)
			}
		})
	}
}

func TestWholeNumber_RejectsNonNumericTypes(t *testing.T) {
	// FieldChange.NewValue is `any` decoded from JSONB, so every one of these
	// is reachable and would otherwise land in an untyped GORM Updates() map.
	// A numeric STRING is rejected too: this helper narrows, it does not parse.
	cases := []any{nil, "550", "", true, map[string]any{"x": 1}, []any{1}, struct{}{}}
	for _, value := range cases {
		if got, ok := WholeNumber(value); ok {
			t.Errorf("WholeNumber(%#v) = (%d, true), want rejected", value, got)
		}
	}
}
