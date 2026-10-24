import { NextRequest, NextResponse } from 'next/server'
import { cookies } from 'next/headers'
import * as Sentry from '@sentry/nextjs'
import {
  isMutationMethod,
  revalidateAfterProxyMutation,
} from '@/lib/proxy-revalidation'
import { isDevApiProxyEnabled } from '@/lib/dev-api-proxy'

const BACKEND_URL = process.env.BACKEND_URL || 'http://localhost:8080'

/**
 * The browser cookies the proxy hands the backend: the session, and the
 * "Create your account" step's pending sign-up and under-age refusal marker
 * (backend/internal/api/handlers/auth/oauth_signup.go), under the bare name
 * and the "__Host-" name a Secure deployment sets.
 */
const FORWARDED_COOKIES = [
  'auth_token',
  'oauth_pending_signup',
  '__Host-oauth_pending_signup',
  'oauth_signup_age_refused',
  '__Host-oauth_signup_age_refused',
] as const

/** Browser-set headers that say where a request came from. */
const PROVENANCE_HEADERS = ['Origin', 'Referer'] as const

/**
 * Answers in place of the proxy where it is disabled (see
 * `isDevApiProxyEnabled`): the same 404 for every method, with no backend call
 * and no cookie read.
 */
function proxyDisabledResponse() {
  return NextResponse.json({ error: 'Not found' }, { status: 404 })
}

/**
 * Proxy all API requests to the backend.
 * This makes cookies same-origin so SameSite=Lax works in development.
 */
async function proxyRequest(request: NextRequest) {
  if (!isDevApiProxyEnabled()) return proxyDisabledResponse()

  const path = request.nextUrl.pathname.replace(/^\/api/, '')
  const url = `${BACKEND_URL}${path}${request.nextUrl.search}`

  try {
    // Build headers for backend request
    const headers: HeadersInit = {}

    // Forward content-type
    const contentType = request.headers.get('content-type')
    if (contentType) {
      headers['Content-Type'] = contentType
    }

    // Forward the cookies the backend reads on these routes, and no others
    const cookieStore = await cookies()
    const cookieHeader =
      FORWARDED_COOKIES.flatMap(name => {
        const cookie = cookieStore.get(name)
        return cookie ? [`${name}=${cookie.value}`] : []
      }).join('; ') || undefined
    if (cookieHeader) {
      headers['Cookie'] = cookieHeader
    }

    // The backend decides which site a request came from by reading these,
    // so they pass through exactly as the browser sent them and stay absent
    // when it sent none. The proxy never supplies its own: a value it made up
    // would be the proxy vouching for a request the browser did not.
    for (const name of PROVENANCE_HEADERS) {
      const value = request.headers.get(name)
      if (value) {
        headers[name] = value
      }
    }

    // Capture the request body so it can be forwarded AND inspected by the
    // ISR revalidation rules (some rules read entity refs from the request).
    const requestBody =
      request.method !== 'GET' && request.method !== 'HEAD'
        ? await request.text()
        : undefined

    // Make request to backend
    const backendResponse = await fetch(url, {
      method: request.method,
      headers,
      body: requestBody,
    })

    // Capture 5xx errors from backend to Sentry
    if (backendResponse.status >= 500) {
      Sentry.captureMessage(`Backend returned ${backendResponse.status}`, {
        level: 'error',
        tags: { service: 'api-proxy', backend_status: backendResponse.status },
        extra: { path, method: request.method },
      })
    }

    // Read response - handle 204 No Content specially (no body allowed)
    const isNoContent = backendResponse.status === 204
    const responseData = isNoContent ? null : await backendResponse.text()

    // Keep ISR pages in sync after successful mutations (PSY-939). Never
    // throws — see lib/proxy-revalidation.ts.
    if (backendResponse.ok && isMutationMethod(request.method)) {
      await revalidateAfterProxyMutation({
        method: request.method,
        path,
        responseText: responseData,
        requestText: requestBody,
        cookieHeader,
      })
    }

    // Create response
    const response = new NextResponse(responseData, {
      status: backendResponse.status,
      statusText: backendResponse.statusText,
    })

    // Set content-type
    const responseContentType = backendResponse.headers.get('content-type')
    if (responseContentType) {
      response.headers.set('Content-Type', responseContentType)
    }

    // Forward Content-Disposition header (for file downloads)
    const contentDisposition = backendResponse.headers.get('content-disposition')
    if (contentDisposition) {
      response.headers.set('Content-Disposition', contentDisposition)
    }

    // Forward Retry-After (PSY-1542). Every rate limiter in the backend answers
    // 429 with this header, and `ApiError.retryAfter`, which every
    // rate-limit-aware surface renders from, is parsed from it. Browser
    // traffic that goes through this proxy (development) sees only the headers
    // re-emitted here, so dropping it would leave that field undefined.
    // Deployed browsers call the backend cross-origin and read the header
    // through its CORS exposure (`config.CORSExposedHeaders` in the backend).
    const retryAfter = backendResponse.headers.get('retry-after')
    if (retryAfter) {
      response.headers.set('Retry-After', retryAfter)
    }

    // Forward Set-Cookie from backend, but modify for same-origin
    const setCookies = backendResponse.headers.getSetCookie()
    for (const cookie of setCookies) {
      // Remove SameSite=None since we're now same-origin
      // The cookie will work with default Lax
      const modifiedCookie = cookie
        .replace(/;\s*SameSite=None/i, '; SameSite=Lax')
        .replace(/;\s*Domain=[^;]*/i, '') // Remove domain restriction
      response.headers.append('Set-Cookie', modifiedCookie)
    }

    return response
  } catch (error) {
    Sentry.captureException(error, {
      level: 'error',
      tags: { service: 'api-proxy' },
      extra: { path, method: request.method },
    })
    return NextResponse.json(
      { error: 'Backend service unavailable' },
      { status: 502 }
    )
  }
}

export async function GET(request: NextRequest) {
  return proxyRequest(request)
}

export async function POST(request: NextRequest) {
  return proxyRequest(request)
}

export async function PUT(request: NextRequest) {
  return proxyRequest(request)
}

export async function DELETE(request: NextRequest) {
  return proxyRequest(request)
}

export async function PATCH(request: NextRequest) {
  return proxyRequest(request)
}

export async function OPTIONS() {
  if (!isDevApiProxyEnabled()) return proxyDisabledResponse()
  // Preflight is answered here rather than proxied.
  return new NextResponse(null, { status: 204 })
}
