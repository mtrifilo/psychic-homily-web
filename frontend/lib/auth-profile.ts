// The backend session-profile fetch, shared by every BFF route that needs to
// identify the caller (admin artist routes via requireAdmin; the AI extract
// routes for an auth-only check). Single source of truth for the
// /auth/profile contract so a change to it is made once (PSY-1111).

const BACKEND_URL = process.env.BACKEND_URL || 'http://localhost:8080'

export interface UserProfile {
  success: boolean
  user?: {
    id: string
    is_admin?: boolean
    email?: string
  }
}

// Fetches the backend auth profile for a session token. Returns null on any
// failure (network error or non-2xx) so callers treat it as unauthenticated.
export async function getAuthenticatedUser(
  authToken: string
): Promise<UserProfile | null> {
  try {
    const response = await fetch(`${BACKEND_URL}/auth/profile`, {
      headers: { Cookie: `auth_token=${authToken}` },
    })
    if (!response.ok) return null
    return await response.json()
  } catch {
    return null
  }
}
