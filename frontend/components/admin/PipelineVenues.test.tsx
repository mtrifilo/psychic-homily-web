import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, act } from '@testing-library/react'
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
const {
  mockResetMutate,
  mockUpdateConfigMutate,
  mockExtractMutate,
  mockUseVenueSearch,
  mockUsePipelineVenues,
  mockUseImportHistory,
  mockUseVenueRejectionStats,
  mockUseVenueExtractionRuns,
  mockUseUpdateVenueConfig,
  mocks,
  venueFixture,
  runFixture,
  importFixture,
} = vi.hoisted(() => {
  type MutateOpts = {
    onSuccess?: () => void
    onError?: (err: unknown) => void
  }
  const mocks = {
    lastResetMutateOpts: null as MutateOpts | null,
    lastUpdateConfigMutateOpts: null as MutateOpts | null,
    lastUpdateConfigVars: null as
      | { venueId: number; config: Record<string, unknown> }
      | null,
    updateConfigPending: false,
    updateConfigError: null as unknown,
  }
  const mockResetMutate = vi.fn(
    (_vars: { venueId: number }, opts?: MutateOpts) => {
      mocks.lastResetMutateOpts = opts ?? null
    }
  )
  const mockUpdateConfigMutate = vi.fn(
    (
      vars: { venueId: number; config: Record<string, unknown> },
      opts?: MutateOpts
    ) => {
      mocks.lastUpdateConfigVars = vars
      mocks.lastUpdateConfigMutateOpts = opts ?? null
    }
  )
  const mockExtractMutate = vi.fn()
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
  const mockUseVenueSearch = vi.fn()
  const mockUsePipelineVenues = vi.fn()
  const mockUseImportHistory = vi.fn()
  const mockUseVenueRejectionStats = vi.fn()
  const mockUseVenueExtractionRuns = vi.fn()
  const mockUseUpdateVenueConfig = vi.fn()
  return {
    mockResetMutate,
    mockUpdateConfigMutate,
    mockExtractMutate,
    mockUseVenueSearch,
    mockUsePipelineVenues,
    mockUseImportHistory,
    mockUseVenueRejectionStats,
    mockUseVenueExtractionRuns,
    mockUseUpdateVenueConfig,
    mocks,
    venueFixture,
    runFixture,
    importFixture,
  }
})

