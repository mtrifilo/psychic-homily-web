import { createHmac } from 'crypto'
import { SignJWT } from 'jose'

const JWT_SECRET_TEXT = 'e2e-jwt-secret-key-for-testing-only'
const JWT_SECRET = new TextEncoder().encode(JWT_SECRET_TEXT)

/**
 * Create a valid password-reset JWT matching the Go backend's
 * PasswordResetTokenClaims (internal/services/auth/jwt.go
 * CreatePasswordResetToken). `passwordHash` is the account's
 * users.password_hash as it stands, or '' for none: the token carries its
 * fingerprint (passwordStateFingerprint), an HMAC-SHA256 under the signing
 * secret of "password-reset", a NUL byte and the hash, base64url without
 * padding, and the backend honors the token only while the hash is unchanged.
 */
export async function createPasswordResetToken(
  userId: number,
  email: string,
  passwordHash: string
): Promise<string> {
  const pws = createHmac('sha256', JWT_SECRET_TEXT)
    .update('password-reset')
    .update(Buffer.from([0]))
    .update(passwordHash)
    .digest('base64url')
  return new SignJWT({ user_id: userId, email, pws })
    .setProtectedHeader({ alg: 'HS256' })
    .setIssuedAt()
    .setIssuer('psychic-homily-backend')
    .setSubject('password-reset')
    .setExpirationTime('15m')
    .sign(JWT_SECRET)
}

/**
 * Create a valid email verification JWT matching the Go backend's
 * VerificationTokenClaims structure (services/jwt.go:143-166).
 */
export async function createVerificationToken(
  userId: number,
  email: string
): Promise<string> {
  return new SignJWT({ user_id: userId, email })
    .setProtectedHeader({ alg: 'HS256' })
    .setIssuedAt()
    .setIssuer('psychic-homily-backend')
    .setSubject('email-verification')
    .setExpirationTime('24h')
    .sign(JWT_SECRET)
}

/**
 * Create a valid magic link JWT matching the Go backend's
 * MagicLinkTokenClaims structure (services/jwt.go:194-217).
 */
export async function createMagicLinkToken(
  userId: number,
  email: string
): Promise<string> {
  return new SignJWT({ user_id: userId, email })
    .setProtectedHeader({ alg: 'HS256' })
    .setIssuedAt()
    .setIssuer('psychic-homily-backend')
    .setSubject('magic-link')
    .setExpirationTime('15m')
    .sign(JWT_SECRET)
}

/**
 * Create a valid account-recovery JWT matching the Go backend's
 * AccountRecoveryTokenClaims structure (internal/services/auth/jwt.go
 * CreateAccountRecoveryToken + internal/services/contracts/auth.go).
 *
 * Minting the token directly (instead of triggering a recovery email and
 * scraping it) mirrors the magic-link / verification helpers above: no real
 * email is sent in the E2E env, so the backend's recovery email is never
 * delivered. The token is the only thing the page consumes (`?token=` →
 * ConfirmAccountRecoveryHandler), so reproducing the claims here exercises
 * the same code path the real email link would hit.
 */
export async function createAccountRecoveryToken(
  userId: number,
  email: string
): Promise<string> {
  return new SignJWT({ user_id: userId, email })
    .setProtectedHeader({ alg: 'HS256' })
    .setIssuedAt()
    .setIssuer('psychic-homily-backend')
    .setSubject('account-recovery')
    .setExpirationTime('1h')
    .sign(JWT_SECRET)
}
