import { describe, it, expect, vi, beforeEach } from 'vitest'
import React from 'react'
import { screen, act, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
// Use the providers-wrapped render: the local queue-for-review mutation
// (useQueueEntityRequest) calls useMutation, which needs a QueryClient in
// context. renderWithProviders supplies one (re-exported here as `render`).
import { render } from '@/test/utils'

import {
  AddItemsPicker,
  parsePasteLine,
  reorderStagedItems,
} from './AddItemsPicker'

// ──────────────────────────────────────────────
// Mocks
// ──────────────────────────────────────────────

// use-debounce wrappers: in tests we want the debounced value to track the
// raw value synchronously so paste-mode tests don't have to flush timers.
vi.mock('use-debounce', () => ({
  useDebounce: (value: unknown) => [value],
}))

// PSY-994: partial-mock @dnd-kit/core to capture the staged list's drag
// callbacks. Everything stays real (the real DndContext is still rendered, so
// useSortable's context provider is intact and the drag-handle tests below keep
// working) — we only wrap DndContext to grab onDragStart/onDragEnd so a test
// can fire them directly. jsdom can't drive a real keyboard/pointer drag, so
// this is how we exercise the virtualize-when-idle ↔ full-render-while-dragging
// switch that makes off-window reorder targets reachable.
let capturedOnDragStart: (() => void) | undefined
let capturedOnDragEnd: ((event: unknown) => void) | undefined
vi.mock('@dnd-kit/core', async () => {
  const actual =
    await vi.importActual<typeof import('@dnd-kit/core')>('@dnd-kit/core')
  return {
    ...actual,
    DndContext: (props: {
      children: React.ReactNode
      onDragStart?: () => void
      onDragEnd?: (event: unknown) => void
    }) => {
      capturedOnDragStart = props.onDragStart
      capturedOnDragEnd = props.onDragEnd
      const Real = actual.DndContext
      return <Real {...props} />
    },
  }
})

type MockedEntitySearchResult = {
  data: {
    artists: unknown[]
    venues: unknown[]
    shows: unknown[]
    releases: unknown[]
    labels: unknown[]
    festivals: unknown[]
    tags: unknown[]
  }
  isSearching: boolean
  totalResults: number
  searchError: boolean
}

let mockSearchResult: MockedEntitySearchResult = {
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
}

// Per-line plain-text search (PSY-845). Tests drive results by raw query
// string via mockFetchEntitySearch; the default returns empty groups (→
// zero results → queue-for-review path). flattenEntitySearchResults keeps the
// real flatten order so the candidate-count branching is exercised honestly.
type SearchGroups = {
  artists: unknown[]
  venues: unknown[]
  shows: unknown[]
  releases: unknown[]
  labels: unknown[]
  festivals: unknown[]
  tags: unknown[]
}
const EMPTY_GROUPS: SearchGroups = {
  artists: [],
  venues: [],
  shows: [],
  releases: [],
  labels: [],
  festivals: [],
  tags: [],
}
let mockFetchEntitySearch = vi.fn(
  async (_query: string): Promise<{ results: SearchGroups; allFailed: boolean }> => ({
    results: EMPTY_GROUPS,
    allFailed: false,
  })
)

vi.mock('@/lib/hooks/common/useEntitySearch', () => ({
  useEntitySearch: () => mockSearchResult,
  ENTITY_SEARCH_UNAVAILABLE_MESSAGE: 'Search is temporarily unavailable. Try again in a moment.',
  fetchEntitySearch: (query: string) => mockFetchEntitySearch(query),
  flattenEntitySearchResults: (results: SearchGroups) => [
    ...results.artists,
    ...results.shows,
    ...results.venues,
    ...results.releases,
    ...results.labels,
    ...results.festivals,
  ],
}))

// Queue-for-review POST (useQueueEntityRequest → apiRequest). Tests inspect
// mockApiRequest calls to assert the entity_request body; mockApiRequest can
// be made to reject to exercise the queue_failed → Retry path.
const mockApiRequest = vi.fn(async (..._args: unknown[]) => ({}))
vi.mock('@/lib/api', () => ({
  apiRequest: (...args: unknown[]) => mockApiRequest(...args),
  API_ENDPOINTS: {
    COLLECTIONS: { ENTITY_REQUESTS: 'http://test/entity-requests' },
  },
}))

// Resolve mutation — most tests don't fire it; the few that do can override
// the global pair from inside the test.
let mockResolveSuccessHandler: ((data: { resolved: unknown[]; unresolved: unknown[] }) => void) | null = null
const mockResolveMutate = vi.fn(
  (
    _entries: unknown[],
    opts?: {
      onSuccess?: (data: { resolved: unknown[]; unresolved: unknown[] }) => void
      onError?: (err: unknown) => void
    }
  ) => {
    mockResolveSuccessHandler = opts?.onSuccess ?? null
  }
)

vi.mock('../hooks', () => ({
  useResolveCollectionItems: () => ({
    mutate: mockResolveMutate,
    isPending: false,
    error: null,
  }),
}))

// Shared primitives — keep mocks dumb so we can assert on raw text content.
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
    <button onClick={onClick} disabled={disabled} {...(props as Record<string, unknown>)}>
      {children}
    </button>
  ),
}))

