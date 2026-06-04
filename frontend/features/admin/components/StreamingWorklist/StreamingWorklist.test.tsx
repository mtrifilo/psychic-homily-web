/**
 * @vitest-environment jsdom
 */
import React from 'react'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { screen, fireEvent, waitFor, act } from '@testing-library/react'
import { renderWithProviders } from '@/test/utils'

import type {
  StreamingWorklistEntry,
  StreamingWorklistResult,
} from './types'

const mockUseStreamingWorklist = vi.fn()
const mockMutate = vi.fn()
const mockMutationResult = vi.fn()

vi.mock('./useStreamingWorklist', () => ({
  useStreamingWorklist: (params: unknown) => mockUseStreamingWorklist(params),
  useUpdateStreamingDiscoveryStatus: () => mockMutationResult(),
}))

import { StreamingWorklist } from './StreamingWorklist'

function makeEntry(overrides: Partial<StreamingWorklistEntry> = {}): StreamingWorklistEntry {
  return {
    artist_id: 101,
    artist_name: 'Faetooth',
    artist_slug: 'faetooth',
    streaming_discovery_status: 'unreviewed',
    soonest_event_date: '2026-06-01T03:00:00Z',
    venue_name: 'Valley Bar',
    venue_city: 'Phoenix',
    upcoming_show_count: 1,
    ...overrides,
  }
}

function mockWorklistData(data: StreamingWorklistResult | null, opts: Partial<{ isLoading: boolean; isError: boolean; error: Error | null }> = {}) {
  mockUseStreamingWorklist.mockReturnValue({
    data,
    isLoading: opts.isLoading ?? false,
    isError: opts.isError ?? false,
    error: opts.error ?? null,
  })
}

function defaultMutationStubs() {
  mockMutationResult.mockReturnValue({
    mutate: mockMutate,
    isPending: false,
  })
}

