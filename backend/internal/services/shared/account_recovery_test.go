package shared

import (
	"testing"
	"time"

	apperrors "psychic-homily-backend/internal/errors"
	authm "psychic-homily-backend/internal/models/auth"
)

// closedAccount is an account in one sign-in-relevant state, built fresh for
// each case so no case can mutate another's row.
func closedAccount(state string) *authm.User {
	address := "closed@example.com"
	empty := ""
	insideWindow := time.Now().Add(-time.Hour)
	pastWindow := time.Now().Add(-AccountRecoveryGracePeriod - time.Hour)
	switch state {
	case "active":
		return &authm.User{Email: &address, IsActive: true, EmailVerified: true}
	case "deactivated":
		return &authm.User{Email: &address, IsActive: false, EmailVerified: true}
	case "soft-deleted":
		return &authm.User{Email: &address, IsActive: false, DeletedAt: &insideWindow, EmailVerified: true}
	case "soft-deleted-unverified":
		return &authm.User{Email: &address, IsActive: false, DeletedAt: &insideWindow}
	case "soft-deleted-past-window":
		return &authm.User{Email: &address, IsActive: false, DeletedAt: &pastWindow, EmailVerified: true}
	case "soft-deleted-no-email":
		return &authm.User{IsActive: false, DeletedAt: &insideWindow, EmailVerified: true}
	case "soft-deleted-empty-email":
		return &authm.User{Email: &empty, IsActive: false, DeletedAt: &insideWindow, EmailVerified: true}
	}
	panic("unknown state " + state)
}

func TestIsAccountRecoverable(t *testing.T) {
	if IsAccountRecoverable(nil) {
		t.Error("nil account: want false")
	}
	for state, want := range map[string]bool{
		"active":                   false,
		"deactivated":              false,
		"soft-deleted":             true,
		"soft-deleted-unverified":  true,
		"soft-deleted-past-window": false,
		"soft-deleted-no-email":    true,
	} {
		if got := IsAccountRecoverable(closedAccount(state)); got != want {
			t.Errorf("%s: IsAccountRecoverable = %v, want %v", state, got, want)
		}
	}
}

// A caller whose credential or identity resolved to the account learns it is
// recoverable whenever account recovery can restore it, which needs an address
// to mail; the account's own verification does not matter here.
func TestClosedAccountRefusal(t *testing.T) {
	for state, want := range map[string]string{
		"deactivated":              apperrors.CodeAccountInactive,
		"soft-deleted":             apperrors.CodeAccountRecoverable,
		"soft-deleted-unverified":  apperrors.CodeAccountRecoverable,
		"soft-deleted-past-window": apperrors.CodeAccountInactive,
		"soft-deleted-no-email":    apperrors.CodeAccountInactive,
		"soft-deleted-empty-email": apperrors.CodeAccountInactive,
	} {
		if got := ClosedAccountRefusal(closedAccount(state)).Code; got != want {
			t.Errorf("%s: ClosedAccountRefusal code = %s, want %s", state, got, want)
		}
	}
}

// A caller matched only by address learns the account is recoverable only when
// the provider vouches for the address and the account's address is verified.
func TestAddressMatchIsRecoverable(t *testing.T) {
	if AddressMatchIsRecoverable(nil, true) {
		t.Error("nil account: want false")
	}
	for state, want := range map[string]bool{
		"active":                   false,
		"deactivated":              false,
		"soft-deleted":             true,
		"soft-deleted-unverified":  false,
		"soft-deleted-past-window": false,
	} {
		if got := AddressMatchIsRecoverable(closedAccount(state), true); got != want {
			t.Errorf("%s, provider vouches: AddressMatchIsRecoverable = %v, want %v", state, got, want)
		}
		if AddressMatchIsRecoverable(closedAccount(state), false) {
			t.Errorf("%s, provider does not vouch: AddressMatchIsRecoverable = true, want false", state)
		}
	}
}
