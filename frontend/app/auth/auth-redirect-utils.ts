const FALLBACK_RETURN_TO = '/'
const BASE_ORIGIN = 'https://psychichomily.com'

function isAuthPath(pathname: string): boolean {
  return pathname === '/auth' || pathname.startsWith('/auth/')
}

export function sanitizeReturnTo(
  rawReturnTo: string | null | undefined
): string {
  if (!rawReturnTo) {
    return FALLBACK_RETURN_TO
  }

  const trimmed = rawReturnTo.trim()
  if (!trimmed || !trimmed.startsWith('/') || trimmed.startsWith('//')) {
    return FALLBACK_RETURN_TO
  }

  try {
    const parsed = new URL(trimmed, BASE_ORIGIN)
    if (parsed.origin !== BASE_ORIGIN || isAuthPath(parsed.pathname)) {
      return FALLBACK_RETURN_TO
    }
    return `${parsed.pathname}${parsed.search}${parsed.hash}`
  } catch {
    return FALLBACK_RETURN_TO
  }
}

export function safeDecodeQueryParam(
  rawValue: string | null | undefined
): string | null {
  if (!rawValue) {
    return null
  }

  try {
    return decodeURIComponent(rawValue)
  } catch {
    return rawValue
  }
}