describe('StreamingWorklist', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    defaultMutationStubs()
  })

  it('renders the empty state when no rows', () => {
    mockWorklistData({ entries: [], total: 0 })

    renderWithProviders(<StreamingWorklist />)

    expect(screen.getByTestId('streaming-worklist-empty')).toBeInTheDocument()
  })

  it('renders one row per entry with name + venue + status badge', () => {
    mockWorklistData({
      entries: [
        makeEntry({ artist_id: 1, artist_name: 'Alpha', streaming_discovery_status: 'unreviewed' }),
        makeEntry({
          artist_id: 2,
          artist_name: 'Beta',
          artist_slug: 'beta',
          streaming_discovery_status: 'candidates_pending',
          venue_name: 'Crescent Ballroom',
          venue_city: 'Phoenix',
        }),
      ],
      total: 2,
    })

    renderWithProviders(<StreamingWorklist />)

    expect(screen.getByTestId('streaming-worklist-row-1')).toBeInTheDocument()
    expect(screen.getByTestId('streaming-worklist-row-2')).toBeInTheDocument()
    expect(screen.getByText('Alpha')).toBeInTheDocument()
    expect(screen.getByText('Beta')).toBeInTheDocument()
    expect(
      screen.getByTestId('streaming-worklist-status-unreviewed')
    ).toBeInTheDocument()
    expect(
      screen.getByTestId('streaming-worklist-status-candidates_pending')
    ).toBeInTheDocument()
    expect(screen.getByText('Crescent Ballroom · Phoenix')).toBeInTheDocument()
  })

  it('points the Review button at the artist detail page (new tab)', () => {
    mockWorklistData({
      entries: [makeEntry({ artist_id: 42, artist_slug: 'faetooth' })],
      total: 1,
    })

    renderWithProviders(<StreamingWorklist />)

    // Button uses `asChild` so the data-testid lands on the rendered
    // <a> element itself (Slot prop-merging), not on a wrapping div.
    const reviewLink = screen.getByTestId('streaming-worklist-review-42')
    expect(reviewLink.tagName).toBe('A')
    expect(reviewLink).toHaveAttribute('href', '/artists/faetooth')
    expect(reviewLink).toHaveAttribute('target', '_blank')
  })

  it('falls back to the artist ID when no slug is present', () => {
    mockWorklistData({
      entries: [makeEntry({ artist_id: 42, artist_slug: null })],
      total: 1,
    })

    renderWithProviders(<StreamingWorklist />)

    const reviewLink = screen.getByTestId('streaming-worklist-review-42')
    expect(reviewLink).toHaveAttribute('href', '/artists/42')
  })

  it('shows the load-error banner when the query fails', () => {
    mockWorklistData(null, { isError: true, error: new Error('boom') })

    renderWithProviders(<StreamingWorklist />)

    expect(
      screen.getByTestId('streaming-worklist-load-error')
    ).toBeInTheDocument()
    expect(screen.getByText('boom')).toBeInTheDocument()
  })

  it('forwards the status filter to the hook and resets offset on change', () => {
    mockWorklistData({ entries: [], total: 0 })

    renderWithProviders(<StreamingWorklist />)

    // Initial call — empty filter, offset 0.
    expect(mockUseStreamingWorklist).toHaveBeenCalledWith(
      expect.objectContaining({ status: '', offset: 0 })
    )

    const select = screen.getByTestId('streaming-worklist-status-filter') as HTMLSelectElement
    fireEvent.change(select, { target: { value: 'unreviewed' } })

    // After the change the hook is re-invoked with the new filter.
    expect(mockUseStreamingWorklist).toHaveBeenLastCalledWith(
      expect.objectContaining({ status: 'unreviewed', offset: 0 })
    )
  })

  it('shows action buttons by default and opens the inline reason form', () => {
    mockWorklistData({
      entries: [makeEntry({ artist_id: 7 })],
      total: 1,
    })

    renderWithProviders(<StreamingWorklist />)

    const openSkipped = screen.getByTestId('streaming-worklist-open-skipped-7')
    fireEvent.click(openSkipped)

    expect(
      screen.getByTestId('streaming-worklist-action-form-7-skipped')
    ).toBeInTheDocument()
  })

  it('submits the status mutation with the entered reason', async () => {
    mockWorklistData({
      entries: [makeEntry({ artist_id: 7 })],
      total: 1,
    })

    renderWithProviders(<StreamingWorklist />)

    fireEvent.click(screen.getByTestId('streaming-worklist-open-skipped-7'))

    const textarea = screen.getByPlaceholderText(
      /Optional: why skipped/i
    ) as HTMLTextAreaElement
    fireEvent.change(textarea, { target: { value: 'Same-name collision' } })

    // mutate calls onSuccess so the form closes and the parent banner
    // appears.
    mockMutate.mockImplementation((_input, callbacks) => {
      callbacks?.onSuccess?.()
    })

    fireEvent.click(screen.getByTestId('streaming-worklist-submit-7-skipped'))

    expect(mockMutate).toHaveBeenCalledWith(
      {
        artist_id: 7,
        status: 'skipped',
        reason: 'Same-name collision',
      },
      expect.objectContaining({
        onSuccess: expect.any(Function),
        onError: expect.any(Function),
      })
    )

    // Success banner surfaces after the mutation lands.
    await waitFor(() => {
      expect(
        screen.getByTestId('streaming-worklist-success-banner')
      ).toBeInTheDocument()
    })
  })

  // PSY-958: each success must get its own fresh 4s window. The success banner
  // stays mounted with a constant dismissAfterMs, so it relies on the
  // `key={recentMutation.nonce}` remount to re-arm — deleting the key/nonce
  // would silently regress this (a 2nd success would inherit the 1st's
  // countdown). This guards that consumer-side contract.
  it('re-arms the success banner window on a second success (key/nonce remount)', () => {
    vi.useFakeTimers()
    try {
      mockWorklistData({ entries: [makeEntry({ artist_id: 7 })], total: 1 })
      mockMutate.mockImplementation((_input, callbacks) => {
        callbacks?.onSuccess?.()
      })

      renderWithProviders(<StreamingWorklist />)

      const submitSkipped = () => {
        fireEvent.click(screen.getByTestId('streaming-worklist-open-skipped-7'))
        fireEvent.click(screen.getByTestId('streaming-worklist-submit-7-skipped'))
      }

      // First success → banner shows (4s window).
      submitSkipped()
      expect(
        screen.getByTestId('streaming-worklist-success-banner')
      ).toBeInTheDocument()

      // 3s into the first window.
      act(() => {
        vi.advanceTimersByTime(3000)
      })
      expect(
        screen.getByTestId('streaming-worklist-success-banner')
      ).toBeInTheDocument()

      // Second success → nonce bumps → key remount → fresh 4s window.
      submitSkipped()

      // Past the FIRST window's original 4s deadline (3000+1500), but only
      // 1500ms into the SECOND window — still visible because it re-armed.
      act(() => {
        vi.advanceTimersByTime(1500)
      })
      expect(
        screen.getByTestId('streaming-worklist-success-banner')
      ).toBeInTheDocument()

      // A full 4s after the second success → dismisses.
      act(() => {
        vi.advanceTimersByTime(2500)
      })
      expect(
        screen.queryByTestId('streaming-worklist-success-banner')
      ).not.toBeInTheDocument()
    } finally {
      vi.runOnlyPendingTimers()
      vi.useRealTimers()
    }
  })

  it('passes null reason when the textarea is left empty', () => {
    mockWorklistData({
      entries: [makeEntry({ artist_id: 7 })],
      total: 1,
    })

    renderWithProviders(<StreamingWorklist />)

    fireEvent.click(screen.getByTestId('streaming-worklist-open-linked-7'))
    fireEvent.click(screen.getByTestId('streaming-worklist-submit-7-linked'))

    expect(mockMutate).toHaveBeenCalledWith(
      {
        artist_id: 7,
        status: 'linked',
        reason: null,
      },
      expect.objectContaining({ onSuccess: expect.any(Function) })
    )
  })

  it('renders pagination controls only when total exceeds limit', () => {
    mockWorklistData({
      entries: [makeEntry({ artist_id: 1 })],
      total: 100,
    })

    renderWithProviders(<StreamingWorklist />)

    expect(screen.getByTestId('streaming-worklist-prev')).toBeInTheDocument()
    expect(screen.getByTestId('streaming-worklist-next')).toBeInTheDocument()
  })

  it('hides pagination when total fits in one page', () => {
    mockWorklistData({
      entries: [makeEntry({ artist_id: 1 })],
      total: 1,
    })

    renderWithProviders(<StreamingWorklist />)

    expect(screen.queryByTestId('streaming-worklist-prev')).toBeNull()
    expect(screen.queryByTestId('streaming-worklist-next')).toBeNull()
  })
})
