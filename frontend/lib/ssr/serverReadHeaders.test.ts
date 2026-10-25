import { afterEach, describe, expect, it, vi } from 'vitest'
import { SERVER_READ_SECRET_HEADER, serverReadHeaders } from './serverReadHeaders'

describe('serverReadHeaders', () => {
  afterEach(() => {
    vi.unstubAllEnvs()
  })

  it('carries the configured secret in the server-read header', () => {
    vi.stubEnv('SERVER_READ_SECRET', 'a-secret-value')
    expect(serverReadHeaders()).toEqual({ 'X-Server-Read-Secret': 'a-secret-value' })
    expect(SERVER_READ_SECRET_HEADER).toBe('X-Server-Read-Secret')
  })

  it('sends nothing when the secret is unset or blank', () => {
    vi.stubEnv('SERVER_READ_SECRET', '')
    expect(serverReadHeaders()).toEqual({})
    vi.stubEnv('SERVER_READ_SECRET', '  \n')
    expect(serverReadHeaders()).toEqual({})
  })

  it('trims surrounding whitespace, the way the backend does', () => {
    vi.stubEnv('SERVER_READ_SECRET', '  a-secret-value\n')
    expect(serverReadHeaders()).toEqual({ 'X-Server-Read-Secret': 'a-secret-value' })
  })

  // `fetch` throws on a header value it cannot send, which would fail the read
  // instead of only losing the exemption.
  it.each(['has space', 'tab\there', 'née', 'line\nbreak'])(
    'drops a value fetch cannot send: %j',
    value => {
      vi.stubEnv('SERVER_READ_SECRET', value)
      expect(serverReadHeaders()).toEqual({})
    }
  )

  it('produces headers fetch accepts', () => {
    vi.stubEnv('SERVER_READ_SECRET', 'a-secret-value')
    expect(() => new Headers(serverReadHeaders())).not.toThrow()
  })
})
