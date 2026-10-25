import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

vi.mock('@sentry/nextjs', () => ({
  captureMessage: vi.fn(),
}))

import * as Sentry from '@sentry/nextjs'
import { getAuthenticatedUser } from './auth-profile'

const fetchMock = vi.fn()

describe('getAuthenticatedUser', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.stubGlobal('fetch', fetchMock)
  })

  afterEach(() => {
    vi.unstubAllGlobals()
    vi.unstubAllEnvs()
  })

  it('returns the profile and sends the server-read secret with the session', async () => {
    vi.stubEnv('SERVER_READ_SECRET', 'auth-profile-secret-0123456789abcdef')
    fetchMock.mockResolvedValueOnce(
      new Response(JSON.stringify({ success: true, user: { id: '1' } }), { status: 200 })
    )

    await expect(getAuthenticatedUser('tok')).resolves.toEqual({
      success: true,
      user: { id: '1' },
    })
    expect(fetchMock.mock.calls[0][1]).toEqual({
      headers: {
        'X-Server-Read-Secret': 'auth-profile-secret-0123456789abcdef',
        Cookie: 'auth_token=tok',
      },
    })
  })

  it('returns null and stays silent on a 401, which is an answer', async () => {
    fetchMock.mockResolvedValueOnce(new Response(null, { status: 401 }))

    await expect(getAuthenticatedUser('expired')).resolves.toBeNull()
    expect(Sentry.captureMessage).not.toHaveBeenCalled()
  })

  it.each([429, 500])('returns null and reports a %d', async status => {
    fetchMock.mockResolvedValueOnce(new Response(null, { status }))

    await expect(getAuthenticatedUser('tok')).resolves.toBeNull()
    expect(Sentry.captureMessage).toHaveBeenCalledWith(
      `bff-auth-profile: API returned ${status}`,
      expect.objectContaining({ extra: { pathFamily: '/auth/profile', status } })
    )
  })
})
