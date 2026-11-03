import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { HomeLayoutRuntime } from './HomeLayoutRuntime'
import {
  moveHomeSection,
  resolveHomeLayout,
  setHomeSectionVisibility,
  type ResolvedHomeSection,
} from '../sections'

let layout: ResolvedHomeSection[] = resolveHomeLayout(null)
let hasWriteFailed = false

vi.mock('../hooks/useHomeLayout', () => ({
  useHomeLayout: () => ({
    sections: layout,
    isReady: true,
    status: 'ready',
    retry: vi.fn(),
    hasStoredLayout: true,
  }),
  useHomeLayoutWriteFailed: () => hasWriteFailed,
  usePersistHomeLayout: () => ({
    persist: vi.fn(),
    reset: vi.fn(),
    isResetting: false,
  }),
}))

const mockRefresh = vi.fn()
vi.mock('next/navigation', () => ({
  useRouter: () => ({ refresh: mockRefresh, push: vi.fn() }),
}))

let authStatus: 'pending' | 'authenticated' | 'anonymous' = 'authenticated'
vi.mock('@/lib/context/AuthContext', () => ({
  useAuthContext: () => ({
    authStatus,
    isAuthenticated: authStatus === 'authenticated',
    user: authStatus === 'authenticated' ? { id: '42' } : null,
  }),
}))

vi.mock('./HomeCityShowsLink', async importOriginal => ({
  ...(await importOriginal<object>()),
  ResolvedHomeCityShowsLink: () => (
    <span data-testid="relocated-city-link">All upcoming shows →</span>
  ),
}))

// The card's own eligibility is PasskeyOfferCard.test.tsx's; here it stands
// in so the test can place it.
vi.mock('./PasskeyOfferCard', () => ({
  PasskeyOfferCard: ({ onLeave }: { onLeave?: () => void }) => (
    <button type="button" data-testid="passkey-offer" onClick={onLeave}>
      Not now
    </button>
  ),
}))

const SECTIONS = {
  saved_shows: <div data-testid="section">saved_shows</div>,
  nearby_shows: <div data-testid="section">nearby_shows</div>,
  community_stats: <div data-testid="section">community_stats</div>,
  city_graph: <div data-testid="section">city_graph</div>,
  radio_shows: <div data-testid="section">radio_shows</div>,
}

function renderedOrder() {
  return screen.queryAllByTestId('section').map(node => node.textContent)
}

function customizeButtons() {
  return screen.queryAllByRole('button', { name: 'Customize home →' })
}

/** Whether `a` comes before `b` in document order. */
function precedes(a: Node, b: Node) {
  return !!(a.compareDocumentPosition(b) & Node.DOCUMENT_POSITION_FOLLOWING)
}

function section(name: string) {
  const node = screen
    .getAllByTestId('section')
    .find(candidate => candidate.textContent === name)
  if (!node) throw new Error(`section ${name} is not rendered`)
  return node
}

beforeEach(() => {
  layout = resolveHomeLayout(null)
  hasWriteFailed = false
  authStatus = 'authenticated'
  mockRefresh.mockClear()
})

