import { describe, it, expect, vi, beforeEach } from 'vitest'
import { screen } from '@testing-library/react'
import { renderWithProviders } from '@/test/utils'
import type { SceneListResponse } from '../types'

// PSY-690: SceneList is a thin presentational wrapper over useScenes with four
// branches — loading, error, empty, and the populated card grid. Mock the hook
// so each branch can be driven directly.

// Mock next/link so the grid renders plain anchors in jsdom.
vi.mock('next/link', () => ({
  default: ({
    href,
    children,
    ...rest
  }: {
    href: string
    children: React.ReactNode
  }) => (
    <a href={href} {...rest}>
      {children}
    </a>
  ),
}))

const mockUseScenes = vi.fn()
vi.mock('../hooks', () => ({
  useScenes: () => mockUseScenes(),
}))

import { SceneList } from './SceneList'

const sampleData: SceneListResponse = {
  scenes: [
    {
      city: 'Phoenix',
      state: 'AZ',
      slug: 'phoenix-az',
      venue_count: 12,
      upcoming_show_count: 45,
      total_show_count: 200,
      shows_this_week: 23,
    },
    {
      city: 'Tucson',
      state: 'AZ',
      slug: 'tucson-az',
      venue_count: 1,
      upcoming_show_count: 0,
      total_show_count: 1,
      shows_this_week: 0,
    },
  ],
  count: 2,
}

describe('SceneList', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('renders a loading spinner while fetching', () => {
    mockUseScenes.mockReturnValue({ data: undefined, isLoading: true, error: null })
    renderWithProviders(<SceneList />)
    // LoadingSpinner exposes role="status" so the loading state is
    // announceable to assistive tech and addressable by tests.
    expect(screen.getByRole('status', { name: /loading/i })).toBeInTheDocument()
    // No scene cards while loading.
    expect(screen.queryByText(/, AZ$/)).not.toBeInTheDocument()
  })

  it('renders an error message when the request fails', () => {
    mockUseScenes.mockReturnValue({
      data: undefined,
      isLoading: false,
      error: new Error('boom'),
    })
    renderWithProviders(<SceneList />)
    expect(screen.getByText(/Failed to load scenes/i)).toBeInTheDocument()
  })

  it('renders the empty state when there are no scenes', () => {
    mockUseScenes.mockReturnValue({
      data: { scenes: [], count: 0 },
      isLoading: false,
      error: null,
    })
    renderWithProviders(<SceneList />)
    expect(screen.getByText('No scenes yet')).toBeInTheDocument()
    expect(
      screen.getByText(/Scene pages appear for cities with venue and show activity/i)
    ).toBeInTheDocument()
  })

  describe('populated grid', () => {
    beforeEach(() => {
      mockUseScenes.mockReturnValue({
        data: sampleData,
        isLoading: false,
        error: null,
      })
    })

    it('renders a card per scene linking to its detail page', () => {
      renderWithProviders(<SceneList />)

      expect(screen.getByText('Phoenix, AZ')).toBeInTheDocument()
      expect(screen.getByText('Tucson, AZ')).toBeInTheDocument()

      const phoenixLink = screen.getByText('Phoenix, AZ').closest('a')!
      expect(phoenixLink).toHaveAttribute('href', '/scenes/phoenix-az')
      const tucsonLink = screen.getByText('Tucson, AZ').closest('a')!
      expect(tucsonLink).toHaveAttribute('href', '/scenes/tucson-az')
    })

    it('pluralizes venue and show counts', () => {
      renderWithProviders(<SceneList />)
      // Phoenix: plural forms.
      expect(screen.getByText('12 venues')).toBeInTheDocument()
      expect(screen.getByText('200 shows')).toBeInTheDocument()
      // Tucson: singular forms (count === 1).
      expect(screen.getByText('1 venue')).toBeInTheDocument()
      expect(screen.getByText('1 show')).toBeInTheDocument()
    })

    it('shows the upcoming badge only when upcoming_show_count > 0', () => {
      renderWithProviders(<SceneList />)
      // Phoenix has 45 upcoming.
      expect(screen.getByText('45 upcoming')).toBeInTheDocument()
      // Tucson has 0 upcoming — no upcoming label rendered.
      expect(screen.queryByText('0 upcoming')).not.toBeInTheDocument()
    })

    // PSY-1623: the week page has no other inbound link from this page, so the
    // count doubles as the link to it — for the quiet scenes too, or the edge
    // would come and go with the week's programming.
    it('links every card to the scene week page', () => {
      renderWithProviders(<SceneList />)

      const phoenixWeek = screen.getByRole('link', { name: /23 shows this week/ })
      expect(phoenixWeek).toHaveAttribute('href', '/scenes/phoenix-az/week')

      const tucsonWeek = screen.getByRole('link', { name: /No shows this week/ })
      expect(tucsonWeek).toHaveAttribute('href', '/scenes/tucson-az/week')
    })

    // Nesting the week link inside the card's own link would be invalid HTML —
    // the parser lifts the inner anchor out, so what a crawler is handed stops
    // matching what was written. The card link is stretched over the card with
    // a pseudo-element instead.
    it('keeps the two card links siblings rather than nested', () => {
      const { container } = renderWithProviders(<SceneList />)

      expect(container.querySelector('a a')).toBeNull()
    })
  })
})
