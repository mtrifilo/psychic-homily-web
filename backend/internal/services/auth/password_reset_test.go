package auth

import (
	"errors"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"

	"psychic-homily-backend/internal/config"
	apperrors "psychic-homily-backend/internal/errors"
	authm "psychic-homily-backend/internal/models/auth"
	"psychic-homily-backend/internal/services/contracts"
	usersvc "psychic-homily-backend/internal/services/user"
)

func passwordResetJWT() *JWTService {
	return NewJWTService(nil, &config.Config{JWT: config.JWTConfig{SecretKey: "password-reset-unit-test-secret-32ch!", Expiry: 24}}, nil)
}

func resetTestUser(hash *string) *authm.User {
	email := "reset@example.com"
	return &authm.User{ID: 7, Email: &email, PasswordHash: hash, IsActive: true}
}

func TestPasswordResetToken_RoundTrip(t *testing.T) {
	svc := passwordResetJWT()
	hash := "$2a$10$somehash"
	user := resetTestUser(&hash)

	token, err := svc.CreatePasswordResetToken(user)
	if err != nil {
		t.Fatal(err)
	}
	claims, err := svc.ValidatePasswordResetToken(token)
	if err != nil {
		t.Fatalf("a fresh reset link must validate: %v", err)
	}
	if claims.UserID != user.ID || claims.Email != *user.Email {
		t.Errorf("claims name user %d %q, want %d %q", claims.UserID, claims.Email, user.ID, *user.Email)
	}
	if lifetime := time.Until(claims.ExpiresAt.Time); lifetime > contracts.PasswordResetTokenTTL || lifetime < contracts.PasswordResetTokenTTL-time.Minute {
		t.Errorf("link lives %v, want contracts.PasswordResetTokenTTL (%v)", lifetime, contracts.PasswordResetTokenTTL)
	}
	if !svc.passwordStateMatches(claims, &hash) {
		t.Error("the link must match the hash it was minted against")
	}
	if claims.PasswordState == hash || claims.PasswordState == "" {
		t.Errorf("PasswordState %q must be a fingerprint, not the hash or empty", claims.PasswordState)
	}
}