describe('HomeLayoutRuntime', () => {
  it('hands focus to the page heading when the passkey offer leaves', async () => {
    const user = userEvent.setup()
    render(<HomeLayoutRuntime initialLayout={null} sections={SECTIONS} />)

    await user.click(screen.getByTestId('passkey-offer'))

    expect(screen.getByRole('heading', { level: 1, name: 'Home' })).toHaveFocus()
  })

  it('places the passkey offer under the saved-shows module, not above the sections', () => {
    render(<HomeLayoutRuntime initialLayout={null} sections={SECTIONS} />)

    const offer = screen.getByTestId('passkey-offer')
    expect(precedes(section('saved_shows'), offer)).toBe(true)
    expect(precedes(offer, section('nearby_shows'))).toBe(true)
  })

  it('follows the saved-shows module wherever the viewer moved it', () => {
    layout = moveHomeSection(
      moveHomeSection(resolveHomeLayout(null), 'saved_shows', 'down')!,
      'saved_shows',
      'down'
    )!
    render(<HomeLayoutRuntime initialLayout={null} sections={SECTIONS} />)

    const offer = screen.getByTestId('passkey-offer')
    expect(renderedOrder().slice(0, 3)).toEqual([
      'nearby_shows',
      'community_stats',
      'saved_shows',
    ])
    expect(precedes(section('saved_shows'), offer)).toBe(true)
    expect(precedes(offer, section('radio_shows'))).toBe(true)
  })

  it('places the passkey offer under the first visible section when saved shows is hidden', () => {
    layout = setHomeSectionVisibility(resolveHomeLayout(null), 'saved_shows', false)
    render(<HomeLayoutRuntime initialLayout={null} sections={SECTIONS} />)

    const offer = screen.getByTestId('passkey-offer')
    expect(renderedOrder()[0]).toBe('nearby_shows')
    expect(precedes(section('nearby_shows'), offer)).toBe(true)
    expect(precedes(offer, section('community_stats'))).toBe(true)
  })

  it('places the passkey offer under the empty-state line when every section is hidden', () => {
    layout = resolveHomeLayout(null).map(section => ({
      ...section,
      visible: false,
    }))
    render(<HomeLayoutRuntime initialLayout={null} sections={SECTIONS} />)

    const offer = screen.getByTestId('passkey-offer')
    expect(
      precedes(screen.getByText('You have hidden every section.'), offer)
    ).toBe(true)
    expect(precedes(offer, customizeButtons()[0])).toBe(true)
  })

  it('renders the shipped order with one Customize link after the last section and no kicker', () => {
    render(<HomeLayoutRuntime initialLayout={null} sections={SECTIONS} />)

    expect(renderedOrder()).toEqual([
      'saved_shows',
      'nearby_shows',
      'community_stats',
      'radio_shows',
      'city_graph',
    ])
    expect(screen.queryByText(/sections · Your layout/)).not.toBeInTheDocument()
    expect(customizeButtons()).toHaveLength(1)
    expect(precedes(section('city_graph'), customizeButtons()[0])).toBe(true)
  })

  it('opens the Customize popover from the page-end link', async () => {
    const user = userEvent.setup()
    render(<HomeLayoutRuntime initialLayout={null} sections={SECTIONS} />)

    await user.click(customizeButtons()[0])

    expect(
      screen.getByRole('dialog', { name: 'Customize home' })
    ).toBeInTheDocument()
    expect(
      screen.getByRole('checkbox', { name: 'Community stats' })
    ).toBeInTheDocument()
    expect(screen.getByRole('link', { name: 'All settings →' })).toBeInTheDocument()
  })

  it('renders a custom order on the FIRST paint, with no reflow', () => {
    layout = moveHomeSection(resolveHomeLayout(null), 'city_graph', 'up')!

    render(<HomeLayoutRuntime initialLayout={null} sections={SECTIONS} />)

    expect(renderedOrder()).toEqual([
      'saved_shows',
      'nearby_shows',
      'community_stats',
      'city_graph',
      'radio_shows',
    ])
  })

  it('does not mount a hidden section', () => {
    layout = setHomeSectionVisibility(
      resolveHomeLayout(null),
      'community_stats',
      false
    )

    render(<HomeLayoutRuntime initialLayout={null} sections={SECTIONS} />)

    expect(renderedOrder()).toEqual([
      'saved_shows',
      'nearby_shows',
      'radio_shows',
      'city_graph',
    ])
  })

  it('shows one line and exactly one Customize control when every section is hidden', () => {
    layout = resolveHomeLayout(null).map(section => ({
      ...section,
      visible: false,
    }))

    render(<HomeLayoutRuntime initialLayout={null} sections={SECTIONS} />)

    expect(renderedOrder()).toEqual([])
    expect(
      screen.getByText(/You have hidden every section\./)
    ).toBeInTheDocument()
    expect(
      screen.getAllByRole('button', { name: /customize home/i })
    ).toHaveLength(1)
  })

  it('carries the city link on the page-end row only when both carriers are hidden', () => {
    render(<HomeLayoutRuntime initialLayout={null} sections={SECTIONS} />)
    expect(screen.queryByTestId('relocated-city-link')).not.toBeInTheDocument()
  })

  it('moves the city link to the page-end row, above Customize, when saved and nearby are hidden', () => {
    layout = setHomeSectionVisibility(
      setHomeSectionVisibility(resolveHomeLayout(null), 'nearby_shows', false),
      'saved_shows',
      false
    )

    render(<HomeLayoutRuntime initialLayout={null} sections={SECTIONS} />)

    const cityLink = screen.getByTestId('relocated-city-link')
    expect(precedes(section('city_graph'), cityLink)).toBe(true)
    expect(precedes(cityLink, customizeButtons()[0])).toBe(true)
  })

  // The empty state is read off what is RENDERED, not off the layout. Reading
  // the layout would call the page empty the instant the last section's hide
  // commits, unmounting it before its collapse finished and stranding it in
  // the in-flight set, where it would keep that hidden section mounted for the
  // life of the page.
  it('still paints the last section while it is collapsing', async () => {
    const user = userEvent.setup()
    render(<HomeLayoutRuntime initialLayout={null} sections={SECTIONS} />)

    await user.click(customizeButtons()[0])
    for (const section of resolveHomeLayout(null)) {
      layout = setHomeSectionVisibility(layout, section.id, false)
    }
    await user.click(
      screen.getByRole('checkbox', { name: 'Latest radio shows' })
    )

    // jsdom has no Web Animations, so the collapse settles synchronously and
    // the page reaches its empty state rather than hanging in the transition.
    expect(
      screen.getByText(/You have hidden every section\./)
    ).toBeInTheDocument()
    expect(renderedOrder()).toEqual([])
    // The popover's anchor survived the page emptying, so it is still open.
    expect(
      screen.getByRole('dialog', { name: 'Customize home' })
    ).toBeInTheDocument()
  })

  // The page's h1 lives on the shell, not in a section, because every section
  // is hideable and a heading inside one would take the document's top-level
  // heading with it.
  it('keeps exactly one h1 whatever the viewer hides', () => {
    layout = setHomeSectionVisibility(
      resolveHomeLayout(null),
      'saved_shows',
      false
    )
    const { rerender } = render(
      <HomeLayoutRuntime initialLayout={null} sections={SECTIONS} />
    )
    expect(screen.getAllByRole('heading', { level: 1 })).toHaveLength(1)

    layout = resolveHomeLayout(null).map(section => ({
      ...section,
      visible: false,
    }))
    rerender(<HomeLayoutRuntime initialLayout={null} sections={SECTIONS} />)
    expect(screen.getAllByRole('heading', { level: 1 })).toHaveLength(1)
  })

  // The write outlives the popover, so the failure has to be reported by
  // something the popover closing cannot unmount.
  it('reports a failed write beside the Customize link, outside the popover', () => {
    hasWriteFailed = true
    render(<HomeLayoutRuntime initialLayout={null} sections={SECTIONS} />)

    const alert = screen.getByRole('alert')
    expect(alert).toHaveTextContent('Could not save your layout. Try again.')
    expect(precedes(customizeButtons()[0], alert)).toBe(true)
    expect(precedes(section('city_graph'), alert)).toBe(true)
    // The popover is closed; the line is on the page regardless.
    expect(
      screen.queryByRole('checkbox', { name: 'Community stats' })
    ).not.toBeInTheDocument()
  })

  // The post-logout re-pick used to live inside the saved-shows section, which
  // a viewer can hide. Hidden, nothing asked the server for the anonymous page
  // and the signed-out viewer kept the previous account's layout on screen.
  it('asks the server to re-pick the variant when the viewer signs out, with every section hidden', () => {
    authStatus = 'anonymous'
    layout = resolveHomeLayout(null).map(section => ({
      ...section,
      visible: false,
    }))

    render(<HomeLayoutRuntime initialLayout={null} sections={SECTIONS} />)

    expect(mockRefresh).toHaveBeenCalledTimes(1)
    expect(
      screen.queryByRole('button', { name: /customize home/i })
    ).not.toBeInTheDocument()
    expect(renderedOrder()).toEqual([])
  })
})
