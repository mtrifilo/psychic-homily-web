package utils

import "testing"

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
