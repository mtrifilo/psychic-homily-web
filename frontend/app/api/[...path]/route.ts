import { NextRequest, NextResponse } from 'next/server'
import { cookies } from 'next/headers'

const BACKEND_URL = 'http://localhost:8080'

/**
 * Proxy all API requests to the backend.
 * This makes cookies same-origin so SameSite=Lax works in development.
 */
async function proxyRequest(request: NextRequest) {
  const path = request.nextUrl.pathname.replace(/^\/api/, '')
  const url = `${BACKEND_URL}${path}${request.nextUrl.search}`

  // Build headers for backend request
  const headers: HeadersInit = {}

  // Forward content-type
  const contentType = request.headers.get('content-type')
  if (contentType) {
    headers['Content-Type'] = contentType
  }

  // Forward auth cookie from browser
  const cookieStore = await cookies()
  const authToken = cookieStore.get('auth_token')
  if (authToken) {
    headers['Cookie'] = `auth_token=${authToken.value}`
  }

  // Make request to backend
  const backendResponse = await fetch(url, {
    method: request.method,
    headers,
    body:
      request.method !== 'GET' && request.method !== 'HEAD'
        ? await request.text()
        : undefined,
  })

  // Read response - handle 204 No Content specially (no body allowed)
  const isNoContent = backendResponse.status === 204
  const responseData = isNoContent ? null : await backendResponse.text()

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

export async function OPTIONS(request: NextRequest) {
  // Handle preflight - no need to proxy, just respond
  return new NextResponse(null, { status: 204 })
}
