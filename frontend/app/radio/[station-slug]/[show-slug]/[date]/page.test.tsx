import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

vi.mock('@sentry/nextjs', () => ({
  captureMessage: vi.fn(),
  captureException: vi.fn(),
}))

vi.mock('./_components/EpisodeDateDetail', () => ({
  default: (): null => null,
}))

import * as Sentry from '@sentry/nextjs'
import { generateMetadata } from './page'

const fetchMock = vi.fn()

function paramsFor(date: string) {
  return {
    params: Promise.resolve({ 'station-slug': 'kexp', 'show-slug': 'the-morning-show', date }),
  }
}

describe('radio episode metadata read', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.stubGlobal('fetch', fetchMock)
  })

  afterEach(() => {
    vi.unstubAllGlobals()
  })

  // The backend answers 400 for a date it cannot parse, and a 400 is reported,
  // so a date no episode can have is never sent.
  it.each(['not-a-date', '2026-02-30', '2026-13-01', '2026-1-1', '20260101'])(
    'does not ask the backend about %s',
    async date => {
      const meta = await generateMetadata(paramsFor(date))

      expect(fetchMock).not.toHaveBeenCalled()
      expect(Sentry.captureMessage).not.toHaveBeenCalled()
      expect(meta.title).toBe('Radio Episode')
    }
  )

  it('asks for a real date, with the show slug encoded', async () => {
    fetchMock.mockResolvedValueOnce(new Response(null, { status: 404 }))

    await generateMetadata(paramsFor('2026-02-28'))

    expect(fetchMock).toHaveBeenCalledWith(
      'http://localhost:8080/radio-shows/the-morning-show/episodes/2026-02-28',
      expect.anything()
    )
    expect(Sentry.captureMessage).not.toHaveBeenCalled()
  })

  it('reports a 429 for a real date', async () => {
    fetchMock.mockResolvedValueOnce(new Response(null, { status: 429 }))

    await generateMetadata(paramsFor('2026-02-28'))

    expect(Sentry.captureMessage).toHaveBeenCalledWith(
      'radio-episode-page: API returned 429',
      expect.objectContaining({
        extra: expect.objectContaining({
          pathFamily: '/radio-shows/{slug}/episodes/{date}',
          status: 429,
        }),
      })
    )
  })
})
