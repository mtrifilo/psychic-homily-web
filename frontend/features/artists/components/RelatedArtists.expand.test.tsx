import { describe, it, expect, vi, beforeEach } from 'vitest'
import { act, fireEvent, screen } from '@testing-library/react'
import { renderWithProviders } from '@/test/utils'
import type { ArtistGraph, ArtistGraphLink, ArtistGraphNode } from '../types'

// PSY-1259: integration coverage for RecenteringGraph's expand-on-demand STATE MACHINE —
// expand (fetch+merge), collapse (re-click), Collapse-all, and the in-flight cancellation
// guard. The merge math is unit-tested in mergeEgoGraphs.test.ts and the canvas gesture in
// ArtistGraphVisualization.test.tsx; this pins the orchestration the canvas can't reach in
// jsdom by capturing the props handed to a mocked ArtistGraphVisualization and resolving the
// expand fetch by hand.

// Capture the props the mocked canvas receives so we can drive onExpand + read back the merged
// data / expandedIds / expandingIds the orchestration produced.
// eslint-disable-next-line @typescript-eslint/no-explicit-any
let vizProps: any = null
vi.mock('./ArtistGraph', async () => {
  const { createContext } = await import('react')
  return {
    ArtistGraphVisualization: (props: Record<string, unknown>) => {
      vizProps = props
      return <div data-testid="viz" />
    },
    // PSY-1351: ArtistGraphDialog wraps the graph in this context's Provider.
    ConnectionPanelDismissContext: createContext(null),
  }
})

// A controllable expand fetcher — each call parks a {id, resolve} we settle manually.
const fetchCalls: Array<{ id: number; resolve: (g: ArtistGraph) => void; reject: (e?: unknown) => void }> = []
const mockUseArtistGraph = vi.fn()
vi.mock('../hooks/useArtistGraph', () => ({
  useArtistGraph: (opts: unknown) => mockUseArtistGraph(opts),
  useFetchArtistGraph: () => (id: number) =>
    new Promise<ArtistGraph>((resolve, reject) => {
      fetchCalls.push({ id, resolve, reject })
    }),
  useArtistRelationshipVote: () => ({ mutate: vi.fn(), isPending: false }),
  useCreateArtistRelationship: () => ({ mutate: vi.fn(), isPending: false }),
}))

vi.mock('@/features/auth', () => ({
  useIsAuthenticated: () => ({ user: null, isAuthenticated: false }),
}))
// Stable push spy so a re-center can be asserted end-to-end (PSY-1361).
const routerPush = vi.fn()
vi.mock('next/navigation', () => ({
  useRouter: () => ({ push: routerPush, replace: vi.fn() }),
  usePathname: () => '/artists/a1',
  useSearchParams: () => new URLSearchParams(),
}))
vi.mock('../hooks/useArtists', () => ({
  useArtist: () => ({ data: undefined, isLoading: false }),
}))
// Non-reduced-motion so the PSY-1260 discovery-bias slider renders (it's hidden for reduced-motion
// users, whose canvas repaint is gated off). Explicit so these tests don't ride jsdom's matchMedia.
vi.mock('../hooks/useReducedMotion', () => ({ useReducedMotion: () => false }))

import { ArtistGraphDialog } from './RelatedArtists'

const node = (id: number): ArtistGraphNode => ({ id, name: `a${id}`, slug: `a${id}`, upcoming_show_count: 0 })
const link = (s: number, t: number): ArtistGraphLink => ({ source_id: s, target_id: t, type: 'similar', score: 0.5, votes_up: 0, votes_down: 0 })
const ego = (centerId: number, neighborIds: number[], links: ArtistGraphLink[]): ArtistGraph => ({
  center: node(centerId),
  nodes: neighborIds.map(node),
  links,
  user_votes: {},
})