// Any other password state, the same password rehashed included, spends the
// link; no password and an empty hash are one state.
func TestPasswordResetToken_StateFingerprint(t *testing.T) {
	svc := passwordResetJWT()
	first, err := bcrypt.GenerateFromPassword([]byte("the-same-password-1!"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	second, err := bcrypt.GenerateFromPassword([]byte("the-same-password-1!"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	firstHash, secondHash, empty := string(first), string(second), ""

	token, err := svc.CreatePasswordResetToken(resetTestUser(&firstHash))
	if err != nil {
		t.Fatal(err)
	}
	claims, err := svc.ValidatePasswordResetToken(token)
	if err != nil {
		t.Fatal(err)
	}
	if svc.passwordStateMatches(claims, &secondHash) {
		t.Error("a rehash of the same password must spend the link")
	}
	if svc.passwordStateMatches(claims, nil) {
		t.Error("clearing the password must spend the link")
	}

	noPassword, err := svc.CreatePasswordResetToken(resetTestUser(nil))
	if err != nil {
		t.Fatal(err)
	}
	noPasswordClaims, err := svc.ValidatePasswordResetToken(noPassword)
	if err != nil {
		t.Fatal(err)
	}
	if !svc.passwordStateMatches(noPasswordClaims, &empty) {
		t.Error("an empty hash is no password, the state the link was minted against")
	}
	if svc.passwordStateMatches(noPasswordClaims, &firstHash) {
		t.Error("setting a password must spend a link minted while there was none")
	}

	other := NewJWTService(nil, &config.Config{JWT: config.JWTConfig{SecretKey: "a-different-secret-key-for-this-test!", Expiry: 24}}, nil)
	if other.passwordStateFingerprint(&firstHash) == svc.passwordStateFingerprint(&firstHash) {
		t.Error("the fingerprint must depend on the signing secret")
	}
}

// Each emailed token is accepted by its own validator only.
func TestPasswordResetToken_SubjectIsolation(t *testing.T) {
	svc := passwordResetJWT()
	user := resetTestUser(nil)

	reset, err := svc.CreatePasswordResetToken(user)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ValidateMagicLinkToken(reset); err == nil {
		t.Error("a reset link must not sign in as a magic link")
	}
	if _, err := svc.ValidateAccountRecoveryToken(reset); err == nil {
		t.Error("a reset link must not recover an account")
	}
	if _, err := svc.parseSessionToken(reset); err == nil {
		t.Error("a reset link must not be honored as a session")
	}

	magic, err := svc.CreateMagicLinkToken(user.ID, *user.Email)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ValidatePasswordResetToken(magic); err == nil {
		t.Error("a magic link must not reset a password")
	}
	session, err := svc.CreateToken(user)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ValidatePasswordResetToken(session); err == nil {
		t.Error("a session must not reset a password")
	}
}

func TestPasswordResetToken_ExpiredIsRefused(t *testing.T) {
	svc := passwordResetJWT()
	claims := contracts.PasswordResetTokenClaims{
		UserID: 7, Email: "reset@example.com", PasswordState: svc.passwordStateFingerprint(nil),
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(-time.Second)),
			Issuer:    jwtIssuer,
			Subject:   passwordResetTokenSubject,
		},
	}
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(svc.config.JWT.SecretKey))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ValidatePasswordResetToken(token); err == nil {
		t.Error("an expired reset link must be refused")
	}
}

func TestPasswordResetToken_RequiresAnAddress(t *testing.T) {
	if _, err := passwordResetJWT().CreatePasswordResetToken(&authm.User{ID: 7}); err == nil {
		t.Error("an account with no address has nowhere to send a reset link")
	}
}

// --- Against the database ---------------------------------------------------

// resetClaimsFor mints a reset link for the account as it stands and returns
// its claims.
func (s *JWTIntegrationSuite) resetClaimsFor(userID uint) *contracts.PasswordResetTokenClaims {
	s.T().Helper()
	var account authm.User
	s.Require().NoError(s.db.Take(&account, userID).Error)
	token, err := s.svc.CreatePasswordResetToken(&account)
	s.Require().NoError(err)
	claims, err := s.svc.ValidatePasswordResetToken(token)
	s.Require().NoError(err)
	return claims
}

func (s *JWTIntegrationSuite) setPasswordHash(userID uint, password string) string {
	s.T().Helper()
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.MinCost)
	s.Require().NoError(err)
	s.Require().NoError(s.db.Model(&authm.User{}).Where("id = ?", userID).Update("password_hash", string(hash)).Error)
	return string(hash)
}

func (s *JWTIntegrationSuite) passwordIs(userID uint, password string) bool {
	s.T().Helper()
	user := s.reloadUser(userID)
	return user.PasswordHash != nil && bcrypt.CompareHashAndPassword([]byte(*user.PasswordHash), []byte(password)) == nil
}

// A link replaces a password the owner did not set, without the current one,
// and works once.
func (s *JWTIntegrationSuite) TestCompletePasswordReset_ReplacesThePasswordOnce() {
	user := s.createActiveUser("reset-once@test.com")
	s.setPasswordHash(user.ID, "planted-password-1!")
	claims := s.resetClaimsFor(user.ID)

	account, err := s.authSvc.PasswordResetAccount(claims)
	s.Require().NoError(err, "opening the link spends nothing")
	s.Equal("reset-once@test.com", *account.Email)

	completion, err := s.authSvc.CompletePasswordReset(claims, "owners-new-password-1!")
	s.Require().NoError(err)
	s.Equal(user.ID, completion.User.ID)
	s.True(s.passwordIs(user.ID, "owners-new-password-1!"))
	s.False(s.passwordIs(user.ID, "planted-password-1!"))

	_, err = s.authSvc.CompletePasswordReset(claims, "someone-elses-password-1!")
	s.Require().ErrorIs(err, contracts.ErrPasswordResetLinkSpent, "the link works once")
	s.True(s.passwordIs(user.ID, "owners-new-password-1!"), "a refused reuse changes nothing")
	_, err = s.authSvc.PasswordResetAccount(claims)
	s.ErrorIs(err, contracts.ErrPasswordResetLinkSpent, "opening a used link says it no longer works")
}

