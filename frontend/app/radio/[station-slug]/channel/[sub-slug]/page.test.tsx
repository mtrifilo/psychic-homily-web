import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { okResponse, errorResponse } from '@/lib/seo/test-helpers'

vi.mock('next/navigation', () => ({
  notFound: vi.fn(() => {
    throw new Error('NEXT_NOT_FOUND')
  }),
}))

vi.mock('../../_components/StationDetail', () => ({
  default: (): null => null,
}))

import { notFound } from 'next/navigation'
import SubStationPage from './page'

function renderPage(networkSlug: string, subSlug: string) {
  return SubStationPage({
    params: Promise.resolve({ 'station-slug': networkSlug, 'sub-slug': subSlug }),
  })
}

const fetchMock = vi.fn()

beforeEach(() => {
  vi.clearAllMocks()
  vi.stubGlobal('fetch', fetchMock)
})

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('radio/[station-slug]/channel/[sub-slug] page', () => {
  it('renders the station the network-prefixed slug names', async () => {
    fetchMock.mockResolvedValueOnce(
      okResponse({ name: 'Give the Drummer Radio', slug: 'wfmu-drummer' })
    )

    const element = await renderPage('wfmu', 'drummer')

    expect(fetchMock).toHaveBeenCalledWith(
      expect.stringMatching(/\/radio-stations\/wfmu-drummer$/),
      expect.anything()
    )
    expect(element.props).toEqual({ stationSlug: 'wfmu-drummer' })
    expect(notFound).not.toHaveBeenCalled()
  })

  it('answers not found for a channel the backend does not have', async () => {
    fetchMock.mockResolvedValueOnce(errorResponse(404))

    await expect(renderPage('wfmu', 'does-not-exist')).rejects.toThrow(
      'NEXT_NOT_FOUND'
    )
    expect(notFound).toHaveBeenCalledTimes(1)
  })
})