const baseGraph = ego(1, [2, 3], [link(1, 2), link(1, 3)])
const exp2 = ego(2, [4], [link(2, 4)]) // expanding node 2 reveals node 4 at hop 2
const exp3 = ego(3, [5], [link(3, 5)]) // expanding node 3 reveals node 5 at hop 2

const nodeIds = () => (vizProps.data.nodes as ArtistGraphNode[]).map(n => n.id).sort()
const expandedIds = () => [...(vizProps.expandedIds as Set<number>)].sort()
const expandingIds = () => [...(vizProps.expandingIds as Set<number>)].sort()
const suggestedIds = () => [...(vizProps.suggestedIds as Set<number>)].sort((a, b) => a - b)

const renderDialog = () =>
  renderWithProviders(
    <ArtistGraphDialog artistId={1} artistSlug="a1" artistName="A1" open onOpenChange={() => {}} />,
  )

describe('RecenteringGraph — expand-on-demand orchestration (PSY-1259)', () => {
  beforeEach(() => {
    vizProps = null
    fetchCalls.length = 0
    routerPush.mockClear()
    mockUseArtistGraph.mockReturnValue({ data: baseGraph, isFetching: false })
  })

  it('opens on the base 1-hop ego with nothing expanded', () => {
    renderDialog()
    expect(nodeIds()).toEqual([2, 3])
    expect(expandedIds()).toEqual([])
  })

  it('expand fetches, marks the node loading, then merges its neighbors on resolve', async () => {
    renderDialog()
    act(() => vizProps.onExpand({ id: 2, slug: 'a2', name: 'a2' }))
    expect(expandingIds()).toEqual([2]) // loading while the fetch is in flight
    expect(fetchCalls).toHaveLength(1)

    await act(async () => {
      fetchCalls[0].resolve(exp2)
    })
    expect(expandedIds()).toEqual([2])
    expect(nodeIds()).toEqual([2, 3, 4]) // node 4 revealed at hop 2
    expect(vizProps.hopByNodeId.get(4)).toBe(2)
    expect(expandingIds()).toEqual([]) // loading cleared
  })

  it('collapses an already-expanded node on re-click (prunes its ring)', async () => {
    renderDialog()
    act(() => vizProps.onExpand({ id: 2, slug: 'a2', name: 'a2' }))
    await act(async () => fetchCalls[0].resolve(exp2))
    expect(nodeIds()).toEqual([2, 3, 4])

    act(() => vizProps.onExpand({ id: 2, slug: 'a2', name: 'a2' })) // re-click = collapse
    expect(expandedIds()).toEqual([])
    expect(nodeIds()).toEqual([2, 3]) // node 4 pruned
  })

  it('ignores a click on a node whose fetch is still in flight', () => {
    renderDialog()
    act(() => vizProps.onExpand({ id: 2, slug: 'a2', name: 'a2' }))
    act(() => vizProps.onExpand({ id: 2, slug: 'a2', name: 'a2' })) // re-click while loading
    expect(fetchCalls).toHaveLength(1) // not re-fetched
  })

  it('Collapse-all CANCELS an in-flight expand — the ring does not pop back (the race guard)', async () => {
    renderDialog()
    // One landed expansion (so the Collapse-all control is rendered)…
    act(() => vizProps.onExpand({ id: 2, slug: 'a2', name: 'a2' }))
    await act(async () => fetchCalls[0].resolve(exp2))
    expect(nodeIds()).toEqual([2, 3, 4])

    // …and a SECOND expand still in flight.
    act(() => vizProps.onExpand({ id: 3, slug: 'a3', name: 'a3' }))
    expect(expandingIds()).toEqual([3])

    // User hits "Collapse all" before node 3's fetch resolves.
    fireEvent.click(screen.getByRole('button', { name: /collapse all/i }))
    expect(nodeIds()).toEqual([2, 3]) // collapsed back to base immediately

    // node 3's fetch resolves AFTER the collapse — the generation guard must drop it.
    await act(async () => fetchCalls[1].resolve(exp3))
    expect(expandedIds()).toEqual([]) // nothing re-added
    expect(nodeIds()).toEqual([2, 3]) // node 5 never appears, node 4 stays gone
  })
})