// The failed-attempt lockout goes with the old password, so the new one signs
// in at once rather than after the lock runs out, and the next typo does not
// relock the account.
func (s *JWTIntegrationSuite) TestCompletePasswordReset_ClearsTheLockout() {
	user := s.createActiveUser("reset-locked@test.com")
	s.setPasswordHash(user.ID, "forgotten-password-1!")
	lockedUntil := time.Now().Add(15 * time.Minute)
	s.Require().NoError(s.db.Model(&authm.User{}).Where("id = ?", user.ID).
		Updates(map[string]any{"failed_login_attempts": 5, "locked_until": lockedUntil}).Error)
	claims := s.resetClaimsFor(user.ID)

	_, err := s.authSvc.CompletePasswordReset(claims, "owners-new-password-1!")
	s.Require().NoError(err)

	signedIn, err := usersvc.NewUserService(s.db).AuthenticateUserWithPassword("reset-locked@test.com", "owners-new-password-1!")
	s.Require().NoError(err, "the new password signs in at once")
	s.Equal(user.ID, signedIn.ID)
	reloaded := s.reloadUser(user.ID)
	s.Equal(0, reloaded.FailedLoginAttempts)
	s.Nil(reloaded.LockedUntil)
}

// An account with no password gets one.
func (s *JWTIntegrationSuite) TestCompletePasswordReset_SetsAPasswordWhereThereWasNone() {
	user := s.createActiveUser("reset-no-password@test.com")
	claims := s.resetClaimsFor(user.ID)

	_, err := s.authSvc.CompletePasswordReset(claims, "first-password-ever-1!")
	s.Require().NoError(err)
	s.True(s.passwordIs(user.ID, "first-password-ever-1!"))
}

// A reset is a reclaim: every earlier session is refused, and the session it
// issues works and carries an authentication time.
func (s *JWTIntegrationSuite) TestCompletePasswordReset_RevokesEarlierSessionsAndSignsThisBrowserIn() {
	user := s.createActiveUser("reset-reclaim@test.com")
	s.setPasswordHash(user.ID, "planted-password-1!")
	squatter, err := s.svc.CreateToken(user)
	s.Require().NoError(err)
	claims := s.resetClaimsFor(user.ID)

	before := time.Now().Add(-time.Second)
	completion, err := s.authSvc.CompletePasswordReset(claims, "owners-new-password-1!")
	s.Require().NoError(err)

	_, _, err = s.svc.ValidateSession(squatter)
	s.requireRevoked(err)

	got, authAt, err := s.svc.ValidateSession(completion.Token)
	s.Require().NoError(err, "the session the reset issues must survive its own revocation")
	s.Equal(user.ID, got.ID)
	s.False(authAt.IsZero(), "the reset proves the mailbox, so the session carries auth_at")
	s.True(authAt.After(before), "auth_at %v is the reset's", authAt)
}

// Another password change after the link was sent spends the link.
func (s *JWTIntegrationSuite) TestCompletePasswordReset_RefusesALinkMintedBeforeAPasswordChange() {
	user := s.createActiveUser("reset-changed@test.com")
	s.setPasswordHash(user.ID, "first-password-1!")
	claims := s.resetClaimsFor(user.ID)
	s.setPasswordHash(user.ID, "changed-in-settings-1!")

	_, err := s.authSvc.CompletePasswordReset(claims, "owners-new-password-1!")
	s.Require().ErrorIs(err, contracts.ErrPasswordResetLinkSpent)
	s.True(s.passwordIs(user.ID, "changed-in-settings-1!"))
	s.Nil(s.reloadUser(user.ID).SessionsValidAfter, "a refused reset revokes nothing")
}

// A link names the address it was sent to; one sent to an address the
// account no longer uses does nothing.
func (s *JWTIntegrationSuite) TestCompletePasswordReset_RefusesALinkForAnOldAddress() {
	user := s.createActiveUser("reset-old-address@test.com")
	claims := s.resetClaimsFor(user.ID)
	s.Require().NoError(s.db.Model(&authm.User{}).Where("id = ?", user.ID).Update("email", "reset-new-address@test.com").Error)

	_, err := s.authSvc.PasswordResetAccount(claims)
	s.ErrorIs(err, contracts.ErrPasswordResetLinkSpent)
	_, err = s.authSvc.CompletePasswordReset(claims, "owners-new-password-1!")
	s.ErrorIs(err, contracts.ErrPasswordResetLinkSpent)
}

