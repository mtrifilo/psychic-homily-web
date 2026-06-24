/**
 * PSY-857: AddItemsPicker ↔ AICollectionFiller integration.
 *
 * The main AddItemsPicker.test.tsx mocks `./AICollectionFiller` (a stub) so it
 * can test the picker chrome in isolation. This file does the opposite: it
 * renders the REAL AICollectionFiller inside AddItemsPicker, mocking only the
 * extraction hook, to prove the seam between them — that items staged from the
 * AI pane flow through AddItemsPicker's stageBatch and reach
 * `onStagedItemsChange`. That wiring (the `onStageItems={stageBatch}` +
 * `alreadyStaged` predicate hand-off) is invisible to both components'
 * standalone suites.
 *
 * Kept in a separate file because vi.mock is hoisted file-wide — this file
 * must NOT mock ./AICollectionFiller, whereas the sibling suite must.
 */
import { describe, it, expect, vi, beforeEach } from 'vitest'
import React from 'react'
import { screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { render } from '@/test/utils'

import { AddItemsPicker } from './AddItemsPicker'
import type { ExtractedCollectionData } from '@/lib/types/extraction'

// ──────────────────────────────────────────────
// Mocks — extraction hook is the only seam we drive; everything else is the
// real component so the staged-items hand-off is exercised end-to-end.
// ──────────────────────────────────────────────

vi.mock('use-debounce', () => ({
  useDebounce: (value: unknown) => [value],
}))

// Search hook + helpers (AddItemsPicker's Search/Paste tabs). The AI test
// never touches search, but the module must export the full surface.
vi.mock('@/lib/hooks/common/useEntitySearch', () => ({
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
    totalResults: 0,
    searchError: false,
  }),
  ENTITY_SEARCH_UNAVAILABLE_MESSAGE:
    'Search is temporarily unavailable. Try again in a moment.',
  fetchEntitySearch: vi.fn(async () => ({
    results: {
      artists: [],
      venues: [],
      shows: [],
      releases: [],
      labels: [],
      festivals: [],
      tags: [],
    },
    allFailed: false,
  })),
  flattenEntitySearchResults: () => [],
}))

vi.mock('@/lib/api', () => ({
  apiRequest: vi.fn(async () => ({})),
  API_ENDPOINTS: {
    COLLECTIONS: { ENTITY_REQUESTS: 'http://test/entity-requests' },
  },
}))

// Both hooks come from the `../hooks` barrel: useResolveCollectionItems (paste
// resolver, AddItemsPicker) AND useCollectionExtraction (AI pane). The AI test
// drives the extraction hook; the resolver is a no-op stub.
let mockExtractResult: { data?: ExtractedCollectionData; warnings?: string[] } =
  { data: undefined, warnings: undefined }
vi.mock('../hooks', () => ({
  useResolveCollectionItems: () => ({
    mutate: vi.fn(),
    isPending: false,
    error: null,
  }),
  useCollectionExtraction: () => ({
    mutate: (
      _input: unknown,
      opts?: { onSuccess?: (data: typeof mockExtractResult) => void }
    ) => {
      opts?.onSuccess?.(mockExtractResult)
    },
    isPending: false,
    error: null,
    reset: vi.fn(),
  }),
}))

// AICollectionFiller reads the tier from auth context — anonymous is fine here
// (the test only exercises MATCH rows, which have no tier-gated affordance).
vi.mock('@/lib/context/AuthContext', () => ({
  useAuthContext: () => ({ user: null }),
}))

// Dumb primitive mocks so testids + text survive (same approach as the sibling
// suites). Both components share these.
vi.mock('@/components/ui/button', () => ({
  Button: ({
    children,
    onClick,
    disabled,
    ...props
  }: {
    children: React.ReactNode
    onClick?: () => void
    disabled?: boolean
    [key: string]: unknown
  }) => (
    <button
      onClick={onClick}
      disabled={disabled}
      {...(props as Record<string, unknown>)}
    >
      {children}
    </button>
  ),
}))

