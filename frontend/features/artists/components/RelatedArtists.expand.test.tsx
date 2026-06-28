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
vi.mock('./ArtistGraph', () => ({
  ArtistGraphVisualization: (props: Record<string, unknown>) => {
    vizProps = props
    return <div data-testid="viz" />
  },
}))

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
vi.mock('next/navigation', () => ({
  useRouter: () => ({ push: vi.fn(), replace: vi.fn() }),
  usePathname: () => '/artists/a1',
  useSearchParams: () => new URLSearchParams(),
}))
vi.mock('../hooks/useArtists', () => ({
  useArtist: () => ({ data: undefined, isLoading: false }),
}))

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
