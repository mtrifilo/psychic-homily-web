import { describe, it, expect, vi, beforeEach } from 'vitest'
import React from 'react'
import { render, screen, act } from '@testing-library/react'
import userEvent from '@testing-library/user-event'

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

vi.mock('@/lib/hooks/common/useEntitySearch', () => ({
  useEntitySearch: () => mockSearchResult,
  ENTITY_SEARCH_UNAVAILABLE_MESSAGE: 'Search is temporarily unavailable. Try again in a moment.',
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
  Badge: ({ children }: { children: React.ReactNode }) => <span>{children}</span>,
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
    mockResolveMutate.mockClear()
    mockResolveSuccessHandler = null
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

  it('Paste mode: shows the unresolved-help copy when only plain text is pasted', async () => {
    const user = userEvent.setup()
    render(<AddItemsPicker stagedItems={[]} onStagedItemsChange={vi.fn()} />)
    await user.click(screen.getByTestId('tab-paste'))
    await user.type(
      screen.getByTestId('add-items-picker-paste-textarea'),
      'Mount Eerie - A Crow'
    )

    // Resolver should NOT be called when there are zero URL entries.
    expect(mockResolveMutate).not.toHaveBeenCalled()
    expect(screen.getByText(/canonical PH paths/i)).toBeInTheDocument()
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
