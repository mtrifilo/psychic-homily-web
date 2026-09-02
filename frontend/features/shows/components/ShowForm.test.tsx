import { describe, it, expect, vi, beforeEach } from 'vitest'
import { screen, waitFor, fireEvent, act } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { renderWithProviders } from '@/test/utils'
import type { ExtractedShowData } from '@/lib/types/extraction'
import type { ShowResponse } from '../types'
import { SET_TYPE_OPTIONS } from './show-form-utils'

// ─────────────────────────────────────────────────────────────
// Shared mock state
//
// All mocks are configurable per-test via these module-scoped objects.
// Tests mutate the relevant fields in beforeEach / inside it() blocks
// BEFORE rendering ShowForm; the mocks read them at call time so each
// test gets the wiring it needs without duplicating the vi.mock setup.
// ─────────────────────────────────────────────────────────────

const mockRouter = {
  push: vi.fn(),
  replace: vi.fn(),
  back: vi.fn(),
}

const mockAuth = {
  user: { id: 1, is_admin: false } as { id: number; is_admin: boolean } | null,
  isAuthenticated: true,
  isLoading: false,
}

type MutateFn = (vars: unknown, opts?: {
  onSuccess?: (data: unknown) => void
  onError?: (err: Error) => void
}) => void

const mockShowSubmit = {
  mutate: vi.fn() as MutateFn & ReturnType<typeof vi.fn>,
  isPending: false,
  error: null as Error | null,
  reset: vi.fn(),
}

const mockShowUpdate = {
  mutate: vi.fn() as MutateFn & ReturnType<typeof vi.fn>,
  isPending: false,
  error: null as Error | null,
  reset: vi.fn(),
}

interface MockVenue {
  id: number
  slug: string
  name: string
  address: string | null
  city: string
  state: string
  /** Geocoded IANA zone; drives event_date anchoring (PSY-1873). */
  timezone?: string | null
  verified: boolean
}

const mockVenueSearch: { venues: MockVenue[] } = { venues: [] }

vi.mock('next/navigation', () => ({
  useRouter: () => mockRouter,
}))

vi.mock('@/lib/context/AuthContext', () => ({
  useAuthContext: () => mockAuth,
}))

vi.mock('../hooks/useShowSubmit', () => ({
  useShowSubmit: () => mockShowSubmit,
}))

vi.mock('../hooks/useShowUpdate', () => ({
  useShowUpdate: () => mockShowUpdate,
}))

// Return at least one search result so ArtistInput's dropdown can transition
// to aria-expanded="true". The component gates aria-expanded on
// `showDropdown && filteredArtists.length > 0`, so an empty result would
// suppress the canary state the PSY-724 test relies on.
vi.mock('@/features/artists/hooks/useArtistSearch', () => ({
  useArtistSearch: () => ({
    data: { artists: [{ id: 999, name: 'Match', city: 'Phoenix', state: 'AZ' }] },
    isLoading: false,
  }),
}))

vi.mock('@/features/artists/types', async importOriginal => ({
  ...(await importOriginal<typeof import('@/features/artists/types')>()),
  getArtistLocation: () => '',
}))

vi.mock('@/features/venues/hooks/useVenueSearch', () => ({
  useVenueSearch: () => ({ data: { venues: mockVenueSearch.venues }, isLoading: false }),
}))

vi.mock('@/features/venues/types', async importOriginal => ({
  ...(await importOriginal<typeof import('@/features/venues/types')>()),
  getVenueLocation: () => '',
}))

// Imported AFTER mocks so the component picks up the stubbed modules.
import { ShowForm } from './ShowForm'

// ─────────────────────────────────────────────────────────────
// Helpers
// ─────────────────────────────────────────────────────────────

/**
 * Reset shared mock state between tests. Each test starts from the
 * canonical "non-admin, no pending mutations, empty venue search" baseline.
 * vi.clearAllMocks() resets the spy call history; this resets the data
 * the spies read at call time.
 */
function resetMockState() {
  mockAuth.user = { id: 1, is_admin: false }
  mockAuth.isAuthenticated = true
  mockShowSubmit.isPending = false
  mockShowSubmit.error = null
  mockShowSubmit.mutate.mockReset()
  mockShowUpdate.isPending = false
  mockShowUpdate.error = null
  mockShowUpdate.mutate.mockReset()
  mockVenueSearch.venues = []
}

/**
 * Build a future date string (YYYY-MM-DD) one year out, to dodge the
 * "date cannot be in the past" zod refinement without hardcoding a
 * historical date that will rot under vi.useRealTimers() (the PSY-859
 * anti-pattern called out in the ticket).
 */
function futureDate(): string {
  const d = new Date()
  d.setFullYear(d.getFullYear() + 1)
  return d.toISOString().slice(0, 10) // YYYY-MM-DD
}

/**
 * Build a past date string (one year ago) for date-validation tests.
 * Same anti-rot reasoning as futureDate().
 */
function pastDate(): string {
  const d = new Date()
  d.setFullYear(d.getFullYear() - 1)
  return d.toISOString().slice(0, 10)
}

function makeShow(overrides: Partial<ShowResponse> = {}): ShowResponse {
  return {
    id: 42,
    slug: 'edit-me',
    title: 'Edit Me Show',
    event_date: '2099-06-15T03:00:00Z', // far-future to skip past-date validator on re-submit
    city: 'Phoenix',
    state: 'AZ',
    price: 25,
    age_requirement: '21+',
    description: 'A pre-existing show.',
    image_url: 'https://example.com/flyer.jpg',
    ticket_url: null,
    status: 'approved',
    submitted_by: 1,
    rejection_reason: null,
    rejection_category: null,
    is_sold_out: false,
    is_cancelled: false,
    venues: [
      {
        id: 5,
        slug: 'valley-bar',
        name: 'Valley Bar',
        address: '130 N Central Ave',
        city: 'Phoenix',
        state: 'AZ',
        verified: true,
      },
    ],
    artists: [
      {
        id: 11,
        slug: 'the-mountain-goats',
        name: 'The Mountain Goats',
        is_headliner: true,
        set_type: 'headliner',
        position: 1,
        socials: {},
      },
    ],
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z',
    ...overrides,
  }
}

// ─────────────────────────────────────────────────────────────
// Regression guards (PSY-724 stable keys)
//
// These were the original tests. They use the shared mock state defined
// above. Kept in their own describe blocks so the file's intent is clear
// at a scan: regressions first, behavioral coverage second.
// ─────────────────────────────────────────────────────────────

