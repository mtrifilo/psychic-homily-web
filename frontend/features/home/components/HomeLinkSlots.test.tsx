import { describe, it, expect, vi, beforeEach } from 'vitest'
import { act, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { HomeLayoutRuntime } from './HomeLayoutRuntime'
import { NearbyShowsSection } from './NearbyShowsSection'
import { SavedShowsModule } from './SavedShowsModule'
import { animateSlotHeight } from '../homeLayoutMotion'
import {
  HOME_SECTIONS,
  resolveHomeLayout,
  setHomeSectionVisibility,
  type HomeDiscoverLinksSlot,
  type HomeSectionId,
  type ResolvedHomeSection,
} from '../sections'

// The real runtime, toolbar, saved-shows module and nearby section, so every
// surface that can carry a link row reads the one slot value the runtime
// provides. Stubbed: the data hooks, the router, the slot animation, the
// nearby list and save button, and the three sections that carry no row.

let layout: ResolvedHomeSection[] = resolveHomeLayout(null)

vi.mock('../hooks/useHomeLayout', () => ({
  useHomeLayout: () => ({
    sections: layout,
    isReady: true,
    status: 'ready',
    retry: vi.fn(),
    hasStoredLayout: true,
  }),
  useHomeLayoutWriteFailed: () => false,
  usePersistHomeLayout: () => ({
    persist: vi.fn(),
    reset: vi.fn(),
    isResetting: false,
  }),
}))

// jsdom has no Web Animations, so the real helper answers null (settle now)
// on every call; the stand-in does the same unless a test holds a collapse.
vi.mock('../homeLayoutMotion', async importOriginal => ({
  ...(await importOriginal<object>()),
  animateSlotHeight: vi.fn(() => null),
}))

vi.mock('next/navigation', () => ({
  useRouter: () => ({ refresh: vi.fn(), push: vi.fn() }),
}))

vi.mock('@/lib/context/AuthContext', () => ({
  useAuthContext: () => ({
    authStatus: 'authenticated',
    isAuthenticated: true,
    user: { id: '42' },
  }),
}))

vi.mock('@/features/shows/hooks/useHomeShowCitySelection', () => ({
  useHomeShowCitySelection: () => ({
    cities: [],
    favoriteCities: [],
    effectiveCities: [{ city: 'Phoenix', state: 'AZ' }],
    source: 'geo',
    isResolving: false,
    geoAffordanceCity: null,
    selectionDiffersFromFavorites: false,
    onFilterChange: vi.fn(),
  }),
}))

vi.mock('@/features/shows/components/HomeShowListView', () => ({
  HomeShowListView: () => <div data-testid="home-show-list" />,
}))

vi.mock('@/features/shows/hooks/useSavedShows', () => ({
  SAVED_SHOWS_COLLAPSED_COUNT: 4,
  useSavedShows: () => ({
    data: {
      shows: [
        {
          id: 1,
          title: 'Show 1',
          slug: 'show-1',
          event_date: '2026-10-01T20:00:00-07:00',
          state: 'AZ',
          saved_at: '2026-09-20T12:00:00Z',
          artists: [{ id: 1, name: 'Artist 1' }],
          venues: [{ id: 1, name: 'Valley Bar', city: 'Phoenix', state: 'AZ' }],
        },
      ],
      total: 1,
    },
    isPending: false,
    error: null,
  }),
  useShowSaveCountBatch: () => ({ data: undefined }),
}))

vi.mock('@/components/shared/SaveButton', () => ({
  SaveButton: () => <button type="button">save</button>,
}))

const SECTIONS = {
  saved_shows: <SavedShowsModule nearbySectionId="nearby" />,
  nearby_shows: <NearbyShowsSection id="nearby" />,
  community_stats: <div>community_stats</div>,
  city_graph: <div>city_graph</div>,
  radio_shows: <div>radio_shows</div>,
}

/**
 * Holds every collapse in flight with an animation that does not finish on its
 * own. Returns a function that finishes the held ones, as the browser would.
 */
function holdCollapses() {
  const finishers: Array<() => void> = []
  vi.mocked(animateSlotHeight).mockImplementation(
    (_node, _direction, _reducedMotion, onFinish) => {
      finishers.push(onFinish)
      return { cancel: vi.fn() } as unknown as Animation
    }
  )
  return () => act(() => finishers.splice(0).forEach(finish => finish()))
}

/**
 * The one Discover row on the page and the surface it sits in. Counted with
 * `hidden: true`: a section mid-collapse is aria-hidden, and a row left
 * inside one would still be painted.
 */
function discoverRowSlot(): HomeDiscoverLinksSlot | 'unknown' {
  const rows = screen.queryAllByRole('navigation', {
    name: 'Discover',
    hidden: true,
  })
  expect(rows).toHaveLength(1)
  const row = rows[0]
  if (row.closest('#nearby')) return 'nearby'
  if (row.closest('section[aria-labelledby="signed-in-home-heading"]')) {
    return 'saved'
  }
  const surface = row.parentElement?.textContent ?? ''
  if (surface.includes('Your layout')) return 'toolbar'
  if (surface.includes('You have hidden every section.')) return 'empty'
  return 'unknown'
}

function cityLinkCount() {
  return screen.queryAllByRole('link', {
    name: /All upcoming shows/,
    hidden: true,
  }).length
}

const IDS = HOME_SECTIONS.map(section => section.id)

/** Every visibility combination of the five sections, all 32 of them. */
const COMBINATIONS = Array.from({ length: 2 ** IDS.length }, (_, mask) =>
  IDS.filter((_, bit) => mask & (1 << bit))
)

function layoutShowing(visible: readonly HomeSectionId[]) {
  return resolveHomeLayout(null).map(section => ({
    ...section,
    visible: visible.includes(section.id),
  }))
}

function expectedSlot(visible: readonly HomeSectionId[]): HomeDiscoverLinksSlot {
  if (visible.includes('nearby_shows')) return 'nearby'
  if (visible.includes('saved_shows')) return 'saved'
  if (visible.length > 0) return 'toolbar'
  return 'empty'
}

beforeEach(() => {
  layout = resolveHomeLayout(null)
  vi.mocked(animateSlotHeight).mockImplementation(() => null)
})

describe('the relocating link rows across every layout', () => {
  it.each(COMBINATIONS.map(visible => [visible.join(', ') || 'none', visible]))(
    'renders the Discover row and the city link exactly once (visible: %s)',
    (_name, visible) => {
      layout = layoutShowing(visible as HomeSectionId[])

      render(<HomeLayoutRuntime initialLayout={null} sections={SECTIONS} />)

      expect(discoverRowSlot()).toBe(expectedSlot(visible as HomeSectionId[]))
      expect(cityLinkCount()).toBe(1)
    }
  )

  it('moves the row live as sections are hidden one by one', () => {
    const { rerender } = render(
      <HomeLayoutRuntime initialLayout={null} sections={SECTIONS} />
    )
    expect(discoverRowSlot()).toBe('nearby')

    const steps: [HomeSectionId, HomeDiscoverLinksSlot][] = [
      ['nearby_shows', 'saved'],
      ['saved_shows', 'toolbar'],
      ['community_stats', 'toolbar'],
      ['city_graph', 'toolbar'],
      ['radio_shows', 'empty'],
    ]
    for (const [id, slot] of steps) {
      layout = setHomeSectionVisibility(layout, id, false)
      rerender(<HomeLayoutRuntime initialLayout={null} sections={SECTIONS} />)
      expect(discoverRowSlot()).toBe(slot)
    }

    // And back: re-showing a section takes the row off the empty state.
    layout = setHomeSectionVisibility(layout, 'nearby_shows', true)
    rerender(<HomeLayoutRuntime initialLayout={null} sections={SECTIONS} />)
    expect(discoverRowSlot()).toBe('nearby')
  })

  // A collapsing section stays mounted, so a row that followed the section's
  // presence instead of the slot would be painted twice for the animation.
  it('hands the row to the footer the moment the nearby section starts collapsing', async () => {
    const user = userEvent.setup()
    holdCollapses()
    render(<HomeLayoutRuntime initialLayout={null} sections={SECTIONS} />)
    expect(discoverRowSlot()).toBe('nearby')

    await user.click(screen.getByRole('button', { name: /^Customize home$/ }))
    layout = setHomeSectionVisibility(layout, 'nearby_shows', false)
    await user.click(
      screen.getByRole('checkbox', { name: 'Shows near you this week' })
    )

    expect(screen.getByTestId('home-show-list')).toBeInTheDocument()
    expect(discoverRowSlot()).toBe('saved')
    expect(cityLinkCount()).toBe(1)
  })

  // The empty state waits for the last section's collapse to finish. Until it
  // renders, the row stays where it already was (it never stops in the toolbar
  // on its way from a section to the empty-state line), and it moves there
  // once the collapse settles.
  it.each([
    ['radio_shows', 'Latest radio shows', 'toolbar'],
    ['nearby_shows', 'Shows near you this week', 'nearby'],
    ['saved_shows', 'Your upcoming shows', 'saved'],
  ] as const)(
    'keeps the row in place while %s collapses last, then moves it to the empty state',
    async (id, title, slot) => {
      const user = userEvent.setup()
      layout = layoutShowing([id])
      const finishCollapses = holdCollapses()
      render(<HomeLayoutRuntime initialLayout={null} sections={SECTIONS} />)
      expect(discoverRowSlot()).toBe(slot)

      await user.click(screen.getByRole('button', { name: /^Customize home$/ }))
      layout = layoutShowing([])
      await user.click(screen.getByRole('checkbox', { name: title }))

      expect(
        screen.queryByText(/You have hidden every section\./)
      ).not.toBeInTheDocument()
      expect(discoverRowSlot()).toBe(slot)
      expect(cityLinkCount()).toBe(1)

      finishCollapses()

      expect(
        screen.getByText(/You have hidden every section\./)
      ).toBeInTheDocument()
      expect(discoverRowSlot()).toBe('empty')
      expect(cityLinkCount()).toBe(1)
    }
  )
})
