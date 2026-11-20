package auth

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"psychic-homily-backend/internal/api/handlers/shared/testhelpers"
	authm "psychic-homily-backend/internal/models/auth"
	"psychic-homily-backend/internal/services/contracts"
)

// refusedMagicLinkDestinations are destinations the sign-in destination rule
// refuses, each named for the attack or mistake it stands for. A magic link
// asked for with any of them carries none.
var refusedMagicLinkDestinations = []struct {
	name  string
	input string
}{
	{"absolute URL off the site", "https://evil.example/library"},
	{"absolute URL on the site's own host", "https://www.psychichomily.com/library"},
	{"protocol-relative", "//evil.example"},
	{"protocol-relative with a path", "//evil.example/library"},
	{"scheme-carrying javascript", "javascript:alert(1)"},
	{"scheme-carrying data", "data:text/html,hi"},
	{"backslash after the slash", `/\evil.example`},
	{"backslash later in the path", `/library\..\evil.example`},
	{"encoded dot-dot segment into protocol-relative", "/%2e%2e//evil.example"},
	{"mixed encoded dot-dot segment", "/a/.%2E/library"},
	{"encoded spelling of the sign-in page", "/%61uth"},
	{"the sign-in page", "/auth"},
	{"under the sign-in page", "/auth/magic-link?token=abc"},
	{"tab inside", "/\t/evil.example"},
	{"newline inside", "/library\r\nSet-Cookie: x=1"},
	{"non-ASCII character", "/artists/björk"},
	{"undecodable escape in the path", "/library/%zz"},
	{"relative path", "library"},
	{"one byte over the length limit", "/" + strings.Repeat("a", oauthReturnToMaxLength)},
	{"the site root", "/"},
}

