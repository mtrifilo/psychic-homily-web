package utils

import "testing"

// NilIfBlank is the trimming counterpart to NilIfEmpty. The distinction matters:
// NilIfEmpty deliberately preserves whitespace ("caller decides trimming
// policy"), so a caller that wants blank-means-NULL must reach for this one.
func TestNilIfBlank(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantNil   bool
		wantValue string
	}{
		{name: "empty string returns nil", input: "", wantNil: true},
		{name: "single space collapses to nil", input: " ", wantNil: true},
		{name: "tabs and spaces collapse to nil", input: "   \t  ", wantNil: true},
		{name: "padding is stripped from a real value", input: "  21+  ", wantNil: false, wantValue: "21+"},
		{name: "unpadded value is unchanged", input: "all ages", wantNil: false, wantValue: "all ages"},
		{name: "interior whitespace is preserved", input: " 18+ w/ guardian ", wantNil: false, wantValue: "18+ w/ guardian"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NilIfBlank(tt.input)
			if tt.wantNil {
				if got != nil {
					t.Errorf("NilIfBlank(%q) = %q, want nil", tt.input, *got)
				}
				return
			}
			if got == nil {
				t.Fatalf("NilIfBlank(%q) = nil, want %q", tt.input, tt.wantValue)
			}
			if *got != tt.wantValue {
				t.Errorf("NilIfBlank(%q) = %q, want %q", tt.input, *got, tt.wantValue)
			}
		})
	}
}

func TestNilIfBlankPtr(t *testing.T) {
	// A nil input means "not supplied" and must stay nil rather than panic.
	if got := NilIfBlankPtr(nil); got != nil {
		t.Errorf("NilIfBlankPtr(nil) = %q, want nil", *got)
	}

	blank := "   "
	if got := NilIfBlankPtr(&blank); got != nil {
		t.Errorf("NilIfBlankPtr(pointer to blank) = %q, want nil", *got)
	}

	padded := "  21+  "
	got := NilIfBlankPtr(&padded)
	if got == nil {
		t.Fatal("NilIfBlankPtr(pointer to padded value) = nil, want trimmed value")
	}
	if *got != "21+" {
		t.Errorf("NilIfBlankPtr(%q) = %q, want %q", padded, *got, "21+")
	}
	// The input must not be mutated in place.
	if padded != "  21+  " {
		t.Errorf("input was mutated: %q", padded)
	}
}

func TestNilIfEmpty(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantNil   bool
		wantValue string // only checked when wantNil is false
	}{
		{
			name:    "empty string returns nil",
			input:   "",
			wantNil: true,
		},
		{
			name:      "single space is preserved (not normalized)",
			input:     " ",
			wantNil:   false,
			wantValue: " ",
		},
		{
			name:      "whitespace-only string is preserved (caller decides trimming policy)",
			input:     "   \t  ",
			wantNil:   false,
			wantValue: "   \t  ",
		},
		{
			name:      "non-empty string returns pointer to value",
			input:     "hello",
			wantNil:   false,
			wantValue: "hello",
		},
		{
			name:      "URL string returns pointer to value",
			input:     "https://example.com/image.png",
			wantNil:   false,
			wantValue: "https://example.com/image.png",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NilIfEmpty(tt.input)
			if tt.wantNil {
				if got != nil {
					t.Errorf("NilIfEmpty(%q) = %v, want nil", tt.input, got)
				}
				return
			}
			if got == nil {
				t.Fatalf("NilIfEmpty(%q) = nil, want non-nil pointer", tt.input)
			}
			if *got != tt.wantValue {
				t.Errorf("NilIfEmpty(%q) = %q, want %q", tt.input, *got, tt.wantValue)
			}
		})
	}
}
