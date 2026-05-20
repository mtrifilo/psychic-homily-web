import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import type { PipelineVenueInfo } from '@/lib/hooks/usePipeline'

// `vi.hoisted` lets us share these handles between the hoisted vi.mock factory
// and the test bodies below. Anything referenced inside `vi.mock` factories
// MUST live in here, since `vi.mock` calls are hoisted above module-level
// const declarations.
const { mockResetMutate, mocks, venueFixture } = vi.hoisted(() => {
  type MutateOpts = {
    onSuccess?: () => void
    onError?: (err: unknown) => void
  }
  const mocks = {
    lastResetMutateOpts: null as MutateOpts | null,
  }
  const mockResetMutate = vi.fn(
    (_vars: { venueId: number }, opts?: MutateOpts) => {
      mocks.lastResetMutateOpts = opts ?? null
    }
  )
  const venueFixture: PipelineVenueInfo = {
    venue_id: 42,
    venue_name: 'The Rebel Lounge',
    venue_slug: 'the-rebel-lounge',
    preferred_source: 'ai',
    render_method: 'static',
    events_expected: 0,
    consecutive_failures: 0,
    strategy_locked: false,
    auto_approve: false,
    total_runs: 0,
  }
  return { mockResetMutate, mocks, venueFixture }
})

vi.mock('@/lib/hooks/usePipeline', () => ({
  usePipelineVenues: () => ({
    data: { venues: [venueFixture], total: 1 },
    isLoading: false,
    error: null,
  }),
  useVenueRejectionStats: () => ({ data: null, isLoading: false }),
  useVenueExtractionRuns: () => ({
    data: { runs: [], total: 0 },
    isLoading: false,
  }),
  useImportHistory: () => ({
    data: { imports: [], total: 0 },
    isLoading: false,
    error: null,
  }),
  useUpdateVenueConfig: () => ({
    mutate: vi.fn(),
    isPending: false,
    error: null,
  }),
  useExtractVenue: () => ({
    mutate: vi.fn(),
    isPending: false,
    data: undefined,
    error: null,
  }),
  useResetRenderMethod: () => ({
    mutate: mockResetMutate,
    isPending: false,
  }),
}))

vi.mock('@/features/venues', () => ({
  useVenueSearch: () => ({ data: { venues: [] } }),
}))

import { PipelineVenues } from './PipelineVenues'

describe('PipelineVenues — resetRenderMethod error banner', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mocks.lastResetMutateOpts = null
  })

  async function openVenueDetail(user: ReturnType<typeof userEvent.setup>) {
    // Expand the venue row to reveal the detail panel with the Reset button.
    await user.click(screen.getByText('The Rebel Lounge'))
  }

  it('surfaces an inline error banner when the reset mutation fails, then clears it on the next successful reset', async () => {
    const user = userEvent.setup()
    const confirmSpy = vi.spyOn(window, 'confirm').mockReturnValue(true)

    render(<PipelineVenues />)
    await openVenueDetail(user)

    // First reset — fails.
    await user.click(screen.getByRole('button', { name: 'Reset' }))

    expect(mockResetMutate).toHaveBeenCalledWith(
      { venueId: 42 },
      expect.objectContaining({
        onError: expect.any(Function),
        onSuccess: expect.any(Function),
      })
    )
    expect(screen.queryByTestId('reset-render-method-error')).not.toBeInTheDocument()

    // Simulate the mutation rejecting.
    mocks.lastResetMutateOpts?.onError?.(new Error('Server exploded'))
    const banner = await screen.findByTestId('reset-render-method-error')
    expect(banner).toHaveTextContent('Server exploded')

    // Second reset — succeeds. The banner should disappear.
    await user.click(screen.getByRole('button', { name: 'Reset' }))
    mocks.lastResetMutateOpts?.onSuccess?.()

    expect(screen.queryByTestId('reset-render-method-error')).not.toBeInTheDocument()

    confirmSpy.mockRestore()
  })

  it('falls back to a generic message when the error is not an Error instance', async () => {
    const user = userEvent.setup()
    const confirmSpy = vi.spyOn(window, 'confirm').mockReturnValue(true)

    render(<PipelineVenues />)
    await openVenueDetail(user)

    await user.click(screen.getByRole('button', { name: 'Reset' }))
    mocks.lastResetMutateOpts?.onError?.('string failure mode')

    const banner = await screen.findByTestId('reset-render-method-error')
    expect(banner).toHaveTextContent('Failed to reset render method')

    confirmSpy.mockRestore()
  })

  it('does not fire the mutation when the user cancels the confirm dialog', async () => {
    const user = userEvent.setup()
    const confirmSpy = vi.spyOn(window, 'confirm').mockReturnValue(false)

    render(<PipelineVenues />)
    await openVenueDetail(user)

    await user.click(screen.getByRole('button', { name: 'Reset' }))

    expect(mockResetMutate).not.toHaveBeenCalled()
    expect(screen.queryByTestId('reset-render-method-error')).not.toBeInTheDocument()

    confirmSpy.mockRestore()
  })
})