describe('ShowForm — artists list stable keys', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    resetMockState()
  })

  it('removing the middle artist keeps each remaining row\'s component state with its row (not its index)', () => {
    // PSY-1006 flake fix: drive this test with fireEvent, NOT userEvent.
    // userEvent moves real focus between rows; ArtistInput.handleBlur schedules
    // setTimeout(close, 150ms) on blur, and that timer raced the synchronous
    // aria-expanded assertions below — under CI load the 150ms elapsed first,
    // the dropdown closed (aria-expanded → "false"), and the test failed
    // intermittently. fireEvent doesn't manage focus, so no blur fires, no
    // close timer is scheduled, and the open state stays deterministic. The
    // test only exercises onChange-driven open state + key stability, which
    // fireEvent covers fully.
    renderWithProviders(<ShowForm mode="create" />)

    // Start with 1, add two more → 3 artist rows.
    const addButton = screen.getByRole('button', { name: /add another artist/i })
    fireEvent.click(addButton)
    fireEvent.click(addButton)

    const getInputs = () =>
      screen.getAllByPlaceholderText('Enter artist name') as HTMLInputElement[]
    expect(getInputs()).toHaveLength(3)

    // Set rows 0 and 2 to distinct values (opens their dropdowns). Row 1 stays
    // empty so the bug — if present — surfaces as the third row's local
    // ArtistInput state (typed value + aria-expanded dropdown state) leaking
    // onto the new second slot after the middle row is removed.
    fireEvent.change(getInputs()[0], { target: { value: 'Artist A' } })
    fireEvent.change(getInputs()[2], { target: { value: 'Artist C' } })

    // ArtistInput.tsx opens its autocomplete listbox the moment the input has
    // any value (see isOpen / showDropdown). aria-expanded reflects the local
    // useState in that specific component instance, which is exactly what
    // leaks across rows when React reuses an instance via a stale key.
    //
    // These three assertions are a load-bearing PRECONDITION guard, not just
    // description: they prove the per-row open state genuinely diverged before
    // removal. Don't delete them — without this guard the canary at the end of
    // the test could false-pass if the open-on-change wiring ever broke.
    expect(getInputs()[0]).toHaveAttribute('aria-expanded', 'true')
    expect(getInputs()[1]).toHaveAttribute('aria-expanded', 'false')
    expect(getInputs()[2]).toHaveAttribute('aria-expanded', 'true')

    // Remove the empty middle row.
    const removeButtons = screen.getAllByRole('button', { name: /remove artist/i })
    expect(removeButtons).toHaveLength(3)
    fireEvent.click(removeButtons[1])

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

// ─────────────────────────────────────────────────────────────
// PSY-693 coverage: behavioral surface
//
// The PSY-724 test above proves the stable-keys regression cannot return.
// The blocks below cover the load-bearing user flows that, if broken,
// would silently block show submissions:
//   - All required fields render in create mode
//   - Artists list add/remove updates the visible row count
//   - Venue select auto-fills city/state; verified venues lock the fields
//   - Past-date validation blocks submit + surfaces a message
//   - Successful submit calls useShowSubmit.mutate; onSuccess wires through
//   - AI extraction seeds the form via defaultValues at mount; a new
//     extraction re-seeds it on key-remount (PSY-795)
//   - Edit mode pre-fills from `initialData`
//   - The "do not publish" private toggle is create-only (hidden in edit)
// ─────────────────────────────────────────────────────────────

describe('ShowForm — required fields render in create mode', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    resetMockState()
  })

  it('renders artist, venue, date, time, cost, ages, and description fields', () => {
    renderWithProviders(<ShowForm mode="create" />)

    // Artist field starts at one row (defaultFormValues).
    expect(screen.getByPlaceholderText('Enter artist name')).toBeInTheDocument()
    // Venue input (combobox role) is present.
    expect(screen.getByLabelText(/^Venue$/i)).toBeInTheDocument()
    // City + State live in the venue grid.
    expect(screen.getByLabelText(/^City$/i)).toBeInTheDocument()
    expect(screen.getByLabelText(/^State$/i)).toBeInTheDocument()
    // Date + Time in the date grid.
    expect(screen.getByLabelText(/^Date$/i)).toBeInTheDocument()
    expect(screen.getByLabelText(/^Time$/i)).toBeInTheDocument()
    // Cost + Door Cost + Ages + Description in additional details. The cost
    // queries are anchored because "Door Cost" also matches a bare /cost/.
    expect(screen.getByLabelText(/^cost \(optional\)$/i)).toBeInTheDocument()
    expect(screen.getByLabelText(/^door cost \(optional\)$/i)).toBeInTheDocument()
    expect(screen.getByLabelText(/ages/i)).toBeInTheDocument()
    expect(screen.getByLabelText(/description/i)).toBeInTheDocument()
    // Submit button uses create-mode copy.
    expect(screen.getByRole('button', { name: /submit show/i })).toBeInTheDocument()
  })

  it('hides the image_url field in create mode (per PSY-521 — edit-only)', () => {
    renderWithProviders(<ShowForm mode="create" />)
    expect(screen.queryByLabelText(/image url/i)).not.toBeInTheDocument()
  })
})

describe('ShowForm — add / remove artist', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    resetMockState()
  })

  it('clicking "Add another artist" appends a row; clicking remove drops it', async () => {
    const user = userEvent.setup()
    renderWithProviders(<ShowForm mode="create" />)

    expect(screen.getAllByPlaceholderText('Enter artist name')).toHaveLength(1)
    // First row has no remove button (showRemoveButton hides when length <= 1).
    expect(screen.queryByRole('button', { name: /remove artist/i })).not.toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: /add another artist/i }))
    expect(screen.getAllByPlaceholderText('Enter artist name')).toHaveLength(2)
    // Now both rows expose a remove button.
    expect(screen.getAllByRole('button', { name: /remove artist/i })).toHaveLength(2)

    await user.click(screen.getByRole('button', { name: /add another artist/i }))
    expect(screen.getAllByPlaceholderText('Enter artist name')).toHaveLength(3)

    // Remove the middle row → back to 2.
    const removeButtons = screen.getAllByRole('button', { name: /remove artist/i })
    await user.click(removeButtons[1])
    expect(screen.getAllByPlaceholderText('Enter artist name')).toHaveLength(2)
  })
})

describe('ShowForm — venue selection auto-fill + verified-venue lock', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    resetMockState()
  })

  it('selecting an unverified venue from the dropdown auto-fills city/state and leaves them editable', async () => {
    mockVenueSearch.venues = [
      {
        id: 7,
        slug: 'unverified-spot',
        name: 'Unverified Spot',
        address: '99 Test Rd',
        city: 'Tucson',
        state: 'AZ',
        verified: false,
      },
    ]
    const user = userEvent.setup()
    renderWithProviders(<ShowForm mode="create" />)

    const venueInput = screen.getByLabelText(/^Venue$/i)
    // Type to open the dropdown — VenueInput gates the dropdown on
    // searchValue.length > 0 (handleInputChange → setIsOpen(value.length > 0)).
    await user.type(venueInput, 'Unverified')

    // Click the existing-venue option. data-testid is set on VenueInput's
    // option button; this is the most stable selector for the dropdown items.
    const option = await screen.findByTestId('search-result-venue')
    // Use mouseDown because VenueInput.handleVenueSelect runs on onMouseDown
    // (so a click event isn't needed; mouse-down propagates synchronously).
    await user.pointer({ keys: '[MouseLeft>]', target: option })

    // city/state are populated, but remain editable for a non-admin selecting
    // an unverified venue (isVenueLocationEditable: !verified → true).
    const city = screen.getByLabelText(/^City$/i) as HTMLInputElement
    const state = screen.getByLabelText(/^State$/i) as HTMLInputElement
    await waitFor(() => expect(city.value).toBe('Tucson'))
    expect(state.value).toBe('AZ')
    expect(city).not.toBeDisabled()
    expect(state).not.toBeDisabled()
  })

  it('selecting a verified venue locks city/state/address for non-admins', async () => {
    mockVenueSearch.venues = [
      {
        id: 9,
        slug: 'verified-spot',
        name: 'Verified Spot',
        address: '1 Verified Way',
        city: 'Phoenix',
        state: 'AZ',
        verified: true,
      },
    ]
    const user = userEvent.setup()
    renderWithProviders(<ShowForm mode="create" />)

    await user.type(screen.getByLabelText(/^Venue$/i), 'Verified')
    const option = await screen.findByTestId('search-result-venue')
    await user.pointer({ keys: '[MouseLeft>]', target: option })

    const city = screen.getByLabelText(/^City$/i) as HTMLInputElement
    const state = screen.getByLabelText(/^State$/i) as HTMLInputElement
    const address = screen.getByLabelText(/^Address/i) as HTMLInputElement

    await waitFor(() => expect(city.value).toBe('Phoenix'))
    expect(state.value).toBe('AZ')
    // address from the venue is filled in too
    expect(address.value).toBe('1 Verified Way')

    // Verified venue → fields are locked for non-admin
    // (computeVenueEditable returns false when selectedVenue.verified is true
    // and the user is not admin).
    expect(city).toBeDisabled()
    expect(state).toBeDisabled()
    expect(address).toBeDisabled()

    // And the "Verified Venue" admin-info banner appears.
    expect(screen.getByText(/Verified Venue/i)).toBeInTheDocument()
  })
})

