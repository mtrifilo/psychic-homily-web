import { describe, it, expect, vi, beforeEach } from 'vitest'
import { revalidatePath } from 'next/cache'
import * as Sentry from '@sentry/nextjs'
import { revalidateArtistDetail, safeRevalidatePath } from './revalidate-entity'

vi.mock('next/cache', () => ({
  revalidatePath: vi.fn(),
}))

vi.mock('@sentry/nextjs', () => ({
  captureMessage: vi.fn(),
  captureException: vi.fn(),
}))

const mockRevalidatePath = vi.mocked(revalidatePath)
const mockCaptureMessage = vi.mocked(Sentry.captureMessage)
const mockCaptureException = vi.mocked(Sentry.captureException)

beforeEach(() => {
  // resetAllMocks (not clearAllMocks) so mockImplementation overrides from
  // throwing-path tests don't leak into later tests.
  vi.resetAllMocks()
})

describe('revalidateArtistDetail', () => {
  it('revalidates the artist detail path', () => {
    revalidateArtistDetail('bright-eyes')

    expect(mockRevalidatePath).toHaveBeenCalledTimes(1)
    expect(mockRevalidatePath).toHaveBeenCalledWith('/artists/bright-eyes')
    expect(mockCaptureMessage).not.toHaveBeenCalled()
  })

  it('skips revalidation and reports to Sentry when slug is missing', () => {
    revalidateArtistDetail(undefined)
    revalidateArtistDetail(null)
    revalidateArtistDetail('')

    expect(mockRevalidatePath).not.toHaveBeenCalled()
    expect(mockCaptureMessage).toHaveBeenCalledTimes(3)
  })

  it('never throws when revalidatePath throws — reports to Sentry instead', () => {
    mockRevalidatePath.mockImplementation(() => {
      throw new Error('static generation store missing')
    })

    expect(() => revalidateArtistDetail('bright-eyes')).not.toThrow()
    expect(mockCaptureException).toHaveBeenCalledTimes(1)
  })
})

describe('safeRevalidatePath', () => {
  it('revalidates the given path', () => {
    safeRevalidatePath('/collections/my-list', 'collection-engagement')

    expect(mockRevalidatePath).toHaveBeenCalledTimes(1)
    expect(mockRevalidatePath).toHaveBeenCalledWith('/collections/my-list')
    expect(mockCaptureException).not.toHaveBeenCalled()
  })

  it('never throws when revalidatePath throws — reports to Sentry with the source tag', () => {
    mockRevalidatePath.mockImplementation(() => {
      throw new Error('static generation store missing')
    })

    expect(() => safeRevalidatePath('/explore', 'featured-slots')).not.toThrow()
    expect(mockCaptureException).toHaveBeenCalledTimes(1)
    expect(mockCaptureException).toHaveBeenCalledWith(
      expect.any(Error),
      expect.objectContaining({
        tags: expect.objectContaining({
          service: 'isr-revalidation',
          source: 'featured-slots',
        }),
        extra: expect.objectContaining({ path: '/explore' }),
      })
    )
  })
})
