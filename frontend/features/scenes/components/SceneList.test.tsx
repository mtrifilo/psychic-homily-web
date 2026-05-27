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
    },
    {
      city: 'Tucson',
      state: 'AZ',
      slug: 'tucson-az',
      venue_count: 1,
      upcoming_show_count: 0,
      total_show_count: 1,
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
  })
})