describe('ShowForm — date validation', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    resetMockState()
  })

  it('blocks submit and surfaces "Date cannot be in the past" when a past date is selected', async () => {
    const user = userEvent.setup()
    renderWithProviders(<ShowForm mode="create" />)

    // Fill the required fields with a past date.
    await user.type(screen.getByPlaceholderText('Enter artist name'), 'Some Artist')
    await user.type(screen.getByLabelText(/^Venue$/i), 'A Venue')
    await user.type(screen.getByLabelText(/^City$/i), 'Phoenix')
    await user.type(screen.getByLabelText(/^State$/i), 'AZ')
    // Date input — user.type is unreliable on HTML5 date inputs in jsdom,
    // so set the value via the native setter + input event (see fireSet).
    fireSet(screen.getByLabelText(/^Date$/i) as HTMLInputElement, pastDate())

    await user.click(screen.getByRole('button', { name: /submit show/i }))

    // Zod refinement message bubbles into FieldInfo.
    expect(
      await screen.findByText(/Date cannot be in the past/i)
    ).toBeInTheDocument()
    // And the submit mutation was never invoked.
    expect(mockShowSubmit.mutate).not.toHaveBeenCalled()
  })
})

// Direct value-set helper — jsdom's HTML5 date input + user.type combination
// is fiddly enough that the explicit DOM mutation is the most stable path.
// React's onChange handler still fires via the input event dispatched here.
function fireSet(input: HTMLInputElement, value: string) {
  const setter = Object.getOwnPropertyDescriptor(
    window.HTMLInputElement.prototype,
    'value'
  )?.set
  setter?.call(input, value)
  input.dispatchEvent(new Event('input', { bubbles: true }))
  input.dispatchEvent(new Event('change', { bubbles: true }))
}

