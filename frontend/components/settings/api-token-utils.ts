/**
 * Pure utility functions for API token management.
 */

export function formatTokenDate(dateString: string): string {
  return new Date(dateString).toLocaleDateString('en-US', {
    year: 'numeric',
    month: 'short',
    day: 'numeric',
  })
}

export function formatTokenDateTime(dateString: string): string {
  return new Date(dateString).toLocaleDateString('en-US', {
    year: 'numeric',
    month: 'short',
    day: 'numeric',
    hour: 'numeric',
    minute: '2-digit',
  })
}

/**
 * Determine whether a token is expiring within the given threshold (default 7 days).
 */
export function isTokenExpiringSoon(
  expiresAt: string,
  isExpired: boolean,
  thresholdMs: number = 7 * 24 * 60 * 60 * 1000
): boolean {
  if (isExpired) return false
  return new Date(expiresAt).getTime() - Date.now() < thresholdMs
}