vi.mock('@/components/ui/input', () => ({
  Input: (props: React.InputHTMLAttributes<HTMLInputElement>) => <input {...props} />,
}))

vi.mock('@/components/ui/badge', () => ({
  // Forward data-testid (drop the non-DOM `variant` prop) so PSY-845's queued
  // chip (add-items-picker-paste-row-queued) is queryable — a children-only
  // mock would silently drop the testid while still rendering the label.
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

let activeTabValue = 'search'
let activeOnValueChange: ((v: string) => void) | undefined
vi.mock('@/components/ui/tabs', () => ({
  Tabs: ({
    children,
    value,
    onValueChange,
  }: {
    children: React.ReactNode
    value: string
    onValueChange: (v: string) => void
  }) => {
    activeTabValue = value
    activeOnValueChange = onValueChange
    return <div data-testid="tabs">{children}</div>
  },
  TabsList: ({ children }: { children: React.ReactNode }) => <div>{children}</div>,
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

// The AI pane pulls in useCollectionExtraction (not mocked here); stub it so
// the AC#3 guard can activate the AI tab without dragging in that dependency.
vi.mock('./AICollectionFiller', () => ({
  AICollectionFiller: () => <div data-testid="ai-collection-filler-stub" />,
}))

// Paste the whole textarea value in ONE operation. The component debounces in
// production; the test mocks useDebounce to pass-through, so char-by-char
// `user.type` would re-run usePastePreview's effect on EVERY keystroke and
// spawn a stale async search worker per char (a test-only artifact). A single
// paste runs the effect once — exactly the production code path after debounce.
async function pasteInto(
  user: ReturnType<typeof userEvent.setup>,
  text: string
) {
  const ta = screen.getByTestId('add-items-picker-paste-textarea')
  await user.click(ta)
  await user.paste(text)
}

// ──────────────────────────────────────────────
// parsePasteLine — pure function tests
// ──────────────────────────────────────────────

describe('parsePasteLine', () => {
  it('parses a fully-qualified PH artist URL', () => {
    expect(parsePasteLine('https://psychichomily.com/artists/kendrick-lamar')).toEqual({
      raw: 'https://psychichomily.com/artists/kendrick-lamar',
      url: { entityType: 'artist', slug: 'kendrick-lamar' },
    })
  })

  it('parses a bare PH path', () => {
    expect(parsePasteLine('/releases/to-pimp-a-butterfly')).toEqual({
      raw: '/releases/to-pimp-a-butterfly',
      url: { entityType: 'release', slug: 'to-pimp-a-butterfly' },
    })
  })

  it('parses a path without leading slash', () => {
    expect(parsePasteLine('artists/frank-ocean')).toEqual({
      raw: 'artists/frank-ocean',
      url: { entityType: 'artist', slug: 'frank-ocean' },
    })
  })

  it('lowercases the slug', () => {
    const parsed = parsePasteLine('/artists/Kendrick-LAMAR')
    expect(parsed.url?.slug).toBe('kendrick-lamar')
  })

  it('returns url:null for plain text', () => {
    expect(parsePasteLine('Mount Eerie - A Crow Looked At Me')).toEqual({
      raw: 'Mount Eerie - A Crow Looked At Me',
      url: null,
    })
  })

  it('returns url:null for external URLs (Bandcamp, Spotify)', () => {
    // External hosts must be excluded — PSY-824 handles those via AI mode.
    // Even though the path "/track/foo" exists on Bandcamp, the regex
    // anchors on PH's entity-plural paths so this falls through.
    expect(
      parsePasteLine('https://bandcamp.com/artists/foo')
    ).toEqual({
      raw: 'https://bandcamp.com/artists/foo',
      url: { entityType: 'artist', slug: 'foo' },
    })
    // NOTE: V1 parser accepts any host so long as the path matches a
    // canonical PH shape — backend resolver is the authoritative gate.
    // This is intentional V1 scope (parser is layout-aware, not host-
    // aware). The follow-up for "queue for review" + external URLs may
    // tighten this.
  })

  it('handles the six entity types', () => {
    const types: Array<['artists' | 'releases' | 'labels' | 'shows' | 'venues' | 'festivals', string]> = [
      ['artists', 'artist'],
      ['releases', 'release'],
      ['labels', 'label'],
      ['shows', 'show'],
      ['venues', 'venue'],
      ['festivals', 'festival'],
    ]
    for (const [plural, singular] of types) {
      const parsed = parsePasteLine(`/${plural}/some-slug`)
      expect(parsed.url?.entityType).toBe(singular)
    }
  })

  it('returns url:null for empty strings', () => {
    expect(parsePasteLine('')).toEqual({ raw: '', url: null })
    expect(parsePasteLine('   ')).toEqual({ raw: '', url: null })
  })
})

// ──────────────────────────────────────────────
// Component behavior
// ──────────────────────────────────────────────

describe('AddItemsPicker', () => {
  beforeEach(() => {
    activeTabValue = 'search'
    capturedOnDragStart = undefined
    capturedOnDragEnd = undefined
    mockResolveMutate.mockClear()
    mockResolveSuccessHandler = null
    mockApiRequest.mockClear()
    mockApiRequest.mockResolvedValue({})
    mockFetchEntitySearch = vi.fn(async () => ({
      results: EMPTY_GROUPS,
      allFailed: false,
    }))
    mockSearchResult = {
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
    }
  })

  it('renders Search, Paste, and AI tabs', () => {
    // PSY-824 enabled the AI tab (was disabled with a "Coming in PSY-824"
    // tooltip in PSY-823). All three tabs should now be present + active.
    render(<AddItemsPicker stagedItems={[]} onStagedItemsChange={vi.fn()} />)
    expect(screen.getByTestId('tab-search')).toBeInTheDocument()
    expect(screen.getByTestId('tab-paste')).toBeInTheDocument()
    expect(screen.getByTestId('tab-ai')).toBeInTheDocument()
    expect(screen.getByTestId('tab-ai')).not.toBeDisabled()
  })

  it('labels the AI tab "From text (AI)" with an ⓘ explainer as a SIBLING of the tab', () => {
    // PSY-867: "From article (AI)" was misleadingly narrow — the route
    // accepts any text. The ⓘ must be a SIBLING of the tab trigger (its own
    // focus stop), NOT nested inside the trigger <button> (interactive
    // content inside a button is invalid). The not.toContainElement guard
    // is what fails if a refactor nests the glyph back into the tab.
    render(<AddItemsPicker stagedItems={[]} onStagedItemsChange={vi.fn()} />)
    expect(screen.getByText('From text (AI)')).toBeInTheDocument()
    expect(screen.queryByText(/From article/i)).not.toBeInTheDocument()
    expect(screen.getByTestId('ai-tab-info')).toBeInTheDocument()
    expect(screen.getByTestId('tab-ai')).not.toContainElement(
      screen.getByTestId('ai-tab-info')
    )
  })

  it('opens the AI explainer tooltip on keyboard focus of the ⓘ glyph', async () => {
    // AC: tooltip renders the locked copy on keyboard focus of the glyph.
    render(<AddItemsPicker stagedItems={[]} onStagedItemsChange={vi.fn()} />)
    await act(async () => {
      screen.getByTestId('ai-tab-info').focus()
    })
    // Radix renders the copy twice (visible content + a visually-hidden
    // role="tooltip" span for screen readers), so match all and assert ≥1.
    const matches = await screen.findAllByText(
      /paste any text, and the ai will do its best to extract/i
    )
    expect(matches.length).toBeGreaterThan(0)
  })

  it('opens the AI explainer tooltip on hover of the ⓘ glyph', async () => {
    // AC: tooltip also renders on hover (the path real pointer users hit).
    const user = userEvent.setup()
    render(<AddItemsPicker stagedItems={[]} onStagedItemsChange={vi.fn()} />)
    await user.hover(screen.getByTestId('ai-tab-info'))
    const matches = await screen.findAllByText(
      /paste any text, and the ai will do its best to extract/i
    )
    expect(matches.length).toBeGreaterThan(0)
  })

  it('does not open the explainer when the AI tab itself is activated', async () => {
    // AC: the trigger surface is JUST the ⓘ glyph — activating the tab must
    // not surface the explainer.
    const user = userEvent.setup()
    render(<AddItemsPicker stagedItems={[]} onStagedItemsChange={vi.fn()} />)
    await user.click(screen.getByTestId('tab-ai'))
    expect(
      screen.queryByText(
        /paste any text, and the ai will do its best to extract/i
      )
    ).not.toBeInTheDocument()
    // ...and the tab still activates — it routes to the AI pane (stubbed).
    expect(
      screen.getByTestId('ai-collection-filler-stub')
    ).toBeInTheDocument()
  })

  it('shows the search empty-state copy by default', () => {
    render(<AddItemsPicker stagedItems={[]} onStagedItemsChange={vi.fn()} />)
    expect(
      screen.getByText(/search artists, shows, venues, releases, labels, festivals/i)
    ).toBeInTheDocument()
  })

  it('renders search results and stages a clicked row', async () => {
    const onStaged = vi.fn()
    mockSearchResult = {
      data: {
        artists: [
          {
            id: 99,
            slug: 'kendrick-lamar',
            name: 'Kendrick Lamar',
            subtitle: 'Compton, CA',
            entityType: 'artist',
            href: '/artists/kendrick-lamar',
          },
        ],
        venues: [],
        shows: [],
        releases: [],
        labels: [],
        festivals: [],
        tags: [],
      },
      isSearching: false,
      totalResults: 1,
      searchError: false,
    }
    const user = userEvent.setup()
    render(<AddItemsPicker stagedItems={[]} onStagedItemsChange={onStaged} />)
    const input = screen.getByTestId('add-items-picker-search-input')
    await user.type(input, 'kk')

    expect(screen.getByText('Kendrick Lamar')).toBeInTheDocument()
    expect(screen.getByText('Compton, CA')).toBeInTheDocument()

    // Find the Add button on the result row and click it.
    const addBtn = screen.getAllByRole('button', { name: /add/i }).find(
      (b) => b.textContent?.trim().toLowerCase() === 'add'
    )!
    await user.click(addBtn)

    // Last call carries the staged-items array with the row in it.
    const last = onStaged.mock.calls.at(-1)?.[0]
    expect(last).toHaveLength(1)
    expect(last?.[0]).toMatchObject({
      entityType: 'artist',
      entityId: 99,
      name: 'Kendrick Lamar',
    })
  })

  it('marks an "Already added" row when the entity is in existingItems', async () => {
    mockSearchResult = {
      data: {
        artists: [
          {
            id: 99,
            slug: 'kendrick-lamar',
            name: 'Kendrick Lamar',
            subtitle: 'Compton, CA',
            entityType: 'artist',
            href: '/artists/kendrick-lamar',
          },
        ],
        venues: [],
        shows: [],
        releases: [],
        labels: [],
        festivals: [],
        tags: [],
      },
      isSearching: false,
      totalResults: 1,
      searchError: false,
    }
    const user = userEvent.setup()
    render(
      <AddItemsPicker
        existingItems={[{ entity_type: 'artist', entity_id: 99 }]}
        stagedItems={[]}
        onStagedItemsChange={vi.fn()}
      />
    )
    await user.type(screen.getByTestId('add-items-picker-search-input'), 'kk')

    // The "Added" chip renders instead of the Add button when the entity
    // is already in the collection (existingItems).
    expect(screen.getByText('Added')).toBeInTheDocument()
  })

  it('surfaces the outage banner when searchError is true', async () => {
    mockSearchResult.searchError = true
    const user = userEvent.setup()
    render(<AddItemsPicker stagedItems={[]} onStagedItemsChange={vi.fn()} />)
    await user.type(screen.getByTestId('add-items-picker-search-input'), 'tt')

    expect(
      screen.getByTestId('add-items-picker-search-error-banner')
    ).toHaveTextContent(/Search is temporarily unavailable/i)
  })

  it('switches to Paste mode and renders the textarea', async () => {
    const user = userEvent.setup()
    render(<AddItemsPicker stagedItems={[]} onStagedItemsChange={vi.fn()} />)
    await user.click(screen.getByTestId('tab-paste'))
    expect(screen.getByTestId('add-items-picker-paste-textarea')).toBeInTheDocument()
  })

  it('Paste mode: hits the resolver for URL lines only (plain text falls through)', async () => {
    const user = userEvent.setup()
    render(<AddItemsPicker stagedItems={[]} onStagedItemsChange={vi.fn()} />)
    await user.click(screen.getByTestId('tab-paste'))

    const ta = screen.getByTestId('add-items-picker-paste-textarea')
    await user.type(
      ta,
      '/artists/kendrick-lamar\n/releases/to-pimp-a-butterfly\nplain text line'
    )

    // Resolver was called; the last call carries exactly the URL entries.
    const lastCallArgs = mockResolveMutate.mock.calls.at(-1)
    expect(lastCallArgs).toBeTruthy()
    const entries = lastCallArgs?.[0] as Array<{ entity_type: string; slug: string }>
    expect(entries).toHaveLength(2)
    expect(entries).toContainEqual({ entity_type: 'artist', slug: 'kendrick-lamar' })
    expect(entries).toContainEqual({ entity_type: 'release', slug: 'to-pimp-a-butterfly' })
  })

  // Regression guard for the /simplify fix to AddItemsPicker.tsx's addAll path.
  // Earlier implementation looped per-row and called onStagedItemsChange N times;
  // each call read the same stale stagedItems prop, so React's setState batching
  // collapsed N updates into one — only the LAST item ended up staged. This test
  // asserts the single-call batch shape: clicking "Add all" emits exactly ONE
  // onStagedItemsChange call with all matched rows in the array.
  it('Paste mode: Add all batches into a single onStagedItemsChange call', async () => {
    const onStaged = vi.fn()
    const user = userEvent.setup()
    render(<AddItemsPicker stagedItems={[]} onStagedItemsChange={onStaged} />)
    await user.click(screen.getByTestId('tab-paste'))
    await user.type(
      screen.getByTestId('add-items-picker-paste-textarea'),
      '/artists/kendrick-lamar\n/artists/frank-ocean'
    )

    // Drive the resolver success synchronously — both rows become MATCH.
    mockResolveSuccessHandler?.({
      resolved: [
        {
          entity_type: 'artist',
          slug: 'kendrick-lamar',
          entity_id: 42,
          name: 'Kendrick Lamar',
          subtitle: null,
        },
        {
          entity_type: 'artist',
          slug: 'frank-ocean',
          entity_id: 43,
          name: 'Frank Ocean',
          subtitle: null,
        },
      ],
      unresolved: [],
    })

    const addAll = await screen.findByTestId('add-items-picker-paste-add-all')
    onStaged.mockClear()
    await user.click(addAll)

    // ONE call (not two) — batched. Length matches the 2 matched rows. If the
    // regression returns (per-row onStage), this assertion drops to a single
    // entry (last-write-wins) or N calls.
    expect(onStaged).toHaveBeenCalledTimes(1)
    expect(onStaged.mock.calls[0][0]).toHaveLength(2)
    expect(onStaged.mock.calls[0][0]).toEqual([
      expect.objectContaining({ entityType: 'artist', entityId: 42 }),
      expect.objectContaining({ entityType: 'artist', entityId: 43 }),
    ])
  })

  // Regression guard for the /simplify fix to AddItemsPicker.tsx's
  // resolveGenerationRef. Without the generation counter, a slow earlier
  // resolver response could overwrite the previewRows derived from a newer
  // request — the user would see chips for the prior paste-text. This test
  // fires the resolver twice and invokes the OLDER success handler last;
  // assertion: the older onSuccess is a no-op (no chip render).
  it('Paste mode: stale resolver responses do not overwrite newer preview rows', async () => {
    const user = userEvent.setup()
    render(<AddItemsPicker stagedItems={[]} onStagedItemsChange={vi.fn()} />)
    await user.click(screen.getByTestId('tab-paste'))

    // Two distinct paste-text edits → two resolver fires. Capture the FIRST
    // success handler before the SECOND mutate overwrites it. Cast through
    // typeof because TypeScript narrows the variable to `null` after the
    // explicit reset assignment and can't see the mock re-assigns it.
    type Handler = (data: { resolved: unknown[]; unresolved: unknown[] }) => void
    await user.type(
      screen.getByTestId('add-items-picker-paste-textarea'),
      '/artists/older'
    )
    const olderHandler = mockResolveSuccessHandler as Handler | null
    mockResolveSuccessHandler = null

    await user.clear(screen.getByTestId('add-items-picker-paste-textarea'))
    await user.type(
      screen.getByTestId('add-items-picker-paste-textarea'),
      '/artists/newer'
    )
    const newerHandler = mockResolveSuccessHandler as Handler | null

    // Fire the newer one first, then the older one. Without the guard, the
    // older one would set previewRows to "older"'s data. `act` flushes the
    // setState calls so screen.getByText sees the committed render.
    await act(async () => {
      newerHandler?.({
        resolved: [
          {
            entity_type: 'artist',
            slug: 'newer',
            entity_id: 99,
            name: 'Newer Artist',
            subtitle: null,
          },
        ],
        unresolved: [],
      })
    })
    await act(async () => {
      olderHandler?.({
        resolved: [
          {
            entity_type: 'artist',
            slug: 'older',
            entity_id: 50,
            name: 'Older Artist',
            subtitle: null,
          },
        ],
        unresolved: [],
      })
    })

    // Only "Newer Artist" should render — the older handler's setPreviewRows
    // call should have been bailed by the generation check.
    expect(screen.getByText('Newer Artist')).toBeInTheDocument()
    expect(screen.queryByText('Older Artist')).not.toBeInTheDocument()
  })

  // ── PSY-845: plain-text auto-match / ambiguous Pick / queue-for-review ──

  it('Paste mode: plain-text line never hits the URL resolver', async () => {
    const user = userEvent.setup()
    render(<AddItemsPicker stagedItems={[]} onStagedItemsChange={vi.fn()} />)
    await user.click(screen.getByTestId('tab-paste'))
    await pasteInto(user, 'Mount Eerie')
    // URL resolver is only for canonical-path lines — plain text routes to
    // the per-line entity search (fetchEntitySearch), not the resolver.
    expect(mockResolveMutate).not.toHaveBeenCalled()
    await waitFor(() => expect(mockFetchEntitySearch).toHaveBeenCalled())
  })

  it('Paste mode: a single search result ⇒ MATCH with a stageable [Add]', async () => {
    const onStaged = vi.fn()
    mockFetchEntitySearch = vi.fn(async () => ({
      results: {
        ...EMPTY_GROUPS,
        artists: [
          {
            id: 7,
            slug: 'mount-eerie',
            name: 'Mount Eerie',
            subtitle: 'Anacortes, WA',
            entityType: 'artist',
            href: '/artists/mount-eerie',
          },
        ],
      },
      allFailed: false,
    }))
    const user = userEvent.setup()
    render(<AddItemsPicker stagedItems={[]} onStagedItemsChange={onStaged} />)
    await user.click(screen.getByTestId('tab-paste'))
    await pasteInto(user, 'Mount Eerie')

    // MATCH chip renders once the per-line search resolves. (The name also
    // appears in the textarea value, so scope the name assertion to the row.)
    expect(await screen.findByText('MATCH')).toBeInTheDocument()
    const row = screen.getByTestId('add-items-picker-paste-row')
    expect(within(row).getByText('Mount Eerie')).toBeInTheDocument()

    const addBtn = screen
      .getAllByRole('button', { name: /add/i })
      .find((b) => b.textContent?.trim().toLowerCase() === 'add')!
    await user.click(addBtn)

    const last = onStaged.mock.calls.at(-1)?.[0]
    expect(last).toHaveLength(1)
    expect(last?.[0]).toMatchObject({ entityType: 'artist', entityId: 7 })
  })

  it('Paste mode: 2+ results ⇒ AMBIGUOUS with an inline [Pick] that promotes to MATCH', async () => {
    const onStaged = vi.fn()
    mockFetchEntitySearch = vi.fn(async () => ({
      results: {
        ...EMPTY_GROUPS,
        artists: [
          {
            id: 1,
            slug: 'nirvana',
            name: 'Nirvana (Seattle)',
            subtitle: 'Seattle, WA',
            entityType: 'artist',
            href: '/artists/nirvana',
          },
          {
            id: 2,
            slug: 'nirvana-uk',
            name: 'Nirvana (UK)',
            subtitle: 'London, UK',
            entityType: 'artist',
            href: '/artists/nirvana-uk',
          },
        ],
      },
      allFailed: false,
    }))
    const user = userEvent.setup()
    render(<AddItemsPicker stagedItems={[]} onStagedItemsChange={onStaged} />)
    await user.click(screen.getByTestId('tab-paste'))
    await pasteInto(user, 'Nirvana')

    // PICK chip + the candidate dropdown render.
    expect(await screen.findByText('PICK')).toBeInTheDocument()
    const pickRow = screen.getByTestId('add-items-picker-paste-row-pick')
    expect(pickRow).toHaveTextContent('Did you mean:')

    // Pick the second candidate → row promotes to MATCH, then stage it.
    await user.click(
      within(pickRow).getByRole('button', { name: /Nirvana \(UK\)/i })
    )
    expect(await screen.findByText('MATCH')).toBeInTheDocument()

    const addBtn = screen
      .getAllByRole('button', { name: /add/i })
      .find((b) => b.textContent?.trim().toLowerCase() === 'add')!
    await user.click(addBtn)
    const last = onStaged.mock.calls.at(-1)?.[0]
    expect(last?.[0]).toMatchObject({ entityType: 'artist', entityId: 2 })
  })

  it('Paste mode: caps the AMBIGUOUS Pick dropdown at 5 candidates', async () => {
    mockFetchEntitySearch = vi.fn(async () => ({
      results: {
        ...EMPTY_GROUPS,
        artists: Array.from({ length: 8 }, (_, i) => ({
          id: i + 1,
          slug: `dup-${i}`,
          name: `Dup ${i}`,
          subtitle: null,
          entityType: 'artist',
          href: `/artists/dup-${i}`,
        })),
      },
      allFailed: false,
    }))
    const user = userEvent.setup()
    render(<AddItemsPicker stagedItems={[]} onStagedItemsChange={vi.fn()} />)
    await user.click(screen.getByTestId('tab-paste'))
    await pasteInto(user, 'Dup')

    const pickRow = await screen.findByTestId('add-items-picker-paste-row-pick')
    // 5 candidate buttons max (the "Did you mean:" label is a span, not a button).
    expect(within(pickRow).getAllByRole('button')).toHaveLength(5)
  })

  it('Paste mode: zero results ⇒ queue-for-review (POSTs an entity_request)', async () => {
    // Default mockFetchEntitySearch returns empty groups → zero results.
    const user = userEvent.setup()
    render(<AddItemsPicker stagedItems={[]} onStagedItemsChange={vi.fn()} />)
    await user.click(screen.getByTestId('tab-paste'))
    await pasteInto(user, 'Totally Unknown Artist')

    // The queued affordance renders and the POST fired with the line as the
    // artist payload + paste_mode source.
    expect(
      await screen.findByTestId('add-items-picker-paste-row-queued')
    ).toHaveTextContent('FOR REVIEW')
    await waitFor(() => expect(mockApiRequest).toHaveBeenCalled())
    const [, opts] = mockApiRequest.mock.calls.at(-1)!
    const body = JSON.parse((opts as { body: string }).body)
    expect(body).toMatchObject({
      entity_type: 'artist',
      source_context: 'paste_mode',
      payload: { name: 'Totally Unknown Artist' },
    })
  })

  it('Paste mode: a failed queue POST shows a Retry that re-fires the request', async () => {
    mockApiRequest.mockRejectedValueOnce(new Error('boom'))
    const user = userEvent.setup()
    render(<AddItemsPicker stagedItems={[]} onStagedItemsChange={vi.fn()} />)
    await user.click(screen.getByTestId('tab-paste'))
    await pasteInto(user, 'Flaky Network Artist')

    // First POST rejects → Retry button surfaces.
    const retry = await screen.findByTestId(
      'add-items-picker-paste-row-retry-queue'
    )
    mockApiRequest.mockClear()
    mockApiRequest.mockResolvedValue({})
    await user.click(retry)

    // Retry re-fires the POST and the row settles to FOR REVIEW.
    await waitFor(() => expect(mockApiRequest).toHaveBeenCalledTimes(1))
    expect(
      await screen.findByTestId('add-items-picker-paste-row-queued')
    ).toHaveTextContent('FOR REVIEW')
  })

  it('Paste mode: a per-line search outage ⇒ NO MATCH (not queued)', async () => {
    mockFetchEntitySearch = vi.fn(async () => {
      throw new Error('search down')
    })
    const user = userEvent.setup()
    render(<AddItemsPicker stagedItems={[]} onStagedItemsChange={vi.fn()} />)
    await user.click(screen.getByTestId('tab-paste'))
    await pasteInto(user, 'Some Artist')

    // A transient search failure is NOT a confirmed zero-result: mark
    // unresolved (retryable) and do NOT file a queue request.
    expect(await screen.findByText('NO MATCH')).toBeInTheDocument()
    expect(mockApiRequest).not.toHaveBeenCalled()
  })

  it('Paste mode: queues EACH zero-result line independently (no last-write-wins)', async () => {
    // Guards the multi-junk-line case: usePastePreview reuses ONE
    // queueMutation instance across the bounded worker pool. Each per-call
    // mutate() must fire its own onSuccess targeting its own row index —
    // a last-write-wins bug would queue only one line / one POST.
    const user = userEvent.setup()
    render(<AddItemsPicker stagedItems={[]} onStagedItemsChange={vi.fn()} />)
    await user.click(screen.getByTestId('tab-paste'))
    await pasteInto(user, 'JunkOne\nJunkTwo\nJunkThree')

    await waitFor(() =>
      expect(
        screen.getAllByTestId('add-items-picker-paste-row-queued')
      ).toHaveLength(3)
    )
    expect(mockApiRequest).toHaveBeenCalledTimes(3)
    const names = mockApiRequest.mock.calls
      .map((c) => JSON.parse((c[1] as { body: string }).body).payload.name)
      .sort()
    expect(names).toEqual(['JunkOne', 'JunkThree', 'JunkTwo'])
  })

  // ── PSY-962: overview strip + drag-reorder ──
  // Full drag interaction is exercised by @dnd-kit itself + manual repro;
  // these cover the strip render, count/plural, overflow cap, and the
  // reorder-affordance gating (handle only when there's >1 item to reorder).
  const staged = (n: number) =>
    Array.from({ length: n }, (_, i) => ({
      entityType: 'artist' as const,
      entityId: i + 1,
      name: `Artist ${i + 1}`,
      subtitle: null,
    }))

  it('renders the overview strip with item count + reorder hint (>1 item)', () => {
    render(
      <AddItemsPicker stagedItems={staged(3)} onStagedItemsChange={vi.fn()} />
    )
    const strip = screen.getByTestId('add-items-picker-overview-strip')
    expect(strip).toHaveTextContent('3 items')
    expect(strip).toHaveTextContent('drag to reorder')
  })

  it('overview strip: singular "item" + no reorder hint for a single item', () => {
    render(
      <AddItemsPicker stagedItems={staged(1)} onStagedItemsChange={vi.fn()} />
    )
    const strip = screen.getByTestId('add-items-picker-overview-strip')
    expect(strip).toHaveTextContent('1 item')
    expect(strip).not.toHaveTextContent('drag to reorder')
  })

  it('overview strip caps its preview and shows a +N overflow chip', () => {
    render(
      <AddItemsPicker stagedItems={staged(30)} onStagedItemsChange={vi.fn()} />
    )
    // 24 shown + 6 overflow.
    expect(
      screen.getByTestId('add-items-picker-overview-strip')
    ).toHaveTextContent('+6')
  })

  it('renders a drag handle per row when reorderable (>1 staged item)', () => {
    render(
      <AddItemsPicker stagedItems={staged(3)} onStagedItemsChange={vi.fn()} />
    )
    expect(screen.getAllByTestId('staged-row-drag-handle')).toHaveLength(3)
  })

  it('hides the drag handle for a single staged item (nothing to reorder)', () => {
    render(
      <AddItemsPicker stagedItems={staged(1)} onStagedItemsChange={vi.fn()} />
    )
    expect(
      screen.queryByTestId('staged-row-drag-handle')
    ).not.toBeInTheDocument()
  })

  // ── PSY-994: staged-list virtualization (threshold + drag-fallback) ──
  // The DOM-bounding behavior (only a WINDOW of rows mounts at rest) and the
  // visual reorder are verified end-to-end in the real-browser manual repro —
  // jsdom's 0-height scroll element + no-op ResizeObserver means
  // @tanstack/react-virtual measures a zero viewport and renders an empty
  // window. These unit tests pin the load-bearing invariants that DON'T need a
  // real layout: the threshold switchover, that the virtual path is NOT
  // rendering all rows, and that an active drag falls back to the full render
  // so every sortable is mounted (off-window reorder targets reachable).

  it('keeps the NON-virtual render at/below the threshold (30 items)', () => {
    // 30 is the threshold; 30 > 30 is false, so the list stays fully mounted.
    render(
      <AddItemsPicker stagedItems={staged(30)} onStagedItemsChange={vi.fn()} />
    )
    const list = screen.getByTestId('add-items-picker-staged-list')
    expect(list).not.toHaveAttribute('data-virtualized')
    // Every row is in the DOM (the cheap, fully-mounted path the design was
    // built against for the scroll-but-no-window band).
    expect(screen.getAllByTestId('add-items-picker-staged-row')).toHaveLength(30)
  })

  it('virtualizes above the threshold (does NOT mount every row)', () => {
    // 60 > 30 → the windowed render. The virtual viewport carries the
    // data-virtualized marker, and (critically) it is NOT mounting all 60 rows
    // — the unbounded-DOM regression this ticket fixes. In jsdom the window is
    // empty (zero-height viewport), which still proves "not all rows": the
    // real-browser repro confirms a non-empty window of ~12.
    render(
      <AddItemsPicker stagedItems={staged(60)} onStagedItemsChange={vi.fn()} />
    )
    const list = screen.getByTestId('add-items-picker-staged-list')
    expect(list).toHaveAttribute('data-virtualized', 'true')
    expect(
      screen.queryAllByTestId('add-items-picker-staged-row').length
    ).toBeLessThan(60)
  })

  it('falls back to the FULL render during a drag so off-window targets are reachable', () => {
    // The load-bearing a11y + dnd-kit coordination contract (Saboteur lens): a
    // windowed list can't hit-test or keyboard-reorder to an off-window row.
    // virtualize-when-idle flips to the full render on drag-start, mounting
    // EVERY sortable, then back on drag-end. Without this, a keyboard reorder
    // (or pointer auto-scroll drag) could never cross the window boundary.
    render(
      <AddItemsPicker stagedItems={staged(60)} onStagedItemsChange={vi.fn()} />
    )
    // Idle: windowed (not all 60 rows mounted).
    expect(
      screen.getByTestId('add-items-picker-staged-list')
    ).toHaveAttribute('data-virtualized', 'true')

    // Drag-start → full render: data-virtualized gone, all 60 rows mounted.
    act(() => {
      capturedOnDragStart?.()
    })
    const dragList = screen.getByTestId('add-items-picker-staged-list')
    expect(dragList).not.toHaveAttribute('data-virtualized')
    expect(screen.getAllByTestId('add-items-picker-staged-row')).toHaveLength(60)

    // Drag-end → back to the windowed render.
    act(() => {
      capturedOnDragEnd?.({ active: { id: 'artist-1' }, over: { id: 'artist-1' } })
    })
    expect(
      screen.getByTestId('add-items-picker-staged-list')
    ).toHaveAttribute('data-virtualized', 'true')
  })

  it('applies a reorder dragged across the window boundary (off-window target)', () => {
    // Keyboard/pointer reorder to an off-window index resolves through the same
    // pure reorder behind onDragEnd. Drive a drag-end that moves the first item
    // PAST the visible window (index 0 → index 59) and assert the parent gets
    // the fully reordered array — proving cross-window targets commit correctly.
    const onStaged = vi.fn()
    render(
      <AddItemsPicker stagedItems={staged(60)} onStagedItemsChange={onStaged} />
    )
    act(() => {
      capturedOnDragStart?.()
    })
    act(() => {
      capturedOnDragEnd?.({
        active: { id: 'artist-1' },
        over: { id: 'artist-60' },
      })
    })
    expect(onStaged).toHaveBeenCalledTimes(1)
    const next = onStaged.mock.calls[0][0] as Array<{ entityId: number }>
    expect(next).toHaveLength(60)
    // artist-1 moved to the end; no item dropped or duplicated.
    expect(next.at(-1)?.entityId).toBe(1)
    expect(new Set(next.map((s) => s.entityId)).size).toBe(60)
  })
})

// PSY-962 adversarial-review: the reorder CONTRACT (preserve all items, no
// dupes/drops, correct order) is the ticket's load-bearing behavior — unit it
// directly via the pure helper rather than driving @dnd-kit.
describe('reorderStagedItems', () => {
  const mk = (n: number) =>
    Array.from({ length: n }, (_, i) => ({
      entityType: 'artist' as const,
      entityId: i + 1,
      name: `Artist ${i + 1}`,
      subtitle: null,
    }))

  it('moves the active item to the over position, preserving every item', () => {
    const next = reorderStagedItems(mk(3), 'artist-1', 'artist-3')
    expect(next).not.toBeNull()
    expect(next!.map((s) => s.entityId)).toEqual([2, 3, 1])
    expect(next).toHaveLength(3)
    expect(new Set(next!.map((s) => s.entityId))).toEqual(new Set([1, 2, 3]))
  })

  it('returns null (no-op) when there is no drop target', () => {
    expect(reorderStagedItems(mk(3), 'artist-1', null)).toBeNull()
  })

  it('returns null (no-op) when an item is dropped on itself', () => {
    expect(reorderStagedItems(mk(3), 'artist-2', 'artist-2')).toBeNull()
  })

  it('returns null (no-op) for an id not in the list', () => {
    expect(reorderStagedItems(mk(3), 'artist-9', 'artist-1')).toBeNull()
    expect(reorderStagedItems(mk(3), 'artist-1', 'artist-9')).toBeNull()
  })
})