describe('ShowForm — successful submit', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    resetMockState()
  })

  it('invokes useShowSubmit.mutate with the assembled submission and fires onSuccess', async () => {
    const onSuccess = vi.fn()
    // The form calls onSuccess after a 1500ms timeout, OR redirects via
    // router.push. We pass redirectOnCreate={false} so the onSuccess
    // path is the one we exercise.
    mockShowSubmit.mutate.mockImplementation((_vars, opts) => {
      opts?.onSuccess?.({ status: 'approved' })
    })

    const user = userEvent.setup()
    renderWithProviders(
      <ShowForm mode="create" onSuccess={onSuccess} redirectOnCreate={false} />
    )

    await user.type(screen.getByPlaceholderText('Enter artist name'), 'Headliner Band')
    await user.type(screen.getByLabelText(/^Venue$/i), 'Some Venue')
    await user.type(screen.getByLabelText(/^City$/i), 'Phoenix')
    await user.type(screen.getByLabelText(/^State$/i), 'AZ')
    fireSet(screen.getByLabelText(/^Date$/i) as HTMLInputElement, futureDate())

    await user.click(screen.getByRole('button', { name: /submit show/i }))

    await waitFor(() => expect(mockShowSubmit.mutate).toHaveBeenCalledTimes(1))

    const submission = mockShowSubmit.mutate.mock.calls[0][0] as {
      artists: Array<{ name: string; is_headliner: boolean }>
      venues: Array<{ name: string; city: string; state: string }>
      city: string
      state: string
    }
    expect(submission.city).toBe('Phoenix')
    expect(submission.state).toBe('AZ')
    expect(submission.venues[0].name).toBe('Some Venue')
    expect(submission.artists[0].name).toBe('Headliner Band')
    // First artist defaults to is_headliner=true (defaultFormValues).
    expect(submission.artists[0].is_headliner).toBe(true)

    // Success branch fires onSuccess after 1500ms timeout.
    await waitFor(() => expect(onSuccess).toHaveBeenCalledTimes(1), {
      timeout: 2500,
    })
  })

  // PSY-1864: the door price is a separate fact from the advance price. The
  // submission must carry both when both are typed, and must leave door_price
  // absent (NOT copied from the cost field) when the door field is blank.
  it('submits the advance and door prices as independent fields', async () => {
    mockShowSubmit.mutate.mockImplementation((_vars, opts) => {
      opts?.onSuccess?.({ status: 'approved' })
    })

    const user = userEvent.setup()
    renderWithProviders(<ShowForm mode="create" redirectOnCreate={false} />)

    await user.type(screen.getByPlaceholderText('Enter artist name'), 'Headliner Band')
    await user.type(screen.getByLabelText(/^Venue$/i), 'Some Venue')
    await user.type(screen.getByLabelText(/^City$/i), 'Phoenix')
    await user.type(screen.getByLabelText(/^State$/i), 'AZ')
    fireSet(screen.getByLabelText(/^Date$/i) as HTMLInputElement, futureDate())
    await user.type(screen.getByLabelText(/^cost \(optional\)$/i), '$35')
    await user.type(screen.getByLabelText(/^door cost \(optional\)$/i), '$40')

    await user.click(screen.getByRole('button', { name: /submit show/i }))

    await waitFor(() => expect(mockShowSubmit.mutate).toHaveBeenCalledTimes(1))
    const submission = mockShowSubmit.mutate.mock.calls[0][0] as {
      price?: number
      door_price?: number
    }
    expect(submission.price).toBe(35)
    expect(submission.door_price).toBe(40)
  })

  it('leaves door_price absent when only the cost field is filled', async () => {
    mockShowSubmit.mutate.mockImplementation((_vars, opts) => {
      opts?.onSuccess?.({ status: 'approved' })
    })

    const user = userEvent.setup()
    renderWithProviders(<ShowForm mode="create" redirectOnCreate={false} />)

    await user.type(screen.getByPlaceholderText('Enter artist name'), 'Headliner Band')
    await user.type(screen.getByLabelText(/^Venue$/i), 'Some Venue')
    await user.type(screen.getByLabelText(/^City$/i), 'Phoenix')
    await user.type(screen.getByLabelText(/^State$/i), 'AZ')
    fireSet(screen.getByLabelText(/^Date$/i) as HTMLInputElement, futureDate())
    await user.type(screen.getByLabelText(/^cost \(optional\)$/i), '$20')

    await user.click(screen.getByRole('button', { name: /submit show/i }))

    await waitFor(() => expect(mockShowSubmit.mutate).toHaveBeenCalledTimes(1))
    const submission = mockShowSubmit.mutate.mock.calls[0][0] as {
      price?: number
      door_price?: number
    }
    expect(submission.price).toBe(20)
    expect(submission.door_price).toBeUndefined()
  })

  // PSY-1873: the selected venue's own IANA zone anchors event_date. Keying on
  // the state alone ran "England" through the US state map, which answers
  // America/Phoenix, so an 8pm Leeds show was submitted as 03:00Z the NEXT day
  // and the show page, which renders in Europe/London, printed 4:00 AM.
  //
  // January deliberately: London is on GMT then, so the expected instant does
  // not depend on any projected future DST rule.
  it("anchors event_date in the selected venue's timezone, not the state map", async () => {
    mockVenueSearch.venues = [
      {
        id: 160,
        slug: 'boom-leeds-leeds-england',
        name: 'Boom Leeds',
        address: '5 Canal Pl',
        city: 'Leeds',
        state: 'England',
        timezone: 'Europe/London',
        verified: true,
      },
    ]
    mockShowSubmit.mutate.mockImplementation((_vars, opts) => {
      opts?.onSuccess?.({ status: 'approved' })
    })

    const user = userEvent.setup()
    renderWithProviders(<ShowForm mode="create" redirectOnCreate={false} />)

    await user.type(
      screen.getByPlaceholderText('Enter artist name'),
      'Din of Celestial Birds'
    )
    await user.type(screen.getByLabelText(/^Venue$/i), 'Boom')
    const option = await screen.findByTestId('search-result-venue')
    await user.pointer({ keys: '[MouseLeft>]', target: option })
    await waitFor(() =>
      expect((screen.getByLabelText(/^City$/i) as HTMLInputElement).value).toBe(
        'Leeds'
      )
    )
    fireSet(screen.getByLabelText(/^Date$/i) as HTMLInputElement, '2099-01-15')

    await user.click(screen.getByRole('button', { name: /submit show/i }))
    await waitFor(() => expect(mockShowSubmit.mutate).toHaveBeenCalledTimes(1))

    const submission = mockShowSubmit.mutate.mock.calls[0][0] as {
      event_date: string
    }
    // 20:00 GMT, not 20:00 Phoenix (which would be 2099-01-16T03:00:00Z).
    expect(submission.event_date).toBe('2099-01-15T20:00:00Z')
  })

  it('routes private submissions to the Contribute console with its dialog trigger', async () => {
    mockShowSubmit.mutate.mockImplementation((_vars, opts) => {
      opts?.onSuccess?.({ status: 'private' })
    })
    const user = userEvent.setup()
    renderWithProviders(<ShowForm mode="create" />)

    await user.type(screen.getByPlaceholderText('Enter artist name'), 'Private Band')
    await user.type(screen.getByLabelText(/^Venue$/i), 'Private Venue')
    await user.type(screen.getByLabelText(/^City$/i), 'Phoenix')
    await user.type(screen.getByLabelText(/^State$/i), 'AZ')
    fireSet(screen.getByLabelText(/^Date$/i) as HTMLInputElement, futureDate())
    await user.click(screen.getByRole('button', { name: /submit show/i }))

    await waitFor(() => {
      expect(mockRouter.push).toHaveBeenCalledWith(
        '/contribute/submissions?submitted=private'
      )
    })
  })

  // PSY-1664: the post-submit success flash used to defer `onSuccess` behind a
  // bare `setTimeout`, so it still fired after the form unmounted and called
  // into a parent that was already gone. No timer may survive unmount.
  it('leaves no pending success timer behind on unmount', async () => {
    mockShowSubmit.mutate.mockImplementation((_vars, opts) => {
      opts?.onSuccess?.({ status: 'approved' })
    })

    vi.useFakeTimers()
    try {
      const onSuccess = vi.fn()
      // redirectOnCreate={false} picks the 1500ms onSuccess path over the
      // 2000ms router redirect.
      const { unmount } = renderWithProviders(
        <ShowForm mode="create" onSuccess={onSuccess} redirectOnCreate={false} />
      )

      fireEvent.change(screen.getByPlaceholderText('Enter artist name'), {
        target: { value: 'Headliner Band' },
      })
      fireEvent.change(screen.getByLabelText(/^Venue$/i), {
        target: { value: 'Some Venue' },
      })
      fireEvent.change(screen.getByLabelText(/^City$/i), {
        target: { value: 'Phoenix' },
      })
      fireEvent.change(screen.getByLabelText(/^State$/i), {
        target: { value: 'AZ' },
      })
      fireSet(screen.getByLabelText(/^Date$/i) as HTMLInputElement, futureDate())

      const baseline = vi.getTimerCount()

      // TanStack Form's submit is async, so the mutation (and the timer it
      // arms) only lands after the promise continuation runs.
      await act(async () => {
        fireEvent.click(screen.getByRole('button', { name: /submit show/i }))
      })

      expect(mockShowSubmit.mutate).toHaveBeenCalledTimes(1)
      expect(vi.getTimerCount()).toBeGreaterThan(baseline)

      unmount()
      expect(vi.getTimerCount()).toBe(0)

      // Well past the 1500ms delay: the callback must never land after the
      // form is gone.
      await vi.advanceTimersByTimeAsync(5000)
      expect(onSuccess).not.toHaveBeenCalled()
    } finally {
      vi.useRealTimers()
    }
  })

  it('routes approved submissions to the Contribute console after success feedback', async () => {
    mockShowSubmit.mutate.mockImplementation((_vars, opts) => {
      opts?.onSuccess?.({ status: 'approved' })
    })
    const user = userEvent.setup()
    renderWithProviders(<ShowForm mode="create" />)

    await user.type(screen.getByPlaceholderText('Enter artist name'), 'Approved Band')
    await user.type(screen.getByLabelText(/^Venue$/i), 'Approved Venue')
    await user.type(screen.getByLabelText(/^City$/i), 'Phoenix')
    await user.type(screen.getByLabelText(/^State$/i), 'AZ')
    fireSet(screen.getByLabelText(/^Date$/i) as HTMLInputElement, futureDate())
    await user.click(screen.getByRole('button', { name: /submit show/i }))

    await waitFor(() => {
      expect(mockRouter.push).toHaveBeenCalledWith('/contribute/submissions')
    }, { timeout: 2500 })
  })
})