describe('RecenteringGraph — DOI suggested expansion directions (PSY-1273)', () => {
  beforeEach(() => {
    vizProps = null
    fetchCalls.length = 0
    routerPush.mockClear()
    mockUseArtistGraph.mockReturnValue({ data: baseGraph, isFetching: false })
  })

  it('flags unexpanded neighbors as DOI-ranked suggestions and passes per-node DOI scores', () => {
    renderDialog()
    expect(suggestedIds()).toEqual([2, 3]) // both hop-1, unexpanded
    expect((vizProps.doiByNodeId as Map<number, number>).size).toBe(2)
  })

  it('drops an expanded node from the suggestions and surfaces its newly-revealed neighbor', async () => {
    renderDialog()
    act(() => vizProps.onExpand({ id: 2, slug: 'a2', name: 'a2' }))
    await act(async () => fetchCalls[0].resolve(exp2)) // reveals node 4 at hop 2
    // node 2 is now expanded (no longer a suggestion); node 3 + the newly-revealed node 4 are.
    expect(suggestedIds()).toEqual([3, 4])
  })

  it('drops an in-flight (expanding) node from the suggestions', () => {
    renderDialog()
    act(() => vizProps.onExpand({ id: 2, slug: 'a2', name: 'a2' })) // fetch in flight, unresolved
    expect(expandingIds()).toEqual([2])
    expect(suggestedIds()).toEqual([3]) // node 2 excluded while loading
  })

  it('caps suggestions at 5 so a hub does not flag all its neighbours', () => {
    const wide = ego(
      1,
      [2, 3, 4, 5, 6, 7, 8],
      [link(1, 2), link(1, 3), link(1, 4), link(1, 5), link(1, 6), link(1, 7), link(1, 8)],
    )
    mockUseArtistGraph.mockReturnValue({ data: wide, isFetching: false })
    renderDialog()
    expect(suggestedIds()).toHaveLength(5)
  })
})

describe('RecenteringGraph — discovery-bias slider (PSY-1260)', () => {
  // node 2 is a hub (center + 4 + 5 → degree 3), node 3 a leaf (center only → degree 1); both
  // tied to the center by an equal `similar` edge, so only degree separates their DOI.
  const hubGraph = ego(1, [2, 3, 4, 5], [link(1, 2), link(1, 3), link(2, 4), link(2, 5)])

  beforeEach(() => {
    vizProps = null
    fetchCalls.length = 0
    mockUseArtistGraph.mockReturnValue({ data: hubGraph, isFetching: false })
  })

  it('defaults to the Popular end (slider value 0)', () => {
    renderDialog()
    expect((screen.getByRole('slider') as HTMLInputElement).value).toBe('0')
  })

  it('dragging Popular → Niche flips the DOI so the leaf outranks the hub', () => {
    renderDialog()
    const doiPopular = vizProps.doiByNodeId as Map<number, number>
    expect(doiPopular.get(2)!).toBeGreaterThan(doiPopular.get(3)!) // default: hub wins

    fireEvent.change(screen.getByRole('slider'), { target: { value: '1' } }) // drag to Niche

    const doiNiche = vizProps.doiByNodeId as Map<number, number>
    expect(doiNiche.get(3)!).toBeGreaterThan(doiNiche.get(2)!) // niche: leaf surfaces above the hub
  })
})