func (s *JWTIntegrationSuite) TestCompletePasswordReset_RefusesAClosedAccount() {
	user := s.createActiveUser("reset-closed@test.com")
	s.setPasswordHash(user.ID, "planted-password-1!")
	claims := s.resetClaimsFor(user.ID)
	s.Require().NoError(s.db.Model(&authm.User{}).Where("id = ?", user.ID).Update("is_active", false).Error)

	_, err := s.authSvc.PasswordResetAccount(claims)
	s.requireAuthCode(err, apperrors.CodeAccountInactive)
	_, err = s.authSvc.CompletePasswordReset(claims, "owners-new-password-1!")
	s.requireAuthCode(err, apperrors.CodeAccountInactive)
	s.True(s.passwordIs(user.ID, "planted-password-1!"))
}

func (s *JWTIntegrationSuite) TestCompletePasswordReset_RefusesAnAccountThatIsGone() {
	user := s.createActiveUser("reset-gone@test.com")
	claims := s.resetClaimsFor(user.ID)
	s.Require().NoError(s.db.Exec("DELETE FROM users WHERE id = ?", user.ID).Error)

	_, err := s.authSvc.PasswordResetAccount(claims)
	s.ErrorIs(err, contracts.ErrPasswordResetLinkSpent)
	_, err = s.authSvc.CompletePasswordReset(claims, "owners-new-password-1!")
	s.ErrorIs(err, contracts.ErrPasswordResetLinkSpent)
}

// bcrypt refuses more than 72 bytes, which the validator's bound admits.
func (s *JWTIntegrationSuite) TestCompletePasswordReset_RefusesAPasswordBcryptCannotHash() {
	user := s.createActiveUser("reset-too-long@test.com")
	claims := s.resetClaimsFor(user.ID)

	long := make([]byte, 73)
	for i := range long {
		long[i] = 'a'
	}
	_, err := s.authSvc.CompletePasswordReset(claims, string(long))
	s.requireAuthCode(err, apperrors.CodeValidationFailed)
	_, err = s.authSvc.PasswordResetAccount(claims)
	s.NoError(err, "a refused password leaves the link usable")
}

// The password, the revocation and the session commit together: a mint that
// fails leaves the password, the sessions and the link as they were.
func (s *JWTIntegrationSuite) TestCompletePasswordReset_MintFailureChangesNothing() {
	user := s.createActiveUser("reset-mint-fails@test.com")
	s.setPasswordHash(user.ID, "planted-password-1!")
	session, err := s.svc.CreateToken(user)
	s.Require().NoError(err)
	claims := s.resetClaimsFor(user.ID)

	mintFailed := errors.New("mint failed")
	_, err = completePasswordReset(s.db, s.svc, claims, "owners-new-password-1!", usersvc.NewUserService(s.db).HashPassword,
		func(*authm.User) (string, error) { return "", mintFailed })
	s.Require().ErrorIs(err, mintFailed)

	s.True(s.passwordIs(user.ID, "planted-password-1!"))
	_, _, err = s.svc.ValidateSession(session)
	s.NoError(err, "a failed reset revokes nothing")
	_, err = s.authSvc.CompletePasswordReset(claims, "owners-new-password-1!")
	s.NoError(err, "the link is still usable after a failure that was not the reader's")
}

func (s *JWTIntegrationSuite) requireAuthCode(err error, code string) {
	s.T().Helper()
	var authErr *apperrors.AuthError
	s.Require().True(errors.As(err, &authErr), "got %v", err)
	s.Equal(code, authErr.Code)
}

// Passkeys keep working after a reset.
func (suite *WebAuthnServiceIntegrationTestSuite) TestCompletePasswordReset_KeepsPasskeys() {
	user := suite.createUser("reset-keeps-passkeys@test.local")
	suite.insertCredential(user.ID, "laptop")
	suite.insertCredential(user.ID, "phone")

	cfg := &config.Config{JWT: config.JWTConfig{SecretKey: "reset-keeps-passkeys-secret-key-32!!", Expiry: 24}}
	jwtSvc := NewJWTService(suite.db, cfg, nil)
	token, err := jwtSvc.CreatePasswordResetToken(user)
	suite.Require().NoError(err)
	claims, err := jwtSvc.ValidatePasswordResetToken(token)
	suite.Require().NoError(err)

	_, err = (&AuthService{db: suite.db, jwtService: jwtSvc, userService: usersvc.NewUserService(suite.db)}).CompletePasswordReset(claims, "owners-new-password-1!")
	suite.Require().NoError(err)

	var count int64
	suite.Require().NoError(suite.db.Model(&authm.WebAuthnCredential{}).Where("user_id = ?", user.ID).Count(&count).Error)
	suite.Equal(int64(2), count)
}
