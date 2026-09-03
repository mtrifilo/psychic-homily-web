import { beforeEach, describe, expect, it, vi } from 'vitest'
import userEvent from '@testing-library/user-event'
import { renderWithProviders, screen, waitFor } from '@/test/utils'
import type { ShowResponse } from '../types'

const { mockInvalidateQueries } = vi.hoisted(() => ({
  mockInvalidateQueries: vi.fn(),
}))
const mockPush = vi.fn()
const mockReplace = vi.fn()
const mockRefetch = vi.fn()
const mockUseAuthContext = vi.fn()
const mockUseMySubmissions = vi.fn()
const mockSetSoldOut = vi.fn()
const mockSetCancelled = vi.fn()
let mockSearchParams = new URLSearchParams()

vi.mock('@tanstack/react-query', async () => {
  const actual = await vi.importActual<typeof import('@tanstack/react-query')>(
    '@tanstack/react-query'
  )
  return {
    ...actual,
    useQueryClient: () => ({ invalidateQueries: mockInvalidateQueries }),
  }
})

vi.mock('next/navigation', () => ({
  useRouter: () => ({ push: mockPush, replace: mockReplace }),
  useSearchParams: () => mockSearchParams,
  usePathname: () => '/contribute/submissions',
  redirect: vi.fn(),
}))

vi.mock('@/lib/context/AuthContext', async () => {
  const { deriveMockAuthSignals } = await import('@/test/authFixture')
  return { useAuthContext: () => deriveMockAuthSignals(mockUseAuthContext()) }
})

vi.mock('../hooks', () => ({
  useMySubmissions: (...args: unknown[]) => mockUseMySubmissions(...args),
}))

vi.mock('@/lib/hooks/admin/useAdminShows', () => ({
  useSetShowSoldOut: () => ({ mutate: mockSetSoldOut, isPending: false }),
  useSetShowCancelled: () => ({ mutate: mockSetCancelled, isPending: false }),
}))

// ShowPrice is spliced in for REAL rather than stubbed: it renders a fact about
// money, so a stub would let the console silently show no price at all.
vi.mock('@/components/shared', async () => {
  const showPrice = await vi.importActual<
    typeof import('@/components/shared/ShowPrice')
  >('@/components/shared/ShowPrice')
  return {
    BracketLink: ({ label, href }: { label: string; href: string }) => (
      <a href={href}>{label}</a>
    ),
    SaveButton: () => null,
    ShowPrice: showPrice.ShowPrice,
    SubmissionSuccessDialog: ({
      open,
      onOpenChange,
    }: {
      open: boolean
      onOpenChange: (open: boolean) => void
    }) =>
      open ? (
        <div role="dialog" aria-label="Private Show Added">
          <button onClick={() => onOpenChange(false)}>Got it</button>
        </div>
      ) : null,
  }
})

vi.mock('@/features/venues', () => ({
  VenueDeniedDialog: () => null,
}))

vi.mock('./DeleteShowDialog', () => ({
  DeleteShowDialog: (props: { open?: boolean; onSuccess?: () => void }) =>
    props.open ? (
      <button onClick={() => props.onSuccess?.()}>Confirm deletion</button>
    ) : null,
}))
vi.mock('./MakePrivateDialog', () => ({ MakePrivateDialog: () => null }))
vi.mock('./PublishShowDialog', () => ({ PublishShowDialog: () => null }))
vi.mock('./UnpublishShowDialog', () => ({ UnpublishShowDialog: () => null }))
vi.mock('./ShowForm', () => ({ ShowForm: () => <div>Edit form</div> }))

import { ShowSubmissionsConsole } from './ShowSubmissionsConsole'

