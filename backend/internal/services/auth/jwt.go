package auth

import (
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"psychic-homily-backend/internal/config"
	apperrors "psychic-homily-backend/internal/errors"
	authm "psychic-homily-backend/internal/models/auth"
	"psychic-homily-backend/internal/services/contracts"
)

// Session JWT claim values. The issuer and audience are shared by every token
// this service mints; the subject distinguishes a full session token from the
// short-lived single-purpose tokens (email-verification, magic-link,
// account-recovery). ValidateToken asserts all three so a leaked short-lived
// token — which travels through email URLs, query strings, and browser history
// — can never be replayed as a session credential.
const (
	jwtIssuer         = "psychic-homily-backend"
	jwtAudience       = "psychic-homily-users"
	jwtSessionSubject = "session"
)

// jwtAuthTimeClaim carries the moment an authentication FACTOR last completed
// for the session, as distinct from iat, which every mint moves.
//
// Renewing a session copies auth_at through unchanged, so a caller holding only
// a session credential cannot manufacture freshness by asking for a new token.
//
// A token carrying no auth_at establishes no authentication time: readers get
// the zero time and treat it as not recent, which is the fail-closed answer.
// API-token principals are of that shape.
const jwtAuthTimeClaim = "auth_at"

type JWTService struct {
	config      *config.Config
	userService contracts.UserServiceInterface
}

func NewJWTService(database interface{}, cfg *config.Config, userService contracts.UserServiceInterface) *JWTService {
	return &JWTService{
		config:      cfg,
		userService: userService,
	}
}

// CreateToken mints a session JWT for a caller that has JUST completed an
// authentication factor, stamping auth_at with the current time. A path that
// issues a session on the strength of a session the caller already holds is a
// renewal and must use RenewSessionToken.
func (s *JWTService) CreateToken(user *authm.User) (string, error) {
	return s.RenewSessionToken(user, time.Now())
}

// RenewSessionToken mints a session JWT carrying authAt as its auth_at claim.
// A zero authAt writes no claim at all.
func (s *JWTService) RenewSessionToken(user *authm.User, authAt time.Time) (string, error) {
	claims := jwt.MapClaims{
		"user_id": user.ID,
		"email":   user.Email,
		"exp":     time.Now().Add(time.Duration(s.config.JWT.Expiry) * time.Hour).Unix(),
		"iat":     time.Now().Unix(),
		"iss":     jwtIssuer,
		"aud":     jwtAudience,
		"sub":     jwtSessionSubject,
	}
	if !authAt.IsZero() {
		claims[jwtAuthTimeClaim] = authAt.Unix()
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(s.config.JWT.SecretKey))
}

// parseSessionToken parses and cryptographically verifies a session JWT —
// signing method (HMAC), signature (secret), issuer, audience, subject, and
// expiry — WITHOUT any database lookup, returning the validated claims.
//
// Enforce the session subject, issuer, and audience as part of parsing.
// WithSubject/WithIssuer/WithAudience require the claim to exist and match, so
// single-purpose tokens (email-verification, magic-link, account-recovery) and
// legacy session tokens minted before the subject was added are rejected here
// rather than being honored as session credentials.
func (s *JWTService) parseSessionToken(tokenString string) (jwt.MapClaims, error) {
	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return []byte(s.config.JWT.SecretKey), nil
	}, jwt.WithIssuer(jwtIssuer), jwt.WithAudience(jwtAudience), jwt.WithSubject(jwtSessionSubject))

	if err != nil {
		if errors.Is(err, jwt.ErrTokenExpired) {
			return nil, apperrors.ErrTokenExpired(err)
		}
		return nil, apperrors.ErrTokenInvalid(err)
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok || !token.Valid {
		return nil, apperrors.ErrTokenInvalid(nil)
	}
	return claims, nil
}

// SessionUserID returns the user id from a validly-signed, unexpired session
// token — WITHOUT loading the user from the database. ok is false for any
// invalid/expired/forged token or a missing/non-numeric user_id claim.
//
// Used by the public-read rate limiter (middleware.RateLimitPublicReadsByAuthState,
// PSY-1373) to (a) tell anonymous traffic from logged-in users and (b) key the
// authenticated per-user rate bucket by id — on EVERY request, so a per-request DB
// query would be wasteful. Deliberately does NOT check IsActive/admin: for
// metering read abuse, a validly-signed session token identifies a real account,
// which is all the per-user cap needs.
func (s *JWTService) SessionUserID(tokenString string) (uint, bool) {
	claims, err := s.parseSessionToken(tokenString)
	if err != nil {
		return 0, false
	}
	uid, ok := claims["user_id"].(float64)
	// Reject <= 0: DB user ids are serial and start at 1, so 0/negative is never a
	// real user. Rejecting 0 also keeps it distinct from any "unset" default in
	// downstream keying (adversarial-review).
	if !ok || uid < 1 {
		return 0, false
	}
	return uint(uid), true
}