vi.mock('@/components/ui/input', () => ({
  Input: (props: React.InputHTMLAttributes<HTMLInputElement>) => (
    <input {...props} />
  ),
}))

vi.mock('@/components/ui/badge', () => ({
  Badge: ({
    children,
    variant: _variant,
    ...props
  }: {
    children: React.ReactNode
    variant?: string
    [key: string]: unknown
  }) => <span {...(props as Record<string, unknown>)}>{children}</span>,
}))

let activeOnValueChange: ((v: string) => void) | undefined
vi.mock('@/components/ui/tabs', () => ({
  Tabs: ({
    children,
    onValueChange,
  }: {
    children: React.ReactNode
    value: string
    onValueChange: (v: string) => void
  }) => {
    activeOnValueChange = onValueChange
    return <div data-testid="tabs">{children}</div>
  },
  TabsList: ({ children }: { children: React.ReactNode }) => (
    <div>{children}</div>
  ),
  TabsTrigger: ({
    children,
    value,
    disabled,
  }: {
    children: React.ReactNode
    value: string
    disabled?: boolean
  }) => (
    <button
      data-testid={`tab-${value}`}
      role="tab"
      disabled={disabled}
      onClick={() => activeOnValueChange?.(value)}
    >
      {children}
    </button>
  ),
}))

vi.mock('@/components/shared', () => ({
  InlineErrorBanner: ({
    children,
    testId,
  }: {
    children: React.ReactNode
    testId?: string
  }) => <div data-testid={testId}>{children}</div>,
}))

describe('AddItemsPicker ↔ AICollectionFiller integration', () => {
  beforeEach(() => {
    mockExtractResult = { data: undefined, warnings: undefined }
  })

  it('stages AI-extracted matched items through AddItemsPicker to onStagedItemsChange', async () => {
    mockExtractResult = {
      data: {
        source: 'Pitchfork — Best Albums of the 2010s',
        items: [
          {
            artist_name: 'Kendrick Lamar',
            release_title: 'To Pimp a Butterfly',
            matched_artist_id: 42,
            matched_artist_name: 'Kendrick Lamar',
          },
          {
            artist_name: 'Frank Ocean',
            release_title: 'Blonde',
            matched_artist_id: 43,
            matched_artist_name: 'Frank Ocean',
          },
        ],
      },
    }
    const onStaged = vi.fn()
    const user = userEvent.setup()
    render(<AddItemsPicker stagedItems={[]} onStagedItemsChange={onStaged} />)

    // Switch to the AI tab — this mounts the REAL AICollectionFiller (no stub).
    await user.click(screen.getByTestId('tab-ai'))
    expect(
      screen.getByTestId('ai-collection-filler')
    ).toBeInTheDocument()

    // Paste text + Extract → the mocked hook fires onSuccess with the fixture.
    await user.type(
      screen.getByTestId('ai-collection-filler-textarea'),
      '1. Kendrick Lamar — TPAB\n2. Frank Ocean — Blonde'
    )
    await user.click(screen.getByTestId('ai-collection-filler-extract'))

    // Both matched rows rendered inside the real filler.
    expect(screen.getByText('Kendrick Lamar')).toBeInTheDocument()
    expect(screen.getByText('Frank Ocean')).toBeInTheDocument()

    // "Add all matched" → AICollectionFiller calls onStageItems(batch), which
    // AddItemsPicker wires to stageBatch → onStagedItemsChange (ONE call).
    await user.click(
      screen.getByTestId('ai-collection-filler-add-all-matched')
    )

    expect(onStaged).toHaveBeenCalledTimes(1)
    const staged = onStaged.mock.calls[0][0] as Array<{
      entityType: string
      entityId: number
    }>
    expect(staged).toHaveLength(2)
    expect(staged.map((s) => s.entityId).sort()).toEqual([42, 43])
    expect(staged.every((s) => s.entityType === 'artist')).toBe(true)
  })
})