function makeShow(id: number, status: ShowResponse['status']): ShowResponse {
  return {
    id,
    title: `Show ${id}`,
    slug: `show-${id}`,
    description: '',
    event_date: '2026-08-20T20:00:00Z',
    doors_time: null,
    price: 20,
    age_requirement: '21+',
    ticket_url: null,
    image_url: null,
    city: 'Phoenix',
    state: 'AZ',
    status,
    is_sold_out: false,
    is_cancelled: false,
    submitted_by: 1,
    created_at: '2026-07-01T00:00:00Z',
    updated_at: '2026-07-01T00:00:00Z',
    artists: [
      {
        id,
        name: `Artist ${id}`,
        slug: `artist-${id}`,
        set_type: 'headliner',
        position: 0,
        socials: {},
      },
    ],
    venues: [
      {
        id,
        name: `Venue ${id}`,
        slug: `venue-${id}`,
        city: 'Phoenix',
        state: 'AZ',
        timezone: 'America/Phoenix',
        verified: true,
      },
    ],
  } as ShowResponse
}

describe('ShowSubmissionsConsole', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockSearchParams = new URLSearchParams()
    window.history.replaceState({}, '', '/contribute/submissions')
    mockUseAuthContext.mockReturnValue({
      authStatus: 'authenticated',
      user: { id: '1', is_admin: false },
    })
    mockUseMySubmissions.mockReturnValue({
      data: { shows: [], total: 0 },
      isLoading: false,
      error: null,
      refetch: mockRefetch,
    })
  })

  it('redirects settled-anonymous viewers with a return path', async () => {
    mockUseAuthContext.mockReturnValue({
      authStatus: 'anonymous',
      user: null,
    })

    renderWithProviders(<ShowSubmissionsConsole />)

    await waitFor(() => {
      expect(mockPush).toHaveBeenCalledWith(
        '/auth?returnTo=%2Fcontribute%2Fsubmissions'
      )
    })
    expect(mockUseMySubmissions).toHaveBeenCalledWith({
      enabled: false,
      userId: undefined,
      limit: 50,
      offset: 0,
    })
  })

  it('preserves the console query when redirecting to authentication', async () => {
    mockSearchParams = new URLSearchParams('submitted=private&source=show-form')
    // The canonical returnTo reads the browser's own query string, so the
    // location has to carry it rather than only the router mock.
    window.history.replaceState(
      {},
      '',
      '/contribute/submissions?submitted=private&source=show-form'
    )
    mockUseAuthContext.mockReturnValue({
      authStatus: 'anonymous',
      user: null,
    })

    renderWithProviders(<ShowSubmissionsConsole />)

    await waitFor(() => {
      expect(mockPush).toHaveBeenCalledWith(
        '/auth?returnTo=%2Fcontribute%2Fsubmissions%3Fsubmitted%3Dprivate%26source%3Dshow-form'
      )
    })
  })

  it('does not redirect while auth is unsettled', async () => {
    // 'pending' is a signed-in viewer whose profile has not arrived as often
    // as it is anyone else, and this guard cannot tell them apart.
    mockUseAuthContext.mockReturnValue({
      // TERMINAL pending: `isLoading` false while the status is still
      // unsettled, which is the window an `isLoading` gate cannot see.
      authStatus: 'pending',
      user: null,
      isLoading: false,
    })

    renderWithProviders(<ShowSubmissionsConsole />)

    await waitFor(() => {
      expect(
        screen.queryByRole('heading', { name: 'Show submissions' })
      ).toBeNull()
    })
    expect(mockPush).not.toHaveBeenCalled()
  })

  it('renders the dedicated empty state and Contribute links', () => {
    renderWithProviders(<ShowSubmissionsConsole />)

    expect(
      screen.getByRole('heading', { name: 'Show submissions' })
    ).toBeTruthy()
    expect(screen.getByText('No show submissions yet.')).toBeTruthy()
    expect(screen.getByRole('link', { name: 'Submit a show' })).toHaveAttribute(
      'href',
      '/shows/submit'
    )
    expect(
      screen.getByRole('link', { name: 'contribution opportunities' })
    ).toHaveAttribute('href', '/contribute')
  })

  it('dismisses the private-success dialog without dropping other query params', async () => {
    const user = userEvent.setup()
    mockSearchParams = new URLSearchParams('submitted=private&source=show-form')

    renderWithProviders(<ShowSubmissionsConsole />)
    await user.click(screen.getByRole('button', { name: 'Got it' }))

    expect(mockReplace).toHaveBeenCalledWith(
      '/contribute/submissions?source=show-form',
      { scroll: false }
    )
  })

  describe('the meta line withholds a clock on a guessed zone', () => {
    // 03:00Z is 8:00 PM the previous day in the fallback zone.
    function zonelessShow() {
      const show = makeShow(1, 'approved')
      return {
        ...show,
        event_date: '2026-09-10T03:00:00Z',
        state: '',
        venues: [{ ...show.venues[0], state: '', timezone: null }],
      } as ShowResponse
    }

    it('names the hour when the venue carries a zone', () => {
      const show = zonelessShow()
      mockUseMySubmissions.mockReturnValue({
        data: {
          shows: [
            {
              ...show,
              venues: [{ ...show.venues[0], timezone: 'America/Phoenix' }],
            },
          ],
          total: 1,
        },
        isLoading: false,
        error: null,
      })
      renderWithProviders(<ShowSubmissionsConsole />)
      expect(screen.getByText(/8:00\s?PM/)).toBeTruthy()
    })

    it('labels the submitted hour, and leaves no doubled bullet, when it does not', () => {
      mockUseMySubmissions.mockReturnValue({
        data: { shows: [zonelessShow()], total: 1 },
        isLoading: false,
        error: null,
      })
      renderWithProviders(<ShowSubmissionsConsole />)

      // The hour the submitter typed, read back on the same fallback the
      // submit path wrote it under, and never unlabelled: the label is what
      // separates it from the published clock the show page withholds.
      const labelled = screen.getByText(/8:00\s?PM\s+\u00b7\s+zone unresolved/)
      expect(labelled).toBeTruthy()
      expect(labelled.className).toContain('font-mono')

      // The segment carries its own leading bullet. A trailing-bullet check
      // could not see a fault here: `Details` always follows, so the line never
      // ends on a divider either way. What a mis-assembled segment can leave
      // behind is two adjacent ones, which is what this asserts.
      const meta = screen.getByText(/Venue 1/).closest('div') as HTMLElement
      const text = meta.textContent ?? ''
      expect(text).toContain('$20')
      expect(text.replace(/\s|\u00a0/g, '')).not.toContain('\u2022\u2022')
    })

    it('shows a reader nothing when the row is not theirs to fix', () => {
      mockUseAuthContext.mockReturnValue({
        isAuthenticated: true,
        isLoading: false,
        user: { id: '99', is_admin: false },
      })
      mockUseMySubmissions.mockReturnValue({
        data: { shows: [zonelessShow()], total: 1 },
        isLoading: false,
        error: null,
      })
      renderWithProviders(<ShowSubmissionsConsole />)

      expect(screen.queryByText(/8:00\s?PM/)).toBeNull()
      expect(screen.queryByText(/zone unresolved/)).toBeNull()
    })

    it('keeps the date on a row whose time it withheld', () => {
      mockUseMySubmissions.mockReturnValue({
        data: { shows: [zonelessShow()], total: 1 },
        isLoading: false,
        error: null,
      })
      renderWithProviders(<ShowSubmissionsConsole />)
      expect(screen.getByText(/Sep 9/)).toBeTruthy()
    })
  })

  it('preserves owner controls for approved and private shows', async () => {
    const user = userEvent.setup()
    mockUseMySubmissions.mockReturnValue({
      data: {
        shows: [makeShow(1, 'approved'), makeShow(2, 'private')],
        total: 2,
      },
      isLoading: false,
      error: null,
    })

    renderWithProviders(<ShowSubmissionsConsole />)

    const actionButtons = screen.getAllByRole('button', {
      name: 'Show actions',
    })
    await user.click(actionButtons[0])
    expect(screen.getByRole('menuitem', { name: 'Edit show' })).toBeTruthy()
    expect(screen.getByRole('menuitem', { name: 'Make private' })).toBeTruthy()
    expect(screen.getByRole('menuitem', { name: 'Mark sold out' })).toBeTruthy()
    expect(
      screen.getByRole('menuitem', { name: 'Mark cancelled' })
    ).toBeTruthy()
    expect(screen.getByRole('menuitem', { name: 'Delete show' })).toBeTruthy()

    await user.click(screen.getByRole('menuitem', { name: 'Mark sold out' }))
    expect(mockSetSoldOut).toHaveBeenCalledWith(
      { showId: 1, value: true },
      { onSuccess: expect.any(Function) }
    )
    mockSetSoldOut.mock.calls[0][1].onSuccess()
    expect(mockInvalidateQueries).toHaveBeenCalledWith({
      queryKey: ['mySubmissions'],
    })

    await user.click(actionButtons[1])
    expect(screen.getByRole('menuitem', { name: 'Publish show' })).toBeTruthy()
  })

  it('pages through all submissions when the response exceeds the page size', async () => {
    const user = userEvent.setup()
    mockUseMySubmissions.mockImplementation(
      ({ offset }: { offset: number }) => ({
        data: {
          shows: [makeShow(offset + 1, 'approved')],
          total: 51,
        },
        isLoading: false,
        error: null,
        refetch: mockRefetch,
      })
    )

    renderWithProviders(<ShowSubmissionsConsole />)

    expect(screen.getByText('1–1 of 51')).toBeTruthy()
    await user.click(screen.getByRole('button', { name: 'Next' }))

    expect(mockUseMySubmissions).toHaveBeenLastCalledWith({
      enabled: true,
      userId: '1',
      limit: 50,
      offset: 50,
    })
    expect(screen.getByText('51–51 of 51')).toBeTruthy()
  })

  it('offers retry and previous-page recovery after a later page fails', async () => {
    const user = userEvent.setup()
    mockUseMySubmissions.mockImplementation(({ offset }: { offset: number }) =>
      offset === 50
        ? {
            data: undefined,
            isLoading: false,
            error: new Error('temporary failure'),
            refetch: mockRefetch,
          }
        : {
            data: { shows: [makeShow(1, 'approved')], total: 51 },
            isLoading: false,
            error: null,
            refetch: mockRefetch,
          }
    )

    renderWithProviders(<ShowSubmissionsConsole />)
    await user.click(screen.getByRole('button', { name: 'Next' }))

    expect(screen.getByText(/Failed to load your submissions/)).toBeTruthy()
    await user.click(screen.getByRole('button', { name: 'Retry' }))
    expect(mockRefetch).toHaveBeenCalled()

    await user.click(screen.getByRole('button', { name: 'Previous page' }))
    expect(screen.getByText('1–1 of 51')).toBeTruthy()
  })

  it('returns to the last valid page when deleting its only submission', async () => {
    const user = userEvent.setup()
    mockUseMySubmissions.mockImplementation(
      ({ offset }: { offset: number }) => ({
        data: {
          shows: [makeShow(offset === 50 ? 51 : 1, 'approved')],
          total: 51,
        },
        isLoading: false,
        error: null,
        refetch: mockRefetch,
      })
    )

    renderWithProviders(<ShowSubmissionsConsole />)
    await user.click(screen.getByRole('button', { name: 'Next' }))
    expect(screen.getByText('51–51 of 51')).toBeTruthy()

    await user.click(screen.getByRole('button', { name: 'Show actions' }))
    await user.click(screen.getByRole('menuitem', { name: 'Delete show' }))
    await user.click(screen.getByRole('button', { name: 'Confirm deletion' }))

    await waitFor(() => {
      expect(mockUseMySubmissions).toHaveBeenLastCalledWith({
        enabled: true,
        userId: '1',
        limit: 50,
        offset: 0,
      })
    })
    expect(screen.queryByText('No show submissions yet.')).toBeNull()
  })
})
