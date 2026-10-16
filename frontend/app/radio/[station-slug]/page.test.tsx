import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { okResponse, errorResponse } from '@/lib/seo/test-helpers'

// Both throw in Next, which is what stops the page body; the stubs do the same
// so a test sees exactly one outcome.
vi.mock('next/navigation', () => ({
  notFound: vi.fn(() => {
    throw new Error('NEXT_NOT_FOUND')
  }),
  redirect: vi.fn((path: string) => {
    throw new Error(`NEXT_REDIRECT ${path}`)
  }),
}))

vi.mock('./_components/StationDetail', () => ({
  default: (): null => null,
}))

import { notFound, redirect } from 'next/navigation'
import StationPage from './page'

const WFMU_SUB_STREAM = { slug: 'wfmu', is_flagship: false }
const WFMU_FLAGSHIP = { slug: 'wfmu', is_flagship: true }

function station(slug: string, network: unknown = null) {
  return { name: 'Station', slug, network }
}

function renderPage(stationSlug: string) {
  return StationPage({
    params: Promise.resolve({ 'station-slug': stationSlug }),
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

describe('radio/[station-slug] page', () => {
  it('answers not found for a station the backend does not have', async () => {
    fetchMock.mockResolvedValueOnce(errorResponse(404))

    await expect(renderPage('does-not-exist')).rejects.toThrow('NEXT_NOT_FOUND')
    expect(notFound).toHaveBeenCalledTimes(1)
    expect(redirect).not.toHaveBeenCalled()
  })

  it('renders a network-less station at its slug URL', async () => {
    fetchMock.mockResolvedValueOnce(okResponse(station('kexp')))

    const element = await renderPage('kexp')

    expect(element.props).toEqual({ stationSlug: 'kexp' })
    expect(redirect).not.toHaveBeenCalled()
    expect(notFound).not.toHaveBeenCalled()
  })

  it('renders a network flagship at its slug URL', async () => {
    fetchMock.mockResolvedValueOnce(okResponse(station('wfmu', WFMU_FLAGSHIP)))

    const element = await renderPage('wfmu')

    expect(element.props).toEqual({ stationSlug: 'wfmu' })
    expect(redirect).not.toHaveBeenCalled()
  })

  it('redirects a sub-stream slug to its channel URL', async () => {
    fetchMock.mockResolvedValueOnce(
      okResponse(station('wfmu-drummer', WFMU_SUB_STREAM))
    )

    await expect(renderPage('wfmu-drummer')).rejects.toThrow('NEXT_REDIRECT')
    expect(redirect).toHaveBeenCalledWith('/radio/wfmu/channel/drummer')
  })

  it('redirects a numeric id to the canonical URL of the station it reads', async () => {
    fetchMock.mockResolvedValueOnce(okResponse(station('kexp')))

    await expect(renderPage('7')).rejects.toThrow('NEXT_REDIRECT')
    expect(redirect).toHaveBeenCalledWith('/radio/kexp')
  })

  it("builds a sub-stream's channel URL from its slug, not its numeric id", async () => {
    fetchMock.mockResolvedValueOnce(
      okResponse(station('wfmu-drummer', WFMU_SUB_STREAM))
    )

    await expect(renderPage('9')).rejects.toThrow('NEXT_REDIRECT')
    expect(redirect).toHaveBeenCalledWith('/radio/wfmu/channel/drummer')
  })
})
