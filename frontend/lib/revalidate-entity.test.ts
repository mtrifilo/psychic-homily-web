import { describe, it, expect, vi, beforeEach } from 'vitest'
import { revalidatePath } from 'next/cache'
import * as Sentry from '@sentry/nextjs'
import { revalidateArtistDetail } from './revalidate-entity'

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
  vi.clearAllMocks()
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
