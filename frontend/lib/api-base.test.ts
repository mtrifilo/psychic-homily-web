import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'

// `getApiBaseUrl` runs at module load, so each branch needs a fresh import
// after env + window stubs are applied. resetModules + dynamic import is
// the canonical pattern (also used by lib/api.test.ts for the same module).
describe('lib/api-base', () => {
  const originalEnv = { ...process.env }

  beforeEach(() => {
    vi.resetModules()
    // Not pinned by vitest.config.mts, so an ambient BACKEND_URL in the
    // developer's shell would otherwise leak into the SSR branch below.
    delete process.env.BACKEND_URL
  })

  afterEach(() => {
    process.env = { ...originalEnv }
    vi.unstubAllGlobals()
  })

  it('returns NEXT_PUBLIC_API_URL when set (highest precedence)', async () => {
    process.env.NEXT_PUBLIC_API_URL = 'https://custom-api.example.com'
    ;(process.env as Record<string, string>).NODE_ENV = 'development'

    const { API_BASE_URL } = await import('./api-base')

    expect(API_BASE_URL).toBe('https://custom-api.example.com')
  })

  it('returns /api proxy in dev when window is defined (browser-side)', async () => {
    delete process.env.NEXT_PUBLIC_API_URL
    ;(process.env as Record<string, string>).NODE_ENV = 'development'

    const { API_BASE_URL } = await import('./api-base')

    expect(API_BASE_URL).toBe('/api')
  })

  it('returns http://localhost:8080 in dev when window is undefined (server-side)', async () => {
    delete process.env.NEXT_PUBLIC_API_URL
    delete process.env.BACKEND_URL
    ;(process.env as Record<string, string>).NODE_ENV = 'development'
    vi.stubGlobal('window', undefined)

    const { API_BASE_URL } = await import('./api-base')

    expect(API_BASE_URL).toBe('http://localhost:8080')
  })

  // PSY-1649: SSR bypasses the /api proxy, so it has to follow the same
  // BACKEND_URL the proxy forwards to. Hardcoding :8080 here meant a backend
  // on any other port was invisible to server rendering.
  it('follows BACKEND_URL in dev when window is undefined (server-side)', async () => {
    delete process.env.NEXT_PUBLIC_API_URL
    process.env.BACKEND_URL = 'http://localhost:8099'
    ;(process.env as Record<string, string>).NODE_ENV = 'development'
    vi.stubGlobal('window', undefined)

    const { API_BASE_URL } = await import('./api-base')

    expect(API_BASE_URL).toBe('http://localhost:8099')
  })

  it('ignores a malformed BACKEND_URL rather than propagating it', async () => {
    delete process.env.NEXT_PUBLIC_API_URL
    process.env.BACKEND_URL = 'not a url'
    ;(process.env as Record<string, string>).NODE_ENV = 'development'
    vi.stubGlobal('window', undefined)

    const { API_BASE_URL } = await import('./api-base')

    expect(API_BASE_URL).toBe('http://localhost:8080')
  })

  // BACKEND_URL is server-only config; the browser must keep using the
  // same-origin proxy so the SameSite=Lax auth cookie rides along.
  it('ignores BACKEND_URL in the browser and still uses the /api proxy', async () => {
    delete process.env.NEXT_PUBLIC_API_URL
    process.env.BACKEND_URL = 'http://localhost:8099'
    ;(process.env as Record<string, string>).NODE_ENV = 'development'

    const { API_BASE_URL } = await import('./api-base')

    expect(API_BASE_URL).toBe('/api')
  })

  it('NEXT_PUBLIC_API_URL still wins over BACKEND_URL', async () => {
    process.env.NEXT_PUBLIC_API_URL = 'http://localhost:3001/api'
    process.env.BACKEND_URL = 'http://localhost:8099'
    ;(process.env as Record<string, string>).NODE_ENV = 'development'
    vi.stubGlobal('window', undefined)

    const { API_BASE_URL } = await import('./api-base')

    expect(API_BASE_URL).toBe('http://localhost:3001/api')
  })

  it('ignores BACKEND_URL in production', async () => {
    delete process.env.NEXT_PUBLIC_API_URL
    process.env.BACKEND_URL = 'http://localhost:8099'
    ;(process.env as Record<string, string>).NODE_ENV = 'production'
    vi.stubGlobal('window', undefined)

    const { API_BASE_URL } = await import('./api-base')

    expect(API_BASE_URL).toBe('https://api.psychichomily.com')
  })

  it('returns the prod fallback URL when NODE_ENV is production', async () => {
    delete process.env.NEXT_PUBLIC_API_URL
    ;(process.env as Record<string, string>).NODE_ENV = 'production'

    const { API_BASE_URL } = await import('./api-base')

    expect(API_BASE_URL).toBe('https://api.psychichomily.com')
  })

  it('NEXT_PUBLIC_API_URL wins over the production fallback', async () => {
    process.env.NEXT_PUBLIC_API_URL = 'https://staging-api.example.com'
    ;(process.env as Record<string, string>).NODE_ENV = 'production'

    const { API_BASE_URL } = await import('./api-base')

    expect(API_BASE_URL).toBe('https://staging-api.example.com')
  })
})

