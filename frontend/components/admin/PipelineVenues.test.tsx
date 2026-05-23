import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import type {
  PipelineVenueInfo,
  ImportHistoryEntry,
  VenueExtractionRun,
} from '@/lib/hooks/usePipeline'
import { formatShortDate, formatTimestamp } from '@/lib/utils/formatters'

// `vi.hoisted` lets us share these handles between the hoisted vi.mock factory
// and the test bodies below. Anything referenced inside `vi.mock` factories
// MUST live in here, since `vi.mock` calls are hoisted above module-level
// const declarations.
const { mockResetMutate, mocks, venueFixture, runFixture, importFixture } =
  vi.hoisted(() => {
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
  const runFixture: VenueExtractionRun = {
    id: 7,
    venue_id: 42,
    run_at: '2026-01-15T19:30:00Z',
    render_method: 'static',
    events_extracted: 12,
    events_imported: 10,
    duration_ms: 4200,
    created_at: '2026-01-15T19:30:00Z',
  }
  const venueFixture: PipelineVenueInfo = {
    venue_id: 42,
    venue_name: 'The Rebel Lounge',
    venue_slug: 'the-rebel-lounge',
    preferred_source: 'ai',
    render_method: 'static',
    last_extracted_at: '2026-01-15T19:30:00Z',
    events_expected: 0,
    consecutive_failures: 0,
    strategy_locked: false,
    auto_approve: false,
    total_runs: 1,
    last_run: runFixture,
  }
  const importFixture: ImportHistoryEntry = {
    id: 99,
    venue_id: 42,
    venue_name: 'The Rebel Lounge',
    venue_slug: 'the-rebel-lounge',
    source_type: 'ai',
    run_at: '2026-01-15T19:30:00Z',
    events_extracted: 12,
    events_imported: 10,
    duration_ms: 4200,
  }
  return { mockResetMutate, mocks, venueFixture, runFixture, importFixture }
})

vi.mock('@/lib/hooks/usePipeline', () => ({
  usePipelineVenues: () => ({
    data: { venues: [venueFixture], total: 1 },
    isLoading: false,
    error: null as Error | null,
  }),
  useVenueRejectionStats: () => ({ data: null as unknown, isLoading: false }),
  useVenueExtractionRuns: () => ({
    data: { runs: [runFixture], total: 1 },
    isLoading: false,
  }),
  useImportHistory: () => ({
    data: { imports: [importFixture], total: 1 },
    isLoading: false,
    error: null as Error | null,
  }),
  useUpdateVenueConfig: () => ({
    mutate: vi.fn(),
    isPending: false,
    error: null as Error | null,
  }),
  useExtractVenue: () => ({
    mutate: vi.fn(),
    isPending: false,
    data: undefined as unknown,
    error: null as Error | null,
  }),
  useResetRenderMethod: () => ({
    mutate: mockResetMutate,
    isPending: false,
  }),
}))

vi.mock('@/features/venues', () => ({
  useVenueSearch: () => ({ data: { venues: [] as unknown[] } }),
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

describe('PipelineVenues — explicit-locale timestamp formatting', () => {
  // Timestamps must render through the canonical explicit-locale formatters
  // (formatShortDate / formatTimestamp), not bare toLocaleString() whose output
  // drifts with the viewer's browser/OS locale. Asserting against the helper's
  // own output keeps these stable across ICU versions while still proving the
  // component delegates to the canonical formatter rather than inlining a call.
  const ISO = '2026-01-15T19:30:00Z'

  it('renders the venue Last Run date via formatShortDate (date only, no time)', () => {
    render(<PipelineVenues />)
    const expected = formatShortDate(ISO)
    expect(screen.getByText(`12 events, ${expected}`)).toBeInTheDocument()
    expect(expected).not.toMatch(/\d{1,2}:\d{2}/)
  })

  it('renders the Last extracted date in the venue detail panel via formatShortDate', async () => {
    const user = userEvent.setup()
    render(<PipelineVenues />)
    await user.click(screen.getByText('The Rebel Lounge'))

    expect(screen.getByText('Last extracted:')).toBeInTheDocument()
    // formatShortDate output appears in both the table row and the detail panel.
    const expected = formatShortDate(ISO)
    expect(screen.getAllByText(expected, { exact: false }).length).toBeGreaterThan(0)
  })

  it('renders Import History timestamps via formatTimestamp (date + time)', async () => {
    const user = userEvent.setup()
    render(<PipelineVenues />)
    await user.click(screen.getByRole('button', { name: 'Import History' }))

    const expected = formatTimestamp(ISO)
    expect(screen.getByText(expected)).toBeInTheDocument()
    expect(expected).toMatch(/\d{1,2}:\d{2}/)
  })
})
