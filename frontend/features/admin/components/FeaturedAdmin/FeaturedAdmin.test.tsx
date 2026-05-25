/**
 * @vitest-environment jsdom
 */
import React from 'react'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { screen } from '@testing-library/react'
import { renderWithProviders } from '@/test/utils'

import type {
  ExploreFeaturedBill,
  ExploreFeaturedCollection,
  ExploreFeaturedResponse,
} from './types'

const mockUseExploreFeatured = vi.fn()
const mockUseSetFeaturedSlot = vi.fn()
const mockUseRetireFeaturedSlot = vi.fn()

vi.mock('./useFeaturedSlots', () => ({
  useExploreFeatured: () => mockUseExploreFeatured(),
  useSetFeaturedSlot: () => mockUseSetFeaturedSlot(),
  useRetireFeaturedSlot: () => mockUseRetireFeaturedSlot(),
}))

// useEntitySearch (cross-entity search hook) makes seven backend calls
// in its real implementation. The bill picker doesn't render in the
// admin-only initial state, but the hook still mounts under the
// SetBillForm — stub it so tests don't hit fetch.
vi.mock('@/lib/hooks/common/useEntitySearch', async () => {
  const actual = await vi.importActual<
    typeof import('@/lib/hooks/common/useEntitySearch')
  >('@/lib/hooks/common/useEntitySearch')
  return {
    ...actual,
    useEntitySearch: () => ({
      data: {
        artists: [],
        venues: [],
        shows: [],
        releases: [],
        labels: [],
        festivals: [],
        tags: [],
      },
      isSearching: false,
      searchError: false,
      totalResults: 0,
    }),
  }
})

import { FeaturedAdmin } from './FeaturedAdmin'

function makeBill(overrides: Partial<ExploreFeaturedBill> = {}): ExploreFeaturedBill {
  return {
    id: 101,
    slug: 'faetooth-at-valley-bar',
    title: 'Faetooth at Valley Bar',
    event_date: '2026-06-01T03:00:00Z',
    headliner_name: 'Faetooth',
    venue_name: 'Valley Bar',
    venue_city: 'Phoenix',
    venue_state: 'AZ',
    image_url: null,
    curator_note: '**Sharp bill.**',
    curator_note_html: '<p><strong>Sharp bill.</strong></p>',
    ...overrides,
  }
}

function makeCollection(
  overrides: Partial<ExploreFeaturedCollection> = {}
): ExploreFeaturedCollection {
  return {
    id: 42,
    slug: 'phx-noise-2026',
    title: 'PHX Noise 2026',
    description: 'A snapshot of the noise scene.',
    description_html: '<p>A snapshot of the noise scene.</p>',
    cover_image_url: null,
    curator_note: 'Bookmark this.',
    curator_note_html: '<p>Bookmark this.</p>',
    ...overrides,
  }
}

function mockExploreFeatured(data: ExploreFeaturedResponse | null) {
  mockUseExploreFeatured.mockReturnValue({
    data,
    isLoading: false,
    isError: false,
  })
}

function defaultMutationStubs() {
  mockUseSetFeaturedSlot.mockReturnValue({
    mutate: vi.fn(),
    isPending: false,
  })
  mockUseRetireFeaturedSlot.mockReturnValue({
    mutate: vi.fn(),
    isPending: false,
  })
}

describe('FeaturedAdmin', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    defaultMutationStubs()
  })

  it('renders both Bill and Collection panels', () => {
    mockExploreFeatured({ bill: null, collection: null })

    renderWithProviders(<FeaturedAdmin />)

    expect(
      screen.getByTestId('featured-admin-panel-bill')
    ).toBeInTheDocument()
    expect(
      screen.getByTestId('featured-admin-panel-collection')
    ).toBeInTheDocument()
  })

  it('shows the empty-state card when no active slot is set', () => {
    mockExploreFeatured({ bill: null, collection: null })

    renderWithProviders(<FeaturedAdmin />)

    expect(
      screen.getByTestId('featured-admin-empty-bill')
    ).toBeInTheDocument()
    expect(
      screen.getByTestId('featured-admin-empty-collection')
    ).toBeInTheDocument()
  })

  it('renders the active bill card with name, venue, and curator note', () => {
    mockExploreFeatured({
      bill: makeBill(),
      collection: null,
    })

    renderWithProviders(<FeaturedAdmin />)

    const card = screen.getByTestId('featured-admin-active-bill')
    expect(card).toBeInTheDocument()
    // Headliner is what the consumer (curator) recognises.
    expect(card).toHaveTextContent('Faetooth')
    expect(card).toHaveTextContent('Valley Bar')
    // Curator-note HTML rendered (markdown bold survives the renderer).
    const note = screen.getByTestId('featured-admin-active-bill-note')
    expect(note.innerHTML).toContain('<strong>Sharp bill.</strong>')
    // Retire button surfaces only on the active panel.
    expect(
      screen.getByTestId('featured-admin-retire-bill')
    ).toBeEnabled()
  })

  it('renders the active collection card with title and curator note', () => {
    mockExploreFeatured({
      bill: null,
      collection: makeCollection(),
    })

    renderWithProviders(<FeaturedAdmin />)

    const card = screen.getByTestId('featured-admin-active-collection')
    expect(card).toBeInTheDocument()
    expect(card).toHaveTextContent('PHX Noise 2026')
    const note = screen.getByTestId('featured-admin-active-collection-note')
    expect(note.innerHTML).toContain('Bookmark this.')
    expect(
      screen.getByTestId('featured-admin-retire-collection')
    ).toBeEnabled()
  })

  it('shows a fallback error banner when the explore endpoint errors', () => {
    mockUseExploreFeatured.mockReturnValue({
      data: undefined,
      isLoading: false,
      isError: true,
    })

    renderWithProviders(<FeaturedAdmin />)

    expect(
      screen.getByTestId('featured-admin-load-error')
    ).toBeInTheDocument()
  })

  it('shows the spinner placeholder while the active state is loading', () => {
    mockUseExploreFeatured.mockReturnValue({
      data: undefined,
      isLoading: true,
      isError: false,
    })

    renderWithProviders(<FeaturedAdmin />)

    expect(
      screen.getByTestId('featured-admin-loading-bill')
    ).toBeInTheDocument()
    expect(
      screen.getByTestId('featured-admin-loading-collection')
    ).toBeInTheDocument()
  })

  it('disables the Save button until an entity is picked', () => {
    mockExploreFeatured({ bill: null, collection: null })

    renderWithProviders(<FeaturedAdmin />)

    expect(screen.getByTestId('featured-admin-save-bill')).toBeDisabled()
    expect(
      screen.getByTestId('featured-admin-save-collection')
    ).toBeDisabled()
  })
})
