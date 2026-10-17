package errors

import "testing"

// ACCOUNT_RECOVERABLE reaches clients under its own name and in its own words,
// on every surface that renders a code: the handler's own message and the
// external-message table agree.
func TestAccountRecoverable_ReachesClientsAsItself(t *testing.T) {
	refusal := ErrAccountRecoverable()
	if refusal.Code != CodeAccountRecoverable {
		t.Fatalf("code = %q, want %q", refusal.Code, CodeAccountRecoverable)
	}
	if got := ToExternalCode(CodeAccountRecoverable); got != CodeAccountRecoverable {
		t.Errorf("ToExternalCode = %q, want the code unchanged", got)
	}
	if got := ToExternalMessage(CodeAccountRecoverable); got != refusal.UserMessage() {
		t.Errorf("ToExternalMessage = %q, want %q", got, refusal.UserMessage())
	}
	if refusal.UserMessage() == ErrAccountInactive().UserMessage() {
		t.Error("the recoverable refusal must not read as the contact-support one")
	}
}
