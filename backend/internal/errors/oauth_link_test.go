package errors

import (
	"strings"
	"testing"
)

// The OAuth link refusals are the only place a user is told how to get past
// one, so their copy is load-bearing rather than decorative.

// A refused sign-in has one way in, and the copy has to name it or the refusal
// is a dead end.
func TestErrOAuthLinkRefused_NamesTheRemediation(t *testing.T) {
	err := ErrOAuthLinkRefused("someone@example.com")

	if err.Code != CodeOAuthLinkRefused {
		t.Errorf("Code = %q, want %q", err.Code, CodeOAuthLinkRefused)
	}
	for _, want := range []string{"Sign in", "Settings", "Connected accounts"} {
		if !strings.Contains(err.UserMessage(), want) {
			t.Errorf("message %q does not name %q", err.UserMessage(), want)
		}
	}
}

// Naming WHICH method the account uses would tell an unauthenticated caller
// more about a stranger's account than the refusal already does. The copy
// points at the account's own method without enumerating the options.
func TestErrOAuthLinkRefused_DoesNotEnumerateSignInMethods(t *testing.T) {
	message := strings.ToLower(ErrOAuthLinkRefused("someone@example.com").UserMessage())
	for _, leak := range []string{"password", "passkey", "magic link", "google", "apple", "github"} {
		if strings.Contains(message, leak) {
			t.Errorf("message names %q, which is a signal about an account the caller has not authenticated as", leak)
		}
	}
}

// Apple's refusal is the one place its exact words are pinned: the constant is
// unexported, and every other suite asserts against this constructor rather
// than against a copy of the string.
func TestErrAppleSignInRefused_Copy(t *testing.T) {
	err := ErrAppleSignInRefused("someone@example.com")

	if err.Code != CodeOAuthLinkRefused {
		t.Errorf("Code = %q, want %q", err.Code, CodeOAuthLinkRefused)
	}
	const want = "An account already uses this email address. Sign in to that account with the method it already has."
	if err.UserMessage() != want {
		t.Errorf("message = %q, want %q", err.UserMessage(), want)
	}
}

// Apple arrives by a native-token POST rather than the goth handshake the link
// route drives, so Settings has no Apple control. Naming one would send a
// refused user looking for something that does not exist, which is why these
// two refusals share a code and not their copy.
func TestErrAppleSignInRefused_NamesNoSettingsControl(t *testing.T) {
	apple := ErrAppleSignInRefused("someone@example.com").UserMessage()
	if strings.Contains(apple, "Settings") {
		t.Errorf("message %q points at a Settings control the Apple path has none of", apple)
	}
	if apple == ErrOAuthLinkRefused("someone@example.com").UserMessage() {
		t.Error("the Apple and goth refusals must not share copy: they share a code and differ only here")
	}
}

// Callers log the whole chain, so the address may not ride along in cleartext.
func TestErrOAuthLinkRefused_MasksTheAddress(t *testing.T) {
	err := ErrOAuthLinkRefused("Very.Distinct@example.com")
	if strings.Contains(err.Error(), "Very.Distinct@example.com") {
		t.Errorf("error chain %q carries the cleartext address", err.Error())
	}
}

// A signup collision and a refused link are different situations with
// different remedies. Sharing one code would mean retuning either one retunes
// the other.
func TestOAuthLinkRefusal_IsDistinctFromSignupCollision(t *testing.T) {
	if CodeOAuthLinkRefused == CodeUserExists {
		t.Fatal("the link refusal must not reuse the signup collision code")
	}
	if ErrOAuthLinkRefused("a@b.com").UserMessage() == ErrUserExists("a@b.com").UserMessage() {
		t.Error("the two refusals must not share copy")
	}
}

// ToExternalMessage is a second table over the same codes, and a code missing
// from it renders as "An error occurred" on whichever surface reads it.
func TestToExternalMessage_CoversTheOAuthLinkCodes(t *testing.T) {
	cases := map[string]string{
		CodeOAuthLinkRefused:           ErrOAuthLinkRefused("").UserMessage(),
		CodeOAuthIdentityInUse:         ErrOAuthIdentityInUse("google").UserMessage(),
		CodeOAuthProviderAlreadyLinked: ErrOAuthProviderAlreadyLinked("google").UserMessage(),
		CodeOAuthLinkExpired:           ErrOAuthLinkExpired().UserMessage(),
	}
	for code, want := range cases {
		if got := ToExternalMessage(code); got != want {
			t.Errorf("ToExternalMessage(%q) = %q, want the constructor's message %q", code, got, want)
		}
	}
}

// The provider name is diagnostic, not user-facing: a raw provider key in the
// copy would read as a typo, and these two are shown in Settings.
func TestOAuthLinkRefusals_KeepTheProviderKeyInternal(t *testing.T) {
	for _, err := range []*AuthError{
		ErrOAuthIdentityInUse("google"),
		ErrOAuthProviderAlreadyLinked("google"),
	} {
		if strings.Contains(err.UserMessage(), "google") {
			t.Errorf("user message %q carries the raw provider key", err.UserMessage())
		}
		if !strings.Contains(err.Error(), "google") {
			t.Errorf("internal error %q dropped the provider, which is what makes the log line diagnosable", err.Error())
		}
	}
}
