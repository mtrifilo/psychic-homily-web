import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { act, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { renderWithProviders } from '@/test/utils'
import type { ExtractedShowData } from '@/lib/types/extraction'

// Regression guard for the artists list React-key contract: when keyed on
// array index, removing a middle row caused React to reuse DOM/component
// state for the wrong artist. These tests mount the real ShowForm with the
// dependencies that touch the network or auth stubbed out, then drive the
// add → remove-middle path through user-event and assert each remaining
// input still holds the state that belongs to its logical row (not the row
// that lived at its old index).

vi.mock('next/navigation', () => ({
  useRouter: () => ({ push: vi.fn(), replace: vi.fn(), back: vi.fn() }),
}))

vi.mock('@/lib/context/AuthContext', () => ({
  useAuthContext: () => ({ user: { id: 1, is_admin: false }, isAuthenticated: true, isLoading: false }),
}))

vi.mock('@/features/shows', () => ({
  useShowSubmit: () => ({ mutate: vi.fn(), isPending: false, error: null, reset: vi.fn() }),
  useShowUpdate: () => ({ mutate: vi.fn(), isPending: false, error: null, reset: vi.fn() }),
}))

// Return at least one search result so ArtistInput's dropdown can transition
// to aria-expanded="true". The component gates aria-expanded on
// `showDropdown && filteredArtists.length > 0`, so an empty result would
// suppress the canary state the test relies on.
vi.mock('@/features/artists', () => ({
  useArtistSearch: () => ({
    data: { artists: [{ id: 999, name: 'Match', city: 'Phoenix', state: 'AZ' }] },
    isLoading: false,
  }),
  getArtistLocation: () => '',
}))

vi.mock('@/features/venues', () => ({
  useVenueSearch: () => ({ data: undefined, isLoading: false }),
  getVenueLocation: () => '',
}))

// Imported AFTER mocks so the component picks up the stubbed modules.
import { ShowForm } from './ShowForm'

describe('ShowForm — artists list stable keys', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('removing the middle artist keeps each remaining row\'s component state with its row (not its index)', async () => {
    const user = userEvent.setup()
    renderWithProviders(<ShowForm mode="create" />)

    // Start with 1, add two more → 3 artist rows.
    const addButton = screen.getByRole('button', { name: /add another artist/i })
    await user.click(addButton)
    await user.click(addButton)

    const getInputs = () =>
      screen.getAllByPlaceholderText('Enter artist name') as HTMLInputElement[]
    expect(getInputs()).toHaveLength(3)

    // Type into rows 0 and 2 with values that visibly differ. Row 1 stays
    // empty so the bug — if present — surfaces as the third row's local
    // ArtistInput state (typed value + aria-expanded dropdown state) leaking
    // onto the new second slot after the middle row is removed.
    await user.type(getInputs()[0], 'Artist A')
    await user.type(getInputs()[2], 'Artist C')

    // ArtistInput.tsx opens its autocomplete listbox the moment the input has
    // any value (see isOpen / showDropdown). aria-expanded reflects the local
    // useState in that specific component instance, which is exactly what
    // leaks across rows when React reuses an instance via a stale key.
    expect(getInputs()[0]).toHaveAttribute('aria-expanded', 'true')
    expect(getInputs()[1]).toHaveAttribute('aria-expanded', 'false')
    expect(getInputs()[2]).toHaveAttribute('aria-expanded', 'true')

    // Remove the empty middle row.
    const removeButtons = screen.getAllByRole('button', { name: /remove artist/i })
    expect(removeButtons).toHaveLength(3)
    await user.click(removeButtons[1])

    const remaining = getInputs()
    expect(remaining).toHaveLength(2)

    // The form value at each row index reflects the new array — both with
    // and without the bug, because that's controlled by TanStack form state,
    // not React keys.
    expect(remaining[0].value).toBe('Artist A')
    expect(remaining[1].value).toBe('Artist C')

    // The dropdown state is the canary: with key={index}, React would reuse
    // the second-row ArtistInput instance for the new second row (which is
    // semantically the old third row). The new second row would display
    // "Artist C" but its internal state (aria-expanded) would belong to the
    // removed empty row, surfacing as aria-expanded="false".
    expect(remaining[1]).toHaveAttribute('aria-expanded', 'true')
  })
})

// Regression guard for the AI-extraction effect's requestAnimationFrame
// cleanup. The effect defers form.setFieldValue calls to the next animation
// frame to batch state updates. Without cancelAnimationFrame in the cleanup,
// unmounting the form between the schedule and the callback let setFieldValue
// run against an unmounted form — producing a React dev warning (and a
// double-schedule under StrictMode). These tests drive that exact ordering by
// controlling rAF deterministically: capture the scheduled callback, unmount,
// then assert the frame was cancelled and that running the stale callback
// afterward neither warns nor writes extracted values into the DOM.
describe('ShowForm — AI extraction rAF cleanup', () => {
  const extraction: ExtractedShowData = {
    artists: [{ name: 'Extracted Artist', is_headliner: true }],
    venue: { name: 'Extracted Venue', city: 'Phoenix', state: 'AZ' },
    date: '2099-12-31',
    time: '20:00',
    cost: '$10',
    ages: '21+',
    description: 'from the flyer',
  }

  let rafCallbacks: Map<number, FrameRequestCallback>
  let cancelled: number[]
  let nextRafId: number

  beforeEach(() => {
    vi.clearAllMocks()
    rafCallbacks = new Map()
    cancelled = []
    nextRafId = 0

    // Capture rAF callbacks instead of running them, so the test controls
    // whether the deferred setFieldValue work happens before or after unmount.
    vi.spyOn(window, 'requestAnimationFrame').mockImplementation(cb => {
      const id = ++nextRafId
      rafCallbacks.set(id, cb)
      return id
    })
    vi.spyOn(window, 'cancelAnimationFrame').mockImplementation(id => {
      cancelled.push(id)
      rafCallbacks.delete(id)
    })
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('cancels the pending extraction frame on unmount before it fires', () => {
    const { unmount } = renderWithProviders(
      <ShowForm mode="create" initialExtraction={extraction} />
    )

    // The effect scheduled exactly one frame and has not run it yet.
    expect(rafCallbacks.size).toBe(1)
    const [scheduledId] = [...rafCallbacks.keys()]

    // Unmount before the frame fires — the cleanup must cancel it.
    unmount()

    expect(cancelled).toContain(scheduledId)
    expect(rafCallbacks.has(scheduledId)).toBe(false)
  })

  it('does not warn or write extracted values when a stale frame runs after unmount', () => {
    const errorSpy = vi.spyOn(console, 'error').mockImplementation(() => {})

    const { unmount } = renderWithProviders(
      <ShowForm mode="create" initialExtraction={extraction} />
    )
    const staleCallback = [...rafCallbacks.values()][0]

    unmount()

    // Force the captured frame to run post-unmount, simulating a frame the
    // browser had already queued slipping through. This proves the deferred
    // setFieldValue work cannot warn even if a frame leaks past the cleanup.
    act(() => {
      staleCallback(performance.now())
    })

    expect(errorSpy).not.toHaveBeenCalled()
    // Nothing extracted should be rendered after unmount.
    expect(screen.queryByDisplayValue('Extracted Artist')).toBeNull()
    expect(screen.queryByDisplayValue('Extracted Venue')).toBeNull()
  })
})