// PSY-795: AI extraction is now folded into TanStack Form's `defaultValues`
// at mount (calculate-during-render via mergeExtraction), replacing the old
// prop-derived useEffect + rAF + lastAppliedExtraction ref. The parent
// (app/shows/submit/page.tsx) remounts ShowForm via a `key` it bumps on each
// extraction; these tests model that contract:
//   1. An extraction passed at mount seeds every field.
//   2. A NEW extraction with a fresh `key` re-seeds the form (remount).
//   3. Without a key change, defaultValues are read once — a re-render that
//      keeps the same key does NOT clobber a user edit.
describe('ShowForm — AI extraction seeds defaultValues + key-remount re-seed', () => {
  const extraction: ExtractedShowData = {
    artists: [{ name: 'AI Artist', is_headliner: true }],
    venue: { name: 'AI Venue', city: 'Tempe', state: 'AZ' },
    date: '2099-09-09',
    time: '21:30',
    cost: '$20',
    ages: 'All Ages',
    description: 'AI flyer description',
  }

  beforeEach(() => {
    vi.clearAllMocks()
    resetMockState()
  })

  it('seeds the extracted artist, venue, date, time, cost, ages, and description into the form at mount', () => {
    renderWithProviders(
      <ShowForm key={0} mode="create" initialExtraction={extraction} />
    )

    // defaultValues are read synchronously at mount — no rAF / waitFor needed.
    expect(
      (screen.getByPlaceholderText('Enter artist name') as HTMLInputElement).value
    ).toBe('AI Artist')
    expect((screen.getByLabelText(/^Venue$/i) as HTMLInputElement).value).toBe('AI Venue')
    expect((screen.getByLabelText(/^City$/i) as HTMLInputElement).value).toBe('Tempe')
    expect((screen.getByLabelText(/^State$/i) as HTMLInputElement).value).toBe('AZ')
    expect((screen.getByLabelText(/^Date$/i) as HTMLInputElement).value).toBe('2099-09-09')
    expect((screen.getByLabelText(/^Time$/i) as HTMLInputElement).value).toBe('21:30')
    expect(
      (screen.getByLabelText(/^cost \(optional\)$/i) as HTMLInputElement).value
    ).toBe('$20')
    expect((screen.getByLabelText(/ages/i) as HTMLInputElement).value).toBe('All Ages')
    expect((screen.getByLabelText(/description/i) as HTMLTextAreaElement).value).toBe(
      'AI flyer description'
    )
  })

  it('re-seeds the form when a new extraction arrives with a changed key (remount)', async () => {
    const { rerender } = renderWithProviders(
      <ShowForm key={0} mode="create" initialExtraction={extraction} />
    )

    expect(
      (screen.getByPlaceholderText('Enter artist name') as HTMLInputElement).value
    ).toBe('AI Artist')

    // A second extraction. The parent bumps `key`, so React unmounts the old
    // form and mounts a fresh one whose defaultValues come from the NEW data.
    const secondExtraction: ExtractedShowData = {
      artists: [{ name: 'Second Artist', is_headliner: true }],
      venue: { name: 'Second Venue', city: 'Mesa', state: 'AZ' },
      date: '2099-10-10',
      time: '19:00',
      cost: 'Free',
      ages: '21+',
      description: 'second flyer',
    }

    rerender(
      <ShowForm key={1} mode="create" initialExtraction={secondExtraction} />
    )

    await waitFor(() =>
      expect(
        (screen.getByPlaceholderText('Enter artist name') as HTMLInputElement).value
      ).toBe('Second Artist')
    )
    expect((screen.getByLabelText(/^Venue$/i) as HTMLInputElement).value).toBe('Second Venue')
    expect((screen.getByLabelText(/^City$/i) as HTMLInputElement).value).toBe('Mesa')
    expect((screen.getByLabelText(/^Date$/i) as HTMLInputElement).value).toBe('2099-10-10')
    expect(
      (screen.getByLabelText(/^cost \(optional\)$/i) as HTMLInputElement).value
    ).toBe('Free')
  })

  it('does not clobber a user edit when re-rendered with the same key (defaultValues read once)', async () => {
    const user = userEvent.setup()
    const { rerender } = renderWithProviders(
      <ShowForm key={0} mode="create" initialExtraction={extraction} />
    )

    const venueInput = screen.getByLabelText(/^Venue$/i) as HTMLInputElement
    expect(venueInput.value).toBe('AI Venue')

    // User edits the seeded value.
    await user.clear(venueInput)
    await user.type(venueInput, 'User Edited Venue')
    expect(venueInput.value).toBe('User Edited Venue')

    // Re-render with the SAME key and the SAME extraction (e.g. an unrelated
    // parent re-render). Without a key change the form is not remounted, so
    // defaultValues are not re-read and the user's edit must survive.
    rerender(<ShowForm key={0} mode="create" initialExtraction={extraction} />)

    await Promise.resolve()
    expect((screen.getByLabelText(/^Venue$/i) as HTMLInputElement).value).toBe(
      'User Edited Venue'
    )
  })
})

describe('ShowForm — edit mode pre-fills from initialData', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    resetMockState()
  })

  it('populates artist, venue (locked label), city, state, ages, description, and price-as-cost from the show prop', () => {
    const show = makeShow()
    renderWithProviders(<ShowForm mode="edit" initialData={show} />)

    // Artist row carries the existing name.
    expect(
      screen.getByPlaceholderText('Enter artist name') as HTMLInputElement
    ).toHaveValue('The Mountain Goats')
    // Venue: edit mode goes through the VenueInput combobox (no prefilledVenue).
    expect(screen.getByLabelText(/^Venue$/i)).toHaveValue('Valley Bar')
    expect(screen.getByLabelText(/^City$/i)).toHaveValue('Phoenix')
    expect(screen.getByLabelText(/^State$/i)).toHaveValue('AZ')
    expect(screen.getByLabelText(/ages/i)).toHaveValue('21+')
    expect(screen.getByLabelText(/description/i)).toHaveValue('A pre-existing show.')
    // price=25 → cost field renders as "$25".
    expect(screen.getByLabelText(/^cost \(optional\)$/i)).toHaveValue('$25')
    // PSY-1864: no door price recorded, so the door field opens EMPTY rather
    // than echoing the advance price back at the editor.
    expect(screen.getByLabelText(/^door cost \(optional\)$/i)).toHaveValue('')
    // image_url is editable in edit mode (PSY-521 carve-out).
    expect(screen.getByLabelText(/image url/i)).toHaveValue('https://example.com/flyer.jpg')
    // Submit copy switches to "Save Changes".
    expect(screen.getByRole('button', { name: /save changes/i })).toBeInTheDocument()
  })

  it('renders Cancel button when onCancel is provided in edit mode', async () => {
    const onCancel = vi.fn()
    const user = userEvent.setup()
    renderWithProviders(
      <ShowForm mode="edit" initialData={makeShow()} onCancel={onCancel} />
    )

    const cancelButton = screen.getByRole('button', { name: /cancel/i })
    expect(cancelButton).toBeInTheDocument()
    await user.click(cancelButton)
    expect(onCancel).toHaveBeenCalledTimes(1)
  })
})