// sendMagicLinkFor runs the send handler for the account the lookup finds
// (nil for none) with returnTo as the request's destination, and returns
// what it queued and the response.
func sendMagicLinkFor(t *testing.T, user *authm.User, returnTo string) ([]queuedAuthEmail, SendMagicLinkResponse) {
	t.Helper()
	var queued []queuedAuthEmail
	h := authHandler(func(ah *AuthHandler) {
		ah.emailService = forbidSends(t)
		ah.emailOutbox = recordingOutbox(&queued, nil)
		ah.userService = &testhelpers.MockUserService{
			GetUserByEmailFn: func(string) (*authm.User, error) { return user, nil },
		}
	})
	input := &SendMagicLinkRequest{}
	input.Body.Email = "verified@example.com"
	input.Body.ReturnTo = returnTo
	resp, err := h.SendMagicLinkHandler(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !resp.Body.Success {
		t.Fatalf("expected success=true, got message=%q", resp.Body.Message)
	}
	return queued, *resp
}

// sendMagicLinkTo is sendMagicLinkFor a verified account, returning what it
// queued.
func sendMagicLinkTo(t *testing.T, returnTo string) []queuedAuthEmail {
	t.Helper()
	email := "verified@example.com"
	queued, _ := sendMagicLinkFor(t, &authm.User{ID: 7, Email: &email, EmailVerified: true}, returnTo)
	return queued
}

func TestSendMagicLinkHandler_QueuesTheRequestsDestination(t *testing.T) {
	const destination = "/library?tab=shows#saved"
	got := sendMagicLinkTo(t, destination)
	want := []queuedAuthEmail{{kind: authm.AuthEmailKindMagicLink, userID: 7, returnTo: destination}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("queued = %v, want %v", got, want)
	}
}

func TestSendMagicLinkHandler_DropsARefusedDestination(t *testing.T) {
	for _, tc := range refusedMagicLinkDestinations {
		t.Run(tc.name, func(t *testing.T) {
			got := sendMagicLinkTo(t, tc.input)
			want := []queuedAuthEmail{{kind: authm.AuthEmailKindMagicLink, userID: 7}}
			if !reflect.DeepEqual(got, want) {
				t.Errorf("queued = %v, want %v", got, want)
			}
		})
	}
}

// The send keeps exactly what Google sign-in keeps: every case of the shared
// destination corpus, with the corpus's fallback queued as no destination.
func TestSendMagicLinkHandler_DestinationFollowsTheSharedCorpus(t *testing.T) {
	corpus := loadReturnToCorpus(t)
	for _, tc := range corpus.Cases {
		t.Run(tc.Name, func(t *testing.T) {
			want := tc.Want
			if want == corpus.Fallback {
				want = ""
			}
			got := sendMagicLinkTo(t, tc.Input)
			if len(got) != 1 || got[0].returnTo != want {
				t.Errorf("queued %v for %q, want destination %q (rule: %s)", got, tc.Input, want, tc.Rule)
			}
		})
	}
}

// A request that names a destination gets the body one that names none gets,
// so the destination changes nothing an asker can read.
func TestSendMagicLinkHandler_DestinationLeavesTheResponseAlone(t *testing.T) {
	email := "verified@example.com"
	verified := &authm.User{ID: 7, Email: &email, EmailVerified: true}
	_, want := sendMagicLinkFor(t, nil, "")
	for name, tc := range map[string]struct {
		user     *authm.User
		returnTo string
	}{
		"known, with a destination":    {verified, "/library"},
		"known, refused destination":   {verified, "//evil.example"},
		"unknown, with a destination":  {nil, "/library"},
		"known, without a destination": {verified, ""},
	} {
		if _, got := sendMagicLinkFor(t, tc.user, tc.returnTo); got.Body != want.Body {
			t.Errorf("%s: body differs from the unknown-email body:\n  got  %+v\n  want %+v", name, got.Body, want.Body)
		}
	}
}

// verifyMagicLinkWithClaims runs the verify handler on a token whose claims
// carry returnTo and returns the response.
func verifyMagicLinkWithClaims(t *testing.T, returnTo string) *VerifyMagicLinkResponse {
	t.Helper()
	email := "test@example.com"
	h := authHandler(func(ah *AuthHandler) {
		ah.jwtService = &testhelpers.MockJWTService{
			ValidateMagicLinkTokenFn: func(string) (*contracts.MagicLinkTokenClaims, error) {
				return &contracts.MagicLinkTokenClaims{UserID: 1, Email: email, ReturnTo: returnTo}, nil
			},
			CreateTokenFn: func(*authm.User) (string, error) { return "session-token", nil },
		}
		ah.userService = &testhelpers.MockUserService{
			GetUserByIDFn: func(uint) (*authm.User, error) {
				return &authm.User{ID: 1, Email: &email, IsActive: true}, nil
			},
		}
	})
	input := &VerifyMagicLinkRequest{}
	input.Body.Token = "valid-magic-token"
	resp, err := h.VerifyMagicLinkHandler(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !resp.Body.Success {
		t.Fatalf("expected success=true, got message=%q", resp.Body.Message)
	}
	return resp
}

func TestVerifyMagicLinkHandler_ReturnsTheTokensDestination(t *testing.T) {
	for name, destination := range map[string]string{
		"a destination":  "/library?tab=shows#saved",
		"no destination": "",
	} {
		if got := verifyMagicLinkWithClaims(t, destination).Body.ReturnTo; got != destination {
			t.Errorf("%s: return_to = %q, want %q", name, got, destination)
		}
	}
}

// The verify checks the token's destination again, so a token minted with a
// destination the rule refuses still returns none.
func TestVerifyMagicLinkHandler_ChecksTheTokensDestinationAgain(t *testing.T) {
	for _, tc := range refusedMagicLinkDestinations {
		t.Run(tc.name, func(t *testing.T) {
			if got := verifyMagicLinkWithClaims(t, tc.input).Body.ReturnTo; got != "" {
				t.Errorf("return_to = %q for a token carrying %q, want none", got, tc.input)
			}
		})
	}
}
