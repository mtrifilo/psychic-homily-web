package auth

import (
	"errors"
	"fmt"
	"time"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	apperrors "psychic-homily-backend/internal/errors"
	authm "psychic-homily-backend/internal/models/auth"
	"psychic-homily-backend/internal/services/contracts"
	"psychic-homily-backend/internal/services/shared"
)

// passwordResetTooLongMessage refuses a password bcrypt cannot hash (over 72
// bytes), which the password validator's 128-character bound admits.
const passwordResetTooLongMessage = "Password is too long. Try a shorter one."

// PasswordResetAccount reads the account claims name and reports whether the
// link may still set its password (contracts.AuthServiceInterface).
func (s *AuthService) PasswordResetAccount(claims *contracts.PasswordResetTokenClaims) (*authm.User, error) {
	var account authm.User
	if err := s.db.Take(&account, claims.UserID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("user %d: %w", claims.UserID, contracts.ErrPasswordResetLinkSpent)
		}
		return nil, fmt.Errorf("failed to load user %d: %w", claims.UserID, err)
	}
	if err := judgePasswordReset(s.jwtService, &account, claims); err != nil {
		return nil, err
	}
	return &account, nil
}

// CompletePasswordReset sets newPassword on the account claims name, revokes
// every session it holds and issues one for this browser
// (contracts.AuthServiceInterface).
func (s *AuthService) CompletePasswordReset(claims *contracts.PasswordResetTokenClaims, newPassword string) (*contracts.PasswordResetCompletion, error) {
	return completePasswordReset(s.db, s.jwtService, claims, newPassword, time.Now(), s.jwtService.CreateToken)
}

// completePasswordReset is CompletePasswordReset with the database, the
// clock and the session mint supplied.
func completePasswordReset(db *gorm.DB, tokens *JWTService, claims *contracts.PasswordResetTokenClaims, newPassword string, now time.Time, mint func(*authm.User) (string, error)) (*contracts.PasswordResetCompletion, error) {
	// Hashed before the transaction opens: bcrypt is deliberately slow and
	// needs nothing from the row, so the row is not held while it runs.
	hashed, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if errors.Is(err, bcrypt.ErrPasswordTooLong) {
		return nil, apperrors.ErrValidationFailed(passwordResetTooLongMessage)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to hash password: %w", err)
	}
	newHash := string(hashed)

	var completion contracts.PasswordResetCompletion
	err = db.Transaction(func(tx *gorm.DB) error {
		// FOR UPDATE: this transaction writes the row, and of two resets
		// with the same link the second waits here, then reads the hash the
		// first wrote, which the link no longer matches.
		var account authm.User
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Take(&account, claims.UserID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return fmt.Errorf("user %d: %w", claims.UserID, contracts.ErrPasswordResetLinkSpent)
			}
			return fmt.Errorf("failed to load user %d: %w", claims.UserID, err)
		}
		if err := judgePasswordReset(tokens, &account, claims); err != nil {
			return err
		}

		// The row lock decides the race; the condition keeps the write to
		// the hash the link was judged against even if a caller ever
		// reaches here without the lock.
		result := tx.Model(&authm.User{}).
			Where("id = ? AND password_hash IS NOT DISTINCT FROM ?", account.ID, account.PasswordHash).
			Update("password_hash", newHash)
		if result.Error != nil {
			return fmt.Errorf("failed to reset password: %w", result.Error)
		}
		if result.RowsAffected == 0 {
			return fmt.Errorf("user %d: %w", account.ID, contracts.ErrPasswordResetLinkSpent)
		}
		account.PasswordHash = &newHash

		if err := shared.RevokeAllSessions(tx, &account, now); err != nil {
			return err
		}
		// RevokeAllSessions set account.SessionsValidAfter to the stamp it
		// wrote, so this session names the stamp the transaction commits and
		// the revocation does not reach it. Minted inside the transaction, so
		// a mint failure rolls the reset back and the link stays usable.
		token, err := mint(&account)
		if err != nil {
			return fmt.Errorf("failed to mint the session: %w", err)
		}
		completion = contracts.PasswordResetCompletion{User: &account, Token: token}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &completion, nil
}

// judgePasswordReset refuses a reset of account by a link carrying
// claims: shared.ClosedAccountRefusal when the account may not sign in, and
// contracts.ErrPasswordResetLinkSpent when its address is no longer the one
// the link was sent to or its password hash is no longer the one the link
// was minted against.
func judgePasswordReset(tokens *JWTService, account *authm.User, claims *contracts.PasswordResetTokenClaims) error {
	if !account.CanSignIn() {
		return shared.ClosedAccountRefusal(account)
	}
	if account.Email == nil || !authm.SameEmailIdentity(*account.Email, claims.Email) {
		return fmt.Errorf("user %d: address changed: %w", account.ID, contracts.ErrPasswordResetLinkSpent)
	}
	if !tokens.passwordStateMatches(claims, account.PasswordHash) {
		return fmt.Errorf("user %d: password changed: %w", account.ID, contracts.ErrPasswordResetLinkSpent)
	}
	return nil
}
