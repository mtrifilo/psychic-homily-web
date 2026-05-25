import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'

// `getApiBaseUrl` runs at module load, so each branch needs a fresh import
// after env + window stubs are applied. resetModules + dynamic import is
// the canonical pattern (also used by lib/api.test.ts for the same module).
describe('lib/api-base', () => {
  const originalEnv = { ...process.env }

  beforeEach(() => {
    vi.resetModules()
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
    ;(process.env as Record<string, string>).NODE_ENV = 'development'
    vi.stubGlobal('window', undefined)

    const { API_BASE_URL } = await import('./api-base')

    expect(API_BASE_URL).toBe('http://localhost:8080')
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
