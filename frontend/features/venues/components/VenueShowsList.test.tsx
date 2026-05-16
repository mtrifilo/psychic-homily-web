import { describe, it, expect, vi, beforeEach } from 'vitest'
import { screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { renderWithProviders } from '@/test/utils'
import type { VenueShow } from '../types'
import type { TimeFilter } from '../hooks/useVenues'

// Two parallel useVenueShows results so upcoming + past fixtures can be set
// independently.
const upcomingResult = {
  data: undefined as { shows: VenueShow[]; total: number } | undefined,
  isLoading: false,
  error: null as Error | null,
}
const pastResult = {
  data: undefined as { shows: VenueShow[]; total: number } | undefined,
  isLoading: false,
  error: null as Error | null,
}

vi.mock('../hooks/useVenues', () => ({
  useVenueShows: ({ timeFilter }: { timeFilter: TimeFilter }) =>
    timeFilter === 'past' ? pastResult : upcomingResult,
}))

vi.mock('@/features/shows', () => ({
  dedupVenueShows: <T,>(shows: T[]) => shows,
}))

const mockAuthIsAuthenticated = { value: false }
vi.mock('@/lib/context/AuthContext', () => ({
  useAuthContext: () => ({
    isAuthenticated: mockAuthIsAuthenticated.value,
    user: mockAuthIsAuthenticated.value ? { id: 1 } : null,
    isLoading: false,
  }),
}))

vi.mock('@/components/shared', () => ({
  BracketLink: ({
    label,
    onClick,
  }: {
    label: string
    onClick?: () => void
  }) => (
    <button data-testid={`bracket-${label}`} onClick={onClick}>
      [{label}]
    </button>
  ),
  SectionHeader: ({
    title,
    action,
  }: {
    title: string
    action?: React.ReactNode
  }) => (
    <div data-testid={`section-header-${title}`}>
      <h2>{title}</h2>
      {action}
    </div>
  ),
  DenseTable: ({
    children,
    'aria-label': ariaLabel,
  }: {
    children: React.ReactNode
    'aria-label'?: string
  }) => (
    <table data-testid={`densetable-${ariaLabel ?? 'unlabeled'}`} aria-label={ariaLabel}>
      {children}
    </table>
  ),
}))

vi.mock('@/features/notifications', () => ({
  NotifyMeButton: ({ entityName }: { entityName: string }) => (
    <button data-testid="notify-me-button">Notify me about {entityName}</button>
  ),
}))

// ShowForm pulls in a lot of form/mutation plumbing the suite doesn't need.
// Render a thin stub so we can assert open/close + submit/cancel handlers.
vi.mock('@/components/forms/ShowForm', () => ({
  ShowForm: ({
    onCancel,
    onSuccess,
    prefilledVenue,
  }: {
    onCancel: () => void
    onSuccess: () => void
    prefilledVenue: { name: string }
  }) => (
    <div data-testid="show-form">
      Form for {prefilledVenue.name}
      <button onClick={onCancel}>cancel</button>
      <button onClick={onSuccess}>save</button>
    </div>
  ),
}))

import { VenueShowsList } from './VenueShowsList'

function makeShow(overrides: Partial<VenueShow> = {}): VenueShow {
  return {
    id: 1,
    slug: 'test-show',
    title: 'Test Show',
    event_date: '2025-06-15T20:00:00Z',
    city: 'Phoenix',
    state: 'AZ',
    price: 15,
    age_requirement: null,
    artists: [
      {
        id: 42,
        slug: 'main-artist',
        name: 'Main Artist',
        set_type: 'headliner',
        position: 1,
        socials: {},
      },
      {
        id: 99,
        slug: 'opener',
        name: 'The Opener',
        set_type: 'headliner',
        position: 2,
        socials: {},
      },
    ],
    ...overrides,
  }
}

function setUpcoming(
  data: { shows: VenueShow[]; total?: number } | null,
  opts?: { isLoading?: boolean; error?: Error | null }
) {
  upcomingResult.data = data
    ? { shows: data.shows, total: data.total ?? data.shows.length }
    : undefined
  upcomingResult.isLoading = opts?.isLoading ?? false
  upcomingResult.error = opts?.error ?? null
}

function setPast(
  data: { shows: VenueShow[]; total?: number } | null,
  opts?: { isLoading?: boolean; error?: Error | null }
) {
  pastResult.data = data
    ? { shows: data.shows, total: data.total ?? data.shows.length }
    : undefined
  pastResult.isLoading = opts?.isLoading ?? false
  pastResult.error = opts?.error ?? null
}

function renderList(overrides?: Partial<Parameters<typeof VenueShowsList>[0]>) {
  return renderWithProviders(
    <VenueShowsList
      venueId={7}
      venueSlug="the-venue"
      venueName="The Venue"
      venueCity="Phoenix"
      venueState="AZ"
      {...overrides}
    />
  )
}

describe('VenueShowsList — upcoming section', () => {
  beforeEach(() => {
    setUpcoming(null)
    setPast(null)
    mockAuthIsAuthenticated.value = false
  })

  it('renders the Upcoming shows section header always', () => {
    renderList()
    expect(screen.getByTestId('section-header-Upcoming shows')).toBeInTheDocument()
  })

  it('shows a loader while upcoming shows are loading', () => {
    setUpcoming(null, { isLoading: true })
    renderList()
    expect(document.querySelector('.animate-spin')).toBeInTheDocument()
  })

  it('shows an error message when the upcoming fetch fails', () => {
    setUpcoming(null, { error: new Error('boom') })
    renderList()
    expect(screen.getByText(/Failed to load shows/i)).toBeInTheDocument()
  })

  it('renders an inline [Notify me] affordance when there are no upcoming shows', () => {
    setUpcoming({ shows: [] })
    renderList({ venueName: 'Rebel Lounge' })
    expect(screen.getByText(/No upcoming shows yet/i)).toBeInTheDocument()
    expect(screen.getByTestId('notify-me-button')).toHaveTextContent('Rebel Lounge')
  })

  it('renders upcoming shows as a DenseTable when data is present', () => {
    setUpcoming({ shows: [makeShow()] })
    renderList()
    expect(screen.getByTestId('densetable-Upcoming shows')).toBeInTheDocument()
    expect(screen.getByText('Main Artist')).toBeInTheDocument()
    expect(screen.getByText('The Opener')).toBeInTheDocument()
  })

  it('shows the "Showing N of M" hint when results are truncated', () => {
    setUpcoming({ shows: [makeShow()], total: 30 })
    renderList()
    expect(screen.getByText(/Showing 1 of 30 shows/)).toBeInTheDocument()
  })
})

describe('VenueShowsList — past section', () => {
  beforeEach(() => {
    setUpcoming(null)
    setPast(null)
    mockAuthIsAuthenticated.value = false
  })

  it('omits the Past shows section entirely when there are 0 past shows', () => {
    setPast({ shows: [] })
    renderList()
    expect(screen.queryByTestId('section-header-Past shows')).not.toBeInTheDocument()
  })

  it('renders the Past shows section header with a [Show] toggle when past shows exist', () => {
    setPast({ shows: [makeShow({ id: 5, title: 'Past Show' })] })
    renderList()
    expect(screen.getByTestId('section-header-Past shows')).toBeInTheDocument()
    expect(screen.getByTestId('bracket-Show')).toBeInTheDocument()
  })

  it('past shows body is collapsed by default', () => {
    setPast({ shows: [makeShow({ id: 5, title: 'Past Show' })] })
    renderList()
    expect(screen.queryByTestId('densetable-Past shows')).not.toBeInTheDocument()
  })

  it('expands the past shows body when [Show] is clicked', async () => {
    const user = userEvent.setup()
    setPast({ shows: [makeShow({ id: 5, title: 'Past Show' })] })
    renderList()
    await user.click(screen.getByTestId('bracket-Show'))
    expect(screen.getByTestId('densetable-Past shows')).toBeInTheDocument()
    expect(screen.getByTestId('bracket-Hide')).toBeInTheDocument()
  })

  it('collapses again when [Hide] is clicked', async () => {
    const user = userEvent.setup()
    setPast({ shows: [makeShow({ id: 5, title: 'Past Show' })] })
    renderList()
    await user.click(screen.getByTestId('bracket-Show'))
    await user.click(screen.getByTestId('bracket-Hide'))
    expect(screen.queryByTestId('densetable-Past shows')).not.toBeInTheDocument()
  })
})

describe('VenueShowsList — add-show affordance', () => {
  beforeEach(() => {
    setUpcoming({ shows: [makeShow()] })
    setPast(null)
  })

  it('does not render the add-show button for unauthenticated users', () => {
    mockAuthIsAuthenticated.value = false
    renderList()
    expect(screen.queryByRole('button', { name: /Add a show/i })).not.toBeInTheDocument()
  })

  it('renders the add-show button for authenticated users', () => {
    mockAuthIsAuthenticated.value = true
    renderList({ venueName: 'Rebel Lounge' })
    expect(
      screen.getByRole('button', { name: /Add a show at Rebel Lounge/i })
    ).toBeInTheDocument()
  })

  it('toggles the ShowForm open when the add-show button is clicked', async () => {
    const user = userEvent.setup()
    mockAuthIsAuthenticated.value = true
    renderList()
    await user.click(screen.getByRole('button', { name: /Add a show/i }))
    expect(screen.getByTestId('show-form')).toBeInTheDocument()
  })

  it('closes the ShowForm and calls onShowAdded on successful submit', async () => {
    const user = userEvent.setup()
    const onShowAdded = vi.fn()
    mockAuthIsAuthenticated.value = true
    renderList({ onShowAdded })
    await user.click(screen.getByRole('button', { name: /Add a show/i }))
    await user.click(screen.getByRole('button', { name: 'save' }))
    expect(screen.queryByTestId('show-form')).not.toBeInTheDocument()
    expect(onShowAdded).toHaveBeenCalled()
  })
})