describe('accessible connections tree + announcements (PSY-1304)', () => {
  beforeEach(() => {
    vizProps = null
    fetchCalls.length = 0
    routerPush.mockClear()
    mockUseArtistGraph.mockReturnValue({ data: baseGraph, isFetching: false })
  })

  it('renders a role=tree of the base neighbours, keyboard-navigable', () => {
    renderDialog()
    const tree = screen.getByRole('tree', { name: /Connections for a1/i })
    expect(tree).toBeInTheDocument()
    const items = screen.getAllByRole('treeitem')
    expect(items.map(el => el.textContent)).toEqual(
      expect.arrayContaining([expect.stringContaining('a2'), expect.stringContaining('a3')]),
    )
    // Exactly one roving tabstop.
    expect(items.filter(el => el.getAttribute('tabindex') === '0')).toHaveLength(1)
  })

  it('R on a tree row re-centers the graph on that artist (PSY-1361)', () => {
    renderDialog()
    // Focus starts on the first row (a2). R is the keyboard twin of the canvas
    // "Center on this artist" — it pushes the center-query URL the graph
    // pipeline re-fetches from (and the shared effect announces on center change).
    fireEvent.keyDown(screen.getByRole('tree', { name: /Connections for a1/i }), { key: 'r' })
    expect(routerPush).toHaveBeenCalledWith('/artists/a1?center=a2', { scroll: false })
  })

  it('associates the canvas with the sr-only note (aria-describedby, AC1)', () => {
    renderDialog()
    // The mocked canvas doesn't apply the attribute (that effect lives in the
    // real component), but the note it points at must exist in the DOM.
    expect(document.getElementById('ego-graph-a11y-note')).toBeInTheDocument()
    expect(vizProps.canvasDescribedById).toBe('ego-graph-a11y-note')
  })

  it('announces expand → collapse in the aria-live region (AC3)', async () => {
    renderDialog()
    // Focus starts on the first row (a2). Enter expands it.
    fireEvent.keyDown(screen.getByRole('tree'), { key: 'Enter' })
    expect(fetchCalls).toHaveLength(1)
    await act(async () => {
      fetchCalls[0].resolve(exp2)
    })
    expect(screen.getByText('Added 1 artist connected to a2.')).toBeInTheDocument()

    // Enter again collapses (a2 is now expanded).
    fireEvent.keyDown(screen.getByRole('tree'), { key: 'Enter' })
    expect(screen.getByText('Collapsed the connections under a2.')).toBeInTheDocument()
  })

  it('announces a relationship-type filter toggle (AC3)', () => {
    renderDialog()
    // 'similar' links are present and active by default → toggling hides them.
    fireEvent.click(screen.getByRole('button', { name: /^Similar$/i }))
    expect(screen.getByText('Similar connections hidden.')).toBeInTheDocument()
  })

  it('does not double-announce a same-center refetch alongside a filter toggle', () => {
    renderDialog()
    // Turning festival_cobill ON changes the fetch shape (same center refetch).
    // The filter announcement should fire; the re-center announcement must NOT.
    fireEvent.click(screen.getByRole('button', { name: /Festival co-lineup/i }))
    expect(screen.getByText('Festival co-lineup connections shown.')).toBeInTheDocument()
    // The initial open already announced the re-center; it must not re-fire for
    // the same center.
    expect(screen.queryByText(/Graph now centered on A1/)).not.toBeInTheDocument()
  })
})

describe('accessible tree — code-review fixes (PSY-1304)', () => {
  beforeEach(() => {
    vizProps = null
    fetchCalls.length = 0
    routerPush.mockClear()
    mockUseArtistGraph.mockReturnValue({ data: baseGraph, isFetching: false })
  })

  it('tree honors the type filter — toggling a type off drops those artists', () => {
    renderDialog()
    expect(screen.getAllByRole('treeitem')).toHaveLength(2) // a2, a3 (both via 'similar')
    // baseGraph's only links are 'similar'; hiding it leaves no active edges.
    fireEvent.click(screen.getByRole('button', { name: /^Similar$/i }))
    expect(screen.queryAllByRole('treeitem')).toHaveLength(0)
    expect(screen.getByText('No connections to navigate.')).toBeInTheDocument()
  })

  it('announces a failed expand instead of going silent', async () => {
    renderDialog()
    fireEvent.keyDown(screen.getByRole('tree'), { key: 'Enter' })
    expect(fetchCalls).toHaveLength(1)
    await act(async () => {
      fetchCalls[0].reject(new Error('network'))
    })
    expect(screen.getByText(/Couldn.t load connections for a2\. Try again\./)).toBeInTheDocument()
  })
})