// maxAuthTimeSkew is how far ahead of the reader's clock an authentication time
// may sit and still be believed.
//
// Some tolerance is needed because the clock that stamped the claim is not
// always the clock that reads it. A bound is needed because a renewal copies
// auth_at forward untouched: a single mint under a clock running hours fast
// would otherwise leave a session that every freshness gate accepts for those
// hours, refreshed indefinitely, with no factor behind it. Beyond the bound the
// claim is discarded rather than trusted, which costs the user one sign-in.
const maxAuthTimeSkew = 2 * time.Minute

// authTimeFromClaims reads auth_at out of already-verified session claims. A
// missing, non-numeric, non-positive, or implausibly future value establishes
// no authentication time, so the result is the zero time and callers treat the
// session as not recently authenticated.
func authTimeFromClaims(claims jwt.MapClaims) time.Time {
	authAt, ok := claims[jwtAuthTimeClaim].(float64)
	// Compared as a float, before any conversion: that bounds the value to the
	// believable range and to int64 in one test, so the conversion below is
	// always defined.
	believableThrough := float64(time.Now().Add(maxAuthTimeSkew).Unix())
	if !ok || authAt <= 0 || authAt > believableThrough {
		return time.Time{}
	}
	return time.Unix(int64(authAt), 0).UTC()
}

// userFromSessionClaims loads the principal named by already-verified session
// claims. The database is the authority on current admin status and on whether
// the account is still active, neither of which the token can be trusted for.
func (s *JWTService) userFromSessionClaims(claims jwt.MapClaims) (*authm.User, error) {
	// Rejecting id < 1 keeps this reader and SessionUserID agreeing on what a
	// valid id is, and keeps a negative value out of the conversion to uint.
	id, ok := claims["user_id"].(float64)
	if !ok || id < 1 {
		return nil, apperrors.ErrTokenInvalid(fmt.Errorf("missing or invalid user_id claim"))
	}

	user, err := s.userService.GetUserByID(uint(id))
	if err != nil {
		return nil, fmt.Errorf("failed to get user: %w", err)
	}

	if !user.IsActive {
		return nil, apperrors.ErrTokenInvalid(fmt.Errorf("user account is not active"))
	}

	return user, nil
}

// ValidateSession validates a session JWT and returns both facts a caller can
// establish from it in one parse: the principal, and when an authentication
// factor last completed for the session. A zero authAt means the token
// establishes no authentication time.
//
// Callers that gate on freshness take the time from here rather than re-reading
// the token, so one parse serves both and there is one notion of what a valid
// session is.
func (s *JWTService) ValidateSession(tokenString string) (user *authm.User, authAt time.Time, err error) {
	claims, err := s.parseSessionToken(tokenString)
	if err != nil {
		return nil, time.Time{}, err
	}
	user, err = s.userFromSessionClaims(claims)
	if err != nil {
		return nil, time.Time{}, err
	}
	return user, authTimeFromClaims(claims), nil
}

// ValidateToken validates and extracts user info from JWT
// Fetches the full user from the database to ensure we have current admin status
func (s *JWTService) ValidateToken(tokenString string) (*authm.User, error) {
	user, _, err := s.ValidateSession(tokenString)
	return user, err
}

// ValidateSessionLenient is ValidateSession for the refresh path: it also
// accepts a token whose expiry is at most gracePeriod in the past, so a client
// whose session lapsed recently can renew rather than sign in again.
func (s *JWTService) ValidateSessionLenient(tokenString string, gracePeriod time.Duration) (*authm.User, time.Time, error) {
	// The unexpired case is the common one and needs no second parse.
	if user, authAt, err := s.ValidateSession(tokenString); err == nil {
		return user, authAt, nil
	}

	claims, err := s.parseSessionTokenLenient(tokenString, gracePeriod)
	if err != nil {
		return nil, time.Time{}, err
	}
	user, err := s.userFromSessionClaims(claims)
	if err != nil {
		return nil, time.Time{}, err
	}
	return user, authTimeFromClaims(claims), nil
}

// ValidateTokenLenient validates a JWT but allows tokens that expired within a grace period.
// This is used for token refresh — the client sends an expired token to get a new one.
// The grace period prevents forcing re-login when the token expired recently.
func (s *JWTService) ValidateTokenLenient(tokenString string, gracePeriod time.Duration) (*authm.User, error) {
	user, _, err := s.ValidateSessionLenient(tokenString, gracePeriod)
	return user, err
}

