package user

import (
	"testing"

	"github.com/markbates/goth"
)

func TestProviderIsAuthoritativeForEmail(t *testing.T) {
	verified := func(email string, extra map[string]any) goth.User {
		raw := map[string]any{"verified_email": true}
		for k, v := range extra {
			raw[k] = v
		}
		return goth.User{Email: email, RawData: raw}
	}
	cases := map[string]struct {
		provider string
		user     goth.User
		want     bool
	}{
		"gmail":                         {"google", verified("alice@gmail.com", nil), true},
		"googlemail, mixed case":        {"google", verified("Alice@GoogleMail.com", nil), true},
		"workspace with matching hd":    {"google", verified("alice@band.example", map[string]any{"hd": "band.example"}), true},
		"hd in another case":            {"google", verified("alice@Band.Example", map[string]any{"hd": "band.example"}), true},
		"consumer account on an ISP":    {"google", verified("alice@isp.example", nil), false},
		"hd naming another domain":      {"google", verified("alice@isp.example", map[string]any{"hd": "band.example"}), false},
		"gmail not verified":            {"google", goth.User{Email: "alice@gmail.com", RawData: map[string]any{"verified_email": false}}, false},
		"github is never the authority": {"github", verified("alice@gmail.com", nil), false},
		"no address":                    {"google", verified("", nil), false},
		"lookalike subdomain":           {"google", verified("alice@gmail.com.evil.example", nil), false},
	}
	for name, c := range cases {
		if got := providerIsAuthoritativeForEmail(c.provider, c.user); got != c.want {
			t.Errorf("%s: got %v, want %v", name, got, c.want)
		}
	}
}
