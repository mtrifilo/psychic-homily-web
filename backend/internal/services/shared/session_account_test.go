package shared

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"gorm.io/gorm"

	apperrors "psychic-homily-backend/internal/errors"
	authm "psychic-homily-backend/internal/models/auth"
)

// sessionAccount creates an account for a LockSessionAccount case and removes
// it when the test ends.
func sessionAccount(t *testing.T, db *gorm.DB, name string, sessionsValidAfter *time.Time) *authm.User {
	t.Helper()
	email := fmt.Sprintf("session-account-%s-%d@test.com", name, time.Now().UnixNano())
	user := &authm.User{Email: &email, IsActive: true, SessionsValidAfter: sessionsValidAfter}
	if err := db.Create(user).Error; err != nil {
		t.Fatalf("create account: %v", err)
	}
	t.Cleanup(func() { db.Unscoped().Delete(&authm.User{}, user.ID) })
	return user
}

func lockInTx(db *gorm.DB, userID uint, sessionsValidAfter *time.Time) (*authm.User, error) {
	var locked *authm.User
	err := db.Transaction(func(tx *gorm.DB) error {
		var err error
		locked, err = LockSessionAccount(tx, userID, sessionsValidAfter)
		return err
	})
	return locked, err
}

func requireRefusalCode(t *testing.T, err error, code string) *apperrors.AuthError {
	t.Helper()
	var refusal *apperrors.AuthError
	if !errors.As(err, &refusal) {
		t.Fatalf("expected an AuthError with %s, got %v", code, err)
	}
	if refusal.Code != code {
		t.Fatalf("code = %s, want %s (%v)", refusal.Code, code, err)
	}
	return refusal
}

func TestLockSessionAccount(t *testing.T) {
	db := packageTestDB(t)
	revokedAt := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	earlier := revokedAt.Add(-time.Hour)

	deactivate := func(t *testing.T, id uint) {
		if err := db.Model(&authm.User{}).Where("id = ?", id).Update("is_active", false).Error; err != nil {
			t.Fatalf("deactivate: %v", err)
		}
	}
	softDelete := func(t *testing.T, id uint) {
		if err := db.Model(&authm.User{}).Where("id = ?", id).
			Updates(map[string]any{"is_active": false, "deleted_at": time.Now()}).Error; err != nil {
			t.Fatalf("soft delete: %v", err)
		}
	}

	cases := []struct {
		name string
		// rowStamp is the account's sessions_valid_after; sessionStamp is the
		// acting session's.
		rowStamp, sessionStamp *time.Time
		close                  func(*testing.T, uint)
		// wantCode is the refusal, or "" when the account is returned.
		wantCode string
	}{
		{name: "never revoked, session from before any revocation", wantCode: ""},
		{name: "revoked, session minted after it", rowStamp: &revokedAt, sessionStamp: &revokedAt, wantCode: ""},
		{name: "revoked, session from before any revocation", rowStamp: &revokedAt, wantCode: apperrors.CodeTokenInvalid},
		{name: "revoked again, session from the earlier revocation", rowStamp: &revokedAt, sessionStamp: &earlier, wantCode: apperrors.CodeTokenInvalid},
		{name: "stamp cleared, session carries one", sessionStamp: &revokedAt, wantCode: apperrors.CodeTokenInvalid},
		{name: "deactivated", close: deactivate, wantCode: apperrors.CodeAccountInactive},
		{name: "soft-deleted", close: softDelete, wantCode: apperrors.CodeAccountInactive},
		// The account's state is judged first, the way session validation
		// judges it.
		{name: "deactivated and revoked", rowStamp: &revokedAt, close: deactivate, wantCode: apperrors.CodeAccountInactive},
	}
	for i, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			account := sessionAccount(t, db, fmt.Sprintf("case%d", i), tc.rowStamp)
			if tc.close != nil {
				tc.close(t, account.ID)
			}

			locked, err := lockInTx(db, account.ID, tc.sessionStamp)

			if tc.wantCode == "" {
				if err != nil {
					t.Fatalf("unexpected refusal: %v", err)
				}
				if locked == nil || locked.ID != account.ID {
					t.Fatalf("locked = %+v, want account %d", locked, account.ID)
				}
				return
			}
			if locked != nil {
				t.Errorf("a refusal returns no account, got %+v", locked)
			}
			refusal := requireRefusalCode(t, err, tc.wantCode)
			if tc.wantCode == apperrors.CodeTokenInvalid && !errors.Is(err, ErrSessionRevoked) {
				t.Errorf("a stamp refusal wraps ErrSessionRevoked: %v", err)
			}
			if tc.wantCode == apperrors.CodeAccountInactive && errors.Is(err, ErrSessionRevoked) {
				t.Errorf("an account refusal is not a revocation: %v", err)
			}
			if refusal.Internal == nil {
				t.Error("the refusal names the account in its internal error")
			}
		})
	}
}

