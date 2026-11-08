package auth

import (
	"net/http"
	"reflect"
	"slices"
	"testing"

	"psychic-homily-backend/internal/api/handlers/shared/testhelpers"
	"psychic-homily-backend/internal/config"
	authm "psychic-homily-backend/internal/models/auth"
)

// parsedSetCookies parses each line of a setCookieLines field, in order.
func parsedSetCookies(t testing.TB, lines setCookieLines) []*http.Cookie {
	t.Helper()
	out := make([]*http.Cookie, 0, len(lines))
	for _, line := range lines {
		cookie, err := http.ParseSetCookie(line)
		if err != nil {
			t.Fatalf("Set-Cookie %q does not parse: %v", line, err)
		}
		out = append(out, cookie)
	}
	return out
}

// assertOnlySetCookies fails unless header answers exactly the named cookies,
// each once and in any order, as Set-Cookie lines, and none of http.Cookie's
// fields as a header of its own. No names means no Set-Cookie line at all: an
// empty line does not parse.
func assertOnlySetCookies(t testing.TB, header http.Header, wantNames ...string) {
	t.Helper()
	lines := header.Values("Set-Cookie")
	gotNames := make([]string, 0, len(lines))
	for _, cookie := range parsedSetCookies(t, lines) {
		gotNames = append(gotNames, cookie.Name)
	}
	slices.Sort(gotNames)
	wantNames = slices.Sorted(slices.Values(wantNames))
	if !slices.Equal(gotNames, wantNames) {
		t.Errorf("Set-Cookie names = %q, want %q (lines %q)", gotNames, wantNames, lines)
	}
	cookieType := reflect.TypeFor[http.Cookie]()
	for i := range cookieType.NumField() {
		field := cookieType.Field(i)
		if !field.IsExported() {
			continue
		}
		if values, present := header[http.CanonicalHeaderKey(field.Name)]; present {
			t.Errorf("response carries http.Cookie field %s as a header: %q", field.Name, values)
		}
	}
}

// The login response's Set-Cookie is exactly its cookies over the wire: the
// session alone, the cleared pending link beside it when the request carried
// one, the cleared link alone when the connecting sign-in is refused, and no
// header at all when nothing is set.
func TestLoginSetCookieHeadersOverTheWire(t *testing.T) {
	pendingLinkCookie := "Cookie: " + oauthPendingLinkCookie.setName(false) + "=pending-token"
	cases := []struct {
		name    string
		body    map[string]any
		headers []any
		want    []string
	}{
		{
			name: "refused before any cookie",
			body: map[string]any{"email": "", "password": ""},
		},
		{
			name: "plain sign-in",
			body: map[string]any{"email": "test@example.com", "password": "pw"},
			want: []string{config.AuthCookieName},
		},
		{
			name:    "sign-in carrying a pending link",
			body:    map[string]any{"email": "test@example.com", "password": "pw"},
			headers: []any{pendingLinkCookie},
			want:    []string{oauthPendingLinkCookie.setName(false), config.AuthCookieName},
		},
		{
			name:    "connecting sign-in refused",
			body:    map[string]any{"email": "test@example.com", "password": "pw", "connect_pending_link": true},
			headers: []any{pendingLinkCookie},
			want:    []string{oauthPendingLinkCookie.setName(false)},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := authHandler(func(ah *AuthHandler) {
				ah.userService = &testhelpers.MockUserService{
					AuthenticateUserWithPasswordFn: func(string, string) (*authm.User, error) {
						return &authm.User{ID: 1}, nil
					},
				}
				ah.authService = &testhelpers.MockAuthService{
					// The pending link is gone: an ordinary sign-in stands on
					// its own and a connecting one is refused.
					LinkOAuthWithPasswordFn: func(string, *authm.User) (bool, error) { return false, nil },
				}
				ah.jwtService = &testhelpers.MockJWTService{
					CreateTokenFn: func(*authm.User) (string, error) { return "session-token", nil },
				}
			})

			args := append([]any{tc.body}, tc.headers...)
			resp := newWireAPI(t, h).Post("/auth/login", args...)

			if resp.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200; body: %s", resp.Code, resp.Body.String())
			}
			assertOnlySetCookies(t, resp.Header(), tc.want...)
		})
	}
}