describe('accessible tree — adversarial-review fix (PSY-1304)', () => {
  beforeEach(() => {
    vizProps = null
    fetchCalls.length = 0
    routerPush.mockClear()
    mockUseArtistGraph.mockReturnValue({ data: baseGraph, isFetching: false })
  })

  it('announces the bulk Collapse-all (AC3), not just single collapses', async () => {
    renderDialog()
    // Expand a node so the Collapse-all control appears.
    fireEvent.keyDown(screen.getByRole('tree'), { key: 'Enter' })
    await act(async () => {
      fetchCalls[0].resolve(exp2)
    })
    fireEvent.click(screen.getByRole('button', { name: /Collapse all/i }))
    expect(screen.getByText('Collapsed all expansions back to the starting graph.')).toBeInTheDocument()
  })
})

describe('depth control (1 / 2 hops, PSY-1303)', () => {
  beforeEach(() => {
    vizProps = null
    fetchCalls.length = 0
    routerPush.mockClear()
    mockUseArtistGraph.mockReturnValue({ data: baseGraph, isFetching: false })
  })

  it('depth 2 auto-expands the base neighbours and announces the batch exactly once', async () => {
    renderDialog()
    fireEvent.click(screen.getByRole('button', { name: '2 hops' }))
    // Base has 2 neighbours (a2, a3), both under the top-K cap → both fetched.
    expect(fetchCalls.map(c => c.id).sort()).toEqual([2, 3])
    await act(async () => {
      fetchCalls.find(c => c.id === 2)!.resolve(exp2)
      fetchCalls.find(c => c.id === 3)!.resolve(exp3)
    })
    // Canvas AND the accessible tree both show the merged second hop (a4 via a2,
    // a5 via a3) — AC3: the sidebar reflects the depth change.
    expect(nodeIds()).toEqual([2, 3, 4, 5])
    expect(screen.getAllByRole('treeitem')).toHaveLength(4)
    // ONE aria-live announcement for the whole batch, not one per node.
    expect(
      screen.getByText('Showing 2 hops — added 2 artists from the top connections.'),
    ).toBeInTheDocument()
  })

  it('depth 1 collapses the auto-expanded set back to the base (reversible)', async () => {
    renderDialog()
    fireEvent.click(screen.getByRole('button', { name: '2 hops' }))
    await act(async () => {
      fetchCalls.find(c => c.id === 2)!.resolve(exp2)
      fetchCalls.find(c => c.id === 3)!.resolve(exp3)
    })
    expect(nodeIds()).toEqual([2, 3, 4, 5])
    fireEvent.click(screen.getByRole('button', { name: '1 hop' }))
    expect(nodeIds()).toEqual([2, 3]) // restored to the base ego
    expect(
      screen.getByText('Back to 1 hop — collapsed the second-hop connections.'),
    ).toBeInTheDocument()
  })

  it('depth 2 caps auto-expansion at the top-K, never every neighbour (no hairball)', () => {
    // A dense ego: 12 neighbours. Depth 2 must fetch only the top 8 (DEPTH_2_TOP_K).
    const dense = ego(
      1,
      [2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13],
      Array.from({ length: 12 }, (_, i) => link(1, i + 2)),
    )
    mockUseArtistGraph.mockReturnValue({ data: dense, isFetching: false })
    renderDialog()
    fireEvent.click(screen.getByRole('button', { name: '2 hops' }))
    expect(fetchCalls).toHaveLength(8)
  })
})
