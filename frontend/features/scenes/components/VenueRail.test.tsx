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
    principalCity: 'Austin',
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
    expect(
      screen.getByText(/2 venues · 25 upcoming · 3 in the next 7 days/i),
    ).toBeInTheDocument()
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

  // PSY-1574: the rail is metro-scoped, so a row can sit in a city the header
  // doesn't name — and its pin can be outside the current frame. The row has to
  // say where it is, or it reads as a mistake.
  it('prints the city of a metro-member venue and omits it for the principal city', () => {
    renderRail({
      principalCity: 'Phoenix',
      venues: [
        venue({ id: 1, name: 'Crescent Ballroom', city: 'Phoenix', state: 'AZ' }),
        venue({ id: 2, name: 'Yucca Tap Room', city: 'Tempe', state: 'AZ' }),
      ],
    })

    expect(
      screen.getByRole('button', { name: /Yucca Tap Room/ }),
    ).toHaveTextContent('Tempe')
    expect(
      screen.getByRole('button', { name: /Crescent Ballroom/ }),
    ).not.toHaveTextContent('Phoenix')
  })

  // The heading names one city; the count under it must not claim that city
  // when the rows also hold Tempe and Mesa.
  it('qualifies the venue count when the list reaches past the principal city', () => {
    renderRail({
      principalCity: 'Phoenix',
      venues: [
        venue({ id: 1, name: 'Crescent Ballroom', city: 'Phoenix' }),
        venue({ id: 2, name: 'Yucca Tap Room', city: 'Tempe' }),
      ],
    })
    expect(screen.getByText(/2 metro venues/i)).toBeInTheDocument()
  })

  it('leaves the count unqualified when every venue is in the principal city', () => {
    renderRail({
      principalCity: 'Phoenix',
      venues: [
        venue({ id: 1, name: 'Crescent Ballroom', city: 'Phoenix' }),
        venue({ id: 2, name: 'Valley Bar', city: 'Phoenix' }),
      ],
    })
    expect(screen.getByText(/2 venues/i)).toBeInTheDocument()
    expect(screen.queryByText(/metro venues/i)).not.toBeInTheDocument()
  })

  it('does not label a principal-city row that differs only in case or padding', () => {
    renderRail({
      principalCity: 'Phoenix',
      venues: [venue({ id: 1, name: 'Crescent Ballroom', city: ' phoenix ' })],
    })
    expect(
      screen.getByRole('button', { name: /Crescent Ballroom/ }),
    ).not.toHaveTextContent(/phoenix/i)
  })

  it('toggles the Next 7 days filter', () => {
    const onFiltersChange = vi.fn()
    renderRail({ onFiltersChange })
    fireEvent.click(screen.getByRole('button', { name: 'Next 7 days' }))
    expect(onFiltersChange).toHaveBeenCalledWith({
      ...NO_CITY_VENUE_FILTERS,
      thisWeekOnly: true,
    })
  })

  it('reflects an active Next 7 days filter as pressed', () => {
    renderRail({ filters: { ...NO_CITY_VENUE_FILTERS, thisWeekOnly: true } })
    expect(screen.getByRole('button', { name: 'Next 7 days' })).toHaveAttribute(
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
      ...NO_CITY_VENUE_FILTERS,
      genreFamily: 'punk_hardcore',
    })
  })

  it('reports an all-ages pick without disturbing the other chips', () => {
    const onFiltersChange = vi.fn()
    renderRail({
      onFiltersChange,
      filters: { ...NO_CITY_VENUE_FILTERS, thisWeekOnly: true },
    })
    fireEvent.click(screen.getByRole('button', { name: 'All-ages shows' }))
    expect(onFiltersChange).toHaveBeenCalledWith({
      ...NO_CITY_VENUE_FILTERS,
      thisWeekOnly: true,
      allAgesOnly: true,
    })
  })

  it('marks the all-ages chip pressed while it is on', () => {
    renderRail({ filters: { ...NO_CITY_VENUE_FILTERS, allAgesOnly: true } })
    expect(
      screen.getByRole('button', { name: 'All-ages shows' }),
    ).toHaveAttribute('aria-pressed', 'true')
  })

  it('names the shows rather than the room, so the chip promises no more than the tag does', () => {
    // The tag means "hosts all-ages shows AT LEAST SOMETIMES" (PSY-1573). A
    // bare "All ages" beside a venue name reads as a claim about every night
    // there, which nobody has made.
    renderRail()
    expect(
      screen.queryByRole('button', { name: 'All ages' }),
    ).not.toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'All-ages shows' })).toHaveAttribute(
      'title',
      'Venues that host all-ages shows at least sometimes',
    )
  })

  it('keeps the record-stores chip disabled rather than guessing at a filter', () => {
    renderRail()
    expect(screen.getByRole('button', { name: 'All-ages shows' })).toBeEnabled()
    expect(screen.getByRole('button', { name: 'Record stores' })).toBeDisabled()
  })

  it('blames the missing tag, not the city, when nothing here is tagged', () => {
    // The honest empty state (PSY-1573). Coverage is near-zero, so this IS the
    // default state of the chip — and "No venues match these filters" would let
    // a reader conclude the city has no all-ages rooms, which we never claimed.
    renderRail({
      venues: [],
      allVenues: [venue({ id: 1, hosts_all_ages: false })],
      filters: { ...NO_CITY_VENUE_FILTERS, allAgesOnly: true },
    })
    expect(
      screen.getByText(/No venue here is tagged for all-ages shows yet/),
    ).toBeInTheDocument()
    expect(screen.getByText(/doesn’t mean the city has none/)).toBeInTheDocument()
    expect(
      screen.queryByText('No venues match these filters.'),
    ).not.toBeInTheDocument()
  })

  it('falls back to the generic empty state when the tag is not what came up empty', () => {
    // The city HAS a tagged venue, so an empty list here is some other chip's
    // doing. Claiming "nothing is tagged" would be a guess, and a false one.
    renderRail({
      venues: [],
      allVenues: [venue({ id: 1, hosts_all_ages: true, shows_this_week: 0 })],
      filters: { ...NO_CITY_VENUE_FILTERS, allAgesOnly: true, thisWeekOnly: true },
    })
    expect(
      screen.getByText('No venues match these filters.'),
    ).toBeInTheDocument()
    expect(
      screen.queryByText(/tagged for all-ages shows yet/),
    ).not.toBeInTheDocument()
  })

  it('admits how far it looked instead of generalising from a truncated page', () => {
    // `allVenues` is ONE busiest-first page, and a tagged DIY room below the
    // cut is exactly the room this chip is for. The sentence has to name the
    // cap rather than make a claim about the whole metro.
    renderRail({
      venues: [],
      allVenues: [venue({ id: 1, hosts_all_ages: false })],
      totalVenueCount: 140,
      filters: { ...NO_CITY_VENUE_FILTERS, allAgesOnly: true },
    })
    expect(
      screen.getByText(/No venue among the 1 busiest here is tagged/),
    ).toBeInTheDocument()
    expect(screen.getByText(/doesn’t mean the city has none/)).toBeInTheDocument()
    expect(
      screen.queryByText('No venues match these filters.'),
    ).not.toBeInTheDocument()
  })

  it('says the check failed rather than blaming the filters, when the tag never answered', () => {
    // `hosts_all_ages` absent means NOT DETERMINED (rail fields not requested,
    // or the backend's tag query failed). Every row is then dropped, so the
    // rail AND the map go blank; "no venues match these filters" would leave
    // that looking like a broken feature, and "no venue here is tagged" would
    // assert an absence on the strength of a query that never ran.
    renderRail({
      venues: [],
      allVenues: [venue({ id: 1, hosts_all_ages: undefined })],
      filters: { ...NO_CITY_VENUE_FILTERS, allAgesOnly: true },
    })
    expect(
      screen.queryByText(/tagged for all-ages shows yet/),
    ).not.toBeInTheDocument()
    expect(
      screen.queryByText('No venues match these filters.'),
    ).not.toBeInTheDocument()
    expect(
      screen.getByText(/Couldn’t check which venues host all-ages shows/),
    ).toBeInTheDocument()
  })

  it('says the request failed rather than calling the city empty', () => {
    // A 429 or a dropped request leaves the same zero rows an empty city does.
    renderRail({ venues: [], allVenues: [], fetchFailed: true })
    expect(
      screen.getByText(/Couldn’t load venues here/),
    ).toBeInTheDocument()
    expect(
      screen.queryByText('No venues listed here yet.'),
    ).not.toBeInTheDocument()
  })

  it('describes the all-ages chip to assistive tech, not only in a tooltip', () => {
    // `title` is dropped by most screen readers on an already-labelled
    // control, so the caveat is also the chip's aria-describedby target — and
    // it must exist BEFORE activation, hence rendered even while the chip is
    // off.
    renderRail()
    const chip = screen.getByRole('button', { name: 'All-ages shows' })
    const describedBy = chip.getAttribute('aria-describedby')
    expect(describedBy).toBeTruthy()
    expect(
      document.getElementById(describedBy as string)?.textContent,
    ).toMatch(/at least sometimes/)
  })

  it('promises no affordance the app does not have', () => {
    // VenueDetail mounts EntityTagList but not the AddTagDialog + "[Add tag]"
    // control its peers pair with it, so there is nowhere to go add the tag.
    //
    // Asserts on RENDERED CONTROLS, not on the absence of particular phrases:
    // a phrase-absence test passes no matter what call-to-action someone adds
    // later, which is the opposite of what a regression guard is for.
    const { container } = renderRail({
      venues: [],
      allVenues: [venue({ id: 1, hosts_all_ages: false })],
      filters: { ...NO_CITY_VENUE_FILTERS, allAgesOnly: true },
    })
    const emptyState = container.querySelector('.min-h-0.flex-1')
    expect(emptyState).not.toBeNull()
    expect(
      within(emptyState as HTMLElement).queryByRole('link'),
    ).not.toBeInTheDocument()
    expect(
      within(emptyState as HTMLElement).queryByRole('button'),
    ).not.toBeInTheDocument()
  })

  it('shows the sometimes caveat on screen, not only in a tooltip', () => {
    // `title` never opens on touch and is skipped by most screen readers on a
    // control that already has a label — so on the likeliest device the bare
    // chip label would be the only thing a reader sees.
    const { rerender } = renderRail()
    // Present for assistive tech even when the chip is off (it is the chip's
    // aria-describedby target), but visually hidden so the chip row stays
    // quiet in the default state.
    expect(screen.getByText(/at least sometimes/)).toHaveClass('sr-only')

    rerender(
      <VenueRail
        cityLabel="Austin, TX"
        principalCity="Austin"
        venues={[venue({ hosts_all_ages: true })]}
        allVenues={[venue({ hosts_all_ages: true })]}
        filters={{ ...NO_CITY_VENUE_FILTERS, allAgesOnly: true }}
        onFiltersChange={vi.fn()}
        selectedVenueId={null}
        onVenueSelect={vi.fn()}
        onBackToGlobe={vi.fn()}
      />,
    )
    expect(
      screen.getByText(/host all-ages shows at least sometimes/),
    ).not.toHaveClass('sr-only')
  })

  it('says the city is empty before it blames any filter', () => {
    renderRail({
      venues: [],
      allVenues: [],
      filters: { ...NO_CITY_VENUE_FILTERS, allAgesOnly: true },
    })
    expect(screen.getByText('No venues listed here yet.')).toBeInTheDocument()
  })

  it('offers no list-level confirm affordance', () => {
    // Deliberately NOT built (user decision, 2026-07-27). There is no "list"
    // object to write to — scenes are computed views with no table — and a
    // bulk confirm of every listed venue would make each confirmation far
    // weaker evidence than the deliberate per-venue one in the venue panel.
    renderRail()
    expect(
      screen.queryByRole('button', { name: /Confirm this list is current/ }),
    ).not.toBeInTheDocument()
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
    unmount()

    renderRail({ venues: [], allVenues: [] })
    expect(screen.getByTestId('rail-provenance')).toHaveTextContent(
      'no update recorded',
    )
  })

  it('sums the city\u2019s edits and confirmations', () => {
    renderRail({
      allVenues: [
        venue({
          id: 1,
          provenance: {
            updated_at: '2026-07-25T00:00:00Z',
            edit_count: 3,
            contributor_count: 2,
            confirmation_count: 4,
            sources: ['community'],
          },
        }),
        venue({
          id: 2,
          name: 'Hotel Vegas',
          provenance: {
            updated_at: '2026-07-24T00:00:00Z',
            edit_count: 1,
            contributor_count: 1,
            confirmation_count: 2,
            sources: ['community'],
          },
        }),
      ],
    })
    const line = screen.getByTestId('rail-provenance')
    expect(line).toHaveTextContent('4 edits')
    expect(line).toHaveTextContent('6 confirmations')
  })

  it('claims no city-wide contributor count', () => {
    // Per-venue contributor counts are DISTINCT-user counts and don't add up:
    // one person maintaining three venues would be counted three times. The
    // exact number lives on the venue panel, where the scope makes it true.
    renderRail({
      allVenues: [
        venue({
          id: 1,
          provenance: {
            updated_at: '2026-07-25T00:00:00Z',
            edit_count: 3,
            contributor_count: 2,
            confirmation_count: 4,
            sources: ['community'],
          },
        }),
      ],
    })
    expect(screen.getByTestId('rail-provenance')).not.toHaveTextContent(
      /contributor/i,
    )
  })

  it('omits the counts entirely when the city has none', () => {
    const line = renderRail().container.querySelector(
      '[data-testid="rail-provenance"]',
    )
    expect(line?.textContent).not.toMatch(/edit|confirmation/i)
  })
})