// PSY-1649: the OAuth base is deliberately NOT the data base. The data path
// wants the same-origin `/api` proxy so the SameSite=Lax auth cookie rides
// along; the OAuth path is a full-page redirect that the proxy breaks.
describe('lib/api-base OAUTH_BACKEND_URL', () => {
  const originalEnv = { ...process.env }

  beforeEach(() => {
    vi.resetModules()
    delete process.env.NEXT_PUBLIC_API_URL
    delete process.env.NEXT_PUBLIC_OAUTH_BACKEND_URL
    // The proxy-mount warning is expected on some of these paths; silence it
    // so a passing run isn't noisy, and so the assertions below are the thing
    // under test rather than console output.
    vi.spyOn(console, 'warn').mockImplementation(() => {})
  })

  afterEach(() => {
    process.env = { ...originalEnv }
    vi.unstubAllGlobals()
    vi.restoreAllMocks()
  })

  it('prefers NEXT_PUBLIC_OAUTH_BACKEND_URL over NEXT_PUBLIC_API_URL', async () => {
    // The isolated-dispatch-stack shape: data goes through the frontend's own
    // /api proxy, OAuth goes straight to the backend port.
    process.env.NEXT_PUBLIC_API_URL = 'http://localhost:3001/api'
    process.env.NEXT_PUBLIC_OAUTH_BACKEND_URL = 'http://localhost:8099'
    ;(process.env as Record<string, string>).NODE_ENV = 'development'

    const { API_BASE_URL, OAUTH_BACKEND_URL } = await import('./api-base')

    expect(API_BASE_URL).toBe('http://localhost:3001/api')
    expect(OAUTH_BACKEND_URL).toBe('http://localhost:8099')
  })

  // Production safety: this is the deployed shape today, and it must not move.
  it('falls back to NEXT_PUBLIC_API_URL when the OAuth variable is unset', async () => {
    process.env.NEXT_PUBLIC_API_URL = 'https://api.example.com'
    ;(process.env as Record<string, string>).NODE_ENV = 'production'

    const { OAUTH_BACKEND_URL } = await import('./api-base')

    expect(OAUTH_BACKEND_URL).toBe('https://api.example.com')
  })

  it('falls back to localhost:8080 when nothing is set outside production', async () => {
    ;(process.env as Record<string, string>).NODE_ENV = 'development'

    const { OAUTH_BACKEND_URL } = await import('./api-base')

    expect(OAUTH_BACKEND_URL).toBe('http://localhost:8080')
  })

  // The guard AC2 asks for: "NEXT_PUBLIC_API_URL is always set in production"
  // is now enforced rather than assumed. A production bundle cannot carry a
  // localhost auth endpoint even if every variable is missing.
  it('never falls back to localhost in a production build', async () => {
    ;(process.env as Record<string, string>).NODE_ENV = 'production'

    const { OAUTH_BACKEND_URL } = await import('./api-base')

    expect(OAUTH_BACKEND_URL).toBe('https://api.psychichomily.com')
    expect(OAUTH_BACKEND_URL).not.toContain('localhost')
  })

  it('ignores a relative NEXT_PUBLIC_OAUTH_BACKEND_URL', async () => {
    // `/api` would make `new URL()` throw inside the Google button.
    process.env.NEXT_PUBLIC_OAUTH_BACKEND_URL = '/api'
    process.env.NEXT_PUBLIC_API_URL = 'https://api.example.com'
    ;(process.env as Record<string, string>).NODE_ENV = 'development'

    const { OAUTH_BACKEND_URL } = await import('./api-base')

    expect(OAUTH_BACKEND_URL).toBe('https://api.example.com')
  })

  it('ignores a non-http NEXT_PUBLIC_OAUTH_BACKEND_URL', async () => {
    process.env.NEXT_PUBLIC_OAUTH_BACKEND_URL = 'javascript:alert(1)'
    ;(process.env as Record<string, string>).NODE_ENV = 'development'

    const { OAUTH_BACKEND_URL } = await import('./api-base')

    expect(OAUTH_BACKEND_URL).toBe('http://localhost:8080')
  })

  it('warns outside production when the OAuth base looks like a proxy mount', async () => {
    process.env.NEXT_PUBLIC_API_URL = 'http://localhost:3001/api'
    ;(process.env as Record<string, string>).NODE_ENV = 'development'

    await import('./api-base')

    expect(console.warn).toHaveBeenCalledWith(
      expect.stringContaining('NEXT_PUBLIC_OAUTH_BACKEND_URL'),
    )
  })

  it('does not warn for a bare origin', async () => {
    process.env.NEXT_PUBLIC_API_URL = 'https://api.example.com'
    ;(process.env as Record<string, string>).NODE_ENV = 'development'

    await import('./api-base')

    expect(console.warn).not.toHaveBeenCalled()
  })
})