// parseSessionTokenLenient verifies a session JWT's signature and claims the way
// parseSessionToken does, except that expiry is allowed to be up to gracePeriod
// in the past. No database lookup.
//
// Subject, issuer, and audience are asserted here by hand: the parse runs with
// claim validation off so the expiry can be judged against the grace period,
// which switches off the built-in checks too. Without them, a leaked
// single-purpose token (magic-link, verification, recovery) caught inside the
// grace window could be renewed into a session.
func (s *JWTService) parseSessionTokenLenient(tokenString string, gracePeriod time.Duration) (jwt.MapClaims, error) {
	parser := jwt.NewParser(jwt.WithoutClaimsValidation())
	token, parseErr := parser.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return []byte(s.config.JWT.SecretKey), nil
	})

	if parseErr != nil {
		return nil, apperrors.ErrTokenInvalid(parseErr)
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return nil, apperrors.ErrTokenInvalid(nil)
	}

	exp, ok := claims["exp"].(float64)
	if !ok {
		return nil, apperrors.ErrTokenInvalid(fmt.Errorf("missing expiration claim"))
	}

	expTime := time.Unix(int64(exp), 0)
	if time.Since(expTime) > gracePeriod {
		return nil, apperrors.ErrTokenExpired(fmt.Errorf("token expired beyond grace period"))
	}

	sub, _ := claims["sub"].(string)
	iss, _ := claims["iss"].(string)
	aud, _ := claims["aud"].(string)
	if sub != jwtSessionSubject || iss != jwtIssuer || aud != jwtAudience {
		return nil, apperrors.ErrTokenInvalid(fmt.Errorf("invalid token subject, issuer, or audience"))
	}

	return claims, nil
}

// CreateVerificationToken generates a JWT token for email verification
// Token expires in 24 hours
func (s *JWTService) CreateVerificationToken(userID uint, email string) (string, error) {
	claims := contracts.VerificationTokenClaims{
		UserID: userID,
		Email:  email,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(24 * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			Issuer:    "psychic-homily-backend",
			Subject:   "email-verification",
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(s.config.JWT.SecretKey))
}

// ValidateVerificationToken validates an email verification token and returns the claims
func (s *JWTService) ValidateVerificationToken(tokenString string) (*contracts.VerificationTokenClaims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &contracts.VerificationTokenClaims{}, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return []byte(s.config.JWT.SecretKey), nil
	})

	if err != nil {
		return nil, fmt.Errorf("invalid verification token: %w", err)
	}

	claims, ok := token.Claims.(*contracts.VerificationTokenClaims)
	if !ok || !token.Valid {
		return nil, fmt.Errorf("invalid verification token claims")
	}

	// Verify the token is for email verification
	if claims.Subject != "email-verification" {
		return nil, fmt.Errorf("invalid token type")
	}

	return claims, nil
}

// CreateMagicLinkToken generates a JWT token for magic link login
// Token expires in 15 minutes for security
func (s *JWTService) CreateMagicLinkToken(userID uint, email string) (string, error) {
	claims := contracts.MagicLinkTokenClaims{
		UserID: userID,
		Email:  email,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(15 * time.Minute)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			Issuer:    "psychic-homily-backend",
			Subject:   "magic-link",
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(s.config.JWT.SecretKey))
}

// ValidateMagicLinkToken validates a magic link token and returns the claims
func (s *JWTService) ValidateMagicLinkToken(tokenString string) (*contracts.MagicLinkTokenClaims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &contracts.MagicLinkTokenClaims{}, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return []byte(s.config.JWT.SecretKey), nil
	})

	if err != nil {
		return nil, fmt.Errorf("invalid magic link token: %w", err)
	}

	claims, ok := token.Claims.(*contracts.MagicLinkTokenClaims)
	if !ok || !token.Valid {
		return nil, fmt.Errorf("invalid magic link token claims")
	}

	// Verify the token is for magic link
	if claims.Subject != "magic-link" {
		return nil, fmt.Errorf("invalid token type")
	}

	return claims, nil
}

// CreateAccountRecoveryToken generates a JWT token for account recovery
// Token expires in 1 hour for security
func (s *JWTService) CreateAccountRecoveryToken(userID uint, email string) (string, error) {
	claims := contracts.AccountRecoveryTokenClaims{
		UserID: userID,
		Email:  email,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(1 * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			Issuer:    "psychic-homily-backend",
			Subject:   "account-recovery",
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(s.config.JWT.SecretKey))
}

// ValidateAccountRecoveryToken validates an account recovery token and returns the claims
func (s *JWTService) ValidateAccountRecoveryToken(tokenString string) (*contracts.AccountRecoveryTokenClaims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &contracts.AccountRecoveryTokenClaims{}, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return []byte(s.config.JWT.SecretKey), nil
	})

	if err != nil {
		return nil, fmt.Errorf("invalid account recovery token: %w", err)
	}

	claims, ok := token.Claims.(*contracts.AccountRecoveryTokenClaims)
	if !ok || !token.Valid {
		return nil, fmt.Errorf("invalid account recovery token claims")
	}

	// Verify the token is for account recovery
	if claims.Subject != "account-recovery" {
		return nil, fmt.Errorf("invalid token type")
	}

	return claims, nil
}