// How an edit names the show's venue, asserted at the SUBMIT boundary rather
// than on showToFormValues, because the payload and the composed event_date
// are what the backend acts on.
//
// The show's own venue rides on the payload as an ID, which `associateVenues`
// resolves by primary key. That is what lets the state field carry the same
// value the instant was READ in: only the (name, city, state) fallback rejects
// an empty state, and a venue with no state on file has nothing else to give.
// The id is not sticky, so an edit that names a different venue still resolves
// that venue instead of the old one.
describe('ShowForm — edit mode venue resolution + event_date round trip', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    resetMockState()
  })

  /**
   * A show whose venue has no state on file and no resolved zone, carrying a
   * US state on the show row itself. `showTimingInput` reads it in
   * America/Phoenix (the venue's '' state resolves no zone), so composing the
   * save from 'NY' instead would land the instant 3 hours away.
   */
  function makeZonelessVenueShow() {
    return makeShow({
      event_date: '2099-06-15T03:00:00Z', // 20:00 Jun 14, America/Phoenix
      city: 'Berlin',
      state: 'NY',
      venues: [
        {
          id: 77,
          slug: 'hall-ohne-zone',
          name: 'Hall Ohne Zone',
          address: null,
          city: 'Berlin',
          state: '',
          timezone: null,
          verified: true,
        },
      ],
    })
  }

  it('saves a show at a state-less venue without moving event_date', async () => {
    mockShowUpdate.mutate.mockImplementation((_vars, opts) => {
      opts?.onSuccess?.({ id: 42 })
    })
    const show = makeZonelessVenueShow()
    const user = userEvent.setup()
    renderWithProviders(<ShowForm mode="edit" initialData={show} />)

    // The precondition the payload assertions hang on: the field opens blank,
    // matching the venue rather than the show row's 'NY'.
    expect(screen.getByLabelText(/^State$/i)).toHaveValue('')

    await user.click(screen.getByRole('button', { name: /save changes/i }))

    await waitFor(() => expect(mockShowUpdate.mutate).toHaveBeenCalledTimes(1))

    const call = mockShowUpdate.mutate.mock.calls[0][0] as {
      updates: {
        event_date: string
        state?: string
        venues: Array<{ id?: number; name?: string; state?: string }>
      }
    }
    // Byte-identical: a save that changes nothing must write back the instant
    // it read. Composing from 'NY' would give 2099-06-15T00:00:00Z.
    expect(call.updates.event_date).toBe('2099-06-15T03:00:00Z')
    // The id is what keeps the blank state resolvable on the backend.
    expect(call.updates.venues[0].id).toBe(77)
    expect(call.updates.venues[0].state).toBe('')
    expect(call.updates.state).toBe('')
  })

  it("sends the show's own venue id on an unrelated edit", async () => {
    mockShowUpdate.mutate.mockImplementation((_vars, opts) => {
      opts?.onSuccess?.({ id: 42 })
    })
    const user = userEvent.setup()
    renderWithProviders(<ShowForm mode="edit" initialData={makeShow()} />)

    await user.type(screen.getByLabelText(/^show title/i), ' (rescheduled)')
    await user.click(screen.getByRole('button', { name: /save changes/i }))

    await waitFor(() => expect(mockShowUpdate.mutate).toHaveBeenCalledTimes(1))

    const call = mockShowUpdate.mutate.mock.calls[0][0] as {
      updates: { venues: Array<{ id?: number; name?: string }> }
    }
    expect(call.updates.venues[0].id).toBe(5)
    expect(call.updates.venues[0].name).toBe('Valley Bar')
  })

  it('sends the picked venue id when the venue is changed from the picker', async () => {
    mockVenueSearch.venues = [
      {
        id: 160,
        slug: 'boom-leeds-leeds-england',
        name: 'Boom Leeds',
        address: null,
        city: 'Leeds',
        state: 'England',
        timezone: 'Europe/London',
        verified: true,
      },
    ]
    mockShowUpdate.mutate.mockImplementation((_vars, opts) => {
      opts?.onSuccess?.({ id: 42 })
    })
    const user = userEvent.setup()
    renderWithProviders(<ShowForm mode="edit" initialData={makeShow()} />)

    const venueInput = screen.getByLabelText(/^Venue$/i)
    await user.clear(venueInput)
    await user.type(venueInput, 'Boom')
    await user.click(await screen.findByText('Boom Leeds'))

    await user.click(screen.getByRole('button', { name: /save changes/i }))

    await waitFor(() => expect(mockShowUpdate.mutate).toHaveBeenCalledTimes(1))

    const call = mockShowUpdate.mutate.mock.calls[0][0] as {
      updates: { venues: Array<{ id?: number; name?: string; state?: string }> }
    }
    // The picked venue, not the one the form opened on.
    expect(call.updates.venues[0].id).toBe(160)
    expect(call.updates.venues[0].name).toBe('Boom Leeds')
    expect(call.updates.venues[0].state).toBe('England')
  })

  it('drops the venue id when a brand new venue name is typed', async () => {
    mockShowUpdate.mutate.mockImplementation((_vars, opts) => {
      opts?.onSuccess?.({ id: 42 })
    })
    const user = userEvent.setup()
    // Admin so the city/state fields stay editable on a verified venue.
    mockAuth.user = { id: 1, is_admin: true }
    renderWithProviders(<ShowForm mode="edit" initialData={makeShow()} />)

    const venueInput = screen.getByLabelText(/^Venue$/i)
    await user.clear(venueInput)
    await user.type(venueInput, 'Some Brand New Room')
    await user.tab()

    await user.click(screen.getByRole('button', { name: /save changes/i }))

    await waitFor(() => expect(mockShowUpdate.mutate).toHaveBeenCalledTimes(1))

    const call = mockShowUpdate.mutate.mock.calls[0][0] as {
      updates: {
        venues: Array<{
          id?: number
          name?: string
          city?: string
          state?: string
        }>
      }
    }
    // No id, so the backend resolves this by (name, city, state) and creates
    // the room when it does not already exist.
    expect(call.updates.venues[0].id).toBeUndefined()
    expect(call.updates.venues[0].name).toBe('Some Brand New Room')
    expect(call.updates.venues[0].city).toBe('Phoenix')
    expect(call.updates.venues[0].state).toBe('AZ')
  })

  it('blocks a new venue with no state at the form instead of at the backend', async () => {
    mockShowUpdate.mutate.mockImplementation((_vars, opts) => {
      opts?.onSuccess?.({ id: 42 })
    })
    const user = userEvent.setup()
    mockAuth.user = { id: 1, is_admin: true }
    renderWithProviders(
      <ShowForm mode="edit" initialData={makeZonelessVenueShow()} />
    )

    // Typing a fresh venue name clears the id, so the payload has to describe
    // the venue by name/city/state again and the blank state is now a gap the
    // backend would reject with a generic failure.
    const venueInput = screen.getByLabelText(/^Venue$/i)
    await user.clear(venueInput)
    await user.type(venueInput, 'Another New Room')
    await user.tab()

    await user.click(screen.getByRole('button', { name: /save changes/i }))

    expect(await screen.findByText('State is required')).toBeInTheDocument()
    expect(mockShowUpdate.mutate).not.toHaveBeenCalled()
  })

  it('leaves a show with no venue on the name/city/state path', async () => {
    mockShowUpdate.mutate.mockImplementation((_vars, opts) => {
      opts?.onSuccess?.({ id: 42 })
    })
    const show = makeShow({
      event_date: '2099-01-16T01:00:00Z', // 20:00 Jan 15, America/New_York
      city: 'Brooklyn',
      state: 'NY',
      venues: [],
    })
    const user = userEvent.setup()
    mockAuth.user = { id: 1, is_admin: true }
    renderWithProviders(<ShowForm mode="edit" initialData={show} />)

    await user.type(screen.getByLabelText(/^Venue$/i), 'Union Pool')
    await user.click(screen.getByRole('button', { name: /save changes/i }))

    await waitFor(() => expect(mockShowUpdate.mutate).toHaveBeenCalledTimes(1))

    const call = mockShowUpdate.mutate.mock.calls[0][0] as {
      updates: {
        event_date: string
        venues: Array<{ id?: number; state?: string }>
      }
    }
    expect(call.updates.venues[0].id).toBeUndefined()
    // The show row's own state still resolves the zone when there is no venue
    // to prefer, so the instant round-trips.
    expect(call.updates.venues[0].state).toBe('NY')
    expect(call.updates.event_date).toBe('2099-01-16T01:00:00Z')
  })
})