vi.mock('@/lib/hooks/usePipeline', () => ({
  usePipelineVenues: () => mockUsePipelineVenues(),
  useVenueRejectionStats: () => mockUseVenueRejectionStats(),
  useVenueExtractionRuns: () => mockUseVenueExtractionRuns(),
  useImportHistory: (limit?: number, offset?: number) =>
    mockUseImportHistory(limit, offset),
  useUpdateVenueConfig: () => mockUseUpdateVenueConfig(),
  useExtractVenue: () => ({
    mutate: mockExtractMutate,
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
  useVenueSearch: (args: { query: string }) => mockUseVenueSearch(args),
}))

import { PipelineVenues } from './PipelineVenues'

beforeEach(() => {
  vi.clearAllMocks()
  mocks.lastResetMutateOpts = null
  mocks.lastUpdateConfigMutateOpts = null
  mocks.lastUpdateConfigVars = null
  mocks.updateConfigPending = false
  mocks.updateConfigError = null
  // Defaults that mirror the original test fixtures.
  mockUsePipelineVenues.mockReturnValue({
    data: { venues: [venueFixture], total: 1 },
    isLoading: false,
    error: null,
  })
  mockUseImportHistory.mockReturnValue({
    data: { imports: [importFixture], total: 1 },
    isLoading: false,
    error: null,
  })
  mockUseVenueRejectionStats.mockReturnValue({
    data: null,
    isLoading: false,
  })
  mockUseVenueExtractionRuns.mockReturnValue({
    data: { runs: [runFixture], total: 1 },
    isLoading: false,
  })
  mockUseVenueSearch.mockReturnValue({ data: { venues: [] } })
  mockUseUpdateVenueConfig.mockReturnValue({
    mutate: mockUpdateConfigMutate,
    get isPending() {
      return mocks.updateConfigPending
    },
    get error() {
      return mocks.updateConfigError
    },
  })
})

describe('PipelineVenues — resetRenderMethod error banner', () => {
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

describe('PipelineVenues — venue selection', () => {
  it('opens the detail panel when a venue row is clicked', async () => {
    const user = userEvent.setup()
    render(<PipelineVenues />)
    // Sanity: the Configuration heading lives only inside the detail panel.
    expect(
      screen.queryByRole('heading', { name: 'Configuration' })
    ).not.toBeInTheDocument()

    await user.click(screen.getByText('The Rebel Lounge'))

    expect(
      screen.getByRole('heading', { name: 'Configuration' })
    ).toBeInTheDocument()
  })

  it('closes the detail panel when the same venue row is clicked again', async () => {
    const user = userEvent.setup()
    render(<PipelineVenues />)
    // Click the venue name cell to open. The detail panel header also renders
    // the venue name, so pin the second click to the row's <tr> instead.
    const row = screen.getByText('The Rebel Lounge').closest('tr')!
    await user.click(row)
    expect(
      screen.getByRole('heading', { name: 'Configuration' })
    ).toBeInTheDocument()

    await user.click(row)
    expect(
      screen.queryByRole('heading', { name: 'Configuration' })
    ).not.toBeInTheDocument()
  })

  it('closes the detail panel via the explicit Close button', async () => {
    const user = userEvent.setup()
    render(<PipelineVenues />)
    await user.click(screen.getByText('The Rebel Lounge'))
    expect(
      screen.getByRole('heading', { name: 'Configuration' })
    ).toBeInTheDocument()

    // The detail panel's own Close button is the first Close in the DOM.
    const closeBtn = screen.getAllByRole('button', { name: 'Close' })[0]
    await user.click(closeBtn)

    expect(
      screen.queryByRole('heading', { name: 'Configuration' })
    ).not.toBeInTheDocument()
  })

  it('renders the empty-state message when no venues are configured', () => {
    mockUsePipelineVenues.mockReturnValue({
      data: { venues: [], total: 0 },
      isLoading: false,
      error: null,
    })
    render(<PipelineVenues />)
    expect(screen.getByText('No venues configured')).toBeInTheDocument()
  })

  it('renders a loading state while the venues list is fetching', () => {
    mockUsePipelineVenues.mockReturnValue({
      data: undefined,
      isLoading: true,
      error: null,
    })
    render(<PipelineVenues />)
    expect(screen.getByText('Loading pipeline venues...')).toBeInTheDocument()
  })

  it('renders an error state when the venues list fetch fails', () => {
    mockUsePipelineVenues.mockReturnValue({
      data: undefined,
      isLoading: false,
      error: new Error('boom'),
    })
    render(<PipelineVenues />)
    expect(screen.getByText('Failed to load pipeline venues')).toBeInTheDocument()
  })
})

describe('PipelineVenues — Configure Venue (add) dialog', () => {
  it('toggles the add-venue dialog open and closed via the header button', async () => {
    const user = userEvent.setup()
    render(<PipelineVenues />)
    // Closed by default.
    expect(
      screen.queryByRole('heading', { name: 'Configure New Venue' })
    ).not.toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: 'Configure Venue' }))
    expect(
      screen.getByRole('heading', { name: 'Configure New Venue' })
    ).toBeInTheDocument()

    // Now the header button reads "Cancel" — clicking it closes the dialog.
    await user.click(screen.getByRole('button', { name: 'Cancel' }))
    expect(
      screen.queryByRole('heading', { name: 'Configure New Venue' })
    ).not.toBeInTheDocument()
  })

  it('renders matching venue search results filtered against the existing list', async () => {
    const user = userEvent.setup()
    // venueFixture.venue_id = 42 — that one should NOT appear in results.
    mockUseVenueSearch.mockReturnValue({
      data: {
        venues: [
          { id: 42, name: 'The Rebel Lounge', city: 'Phoenix', state: 'AZ' },
          { id: 99, name: 'Crescent Ballroom', city: 'Phoenix', state: 'AZ' },
        ],
      },
    })
    render(<PipelineVenues />)
    await user.click(screen.getByRole('button', { name: 'Configure Venue' }))
    await user.type(screen.getByPlaceholderText('Search venues...'), 'phoenix')

    // The already-configured venue is filtered out.
    expect(
      screen.getByRole('button', { name: /Crescent Ballroom/ })
    ).toBeInTheDocument()
    // Specifically no Crescent confusion — the dialog only surfaces the new venue
    // as a clickable button. (The pre-existing one still appears in the venue
    // table; it just isn't a button in the results list.)
    const dialogHeading = screen.getByRole('heading', {
      name: 'Configure New Venue',
    })
    const dialog = dialogHeading.closest('div')!.parentElement!
    expect(dialog.textContent).toContain('Crescent Ballroom')
    expect(dialog.textContent).not.toContain('The Rebel Lounge')
  })

  it('renders the no-match copy when search returns nothing matching', async () => {
    const user = userEvent.setup()
    mockUseVenueSearch.mockReturnValue({ data: { venues: [] } })
    render(<PipelineVenues />)
    await user.click(screen.getByRole('button', { name: 'Configure Venue' }))
    await user.type(screen.getByPlaceholderText('Search venues...'), 'xyz')

    expect(screen.getByText('No matching venues found')).toBeInTheDocument()
  })

  it('fires the create-config mutation when a venue is selected', async () => {
    const user = userEvent.setup()
    mockUseVenueSearch.mockReturnValue({
      data: {
        venues: [
          { id: 99, name: 'Crescent Ballroom', city: 'Phoenix', state: 'AZ' },
        ],
      },
    })
    render(<PipelineVenues />)
    await user.click(screen.getByRole('button', { name: 'Configure Venue' }))
    await user.type(screen.getByPlaceholderText('Search venues...'), 'crescent')
    await user.click(
      screen.getByRole('button', { name: /Crescent Ballroom/ })
    )

    expect(mockUpdateConfigMutate).toHaveBeenCalledWith(
      expect.objectContaining({
        venueId: 99,
        config: expect.objectContaining({
          preferred_source: 'ai',
          auto_approve: false,
          strategy_locked: false,
        }),
      }),
      expect.objectContaining({ onSuccess: expect.any(Function) })
    )
  })
})

describe('PipelineVenues — config edit form', () => {
  async function openConfigForm(user: ReturnType<typeof userEvent.setup>) {
    await user.click(screen.getByText('The Rebel Lounge'))
    await user.click(screen.getByRole('button', { name: 'Edit' }))
  }

  it('reveals the edit form when the Edit button is clicked', async () => {
    const user = userEvent.setup()
    render(<PipelineVenues />)
    await user.click(screen.getByText('The Rebel Lounge'))
    expect(
      screen.queryByRole('heading', { name: 'Edit Configuration' })
    ).not.toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: 'Edit' }))

    expect(
      screen.getByRole('heading', { name: 'Edit Configuration' })
    ).toBeInTheDocument()
  })

  it('hides the edit form when Cancel is clicked without invoking the mutation', async () => {
    const user = userEvent.setup()
    render(<PipelineVenues />)
    await openConfigForm(user)

    await user.click(screen.getByRole('button', { name: 'Cancel' }))

    expect(
      screen.queryByRole('heading', { name: 'Edit Configuration' })
    ).not.toBeInTheDocument()
    expect(mockUpdateConfigMutate).not.toHaveBeenCalled()
  })

  it('fires the update mutation with edited config values when Save is clicked', async () => {
    const user = userEvent.setup()
    render(<PipelineVenues />)
    await openConfigForm(user)

    // Type into the Calendar URL field, change Preferred Source to "ical", and Save.
    const calendarInput = screen.getByPlaceholderText('https://venue.com/events')
    await user.clear(calendarInput)
    await user.type(calendarInput, 'https://example.com/calendar')

    // The selects are unlabeled (label is a sibling div, not an htmlFor link),
    // so query by role and pick the first combobox (Preferred Source).
    const allSelects = screen.getAllByRole('combobox')
    await user.selectOptions(allSelects[0] as HTMLSelectElement, 'ical')

    await user.click(screen.getByRole('button', { name: 'Save' }))

    expect(mockUpdateConfigMutate).toHaveBeenCalledTimes(1)
    expect(mocks.lastUpdateConfigVars?.venueId).toBe(42)
    expect(mocks.lastUpdateConfigVars?.config).toMatchObject({
      calendar_url: 'https://example.com/calendar',
      preferred_source: 'ical',
    })
  })

  it('closes the edit form on the next successful save', async () => {
    const user = userEvent.setup()
    render(<PipelineVenues />)
    await openConfigForm(user)

    await user.click(screen.getByRole('button', { name: 'Save' }))
    // The component wired an onSuccess that flips isEditingConfig back to false.
    act(() => {
      mocks.lastUpdateConfigMutateOpts?.onSuccess?.()
    })

    expect(
      screen.queryByRole('heading', { name: 'Edit Configuration' })
    ).not.toBeInTheDocument()
  })

  it('renders the Save button as "Saving..." and disables it while the mutation is pending', async () => {
    const user = userEvent.setup()
    mocks.updateConfigPending = true
    render(<PipelineVenues />)
    await openConfigForm(user)

    const savingBtn = screen.getByRole('button', { name: 'Saving...' })
    expect(savingBtn).toBeDisabled()
  })

  it('surfaces the save-mutation error message under the form when save fails', async () => {
    const user = userEvent.setup()
    mocks.updateConfigError = new Error('Validation failed')
    render(<PipelineVenues />)
    await openConfigForm(user)

    expect(screen.getByText('Validation failed')).toBeInTheDocument()
  })
})

