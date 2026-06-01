import React from 'react'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import type { EntitySearchResult } from '@/lib/hooks/common/useEntitySearch'

// ── Mocks ──────────────────────────────────────────

vi.mock('@/components/shared', () => ({
  InlineErrorBanner: ({
    children,
    testId,
  }: {
    children: React.ReactNode
    testId?: string
  }) => (
    <div role="alert" data-testid={testId ?? 'inline-error'}>
      {children}
    </div>
  ),
}))

const emptyResults = {
  artists: [] as EntitySearchResult[],
  venues: [] as EntitySearchResult[],
  shows: [] as EntitySearchResult[],
  releases: [] as EntitySearchResult[],
  labels: [] as EntitySearchResult[],
  festivals: [] as EntitySearchResult[],
  tags: [] as EntitySearchResult[],
}

const mockUseEntitySearch = vi.fn(() => ({
  data: emptyResults,
  isSearching: false,
  searchError: false,
}))
vi.mock('@/lib/hooks/common/useEntitySearch', () => ({
  useEntitySearch: () => mockUseEntitySearch(),
  ENTITY_SEARCH_UNAVAILABLE_MESSAGE: 'Search is temporarily unavailable.',
}))

import { FulfillmentEntityPicker } from './FulfillmentEntityPicker'

function artistRow(
  overrides: Partial<EntitySearchResult> = {}
): EntitySearchResult {
  return {
    id: 1,
    slug: 'slowdive',
    name: 'Slowdive',
    subtitle: 'Reading, UK',
    entityType: 'artist',
    href: '/artists/slowdive',
    ...overrides,
  }
}

function setResults(partial: Partial<typeof emptyResults>) {
  mockUseEntitySearch.mockReturnValue({
    data: { ...emptyResults, ...partial },
    isSearching: false,
    searchError: false,
  })
}

describe('FulfillmentEntityPicker', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockUseEntitySearch.mockReturnValue({
      data: emptyResults,
      isSearching: false,
      searchError: false,
    })
  })

  it('confirm is disabled until an entity is selected', async () => {
    const user = userEvent.setup()
    setResults({ artists: [artistRow()] })
    render(
      <FulfillmentEntityPicker
        entityType="artist"
        onSubmit={vi.fn()}
        onCancel={vi.fn()}
      />
    )

    expect(
      screen.getByTestId('fulfillment-entity-picker-confirm')
    ).toBeDisabled()

    await user.type(
      screen.getByTestId('fulfillment-entity-picker-search-input'),
      'slow'
    )
    await user.click(screen.getByTestId('fulfillment-entity-picker-result-row'))

    expect(
      screen.getByTestId('fulfillment-entity-picker-confirm')
    ).toBeEnabled()
  })

  it('calls onSubmit with the selected entity id', async () => {
    const user = userEvent.setup()
    const onSubmit = vi.fn()
    setResults({ artists: [artistRow({ id: 777 })] })
    render(
      <FulfillmentEntityPicker
        entityType="artist"
        onSubmit={onSubmit}
        onCancel={vi.fn()}
      />
    )

    await user.type(
      screen.getByTestId('fulfillment-entity-picker-search-input'),
      'slow'
    )
    await user.click(screen.getByTestId('fulfillment-entity-picker-result-row'))
    await user.click(screen.getByTestId('fulfillment-entity-picker-confirm'))

    expect(onSubmit).toHaveBeenCalledWith(777)
  })

  it('scopes results to the request entity_type (ignores other-type matches)', async () => {
    const user = userEvent.setup()
    // The hook returns a venue too, but a venue request must only show venues.
    setResults({
      artists: [artistRow({ name: 'Should Not Show' })],
      venues: [
        {
          id: 9,
          slug: 'valley-bar',
          name: 'Valley Bar',
          subtitle: 'Phoenix, AZ',
          entityType: 'venue',
          href: '/venues/valley-bar',
        },
      ],
    })
    render(
      <FulfillmentEntityPicker
        entityType="venue"
        onSubmit={vi.fn()}
        onCancel={vi.fn()}
      />
    )

    await user.type(
      screen.getByTestId('fulfillment-entity-picker-search-input'),
      'val'
    )

    expect(screen.getByText('Valley Bar')).toBeInTheDocument()
    expect(screen.queryByText('Should Not Show')).not.toBeInTheDocument()
  })

  it('renders an inline submitError (e.g. backend type-mismatch 400)', () => {
    render(
      <FulfillmentEntityPicker
        entityType="artist"
        submitError="Entity type does not match the request"
        onSubmit={vi.fn()}
        onCancel={vi.fn()}
      />
    )
    expect(
      screen.getByTestId('fulfillment-entity-picker-submit-error')
    ).toHaveTextContent('Entity type does not match the request')
  })

  it('surfaces the search-unavailable banner when search errors', async () => {
    const user = userEvent.setup()
    mockUseEntitySearch.mockReturnValue({
      data: emptyResults,
      isSearching: false,
      searchError: true,
    })
    render(
      <FulfillmentEntityPicker
        entityType="artist"
        onSubmit={vi.fn()}
        onCancel={vi.fn()}
      />
    )

    await user.type(
      screen.getByTestId('fulfillment-entity-picker-search-input'),
      'slow'
    )
    expect(
      screen.getByTestId('fulfillment-entity-picker-search-error')
    ).toBeInTheDocument()
  })

  it('clears the selection when the query changes', async () => {
    const user = userEvent.setup()
    setResults({ artists: [artistRow()] })
    render(
      <FulfillmentEntityPicker
        entityType="artist"
        onSubmit={vi.fn()}
        onCancel={vi.fn()}
      />
    )

    const input = screen.getByTestId('fulfillment-entity-picker-search-input')
    await user.type(input, 'slow')
    await user.click(screen.getByTestId('fulfillment-entity-picker-result-row'))
    expect(
      screen.getByTestId('fulfillment-entity-picker-confirm')
    ).toBeEnabled()

    // Typing again invalidates the prior pick — confirm goes back to disabled.
    await user.type(input, 'x')
    expect(
      screen.getByTestId('fulfillment-entity-picker-confirm')
    ).toBeDisabled()
  })

  it('calls onCancel when Cancel is clicked', async () => {
    const user = userEvent.setup()
    const onCancel = vi.fn()
    render(
      <FulfillmentEntityPicker
        entityType="artist"
        onSubmit={vi.fn()}
        onCancel={onCancel}
      />
    )
    await user.click(screen.getByRole('button', { name: /Cancel/i }))
    expect(onCancel).toHaveBeenCalled()
  })
})
