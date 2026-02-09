import { SignJWT } from 'jose'

const JWT_SECRET = new TextEncoder().encode(
  'e2e-jwt-secret-key-for-testing-only'
)

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