describe('PipelineVenues — import history pagination', () => {
  // Build a paginated fixture: page 1 = items 1–20, total 45, so next is enabled.
  function makeImport(id: number): ImportHistoryEntry {
    return {
      id,
      venue_id: 42,
      venue_name: `Venue ${id}`,
      venue_slug: `venue-${id}`,
      source_type: 'ai',
      run_at: '2026-01-15T19:30:00Z',
      events_extracted: 1,
      events_imported: 1,
      duration_ms: 100,
    }
  }

  it('renders a pagination summary + Next enabled when more pages exist', async () => {
    const user = userEvent.setup()
    mockUseImportHistory.mockReturnValue({
      data: {
        imports: Array.from({ length: 20 }, (_, i) => makeImport(i + 1)),
        total: 45,
      },
      isLoading: false,
      error: null,
    })
    render(<PipelineVenues />)
    await user.click(screen.getByRole('button', { name: 'Import History' }))

    expect(screen.getByText('Showing 1–20 of 45')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Next' })).toBeEnabled()
    expect(screen.getByRole('button', { name: 'Previous' })).toBeDisabled()
  })

  it('does NOT render pagination controls when the page fits in one screen', async () => {
    const user = userEvent.setup()
    // Default fixture has 1 of 1 — both prev and next are unavailable, so the
    // pagination row is skipped entirely.
    render(<PipelineVenues />)
    await user.click(screen.getByRole('button', { name: 'Import History' }))

    expect(
      screen.queryByRole('button', { name: 'Next' })
    ).not.toBeInTheDocument()
    expect(
      screen.queryByRole('button', { name: 'Previous' })
    ).not.toBeInTheDocument()
  })

  it('advances offset on Next and refetches the next page', async () => {
    const user = userEvent.setup()
    mockUseImportHistory.mockReturnValue({
      data: {
        imports: Array.from({ length: 20 }, (_, i) => makeImport(i + 1)),
        total: 45,
      },
      isLoading: false,
      error: null,
    })
    render(<PipelineVenues />)
    await user.click(screen.getByRole('button', { name: 'Import History' }))

    // First render: useImportHistory called with offset=0.
    const firstCallArgs = mockUseImportHistory.mock.calls[0]
    expect(firstCallArgs).toEqual([20, 0])

    await user.click(screen.getByRole('button', { name: 'Next' }))

    // After the offset bump, the hook is re-invoked with offset=20.
    const lastCallArgs =
      mockUseImportHistory.mock.calls[mockUseImportHistory.mock.calls.length - 1]
    expect(lastCallArgs).toEqual([20, 20])
  })

  it('rewinds offset on Previous and disables Previous at the first page', async () => {
    const user = userEvent.setup()
    // Stage a list large enough that Next is available; after one Next, the
    // component re-renders. We then click Previous — the hook should be called
    // with offset 0 again, and the Previous button should disable.
    mockUseImportHistory.mockReturnValue({
      data: {
        imports: Array.from({ length: 20 }, (_, i) => makeImport(i + 1)),
        total: 45,
      },
      isLoading: false,
      error: null,
    })
    render(<PipelineVenues />)
    await user.click(screen.getByRole('button', { name: 'Import History' }))
    await user.click(screen.getByRole('button', { name: 'Next' }))
    // Sanity: now Previous is enabled.
    expect(screen.getByRole('button', { name: 'Previous' })).toBeEnabled()

    await user.click(screen.getByRole('button', { name: 'Previous' }))

    const lastCallArgs =
      mockUseImportHistory.mock.calls[mockUseImportHistory.mock.calls.length - 1]
    expect(lastCallArgs).toEqual([20, 0])
    expect(screen.getByRole('button', { name: 'Previous' })).toBeDisabled()
  })

  it('renders the import-history empty state when no runs have been recorded', async () => {
    const user = userEvent.setup()
    mockUseImportHistory.mockReturnValue({
      data: { imports: [], total: 0 },
      isLoading: false,
      error: null,
    })
    render(<PipelineVenues />)
    await user.click(screen.getByRole('button', { name: 'Import History' }))

    expect(
      screen.getByText('No extraction runs recorded yet.')
    ).toBeInTheDocument()
  })

  it('renders an import-history error message when the fetch fails', async () => {
    const user = userEvent.setup()
    mockUseImportHistory.mockReturnValue({
      data: undefined,
      isLoading: false,
      error: new Error('boom'),
    })
    render(<PipelineVenues />)
    await user.click(screen.getByRole('button', { name: 'Import History' }))

    expect(
      screen.getByText('Failed to load import history')
    ).toBeInTheDocument()
  })
})