describe('ShowForm — private-show toggle visibility (create vs edit)', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    resetMockState()
  })

  it('IS visible in create mode when a non-admin enters a new (unmatched) venue', async () => {
    // Positive branch: venue is non-empty AND no matched venue AND user is
    // not admin AND !isEditMode. That combination satisfies every gate the
    // private-show checkbox sits behind in ShowForm.tsx.
    const user = userEvent.setup()
    renderWithProviders(<ShowForm mode="create" />)

    // Typing into the venue input fires onVenueNameChange → setVenueName,
    // which is the load-bearing trigger for the "New Venue" branch.
    await user.type(screen.getByLabelText(/^Venue$/i), 'Brand New Spot')
    // Trigger blur — the component reads field.state.value to decide whether
    // selectedVenue should be cleared (it stays null because no match).
    await user.tab()

    expect(
      await screen.findByLabelText(/do not publish/i)
    ).toBeInTheDocument()
  })

  it('is NOT visible in edit mode even with a non-admin + new-venue-like state', async () => {
    // Negative branch: the same conditions that surface the checkbox in
    // create mode must NOT surface it in edit mode. ShowForm gates the
    // checkbox specifically on `!isEditMode`; this test pins that.
    const user = userEvent.setup()
    renderWithProviders(<ShowForm mode="edit" initialData={makeShow()} />)

    // Clear the prefilled venue text to drop the selected venue to null —
    // this puts the form into the same "new venue" state the create-mode
    // positive test relies on. If the only thing gating the toggle were
    // venue state, it would now be visible. Edit mode must still hide it.
    const venueInput = screen.getByLabelText(/^Venue$/i)
    await user.clear(venueInput)
    await user.type(venueInput, 'Different Venue')
    await user.tab()

    // Pin the precondition: the "New Venue" banner IS showing (proves we
    // reached the surrounding conditional). Without this, the absence
    // assertion below would also pass if the banner never rendered at all
    // (false pass from a setup failure rather than from the !isEditMode
    // gate firing).
    expect(await screen.findByText(/New Venue/i)).toBeInTheDocument()

    // The "New Venue" admin-info banner uses similar wording; assert
    // specifically on the toggle's own label so we're testing the
    // !isEditMode gate, not the outer banner.
    expect(screen.queryByLabelText(/do not publish/i)).not.toBeInTheDocument()
  })
})


// ─────────────────────────────────────────────────────────────
// Bill role selector (PSY-1673)
//
// set_type used to be dead below the headliner line: the backend stamped
// "opener" on every non-headliner and no surface let anyone say otherwise.
// These pin that the form is now that surface, for EVERY editor and EVERY
// value in the vocabulary.
// ─────────────────────────────────────────────────────────────

