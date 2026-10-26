import { afterEach, describe, expect, it, vi } from 'vitest'
import { SERVER_READ_SECRET_HEADER, serverReadHeaders } from './serverReadHeaders'

describe('serverReadHeaders', () => {
  afterEach(() => {
    vi.unstubAllEnvs()
    vi.restoreAllMocks()
  })

  it('carries the configured secret in the server-read header', () => {
    vi.stubEnv('SERVER_READ_SECRET', 'a-secret-value-0123456789abcdef01234567')
    expect(serverReadHeaders()).toEqual({ 'X-Server-Read-Secret': 'a-secret-value-0123456789abcdef01234567' })
    expect(SERVER_READ_SECRET_HEADER).toBe('X-Server-Read-Secret')
  })

  it('sends nothing when the secret is unset or blank', () => {
    vi.stubEnv('SERVER_READ_SECRET', '')
    expect(serverReadHeaders()).toEqual({})
    vi.stubEnv('SERVER_READ_SECRET', '  \n')
    expect(serverReadHeaders()).toEqual({})
  })

  it('trims surrounding whitespace, the way the backend does', () => {
    vi.stubEnv('SERVER_READ_SECRET', '  a-secret-value-0123456789abcdef01234567\n')
    expect(serverReadHeaders()).toEqual({ 'X-Server-Read-Secret': 'a-secret-value-0123456789abcdef01234567' })
  })

  // The backend refuses such a secret at start, so sending it could never match.
  it.each([
    'has space inside a value long enough',
    'tab\there-inside-a-value-long-enough-to-pass',
    'née-inside-a-value-long-enough-to-pass-it',
    'line\nbreak-inside-a-value-long-enough-to-pass',
    'new-secret-0123456789abcdef,old-secret-0123456789',
    'short-secret',
  ])(
    'drops a value the backend refuses: %j',
    value => {
      vi.spyOn(console, 'error').mockImplementation(() => {})
      vi.stubEnv('SERVER_READ_SECRET', value)
      expect(serverReadHeaders()).toEqual({})
    }
  )

  it('logs a dropped value exactly once per process, without the value', async () => {
    vi.resetModules()
    const fresh = await import('./serverReadHeaders')
    const error = vi.spyOn(console, 'error').mockImplementation(() => {})
    vi.stubEnv('SERVER_READ_SECRET', 'has space')
    fresh.serverReadHeaders()
    fresh.serverReadHeaders()
    expect(error).toHaveBeenCalledTimes(1)
    expect(JSON.stringify(error.mock.calls)).not.toContain('has space')
  })

  it('produces headers fetch accepts', () => {
    vi.stubEnv('SERVER_READ_SECRET', 'a-secret-value-0123456789abcdef01234567')
    expect(() => new Headers(serverReadHeaders())).not.toThrow()
  })
})
