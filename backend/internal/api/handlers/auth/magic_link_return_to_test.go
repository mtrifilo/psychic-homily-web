package auth

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"psychic-homily-backend/internal/api/handlers/shared/testhelpers"
	authm "psychic-homily-backend/internal/models/auth"
	"psychic-homily-backend/internal/services/contracts"
)

// destinationsThatCarryNone are values a magic link asked for with carries no
// destination for: every one but the last is refused by
// sanitizeOAuthReturnTo, each named for the attack or mistake it stands for,
// and the last is its fallback, the site root, which magicLinkReturnTo maps
// to none.
var destinationsThatCarryNone = []struct {
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
	for _, tc := range destinationsThatCarryNone {
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

// testDestinationRef is the outbox row the verify tests' tokens name.
const testDestinationRef = 9

// verifyMagicLinkWith runs the verify handler on a token for account 1 whose
// claims name ref (0 for none), with destination answering the outbox read,
// and returns the response and the reads the handler made.
func verifyMagicLinkWith(t *testing.T, ref uint, destination func() (string, error)) (*VerifyMagicLinkResponse, [][2]uint) {
	t.Helper()
	email := "test@example.com"
	var reads [][2]uint
	h := authHandler(func(ah *AuthHandler) {
		ah.jwtService = &testhelpers.MockJWTService{
			ValidateMagicLinkTokenFn: func(string) (*contracts.MagicLinkTokenClaims, error) {
				return &contracts.MagicLinkTokenClaims{UserID: 1, Email: email, DestinationRef: ref}, nil
			},
			CreateTokenFn: func(*authm.User) (string, error) { return "session-token", nil },
		}
		ah.userService = &testhelpers.MockUserService{
			GetUserByIDFn: func(uint) (*authm.User, error) {
				return &authm.User{ID: 1, Email: &email, IsActive: true}, nil
			},
		}
		ah.emailOutbox = &testhelpers.MockAuthEmailOutbox{
			MagicLinkDestinationFn: func(_ context.Context, ref, userID uint) (string, error) {
				reads = append(reads, [2]uint{ref, userID})
				return destination()
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
	return resp, reads
}

// destinationOf answers the outbox read with destination.
func destinationOf(destination string) func() (string, error) {
	return func() (string, error) { return destination, nil }
}

func TestVerifyMagicLinkHandler_ReturnsTheDestinationItsRowHolds(t *testing.T) {
	const destination = "/library?tab=shows#saved"
	resp, reads := verifyMagicLinkWith(t, testDestinationRef, destinationOf(destination))
	if got := resp.Body.ReturnTo; got != destination {
		t.Errorf("return_to = %q, want %q", got, destination)
	}
	if want := [][2]uint{{testDestinationRef, 1}}; !reflect.DeepEqual(reads, want) {
		t.Errorf("outbox reads = %v, want %v (the token's ref, for the token's account)", reads, want)
	}
}

// A token that names no row reads nothing and returns no destination.
func TestVerifyMagicLinkHandler_NoRefReadsNothing(t *testing.T) {
	resp, reads := verifyMagicLinkWith(t, 0, destinationOf("/library"))
	if got := resp.Body.ReturnTo; got != "" {
		t.Errorf("return_to = %q, want none", got)
	}
	if len(reads) != 0 {
		t.Errorf("outbox reads = %v, want none", reads)
	}
}

// A failed read costs the destination, not the sign-in.
func TestVerifyMagicLinkHandler_AFailedReadSignsInWithNoDestination(t *testing.T) {
	resp, _ := verifyMagicLinkWith(t, testDestinationRef, func() (string, error) {
		return "", errors.New("connection reset")
	})
	if got := resp.Body.ReturnTo; got != "" {
		t.Errorf("return_to = %q, want none", got)
	}
	if resp.Body.Token != "session-token" {
		t.Errorf("token = %q, want the session token", resp.Body.Token)
	}
}

// The verify checks the stored destination again, so a row holding one the
// rule refuses still returns none.
func TestVerifyMagicLinkHandler_ChecksTheStoredDestinationAgain(t *testing.T) {
	for _, tc := range destinationsThatCarryNone {
		t.Run(tc.name, func(t *testing.T) {
			resp, _ := verifyMagicLinkWith(t, testDestinationRef, destinationOf(tc.input))
			if got := resp.Body.ReturnTo; got != "" {
				t.Errorf("return_to = %q for a row holding %q, want none", got, tc.input)
			}
		})
	}
}
