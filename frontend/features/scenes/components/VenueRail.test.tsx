import { describe, it, expect, vi } from 'vitest'
import { fireEvent, render, screen, within } from '@testing-library/react'
import type { VenueWithShowCount } from '@/features/venues/types'
import { NO_CITY_VENUE_FILTERS } from '../cityView'
import { VenueRail } from './VenueRail'

function venue(overrides: Partial<VenueWithShowCount> = {}): VenueWithShowCount {
  return {
    id: 1,
    slug: 'mohawk-austin-tx',
    name: 'Mohawk',
    address: null,
    city: 'Austin',
    state: 'TX',
    verified: true,
    upcoming_show_count: 14,
    shows_this_week: 3,
    next_show_date: '2026-07-28',
    next_show_artists: ['Gouge Away', 'Militarie Gun'],
    dominant_genre: 'punk_hardcore',
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-07-25T00:00:00Z',
    ...overrides,
  } as VenueWithShowCount
}

function renderRail(props: Partial<React.ComponentProps<typeof VenueRail>> = {}) {
  const venues = props.venues ?? [venue()]
  const defaults: React.ComponentProps<typeof VenueRail> = {
    cityLabel: 'Austin, TX',
    venues,
    allVenues: props.allVenues ?? venues,
    filters: NO_CITY_VENUE_FILTERS,
    onFiltersChange: vi.fn(),
    selectedVenueId: null,
    onVenueSelect: vi.fn(),
    onBackToGlobe: vi.fn(),
  }
  return { ...render(<VenueRail {...defaults} {...props} />), props: { ...defaults, ...props } }
}

describe('VenueRail', () => {
  it('names the city and labels itself for assistive tech', () => {
    renderRail()
    expect(
      screen.getByRole('complementary', { name: 'Venues in Austin, TX' }),
    ).toBeInTheDocument()
    expect(
      screen.getByRole('heading', { name: 'Austin, TX' }),
    ).toBeInTheDocument()
  })

  it('derives header counts from the venue rows, not from a scene stat', () => {
    renderRail({
      allVenues: [
        venue({ id: 1, upcoming_show_count: 14, shows_this_week: 3 }),
        venue({ id: 2, upcoming_show_count: 11, shows_this_week: 0 }),
      ],
    })
    expect(screen.getByText(/2 venues · 25 upcoming · 3 this week/i)).toBeInTheDocument()
  })

  it('adds the scene-level roster size when it has loaded', () => {
    renderRail({ localArtistCount: 35 })
    expect(screen.getByText(/· 35 local artists/i)).toBeInTheDocument()
  })

  it('omits the roster clause entirely rather than showing a placeholder count', () => {
    renderRail({ localArtistCount: undefined })
    expect(screen.queryByText(/local artists/i)).not.toBeInTheDocument()
  })

  it('renders a row with its count and the NEXT meta line', () => {
    renderRail()
    const row = screen.getByRole('button', { name: /Mohawk/ })
    expect(within(row).getByText('14 upcoming')).toBeInTheDocument()
    expect(row).toHaveTextContent('NEXT')
    expect(row).toHaveTextContent('Tue, Jul 28')
    expect(row).toHaveTextContent('Gouge Away / Militarie Gun')
    expect(row).toHaveTextContent('punk & hardcore')
  })

  it('prefers a show title over the bill when the show has one', () => {
    renderRail({
      venues: [
        venue({ next_show_title: 'Levitation pre-party', next_show_artists: ['X'] }),
      ],
    })
    expect(screen.getByRole('button', { name: /Mohawk/ })).toHaveTextContent(
      'Levitation pre-party',
    )
  })

  it('says so plainly when a venue has nothing booked', () => {
    renderRail({
      venues: [
        venue({
          upcoming_show_count: 0,
          next_show_date: undefined,
          next_show_artists: undefined,
        }),
      ],
    })
    expect(screen.getByRole('button', { name: /Mohawk/ })).toHaveTextContent(
      'nothing on the calendar',
    )
  })

  it('toggles the This week filter', () => {
    const onFiltersChange = vi.fn()
    renderRail({ onFiltersChange })
    fireEvent.click(screen.getByRole('button', { name: 'This week' }))
    expect(onFiltersChange).toHaveBeenCalledWith({
      thisWeekOnly: true,
      genreFamily: null,
    })
  })

  it('reflects an active This week filter as pressed', () => {
    renderRail({ filters: { thisWeekOnly: true, genreFamily: null } })
    expect(screen.getByRole('button', { name: 'This week' })).toHaveAttribute(
      'aria-pressed',
      'true',
    )
  })

  it('offers only the genre families present in this city', () => {
    renderRail({
      allVenues: [
        venue({ id: 1, dominant_genre: 'punk_hardcore' }),
        venue({ id: 2, dominant_genre: 'electronic' }),
        venue({ id: 3, dominant_genre: undefined }),
      ],
    })
    const select = screen.getByRole('combobox', { name: /genre/i })
    const options = within(select).getAllByRole('option').map((o) => o.textContent)
    expect(options).toEqual(['All genres', 'Punk & Hardcore', 'Electronic'])
  })

  it('reports a genre pick', () => {
    const onFiltersChange = vi.fn()
    renderRail({ onFiltersChange })
    fireEvent.change(screen.getByRole('combobox', { name: /genre/i }), {
      target: { value: 'punk_hardcore' },
    })
    expect(onFiltersChange).toHaveBeenCalledWith({
      thisWeekOnly: false,
      genreFamily: 'punk_hardcore',
    })
  })

  it('ships the undecided chips disabled rather than guessing at a filter', () => {
    renderRail()
    expect(screen.getByRole('button', { name: 'All ages' })).toBeDisabled()
    expect(screen.getByRole('button', { name: 'Record stores' })).toBeDisabled()
    expect(
      screen.getByRole('button', { name: /Confirm this list is current/ }),
    ).toBeDisabled()
  })

  it('selects a venue when its row is clicked', () => {
    const onVenueSelect = vi.fn()
    renderRail({ onVenueSelect })
    fireEvent.click(screen.getByRole('button', { name: /Mohawk/ }))
    expect(onVenueSelect).toHaveBeenCalledWith(1)
  })

  it('marks the selected row as current', () => {
    renderRail({ selectedVenueId: 1 })
    expect(screen.getByRole('button', { name: /Mohawk/ })).toHaveAttribute(
      'aria-current',
      'true',
    )
  })

  it('flies back to the globe from the header affordance', () => {
    const onBackToGlobe = vi.fn()
    renderRail({ onBackToGlobe })
    fireEvent.click(screen.getByRole('button', { name: '← globe' }))
    expect(onBackToGlobe).toHaveBeenCalled()
  })

  it('distinguishes an empty city from an over-filtered one', () => {
    const { unmount } = renderRail({ venues: [], allVenues: [] })
    expect(screen.getByText('No venues listed here yet.')).toBeInTheDocument()
    unmount()

    renderRail({ venues: [], allVenues: [venue()] })
    expect(screen.getByText('No venues match these filters.')).toBeInTheDocument()
  })

  it('reports real provenance, never an invented one', () => {
    const { unmount } = renderRail()
    expect(screen.getByTestId('rail-provenance')).toHaveTextContent(
      /updated .+ ago/,
    )
    // No fabricated edit or contributor counts (PSY-1542's data).
    expect(screen.queryByText(/contributors/i)).not.toBeInTheDocument()
    unmount()

    renderRail({ venues: [], allVenues: [] })
    expect(screen.getByTestId('rail-provenance')).toHaveTextContent(
      'no update recorded',
    )
  })
})
