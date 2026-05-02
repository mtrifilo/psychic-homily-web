package shared

import "testing"

func TestDeref_NilReturnsZero(t *testing.T) {
	var sp *string
	if got := Deref(sp); got != "" {
		t.Errorf("Deref(nil *string) = %q, want \"\"", got)
	}

	var ip *int
	if got := Deref(ip); got != 0 {
		t.Errorf("Deref(nil *int) = %d, want 0", got)
	}

	var bp *bool
	if got := Deref(bp); got != false {
		t.Errorf("Deref(nil *bool) = %v, want false", got)
	}

	var fp *float64
	if got := Deref(fp); got != 0 {
		t.Errorf("Deref(nil *float64) = %v, want 0", got)
	}
}

func TestDeref_NonNilReturnsValue(t *testing.T) {
	s := "hello"
	if got := Deref(&s); got != "hello" {
		t.Errorf("Deref(&\"hello\") = %q, want \"hello\"", got)
	}

	i := 42
	if got := Deref(&i); got != 42 {
		t.Errorf("Deref(&42) = %d, want 42", got)
	}

	b := true
	if got := Deref(&b); got != true {
		t.Errorf("Deref(&true) = %v, want true", got)
	}
}

func TestDerefOr_NilReturnsFallback(t *testing.T) {
	var sp *string
	if got := DerefOr(sp, "default"); got != "default" {
		t.Errorf("DerefOr(nil, \"default\") = %q, want \"default\"", got)
	}

	var bp *bool
	if got := DerefOr(bp, true); got != true {
		t.Errorf("DerefOr(nil, true) = %v, want true", got)
	}

	var ip *int
	if got := DerefOr(ip, 100); got != 100 {
		t.Errorf("DerefOr(nil, 100) = %d, want 100", got)
	}
}

func TestDerefOr_NonNilReturnsValue(t *testing.T) {
	// The pointed-to value wins even when it equals the zero value.
	empty := ""
	if got := DerefOr(&empty, "default"); got != "" {
		t.Errorf("DerefOr(&\"\", \"default\") = %q, want \"\" (pointer present)", got)
	}

	zero := 0
	if got := DerefOr(&zero, 100); got != 0 {
		t.Errorf("DerefOr(&0, 100) = %d, want 0 (pointer present)", got)
	}

	off := false
	if got := DerefOr(&off, true); got != false {
		t.Errorf("DerefOr(&false, true) = %v, want false (pointer present)", got)
	}
}