func TestLockSessionAccount_MissingAccountIsAFailureNotARefusal(t *testing.T) {
	db := packageTestDB(t)

	_, err := lockInTx(db, 9_000_001, nil)

	if !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("expected the lookup failure, got %v", err)
	}
	var refusal *apperrors.AuthError
	if errors.As(err, &refusal) {
		t.Errorf("a missing account is not a refusal with its own copy: %v", err)
	}
}

// awaitLockWaiter returns once a statement on another connection is waiting
// on a lock, or fails the test when none does.
func awaitLockWaiter(t *testing.T, db *gorm.DB, done <-chan error) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if len(done) > 0 {
			t.Fatal("the concurrent statement finished instead of waiting on the lock")
		}
		var waiting int64
		if err := db.Raw(
			"SELECT count(*) FROM pg_stat_activity WHERE datname = current_database() AND wait_event_type = 'Lock'",
		).Scan(&waiting).Error; err == nil && waiting > 0 {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("the concurrent statement never waited on a lock")
}

func awaitDone(t *testing.T, done <-chan error) error {
	t.Helper()
	select {
	case err := <-done:
		return err
	case <-time.After(10 * time.Second):
		t.Fatal("the concurrent statement did not finish")
		return nil
	}
}

const revokeSQL = `UPDATE users SET sessions_valid_after = ? WHERE id = ?`

// A revocation that starts while the write's transaction holds the row waits
// for that transaction, so it can never land between the check and the write.
func TestLockSessionAccount_RevocationWaitsForTheHolder(t *testing.T) {
	db := packageTestDB(t)
	account := sessionAccount(t, db, "holder", nil)
	revokedAt := time.Now().UTC().Truncate(time.Second)

	done := make(chan error, 1)
	err := db.Transaction(func(tx *gorm.DB) error {
		if _, err := LockSessionAccount(tx, account.ID, nil); err != nil {
			return err
		}
		go func() { done <- db.Exec(revokeSQL, revokedAt, account.ID).Error }()
		awaitLockWaiter(t, db, done)
		return nil
	})
	if err != nil {
		t.Fatalf("the holder's transaction: %v", err)
	}
	if err := awaitDone(t, done); err != nil {
		t.Fatalf("the revocation: %v", err)
	}
}

// A write that reaches the row while a revocation holds it waits for the
// revocation, then reads its committed stamp and is refused.
func TestLockSessionAccount_WaitsForAnInFlightRevocationThenRefuses(t *testing.T) {
	db := packageTestDB(t)
	account := sessionAccount(t, db, "behind-revocation", nil)
	revokedAt := time.Now().UTC().Truncate(time.Second)

	revocation := db.Begin()
	if revocation.Error != nil {
		t.Fatalf("begin revocation: %v", revocation.Error)
	}
	if err := revocation.Exec(revokeSQL, revokedAt, account.ID).Error; err != nil {
		revocation.Rollback()
		t.Fatalf("revoke: %v", err)
	}

	done := make(chan error, 1)
	go func() {
		_, err := lockInTx(db, account.ID, nil)
		done <- err
	}()
	awaitLockWaiter(t, db, done)
	if err := revocation.Commit().Error; err != nil {
		t.Fatalf("commit revocation: %v", err)
	}

	err := awaitDone(t, done)
	requireRefusalCode(t, err, apperrors.CodeTokenInvalid)
	if !errors.Is(err, ErrSessionRevoked) {
		t.Errorf("expected ErrSessionRevoked, got %v", err)
	}
}
