import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { AuthError, AuthErrorCode } from './errors'

// We need to test the module functions, but getApiBaseUrl runs at module load time
// So we'll test the exported functions and mock fetch for apiRequest tests

describe('API Module', () => {
  // Store original env
  const originalEnv = { ...process.env }

  beforeEach(() => {
    vi.resetModules()
  })

  afterEach(() => {
    process.env = { ...originalEnv }
    vi.restoreAllMocks()
  })

  describe('getApiBaseUrl behavior', () => {
    it('uses NEXT_PUBLIC_API_URL when set', async () => {
      process.env.NEXT_PUBLIC_API_URL = 'https://custom-api.example.com'

      const { API_BASE_URL } = await import('./api')

      expect(API_BASE_URL).toBe('https://custom-api.example.com')
    })

    it('uses production URL in production mode', async () => {
      delete process.env.NEXT_PUBLIC_API_URL
      process.env.NODE_ENV = 'production'

      const { API_BASE_URL } = await import('./api')

      expect(API_BASE_URL).toBe('https://api.psychichomily.com')
    })

    it('uses /api proxy in development browser-side', async () => {
      delete process.env.NEXT_PUBLIC_API_URL
      process.env.NODE_ENV = 'development'
      // jsdom provides window, so this simulates browser environment

      const { API_BASE_URL } = await import('./api')

      // In browser during development, uses Next.js API proxy
      expect(API_BASE_URL).toBe('/api')
    })
  })

  describe('API_ENDPOINTS', () => {
    it('has auth endpoints', async () => {
      const { API_ENDPOINTS, API_BASE_URL } = await import('./api')

      expect(API_ENDPOINTS.AUTH.LOGIN).toBe(`${API_BASE_URL}/auth/login`)
      expect(API_ENDPOINTS.AUTH.LOGOUT).toBe(`${API_BASE_URL}/auth/logout`)
      expect(API_ENDPOINTS.AUTH.REGISTER).toBe(`${API_BASE_URL}/auth/register`)
      expect(API_ENDPOINTS.AUTH.PROFILE).toBe(`${API_BASE_URL}/auth/profile`)
      expect(API_ENDPOINTS.AUTH.REFRESH).toBe(`${API_BASE_URL}/auth/refresh`)
    })

    it('has dynamic auth endpoints', async () => {
      const { API_ENDPOINTS, API_BASE_URL } = await import('./api')

      expect(API_ENDPOINTS.AUTH.OAUTH_LOGIN('google')).toBe(
        `${API_BASE_URL}/auth/login/google`
      )
      expect(API_ENDPOINTS.AUTH.OAUTH_CALLBACK('github')).toBe(
        `${API_BASE_URL}/auth/callback/github`
      )
    })

    it('has shows endpoints', async () => {
      const { API_ENDPOINTS, API_BASE_URL } = await import('./api')

      expect(API_ENDPOINTS.SHOWS.SUBMIT).toBe(`${API_BASE_URL}/shows`)
      expect(API_ENDPOINTS.SHOWS.UPCOMING).toBe(`${API_BASE_URL}/shows/upcoming`)
      expect(API_ENDPOINTS.SHOWS.GET(123)).toBe(`${API_BASE_URL}/shows/123`)
      expect(API_ENDPOINTS.SHOWS.UPDATE('456')).toBe(`${API_BASE_URL}/shows/456`)
      expect(API_ENDPOINTS.SHOWS.DELETE(789)).toBe(`${API_BASE_URL}/shows/789`)
    })

    it('has artists endpoints', async () => {
      const { API_ENDPOINTS, API_BASE_URL } = await import('./api')

      expect(API_ENDPOINTS.ARTISTS.SEARCH).toBe(`${API_BASE_URL}/artists/search`)
      expect(API_ENDPOINTS.ARTISTS.GET(42)).toBe(`${API_BASE_URL}/artists/42`)
      expect(API_ENDPOINTS.ARTISTS.SHOWS(42)).toBe(
        `${API_BASE_URL}/artists/42/shows`
      )
    })

    it('has venues endpoints', async () => {
      const { API_ENDPOINTS, API_BASE_URL } = await import('./api')

      expect(API_ENDPOINTS.VENUES.LIST).toBe(`${API_BASE_URL}/venues`)
      expect(API_ENDPOINTS.VENUES.SEARCH).toBe(`${API_BASE_URL}/venues/search`)
      expect(API_ENDPOINTS.VENUES.GET(10)).toBe(`${API_BASE_URL}/venues/10`)
      expect(API_ENDPOINTS.VENUES.UPDATE(10)).toBe(`${API_BASE_URL}/venues/10`)
    })

    it('has saved shows endpoints', async () => {
      const { API_ENDPOINTS, API_BASE_URL } = await import('./api')

      expect(API_ENDPOINTS.SAVED_SHOWS.LIST).toBe(`${API_BASE_URL}/saved-shows`)
      expect(API_ENDPOINTS.SAVED_SHOWS.SAVE(5)).toBe(
        `${API_BASE_URL}/saved-shows/5`
      )
      expect(API_ENDPOINTS.SAVED_SHOWS.CHECK('10')).toBe(
        `${API_BASE_URL}/saved-shows/10/check`
      )
    })

    it('has admin endpoints', async () => {
      const { API_ENDPOINTS, API_BASE_URL } = await import('./api')

      expect(API_ENDPOINTS.ADMIN.SHOWS.PENDING).toBe(
        `${API_BASE_URL}/admin/shows/pending`
      )
      expect(API_ENDPOINTS.ADMIN.SHOWS.APPROVE(1)).toBe(
        `${API_BASE_URL}/admin/shows/1/approve`
      )
      expect(API_ENDPOINTS.ADMIN.VENUES.VERIFY(2)).toBe(
        `${API_BASE_URL}/admin/venues/2/verify`
      )
    })
  })

  describe('apiRequest', () => {
    beforeEach(() => {
      vi.stubGlobal('fetch', vi.fn())
    })

    afterEach(() => {
      vi.unstubAllGlobals()
    })

    it('makes request with correct default headers', async () => {
      const mockResponse = {
        ok: true,
        status: 200,
        json: () => Promise.resolve({ data: 'test' }),
        headers: new Headers(),
      }
      vi.mocked(fetch).mockResolvedValue(mockResponse as Response)

      const { apiRequest } = await import('./api')
      await apiRequest('/test-endpoint')

      expect(fetch).toHaveBeenCalledWith(
        '/test-endpoint',
        expect.objectContaining({
          credentials: 'include',
          headers: expect.objectContaining({
            'Content-Type': 'application/json',
          }),
        })
      )
    })

    it('merges custom headers with defaults', async () => {
      const mockResponse = {
        ok: true,
        status: 200,
        json: () => Promise.resolve({ data: 'test' }),
        headers: new Headers(),
      }
      vi.mocked(fetch).mockResolvedValue(mockResponse as Response)

      const { apiRequest } = await import('./api')
      await apiRequest('/test', {
        headers: { 'X-Custom-Header': 'value' },
      })

      expect(fetch).toHaveBeenCalledWith(
        '/test',
        expect.objectContaining({
          headers: expect.objectContaining({
            'Content-Type': 'application/json',
            'X-Custom-Header': 'value',
          }),
        })
      )
    })

    it('returns parsed JSON on success', async () => {
      const responseData = { success: true, data: { id: 1 } }
      const mockResponse = {
        ok: true,
        status: 200,
        json: () => Promise.resolve(responseData),
        headers: new Headers(),
      }
      vi.mocked(fetch).mockResolvedValue(mockResponse as Response)

      const { apiRequest } = await import('./api')
      const result = await apiRequest('/test')

      expect(result).toEqual(responseData)
    })

    it('returns undefined for 204 No Content', async () => {
      const mockResponse = {
        ok: true,
        status: 204,
        headers: new Headers(),
      }
      vi.mocked(fetch).mockResolvedValue(mockResponse as Response)

      const { apiRequest } = await import('./api')
      const result = await apiRequest('/test')

      expect(result).toBeUndefined()
    })

    it('injects request ID from header into response', async () => {
      const headers = new Headers()
      headers.set('X-Request-ID', 'req-from-header')

      const mockResponse = {
        ok: true,
        status: 200,
        json: () => Promise.resolve({ data: 'test' }),
        headers,
      }
      vi.mocked(fetch).mockResolvedValue(mockResponse as Response)

      const { apiRequest } = await import('./api')
      const result = await apiRequest<{ data: string; request_id?: string }>(
        '/test'
      )

      expect(result.request_id).toBe('req-from-header')
    })

    it('does not overwrite existing request_id in response', async () => {
      const headers = new Headers()
      headers.set('X-Request-ID', 'header-id')

      const mockResponse = {
        ok: true,
        status: 200,
        json: () => Promise.resolve({ data: 'test', request_id: 'body-id' }),
        headers,
      }
      vi.mocked(fetch).mockResolvedValue(mockResponse as Response)

      const { apiRequest } = await import('./api')
      const result = await apiRequest<{ data: string; request_id?: string }>(
        '/test'
      )

      expect(result.request_id).toBe('body-id')
    })

    it('throws AuthError for 401 responses', async () => {
      const mockResponse = {
        ok: false,
        status: 401,
        statusText: 'Unauthorized',
        json: () =>
          Promise.resolve({
            message: 'Token expired',
            error_code: 'TOKEN_EXPIRED',
            request_id: 'req-401',
          }),
        headers: new Headers(),
      }
      vi.mocked(fetch).mockResolvedValue(mockResponse as Response)

      const { apiRequest } = await import('./api')

      try {
        await apiRequest('/test')
        expect.fail('Should have thrown')
      } catch (error) {
        expect((error as Error).name).toBe('AuthError')
        expect((error as { code: string }).code).toBe('TOKEN_EXPIRED')
        expect((error as { requestId: string }).requestId).toBe('req-401')
        expect((error as { status: number }).status).toBe(401)
      }
    })

    it('throws AuthError for 403 responses', async () => {
      const mockResponse = {
        ok: false,
        status: 403,
        statusText: 'Forbidden',
        json: () =>
          Promise.resolve({
            message: 'Access denied',
            error_code: 'UNAUTHORIZED',
          }),
        headers: new Headers(),
      }
      vi.mocked(fetch).mockResolvedValue(mockResponse as Response)

      const { apiRequest } = await import('./api')

      try {
        await apiRequest('/test')
        expect.fail('Should have thrown')
      } catch (error) {
        expect((error as Error).name).toBe('AuthError')
        expect((error as { code: string }).code).toBe('UNAUTHORIZED')
        expect((error as { status: number }).status).toBe(403)
      }
    })

    it('throws ApiError for non-auth error responses', async () => {
      const headers = new Headers()
      headers.set('X-Request-ID', 'req-500')

      const mockResponse = {
        ok: false,
        status: 500,
        statusText: 'Internal Server Error',
        json: () =>
          Promise.resolve({
            message: 'Database connection failed',
            error_code: 'DB_ERROR',
          }),
        headers,
      }
      vi.mocked(fetch).mockResolvedValue(mockResponse as Response)

      const { apiRequest } = await import('./api')

      try {
        await apiRequest('/test')
        expect.fail('Should have thrown')
      } catch (error) {
        expect(error).toBeInstanceOf(Error)
        expect((error as Error).message).toBe('Database connection failed')
        expect((error as { status: number }).status).toBe(500)
        expect((error as { requestId: string }).requestId).toBe('req-500')
        expect((error as { errorCode: string }).errorCode).toBe('DB_ERROR')
      }
    })

    it('handles JSON parse errors in error response', async () => {
      const mockResponse = {
        ok: false,
        status: 502,
        statusText: 'Bad Gateway',
        json: () => Promise.reject(new Error('Invalid JSON')),
        headers: new Headers(),
      }
      vi.mocked(fetch).mockResolvedValue(mockResponse as Response)

      const { apiRequest } = await import('./api')

      try {
        await apiRequest('/test')
        expect.fail('Should have thrown')
      } catch (error) {
        expect((error as Error).message).toBe('HTTP 502: Bad Gateway')
      }
    })

    it('extracts detail field from Huma-style errors', async () => {
      const mockResponse = {
        ok: false,
        status: 400,
        statusText: 'Bad Request',
        json: () =>
          Promise.resolve({
            detail: 'Validation failed: email is required',
          }),
        headers: new Headers(),
      }
      vi.mocked(fetch).mockResolvedValue(mockResponse as Response)

      const { apiRequest } = await import('./api')

      try {
        await apiRequest('/test')
        expect.fail('Should have thrown')
      } catch (error) {
        expect((error as Error).message).toBe(
          'Validation failed: email is required'
        )
      }
    })
  })

  describe('getEnvironmentInfo', () => {
    it('returns environment information', async () => {
      const { getEnvironmentInfo, API_BASE_URL } = await import('./api')

      const info = getEnvironmentInfo()

      expect(info.apiBaseUrl).toBe(API_BASE_URL)
      expect(info.environment).toBe(process.env.NODE_ENV)
      expect(typeof info.isDevelopment).toBe('boolean')
      expect(typeof info.isProduction).toBe('boolean')
    })
  })

  describe('getRequestIdFromError', () => {
    it('extracts requestId from AuthError', async () => {
      const { getRequestIdFromError } = await import('./api')

      const error = new AuthError('Test', AuthErrorCode.UNKNOWN, {
        requestId: 'auth-req-id',
      })

      expect(getRequestIdFromError(error)).toBe('auth-req-id')
    })

    it('extracts requestId from ApiError-like object', async () => {
      const { getRequestIdFromError } = await import('./api')

      const error = Object.assign(new Error('Test'), {
        requestId: 'api-req-id',
      })

      expect(getRequestIdFromError(error)).toBe('api-req-id')
    })

    it('returns undefined for regular Error', async () => {
      const { getRequestIdFromError } = await import('./api')

      const error = new Error('Test')

      expect(getRequestIdFromError(error)).toBeUndefined()
    })

    it('returns undefined for null', async () => {
      const { getRequestIdFromError } = await import('./api')

      expect(getRequestIdFromError(null)).toBeUndefined()
    })

    it('returns undefined for undefined', async () => {
      const { getRequestIdFromError } = await import('./api')

      expect(getRequestIdFromError(undefined)).toBeUndefined()
    })
  })
})