describe('ShowForm: bill role selector', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    resetMockState()
  })

  it('renders a bill role selector for every artist row', async () => {
    const user = userEvent.setup()
    renderWithProviders(<ShowForm mode="create" />)

    expect(
      screen.getByRole('combobox', { name: 'Bill role 1' })
    ).toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: /add another artist/i }))

    expect(
      await screen.findByRole('combobox', { name: 'Bill role 2' })
    ).toBeInTheDocument()
  })

  it('seeds the first act as headliner and added acts as the neutral default', async () => {
    const user = userEvent.setup()
    renderWithProviders(<ShowForm mode="create" />)

    expect(
      screen.getByRole('combobox', { name: 'Bill role 1' })
    ).toHaveTextContent('Headliner')

    await user.click(screen.getByRole('button', { name: /add another artist/i }))

    // NOT "Opener" -- nobody has said what slot this act plays.
    expect(
      await screen.findByRole('combobox', { name: 'Bill role 2' })
    ).toHaveTextContent('Performer (slot unknown)')
  })

  it('offers every vocabulary value in the selector', async () => {
    const user = userEvent.setup()
    renderWithProviders(<ShowForm mode="create" />)

    await user.click(
      screen.getByRole('combobox', { name: 'Bill role 1' })
    )

    for (const option of SET_TYPE_OPTIONS) {
      expect(
        await screen.findByRole('option', { name: option.label })
      ).toBeInTheDocument()
    }
  })

  // The acceptance bar for this ticket: the selector has to be able to write
  // each value, not merely display it. One case per value, so a regression
  // names the value it broke.
  for (const option of SET_TYPE_OPTIONS) {
    it(`submits set_type "${option.value}" when "${option.label}" is chosen`, async () => {
      mockShowSubmit.mutate.mockImplementation((_vars, opts) => {
        opts?.onSuccess?.({ status: 'approved' })
      })
      const user = userEvent.setup()
      renderWithProviders(<ShowForm mode="create" redirectOnCreate={false} />)

      await user.type(
        screen.getByPlaceholderText('Enter artist name'),
        'Role Test Band'
      )
      await user.type(screen.getByLabelText(/^Venue$/i), 'Role Venue')
      await user.type(screen.getByLabelText(/^City$/i), 'Phoenix')
      await user.type(screen.getByLabelText(/^State$/i), 'AZ')
      fireSet(screen.getByLabelText(/^Date$/i) as HTMLInputElement, futureDate())

      await user.click(
        screen.getByRole('combobox', { name: 'Bill role 1' })
      )
      await user.click(
        await screen.findByRole('option', { name: option.label })
      )

      await user.click(screen.getByRole('button', { name: /submit show/i }))

      await waitFor(() => expect(mockShowSubmit.mutate).toHaveBeenCalledTimes(1))

      const submission = mockShowSubmit.mutate.mock.calls[0][0] as {
        artists: Array<{ set_type: string; is_headliner: boolean }>
      }
      expect(submission.artists[0].set_type).toBe(option.value)
      // is_headliner is derived from set_type, never tracked beside it.
      expect(submission.artists[0].is_headliner).toBe(
        option.value === 'headliner'
      )
    })
  }

  it('pre-fills the selector from the stored role in edit mode', async () => {
    renderWithProviders(
      <ShowForm
        mode="edit"
        initialData={makeShow({
          artists: [
            {
              id: 11,
              slug: 'top-act',
              name: 'Top Act',
              is_headliner: true,
              set_type: 'headliner',
              position: 0,
              socials: {},
            },
            {
              id: 12,
              slug: 'support-act',
              name: 'Support Act',
              is_headliner: false,
              set_type: 'direct_support',
              position: 1,
              socials: {},
            },
          ],
        })}
      />
    )

    expect(
      await screen.findByRole('combobox', { name: 'Bill role 1' })
    ).toHaveTextContent('Headliner')
    expect(
      screen.getByRole('combobox', { name: 'Bill role 2' })
    ).toHaveTextContent('Direct support')
  })

  it('sends the edited role on the update payload', async () => {
    mockShowUpdate.mutate.mockImplementation((_vars, opts) => {
      opts?.onSuccess?.({ id: 42 })
    })
    const user = userEvent.setup()
    renderWithProviders(<ShowForm mode="edit" initialData={makeShow()} />)

    await user.click(
      screen.getByRole('combobox', { name: 'Bill role 1' })
    )
    await user.click(await screen.findByRole('option', { name: 'DJ' }))

    await user.click(screen.getByRole('button', { name: /save changes/i }))

    await waitFor(() => expect(mockShowUpdate.mutate).toHaveBeenCalledTimes(1))

    const call = mockShowUpdate.mutate.mock.calls[0][0] as {
      updates: { artists: Array<{ set_type: string; is_headliner: boolean }> }
    }
    expect(call.updates.artists[0].set_type).toBe('dj')
    expect(call.updates.artists[0].is_headliner).toBe(false)
  })

  // PSY-1864: editing a door price on the surface a curator actually uses.
  it('sends an edited door price on the update payload', async () => {
    mockShowUpdate.mutate.mockImplementation((_vars, opts) => {
      opts?.onSuccess?.({ id: 42 })
    })
    const user = userEvent.setup()
    renderWithProviders(
      <ShowForm mode="edit" initialData={makeShow({ price: 25, door_price: 30 })} />
    )

    const doorCost = screen.getByLabelText(/^door cost \(optional\)$/i)
    expect(doorCost).toHaveValue('$30')

    await user.clear(doorCost)
    await user.type(doorCost, '$35')
    await user.click(screen.getByRole('button', { name: /save changes/i }))

    await waitFor(() => expect(mockShowUpdate.mutate).toHaveBeenCalledTimes(1))

    const call = mockShowUpdate.mutate.mock.calls[0][0] as {
      updates: { price?: number; door_price?: number }
    }
    expect(call.updates.door_price).toBe(35)
    // The advance price rides along untouched rather than being dropped.
    expect(call.updates.price).toBe(25)
  })

  // The retraction gesture (PSY-1961). Blanking the field sends an explicit
  // null, which the API reads as "clear this column" rather than as silence.
  //
  // This test previously pinned the OPPOSITE, as a documented known limitation:
  // the key was dropped, the backend read that as "unchanged", and the edit
  // reported success while changing nothing.
  it('clears a recorded door price by blanking the field', async () => {
    mockShowUpdate.mutate.mockImplementation((_vars, opts) => {
      opts?.onSuccess?.({ id: 42 })
    })
    const user = userEvent.setup()
    renderWithProviders(
      <ShowForm mode="edit" initialData={makeShow({ price: 25, door_price: 30 })} />
    )

    await user.clear(screen.getByLabelText(/^door cost \(optional\)$/i))
    await user.click(screen.getByRole('button', { name: /save changes/i }))

    await waitFor(() => expect(mockShowUpdate.mutate).toHaveBeenCalledTimes(1))

    const call = mockShowUpdate.mutate.mock.calls[0][0] as {
      updates: { price?: number | null; door_price?: number | null }
    }
    expect(call.updates.door_price).toBeNull()
    // The advance price is a separate fact and must not be retracted with it.
    expect(call.updates.price).toBe(25)
  })

  // The advance price takes the same gesture. Asymmetry here is what the
  // mechanism was made uniform to avoid: a form where one price clears and its
  // twin silently does not is a worse surface than either consistent choice.
  it('clears the advance price by blanking the cost field', async () => {
    mockShowUpdate.mutate.mockImplementation((_vars, opts) => {
      opts?.onSuccess?.({ id: 42 })
    })
    const user = userEvent.setup()
    renderWithProviders(
      <ShowForm mode="edit" initialData={makeShow({ price: 25, door_price: 30 })} />
    )

    await user.clear(screen.getByLabelText(/^cost \(optional\)$/i))
    await user.click(screen.getByRole('button', { name: /save changes/i }))

    await waitFor(() => expect(mockShowUpdate.mutate).toHaveBeenCalledTimes(1))

    const call = mockShowUpdate.mutate.mock.calls[0][0] as {
      updates: { price?: number | null; door_price?: number | null }
    }
    expect(call.updates.price).toBeNull()
    expect(call.updates.door_price).toBe(30)
  })

  // An UNPARSEABLE cost is not a clear. "donation", "sliding scale", "TBA" and
  // a lone "$" all make parseCost return undefined, exactly as a blank field
  // does — so deciding from the parsed value alone would read "donation" as
  // "delete the price" and destroy it. The stored value stands instead.
  it.each(['donation', 'sliding scale', 'TBA', '$'])(
    'leaves a stored price alone when the cost field says %s',
    async unparseable => {
      mockShowUpdate.mutate.mockImplementation((_vars, opts) => {
        opts?.onSuccess?.({ id: 42 })
      })
      const user = userEvent.setup()
      renderWithProviders(
        <ShowForm
          mode="edit"
          initialData={makeShow({ price: 25, door_price: 30 })}
        />
      )

      const cost = screen.getByLabelText(/^cost \(optional\)$/i)
      await user.clear(cost)
      await user.type(cost, unparseable)
      await user.click(screen.getByRole('button', { name: /save changes/i }))

      await waitFor(() => expect(mockShowUpdate.mutate).toHaveBeenCalledTimes(1))

      const call = mockShowUpdate.mutate.mock.calls[0][0] as {
        updates: { price?: number | null }
      }
      expect(call.updates.price).toBeUndefined()
    }
  )

  // A blank field is only evidence of a retraction when the form was opened
  // with something to retract. The snapshot a form is seeded from can be
  // minutes stale, so without this a TITLE-ONLY edit on a card cached before a
  // price was recorded would send `price: null` and destroy a price its author
  // never saw.
  it('does not clear a price the form was never seeded with', async () => {
    mockShowUpdate.mutate.mockImplementation((_vars, opts) => {
      opts?.onSuccess?.({ id: 42 })
    })
    const user = userEvent.setup()
    renderWithProviders(
      <ShowForm
        mode="edit"
        initialData={makeShow({ price: null, door_price: null })}
      />
    )

    const title = screen.getByLabelText(/^show title \(optional\)$/i)
    await user.clear(title)
    await user.type(title, 'Retitled Only')
    await user.click(screen.getByRole('button', { name: /save changes/i }))

    await waitFor(() => expect(mockShowUpdate.mutate).toHaveBeenCalledTimes(1))

    const call = mockShowUpdate.mutate.mock.calls[0][0] as {
      updates: { price?: number | null; door_price?: number | null }
    }
    expect(call.updates.price).toBeUndefined()
    expect(call.updates.door_price).toBeUndefined()
  })

  // A show already recorded as FREE stores 0, and blanking that field is a real
  // retraction — so the stored-value guard tests `!= null`, not truthiness.
  it('clears a price recorded as free', async () => {
    mockShowUpdate.mutate.mockImplementation((_vars, opts) => {
      opts?.onSuccess?.({ id: 42 })
    })
    const user = userEvent.setup()
    renderWithProviders(
      <ShowForm mode="edit" initialData={makeShow({ price: 0, door_price: null })} />
    )

    await user.clear(screen.getByLabelText(/^cost \(optional\)$/i))
    await user.click(screen.getByRole('button', { name: /save changes/i }))

    await waitFor(() => expect(mockShowUpdate.mutate).toHaveBeenCalledTimes(1))

    const call = mockShowUpdate.mutate.mock.calls[0][0] as {
      updates: { price?: number | null }
    }
    expect(call.updates.price).toBeNull()
  })

  // "Free" is a price, not an absence, so it must submit as 0 and never as the
  // null that means "there is no recorded price". The show page spells the two
  // differently: "Free" against no price segment at all.
  it('submits a free show as 0, not as a clear', async () => {
    mockShowUpdate.mutate.mockImplementation((_vars, opts) => {
      opts?.onSuccess?.({ id: 42 })
    })
    const user = userEvent.setup()
    renderWithProviders(
      <ShowForm mode="edit" initialData={makeShow({ price: 25, door_price: 30 })} />
    )

    const cost = screen.getByLabelText(/^cost \(optional\)$/i)
    await user.clear(cost)
    await user.type(cost, 'Free')
    await user.click(screen.getByRole('button', { name: /save changes/i }))

    await waitFor(() => expect(mockShowUpdate.mutate).toHaveBeenCalledTimes(1))

    const call = mockShowUpdate.mutate.mock.calls[0][0] as {
      updates: { price?: number | null }
    }
    expect(call.updates.price).toBe(0)
  })
})
