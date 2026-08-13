import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { Ref } from 'react'
import { act, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'

import { renderWithProviders } from '@/test/utils'
import type { ArtistGraph } from '@/features/artists/types'

const { fetchGraph, graphs, reviewState, shuffleRefetch, shuffleTarget } = vi.hoisted(() => {
  const dinersGraph: ArtistGraph = {
    center: {
      id: 1,
      name: 'Diners',
      slug: 'diners',
      city: 'Phoenix',
      state: 'AZ',
      upcoming_show_count: 1,
    },
    nodes: [
      {
        id: 2,
        name: 'Playboy Manbaby',
        slug: 'playboy-manbaby',
        city: 'Phoenix',
        state: 'AZ',
        upcoming_show_count: 0,
      },
    ],
    links: [
      {
        source_id: 1,
        target_id: 2,
        type: 'shared_bills',
        score: 3,
        votes_up: 0,
        votes_down: 0,
      },
    ],
  }
  const playboyGraph: ArtistGraph = {
    center: dinersGraph.nodes[0],
    nodes: [dinersGraph.center],
    links: dinersGraph.links,
  }
  return {
    fetchGraph: vi.fn(),
    graphs: new Map<number, ArtistGraph>([
      [1, dinersGraph],
      [2, playboyGraph],
    ]),
    reviewState: { graphError: false, throwGraph: false, graphPending: false },
    shuffleRefetch: vi.fn(),
    shuffleTarget: {
      artist_id: 2,
      artist_slug: 'playboy-manbaby',
      artist_name: 'Playboy Manbaby',
    },
  }
})

const { searchRequest, scenesState, geoState, cbsaState, motionState } = vi.hoisted(() => ({
  searchRequest: vi.fn(),
  scenesState: {
    scenes: [] as Array<Record<string, unknown>>,
  },
  geoState: {
    geo: null as { city: string; state: string } | null,
  },
  // Member-slug → principal from GET /scenes/{slug} (ParseSceneSlug). Empty
  // means the slug is not a scene CBSA member (404), which keeps /shows.
  cbsaState: {
    bySlug: {} as Record<string, { city: string; state: string; slug: string }>,
  },
  motionState: { reduced: true },
}))

// The connectivity-ranked starting suggestions (PSY-1749). The DEFAULT pool is
// a SINGLE artist, which makes the rotation's random draw deterministic without
// stubbing Math.random — every pre-existing assertion about the hero's sentence
// still names Diners. Multi-entry pools are opted into per test.
const { startingPointsState } = vi.hoisted(() => ({
  startingPointsState: {
    artists: [{ artist_id: 1, artist_name: 'Diners', artist_slug: 'diners' }] as Array<{
      artist_id: number
      artist_name: string
      artist_slug: string
    }>,
    isPending: false,
    hasFailed: false,
  },
}))

vi.mock('../hooks/useGraphStartingPoints', () => ({
  useGraphStartingPoints: () => ({
    // A settled FAILURE looks like a settled empty pool to this component:
    // no data, not pending. Both reach the random fallback, which is the
    // point — the hero must never depend on this endpoint succeeding.
    data:
      startingPointsState.isPending || startingPointsState.hasFailed
        ? undefined
        : { artists: startingPointsState.artists },
    isPending: startingPointsState.isPending,
  }),
}))

// A pool wide enough that the draw picks a subset. IDs line up with the graph
// fixtures where a click has to land somewhere.
const RANKED_POOL = [
  { artist_id: 1, artist_name: 'Diners', artist_slug: 'diners' },
  { artist_id: 2, artist_name: 'Playboy Manbaby', artist_slug: 'playboy-manbaby' },
  { artist_id: 77, artist_name: 'Gatecreeper', artist_slug: 'gatecreeper' },
  { artist_id: 78, artist_name: 'Sundressed', artist_slug: 'sundressed' },
  { artist_id: 79, artist_name: 'Injury Reserve', artist_slug: 'injury-reserve' },
]

vi.mock('@/lib/api', async importOriginal => {
  const actual = await importOriginal<typeof import('@/lib/api')>()
  return {
    ...actual,
    apiRequest: (url: string, options?: RequestInit) => {
      if (url.includes('/artists/search')) return searchRequest(url, options)
      return actual.apiRequest(url, options)
    },
  }
})

vi.mock('@/features/scenes/hooks/useScenes', () => ({
  useScenes: () => ({
    data: { scenes: scenesState.scenes, count: scenesState.scenes.length },
    isLoading: false,
    isPending: false,
    isError: false,
  }),
  useSceneDetail: (slug: string) => ({
    data: slug ? cbsaState.bySlug[slug] : undefined,
    isPending: false,
    isError: false,
  }),
}))

// The visitor's IP-derived place. Mocked at the hook boundary so no test here
// reaches the `/api/geo` route or the sessionStorage cache behind it.
vi.mock('@/lib/hooks/common/useGeoDefaultScene', () => ({
  useGeoDefaultScene: () => geoState.geo,
}))

vi.mock('@sentry/nextjs', () => ({
  captureException: vi.fn(),
}))

vi.mock('@/components/graph/useContainerWidth', () => ({
  GRAPH_BREAKPOINT_PX: 640,
  useContainerWidth: () => ({ refCallback: vi.fn(), containerWidth: 1024 }),
}))

vi.mock('@/features/artists/hooks/useReducedMotion', () => ({
  useReducedMotion: () => motionState.reduced,
}))

vi.mock('@/features/artists/components/ArtistSearch', () => ({
  ArtistSearch: ({
    onSelect,
    ref,
  }: {
    onSelect: (artist: Record<string, unknown>) => void
    ref?: Ref<HTMLInputElement>
  }) => (
    <>
      <input ref={ref} aria-label="Mock artist search" />
      <button
        type="button"
        onClick={() => onSelect({
          id: 1,
          name: 'Diners',
          slug: 'diners',
          city: 'Phoenix',
          state: 'AZ',
        })}
      >
        Search Diners
      </button>
    </>
  ),
}))

vi.mock('@/features/artists/components/ArtistGraph', () => ({
  ArtistGraphVisualization: ({
    data,
    onSelect,
    onConnectionInspectOpen,
    focusNodeId,
    labelTiers,
  }: {
    data: ArtistGraph
    onSelect: (node: ArtistGraph['center']) => void
    onConnectionInspectOpen?: () => void
    focusNodeId?: number | null
    labelTiers?: readonly { fontSize: number }[]
  }) => {
    if (reviewState.throwGraph) throw new Error('graph chunk failed')
    return (
      <div
        aria-label={`Graph centered on ${data.center.name}`}
        data-has-label-tiers={String(labelTiers !== undefined)}
        data-focus-node-id={String(focusNodeId ?? '')}
      >
        <button type="button" onClick={() => onSelect(data.nodes[0])}>
          Select {data.nodes[0].name}
        </button>
        <button type="button" onClick={() => onConnectionInspectOpen?.()}>
          Inspect edge
        </button>
      </div>
    )
  },
}))

vi.mock('@/features/artists/hooks/useArtistGraph', () => ({
  useArtistGraph: ({ artistId }: { artistId: number }) => ({
    data: reviewState.graphPending ? undefined : graphs.get(artistId),
    isPending: reviewState.graphPending,
    isError: reviewState.graphError,
    refetch: vi.fn(),
  }),
  useFetchArtistGraph: () => fetchGraph,
}))

vi.mock('@/features/artists/hooks/useArtistGraphCard', () => ({
  useArtistGraphCard: ({ artistId }: { artistId: number | null }) => ({
    data: artistId
      ? {
          id: artistId,
          name: graphs.get(artistId)?.center.name ?? 'Artist',
          slug: graphs.get(artistId)?.center.slug ?? 'artist',
          city: 'Phoenix',
          state: 'AZ',
          bandcamp_embed_url: null,
          spotify: null,
          next_show: null,
          labels: [],
          radio: null,
          connections: { bills: 1, similar: 0, members: 0, radio: 0, shared_labels: 0 },
        }
      : undefined,
    isError: false,
  }),
}))

vi.mock('@/features/discovery/useRandomArtistTarget', () => ({
  useRandomArtistTarget: () => ({
    refetch: shuffleRefetch,
    isFetching: false,
  }),
}))

// The Map of the Scene (PSY-1725). The DEFAULT is "no snapshot built yet" —
// the state a dev seed and a cold catalog are both in — so every pre-existing
// assertion about the search-first hero still describes the surface a visitor
// gets there, and the map branches are opted into per test.
const { overviewState, overviewRefetch, sceneMapFixture } = vi.hoisted(() => {
  const notBuilt = Object.assign(new Error('not built'), { status: 503 })
  return {
    overviewRefetch: vi.fn(),
    overviewState: {
      data: undefined as unknown,
      isPending: false,
      isError: true,
      error: notBuilt as unknown,
    },
    sceneMapFixture: {
      nodes: [
        {
          id: 1,
          kind: 'artist' as const,
          name: 'Diners',
          slug: 'diners',
          x: 0,
          y: 0,
          community: 3,
          degree: 2,
          rank: 0,
          hasUpcomingShow: false,
          hasPlayableAudio: false,
          appear: 0,
        },
      ],
      edges: [],
      regions: [
        {
          community: 3,
          label: 'Around Diners',
          memberCount: 1,
          hull: [],
          captionAnchor: null,
        },
      ],
      artistCount: 1240,
      labelCount: 18,
      isolateCount: 42,
      lastMapped: new Date('2026-08-02T04:00:00Z'),
      epoch: new Date('2020-01-01T00:00:00Z'),
    },
  }
})

vi.mock('../hooks/useGraphOverview', () => ({
  useGraphOverview: () => ({ ...overviewState, refetch: overviewRefetch }),
  isGraphOverviewNotBuilt: (error: unknown) =>
    (error as { status?: number } | null)?.status === 503,
}))

// `buildSceneMap` has its own unit coverage; here the decode is short-circuited
// so a branch test can't fail on a hand-encoded base64 fixture.
vi.mock('../sceneMap', async importOriginal => {
  const actual = await importOriginal<typeof import('../sceneMap')>()
  return { ...actual, buildSceneMap: () => sceneMapFixture }
})

// jsdom renders no canvas; the map's own card is covered in
// SceneMapZeroState.test.tsx.
vi.mock('./SceneMapCanvas', () => ({
  SceneMapCanvas: ({ ariaLabel }: { ariaLabel: string }) => (
    <div aria-label={ariaLabel} />
  ),
}))

import { GraphObservatory, resolveZeroStateView } from './GraphObservatory'
import { pickRotationSuggestions } from '../startingSuggestions'

// The seed the component draws is `Math.floor(Math.random() * 0x7fffffff)`.
// Stubbing Math.random therefore pins the draw, and running the SAME pure
// picker here derives what the sentence must then say — so these assert the
// wiring (pool → seed → sentence) rather than restating a shuffle.
function expectedNames(random: number) {
  return pickRotationSuggestions(RANKED_POOL, Math.floor(random * 0x7fffffff)).map(
    anchor => anchor.name,
  )
}

describe('GraphObservatory', () => {
  beforeEach(() => {
    reviewState.graphError = false
    reviewState.throwGraph = false
    reviewState.graphPending = false
    scenesState.scenes = []
    geoState.geo = null
    cbsaState.bySlug = {}
    motionState.reduced = true
    overviewState.data = undefined
    overviewState.isPending = false
    overviewState.isError = true
    overviewState.error = Object.assign(new Error('not built'), { status: 503 })
    overviewRefetch.mockReset()
    fetchGraph.mockReset()
    fetchGraph.mockImplementation(async (artistId: number) => graphs.get(artistId))
    shuffleRefetch.mockReset()
    shuffleRefetch.mockResolvedValue({ data: shuffleTarget, isError: false })
    startingPointsState.artists = [
      { artist_id: 1, artist_name: 'Diners', artist_slug: 'diners' },
    ]
    startingPointsState.isPending = false
    startingPointsState.hasFailed = false
    searchRequest.mockReset()
    searchRequest.mockResolvedValue({
      artists: [
        { id: 1, name: 'Diners', slug: 'diners', city: 'Phoenix', state: 'AZ' },
      ],
      count: 1,
    })
  })

  it('starts from search, opens context, hops with a trail, and resets', async () => {
    const user = userEvent.setup()
    renderWithProviders(<GraphObservatory />)

    expect(screen.getByRole('heading', { name: 'Explore the graph.' })).toBeInTheDocument()
    // Validated curated names are stacked in the crossfade; the ACTIVE one is
    // what the button announces (reduced-motion mock freezes it on index 0).
    // findBy: the names render only after mount-time validation resolves.
    expect(await screen.findByRole('button', { name: 'Search for Diners' })).toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: 'Search Diners' }))
    const canvas = screen.getByLabelText('Graph centered on Diners')
    expect(canvas).toBeInTheDocument()
    // Tool-surface pin: the Observatory passes the tier ladder to the ego
    // canvas (locked spec) — a dropped prop would silently flatten labels.
    expect(canvas).toHaveAttribute('data-has-label-tiers', 'true')

    await user.click(screen.getByRole('button', { name: 'Select Playboy Manbaby' }))
    expect(screen.getByRole('region', { name: 'About Playboy Manbaby' })).toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: /Center here/i }))
    expect(screen.getByLabelText('Graph centered on Playboy Manbaby')).toBeInTheDocument()
    expect(screen.getByRole('navigation', { name: 'Graph traversal history' })).toHaveTextContent(
      'DinersPlayboy Manbaby',
    )

    await user.click(screen.getByRole('button', { name: 'Reset' }))
    expect(screen.getByRole('heading', { name: 'Explore the graph.' })).toBeInTheDocument()
    expect(screen.queryByLabelText('Graph centered on Diners')).not.toBeInTheDocument()
    expect(screen.queryByRole('navigation', { name: 'Graph traversal history' })).not.toBeInTheDocument()
    await waitFor(() => expect(screen.getByRole('textbox', { name: 'Mock artist search' })).toHaveFocus())
  })

  // PSY-1478: the pinned focus follows the selection, a second click on the
  // selected node puts it away (shared toggle grammar), and an edge click
  // opening the ConnectionPanel deselects so the two inspectors never stack
  // (and the lingering selection can't suppress the edge-endpoint pin).
  it('pins focusNodeId to the selection, releases it on second click, and deselects on edge inspect', async () => {
    const user = userEvent.setup()
    renderWithProviders(<GraphObservatory />)
    await user.click(screen.getByRole('button', { name: 'Search Diners' }))
    const canvas = screen.getByLabelText('Graph centered on Diners')
    expect(canvas).toHaveAttribute('data-focus-node-id', '')

    // Select → panel opens and the pin prop carries the selected id.
    await user.click(screen.getByRole('button', { name: 'Select Playboy Manbaby' }))
    expect(screen.getByRole('region', { name: 'About Playboy Manbaby' })).toBeInTheDocument()
    expect(canvas).not.toHaveAttribute('data-focus-node-id', '')

    // Second click on the same node → deselect ("put it away").
    await user.click(screen.getByRole('button', { name: 'Select Playboy Manbaby' }))
    expect(screen.queryByRole('region', { name: 'About Playboy Manbaby' })).not.toBeInTheDocument()
    expect(canvas).toHaveAttribute('data-focus-node-id', '')

    // Re-select, then an edge click (ConnectionPanel opening) deselects too.
    await user.click(screen.getByRole('button', { name: 'Select Playboy Manbaby' }))
    expect(screen.getByRole('region', { name: 'About Playboy Manbaby' })).toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: 'Inspect edge' }))
    expect(screen.queryByRole('region', { name: 'About Playboy Manbaby' })).not.toBeInTheDocument()
    expect(canvas).toHaveAttribute('data-focus-node-id', '')
  })

  it('uses the existing shuffle target as the random rabbit-hole root', async () => {
    const user = userEvent.setup()
    renderWithProviders(<GraphObservatory />)

    await user.click(screen.getByRole('button', { name: 'A random rabbit hole' }))
    expect(shuffleRefetch).toHaveBeenCalled()
    expect(screen.getByLabelText('Graph centered on Playboy Manbaby')).toBeInTheDocument()
  })

  it('runs the rabbit hole from the zero-state badge button', async () => {
    const user = userEvent.setup()
    renderWithProviders(<GraphObservatory />)

    await user.click(screen.getByRole('button', { name: 'Take a random rabbit hole' }))

    expect(shuffleRefetch).toHaveBeenCalled()
    expect(screen.getByLabelText('Graph centered on Playboy Manbaby')).toBeInTheDocument()
  })

  // PSY-1749: the suggestion is a catalog ANCHOR now, not a name, so the click
  // centres synchronously. The search round-trip it used to make was the only
  // way this affordance could fail.
  it('centers the graph directly when the ranked suggestion is clicked', async () => {
    const user = userEvent.setup()
    renderWithProviders(<GraphObservatory />)

    await user.click(await screen.findByRole('button', { name: 'Search for Diners' }))

    expect(searchRequest).not.toHaveBeenCalled()
    expect(screen.getByLabelText('Graph centered on Diners')).toBeInTheDocument()
    expect(screen.queryByRole('status')).not.toBeInTheDocument()
  })

  it('rotates a draw from the ranked pool rather than the whole pool', async () => {
    startingPointsState.artists = RANKED_POOL
    vi.spyOn(Math, 'random').mockReturnValue(0.42)
    try {
      renderWithProviders(<GraphObservatory />)
      const names = expectedNames(0.42)

      await screen.findByRole('button', { name: `Search for ${names[0]}` })
      for (const name of names) {
        expect(screen.getByText(name)).toBeInTheDocument()
      }
      // The crossfade stacks every offered name in one grid cell, so offering
      // the whole pool would size the sentence's slot to the longest of them.
      const offered = RANKED_POOL.filter(a =>
        screen.queryByText(a.artist_name) !== null,
      )
      expect(offered).toHaveLength(names.length)
      expect(names.length).toBeLessThan(RANKED_POOL.length)
    } finally {
      vi.mocked(Math.random).mockRestore()
    }
  })

  // THE TICKET'S COMPLAINT: the hardcoded list was fixed and ordered, so the
  // hero opened on the same name every visit. A fresh seed per mount is what
  // makes two visits differ.
  it('redraws the rotation on a new visit', async () => {
    startingPointsState.artists = RANKED_POOL
    const first = expectedNames(0.05)
    const second = expectedNames(0.83)
    expect(first[0]).not.toBe(second[0])

    const randomSpy = vi.spyOn(Math, 'random').mockReturnValue(0.05)
    try {
      const firstVisit = renderWithProviders(<GraphObservatory />)
      await screen.findByRole('button', { name: `Search for ${first[0]}` })
      firstVisit.unmount()

      randomSpy.mockReturnValue(0.83)
      renderWithProviders(<GraphObservatory />)
      expect(
        await screen.findByRole('button', { name: `Search for ${second[0]}` }),
      ).toBeInTheDocument()
    } finally {
      randomSpy.mockRestore()
    }
  })

  it('drops a suggestion the catalog cannot honour', async () => {
    startingPointsState.artists = [
      { artist_id: 1, artist_name: 'Diners', artist_slug: 'diners' },
      // No slug: the click would promise a page it cannot open.
      { artist_id: 2, artist_name: 'Playboy Manbaby', artist_slug: '' },
    ]
    renderWithProviders(<GraphObservatory />)

    await screen.findByRole('button', { name: 'Search for Diners' })
    expect(screen.queryByText('Playboy Manbaby')).not.toBeInTheDocument()
    // One offerable entry is still an answer — no fallback fetch.
    expect(shuffleRefetch).not.toHaveBeenCalled()
  })

  it('falls back to a random catalog artist when the ranked pool is empty', async () => {
    const user = userEvent.setup()
    // A catalog before its first nightly build: the endpoint answers 200 with
    // nothing to suggest, which is a state rather than a failure.
    startingPointsState.artists = []
    renderWithProviders(<GraphObservatory />)

    const button = await screen.findByRole('button', { name: 'Search for Playboy Manbaby' })
    expect(shuffleRefetch).toHaveBeenCalled()

    await user.click(button)

    // Centers directly from the catalog target — no failable search round-trip.
    expect(screen.getByLabelText('Graph centered on Playboy Manbaby')).toBeInTheDocument()
    expect(screen.queryByRole('status')).not.toBeInTheDocument()
  })

  // The suggestion endpoint going down must cost the sentence its RANKING, not
  // the sentence. A failed fetch and an empty pool are the same state to this
  // surface, and both land on the random catalog artist.
  it('falls back to a random catalog artist when the ranked pool fails to load', async () => {
    const user = userEvent.setup()
    startingPointsState.hasFailed = true
    renderWithProviders(<GraphObservatory />)

    const button = await screen.findByRole('button', { name: 'Search for Playboy Manbaby' })
    expect(shuffleRefetch).toHaveBeenCalled()

    await user.click(button)

    expect(screen.getByLabelText('Graph centered on Playboy Manbaby')).toBeInTheDocument()
    expect(screen.queryByRole('status')).not.toBeInTheDocument()
  })

  it('holds the name slot open while the ranked pool is in flight', () => {
    // No flash of a name that then vanishes, and no fragment of a sentence
    // promising nothing.
    startingPointsState.isPending = true
    renderWithProviders(<GraphObservatory />)

    expect(screen.getByText(/Try searching for/)).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: /^Search for / })).not.toBeInTheDocument()
    expect(shuffleRefetch).not.toHaveBeenCalled()
  })

  it('advances the sentence through the drawn suggestions on a timer', async () => {
    startingPointsState.artists = RANKED_POOL
    motionState.reduced = false
    vi.spyOn(Math, 'random').mockReturnValue(0.42)
    const names = expectedNames(0.42)
    // NOTE: no waitFor/findBy while fake timers are active — RTL polls with
    // the (mocked) setTimeout and hangs. React Query also batches observer
    // notifications through setTimeout(0), so flush with async timer
    // advances instead of plain microtask flushes.
    vi.useFakeTimers()
    try {
      renderWithProviders(<GraphObservatory />)
      await act(async () => {
        await vi.advanceTimersByTimeAsync(50)
      })
      expect(screen.getByRole('button', { name: `Search for ${names[0]}` })).toBeInTheDocument()

      act(() => {
        vi.advanceTimersByTime(4000)
      })
      expect(screen.getByRole('button', { name: `Search for ${names[1]}` })).toBeInTheDocument()
    } finally {
      vi.useRealTimers()
      vi.mocked(Math.random).mockRestore()
    }
  })

  it('wraps a stale rotation index when the offered set shrinks', async () => {
    startingPointsState.artists = RANKED_POOL
    motionState.reduced = false
    vi.spyOn(Math, 'random').mockReturnValue(0.42)
    const names = expectedNames(0.42)
    vi.useFakeTimers()
    try {
      const { rerender } = renderWithProviders(<GraphObservatory />)
      await act(async () => {
        await vi.advanceTimersByTimeAsync(50)
      })
      // Two rotations land the index on the third name.
      act(() => {
        vi.advanceTimersByTime(8000)
      })
      expect(screen.getByRole('button', { name: `Search for ${names[2]}` })).toBeInTheDocument()

      // The pool shrinks under the mounted component (a background refetch
      // returning fewer entries). The stale index must wrap rather than render
      // undefined into the button's accessible name.
      startingPointsState.artists = [RANKED_POOL[0]]
      await act(async () => {
        rerender(<GraphObservatory />)
        await vi.advanceTimersByTimeAsync(50)
      })

      const button = screen.getByRole('button', { name: 'Search for Diners' })
      expect(button).toHaveTextContent('Diners')
      expect(button).not.toHaveTextContent('undefined')
    } finally {
      vi.useRealTimers()
      vi.mocked(Math.random).mockRestore()
    }
  })

  it('middle-collapses long trails behind a disclosure that expands on demand', async () => {
    const user = userEvent.setup()
    renderWithProviders(<GraphObservatory />)

    await user.click(screen.getByRole('button', { name: 'Search Diners' }))
    // 4 hops → trail [Diners, PM, Diners, PM], current Diners.
    for (const artistName of ['Playboy Manbaby', 'Diners', 'Playboy Manbaby', 'Diners']) {
      await user.click(screen.getByRole('button', { name: `Select ${artistName}` }))
      await user.click(screen.getByRole('button', { name: /Center here/i }))
    }

    const disclosure = screen.getByRole('button', { name: 'Show 2 more trail entries' })
    expect(disclosure).toHaveTextContent('… 2 more')
    // Collapsed: only the first and last trail entries stay clickable.
    expect(screen.getAllByRole('button', { name: 'Diners' })).toHaveLength(1)
    expect(screen.getAllByRole('button', { name: 'Playboy Manbaby' })).toHaveLength(1)

    await user.click(disclosure)
    expect(screen.queryByRole('button', { name: 'Show 2 more trail entries' })).not.toBeInTheDocument()
    expect(screen.getAllByRole('button', { name: 'Diners' })).toHaveLength(2)
    expect(screen.getAllByRole('button', { name: 'Playboy Manbaby' })).toHaveLength(2)
    // The disclosure unmounts on expand; focus hands off to the first
    // revealed chip so keyboard users keep their place.
    await waitFor(() =>
      expect(screen.getAllByRole('button', { name: 'Playboy Manbaby' })[0]).toHaveFocus(),
    )

    // A jump remounts the trail (epoch key), which re-collapses it.
    await user.click(screen.getAllByRole('button', { name: 'Playboy Manbaby' })[0])
    expect(screen.getByLabelText('Graph centered on Playboy Manbaby')).toBeInTheDocument()
    expect(screen.getByRole('navigation', { name: 'Graph traversal history' })).toBeInTheDocument()
  })

  it('shows the shared skeleton while the graph loads', async () => {
    const user = userEvent.setup()
    reviewState.graphPending = true
    renderWithProviders(<GraphObservatory />)

    await user.click(screen.getByRole('button', { name: 'Search Diners' }))

    expect(screen.getByText('Mapping Diners…')).toBeInTheDocument()
  })

  it('keeps reset available after jumping to an evicted-trail branch', async () => {
    const user = userEvent.setup()
    renderWithProviders(<GraphObservatory />)

    await user.click(screen.getByRole('button', { name: 'Search Diners' }))
    for (const artistName of ['Playboy Manbaby', 'Diners', 'Playboy Manbaby', 'Diners']) {
      await user.click(screen.getByRole('button', { name: `Select ${artistName}` }))
      await user.click(screen.getByRole('button', { name: /Center here/i }))
    }

    await user.click(screen.getAllByRole('button', { name: 'Playboy Manbaby' })[0])
    expect(screen.getByLabelText('Graph centered on Playboy Manbaby')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Reset' })).toBeInTheDocument()
    await waitFor(() => expect(screen.getByRole('button', { name: 'Reset' })).toHaveFocus())
  })

  it('shows an honest zero state with escape hatches for an artist without mapped connections', async () => {
    const user = userEvent.setup()
    const dinersGraph = graphs.get(1)!
    graphs.set(1, { center: dinersGraph.center, nodes: [], links: [] })
    scenesState.scenes = [
      { city: 'Phoenix', state: 'AZ', slug: 'phoenix-az', venue_count: 5, upcoming_show_count: 9, total_show_count: 90, shows_this_week: 2, shows_calendar_week: 2 },
      { city: 'Tucson', state: 'AZ', slug: 'tucson-az', venue_count: 3, upcoming_show_count: 4, total_show_count: 40, shows_this_week: 1, shows_calendar_week: 1 },
      { city: 'Portland', state: 'OR', slug: 'portland-or', venue_count: 4, upcoming_show_count: 20, total_show_count: 200, shows_this_week: 3, shows_calendar_week: 3 },
    ]

    try {
      renderWithProviders(<GraphObservatory />)
      await user.click(screen.getByRole('button', { name: 'Search Diners' }))

      expect(screen.getByRole('status')).toHaveTextContent('No mapped connections yet')
      expect(screen.queryByRole('list', { name: /Artists connected/ })).not.toBeInTheDocument()

      // 3 escape hatches (PSY-1474 F4): the metro scene, a same-state scene,
      // and the random rabbit hole — plus the existing artist-page link.
      expect(screen.getByRole('link', { name: /The Phoenix scene/ })).toHaveAttribute(
        'href',
        '/scenes/phoenix-az',
      )
      expect(screen.getByRole('link', { name: /The Tucson scene/ })).toHaveAttribute(
        'href',
        '/scenes/tucson-az',
      )
      expect(screen.getAllByRole('button', { name: 'A random rabbit hole' })).toHaveLength(2)
      expect(screen.getByRole('link', { name: /Open Diners/i })).toHaveAttribute(
        'href',
        '/artists/diners',
      )
    } finally {
      graphs.set(1, dinersGraph)
    }
  })

  it('centers the graph from the empty-state random escape hatch', async () => {
    const user = userEvent.setup()
    const dinersGraph = graphs.get(1)!
    graphs.set(1, { center: dinersGraph.center, nodes: [], links: [] })

    try {
      renderWithProviders(<GraphObservatory />)
      await user.click(screen.getByRole('button', { name: 'Search Diners' }))
      const [hatch] = screen.getAllByRole('button', { name: 'A random rabbit hole' })
      await user.click(hatch)

      expect(shuffleRefetch).toHaveBeenCalled()
      expect(screen.getByLabelText('Graph centered on Playboy Manbaby')).toBeInTheDocument()
    } finally {
      graphs.set(1, dinersGraph)
    }
  })

  it('does not reuse a stale shuffle target after a failed refresh', async () => {
    const user = userEvent.setup()
    renderWithProviders(<GraphObservatory />)

    await user.click(screen.getByRole('button', { name: 'A random rabbit hole' }))
    shuffleRefetch.mockResolvedValueOnce({ data: shuffleTarget, isError: true })
    await user.click(screen.getByRole('button', { name: 'A random rabbit hole' }))

    expect(screen.getByRole('status')).toHaveTextContent('No rabbit hole is available')
  })

  it('skips random artists whose graph has no connections', async () => {
    const user = userEvent.setup()
    graphs.set(3, {
      center: {
        id: 3,
        name: 'Solo Artist',
        slug: 'solo-artist',
        city: undefined,
        state: undefined,
        upcoming_show_count: 1,
      },
      nodes: [graphs.get(1)!.center],
      links: [],
    })
    shuffleRefetch
      .mockResolvedValueOnce({
        data: { artist_id: 3, artist_slug: 'solo-artist', artist_name: 'Solo Artist' },
        isError: false,
      })
      .mockResolvedValueOnce({ data: shuffleTarget, isError: false })
    renderWithProviders(<GraphObservatory />)

    await user.click(screen.getByRole('button', { name: 'A random rabbit hole' }))

    expect(shuffleRefetch).toHaveBeenCalledTimes(2)
    expect(screen.getByLabelText('Graph centered on Playboy Manbaby')).toBeInTheDocument()
    graphs.delete(3)
  })

  it('keeps cached graph data usable when a background refresh fails', async () => {
    const user = userEvent.setup()
    reviewState.graphError = true
    renderWithProviders(<GraphObservatory />)

    await user.click(screen.getByRole('button', { name: 'Search Diners' }))

    expect(screen.getByLabelText('Graph centered on Diners')).toBeInTheDocument()
    expect(screen.getByRole('status')).toHaveTextContent('Showing saved connections')
  })

  it('focuses the context panel after selection from the accessible list', async () => {
    const user = userEvent.setup()
    renderWithProviders(<GraphObservatory />)

    await user.click(screen.getByRole('button', { name: 'Search Diners' }))
    await user.click(screen.getByText('Browse connections as a list'))
    const listButton = screen.getByRole('button', { name: /^Playboy Manbaby/ })
    await user.click(listButton)

    await waitFor(() => {
      expect(screen.getByRole('region', { name: 'About Playboy Manbaby' })).toHaveFocus()
    })

    await user.click(screen.getByRole('button', { name: 'Close details for Playboy Manbaby' }))
    await waitFor(() => expect(listButton).toHaveFocus())

    await user.click(listButton)
    await user.click(screen.getByRole('button', { name: /Center here/i }))
    await waitFor(() => expect(screen.getByRole('button', { name: 'Reset' })).toHaveFocus())
  })

  it('does not let a pending random lookup undo Reset', async () => {
    const user = userEvent.setup()
    let resolveShuffle: ((value: { data: typeof shuffleTarget; isError: false }) => void) | undefined
    shuffleRefetch.mockReturnValueOnce(new Promise(resolve => {
      resolveShuffle = resolve
    }))
    renderWithProviders(<GraphObservatory />)

    await user.click(screen.getByRole('button', { name: 'Search Diners' }))
    await user.click(screen.getByRole('button', { name: 'A random rabbit hole' }))
    await user.click(screen.getByRole('button', { name: 'Reset' }))
    await act(async () => {
      resolveShuffle?.({ data: shuffleTarget, isError: false })
    })

    expect(screen.getByRole('heading', { name: 'Explore the graph.' })).toBeInTheDocument()
    expect(fetchGraph).not.toHaveBeenCalled()
  })

  it('contains canvas failures and keeps the accessible graph list available', async () => {
    const user = userEvent.setup()
    reviewState.throwGraph = true
    renderWithProviders(<GraphObservatory />)

    await user.click(screen.getByRole('button', { name: 'Search Diners' }))

    expect(screen.getByRole('status')).toHaveTextContent('interactive graph is unavailable')
    expect(screen.getByText('Browse connections as a list')).toBeInTheDocument()
  })

  describe('resolveZeroStateView', () => {
    const notBuilt = Object.assign(new Error('not built'), { status: 503 })
    const broken = Object.assign(new Error('boom'), { status: 500 })

    it('keeps a map it already has through a FAILED background refetch', () => {
      // React Query keeps `data` when a refetch fails, and refetches fire on
      // window focus and reconnect. Ordering the error test first would tear a
      // good on-screen map down and replace it with an error card.
      expect(
        resolveZeroStateView({ isPending: false, isError: true, error: broken, hasMap: true }),
      ).toBe('map')
      expect(
        resolveZeroStateView({ isPending: false, isError: true, error: notBuilt, hasMap: true }),
      ).toBe('map')
    })

    it('falls back to the hero when the snapshot has never been built', () => {
      expect(
        resolveZeroStateView({ isPending: false, isError: true, error: notBuilt, hasMap: false }),
      ).toBe('hero')
    })

    it('falls back to the hero when a payload arrived but could not be decoded', () => {
      expect(
        resolveZeroStateView({ isPending: false, isError: false, error: null, hasMap: false }),
      ).toBe('hero')
    })

    it('offers a retry only for a real failure with nothing to show', () => {
      expect(
        resolveZeroStateView({ isPending: false, isError: true, error: broken, hasMap: false }),
      ).toBe('unavailable')
    })

    it('reports loading only before anything has arrived', () => {
      expect(
        resolveZeroStateView({ isPending: true, isError: false, error: null, hasMap: false }),
      ).toBe('loading')
    })
  })

  // ── The Map of the Scene zero state (PSY-1725) ────────────────────────
  describe('zero state', () => {
    const showMap = () => {
      overviewState.data = {} as never
      overviewState.isPending = false
      overviewState.isError = false
      overviewState.error = null
    }

    it('opens on the map when a snapshot exists', () => {
      showMap()
      renderWithProviders(<GraphObservatory />)

      expect(screen.getByLabelText(/A map of 1240 connected artists/)).toBeInTheDocument()
      expect(
        screen.queryByRole('heading', { name: 'Explore the graph.' }),
      ).not.toBeInTheDocument()
    })

    it('reports the map size beside the search row', () => {
      showMap()
      renderWithProviders(<GraphObservatory />)

      expect(screen.getByText(/The whole map/)).toBeInTheDocument()
      expect(screen.getByText('1,240 artists')).toBeInTheDocument()
    })

    // The search row's placeholder copy is asserted in e2e/pages/graph.spec.ts:
    // ArtistSearch is mocked out here, so a placeholder assertion in this file
    // would only be checking the mock.

    it('announces the map is being built while the snapshot loads', () => {
      overviewState.isPending = true
      overviewState.isError = false
      renderWithProviders(<GraphObservatory />)

      expect(screen.getByText('Mapping the scene…')).toBeInTheDocument()
      expect(
        screen.queryByRole('heading', { name: 'Explore the graph.' }),
      ).not.toBeInTheDocument()
    })

    it('offers a retry when the map fails, without disturbing search', async () => {
      const user = userEvent.setup()
      overviewState.isError = true
      overviewState.error = Object.assign(new Error('boom'), { status: 500 })
      renderWithProviders(<GraphObservatory />)

      expect(screen.getByRole('alert')).toHaveTextContent('The map couldn’t load.')
      await user.click(screen.getByRole('button', { name: 'Try again' }))
      expect(overviewRefetch).toHaveBeenCalled()

      // Search is untouched by the map's failure — it still re-roots the page.
      await user.click(screen.getByRole('button', { name: 'Search Diners' }))
      expect(screen.getByLabelText('Graph centered on Diners')).toBeInTheDocument()
    })

    it('keeps the search-first hero for a catalog with no snapshot yet', () => {
      // Default state: the endpoint answers 503 until the first nightly build.
      renderWithProviders(<GraphObservatory />)

      expect(screen.getByRole('heading', { name: 'Explore the graph.' })).toBeInTheDocument()
      expect(screen.getByRole('button', { name: 'Take a random rabbit hole' })).toBeInTheDocument()
      expect(screen.queryByText(/The whole map/)).not.toBeInTheDocument()
    })

    it('re-roots from the map exactly as search does', async () => {
      const user = userEvent.setup()
      showMap()
      renderWithProviders(<GraphObservatory />)

      await user.click(screen.getByText('Browse the map as a list'))
      await user.click(screen.getByText('Around Diners'))
      await user.click(screen.getByRole('button', { name: 'Diners' }))

      // The ego canvas + trail are up, and the map's status line has given way
      // to the centered-on line: the same surface a search lands on.
      expect(screen.getByLabelText('Graph centered on Diners')).toBeInTheDocument()
      expect(
        screen.getByRole('navigation', { name: 'Graph traversal history' }),
      ).toBeInTheDocument()
      expect(screen.queryByText(/The whole map/)).not.toBeInTheDocument()
    })
  })

  describe('the "Tonight’s shows" escape hatch', () => {
    const phoenix = {
      city: 'Phoenix',
      state: 'AZ',
      slug: 'phoenix-az',
      venue_count: 10,
      upcoming_show_count: 12,
      total_show_count: 400,
      shows_this_week: 4,
      shows_calendar_week: 6,
      latitude: 33.45,
      longitude: -112.07,
    }
    const inPhoenix = { city: 'Phoenix', state: 'AZ' }

    // The EXACT accessible name, shared by every case below: the label is
    // fixed on purpose and only the href moves. This link sits in a wrap row
    // ahead of the shuffle pill, so a label that grew a city name when geo
    // landed would drag its siblings sideways after the reader had aimed.
    const tonightLink = () =>
      screen.getByRole('link', { name: 'Tonight’s shows' })

    // The scenes list and the geo suggestion both arrive after mount, so the
    // global listing is what the link is until they do — and what it stays for
    // a visitor we cannot place.
    it('points at the global listing when no scene can be resolved', () => {
      renderWithProviders(<GraphObservatory />)

      expect(tonightLink()).toHaveAttribute('href', '/shows')
    })

    it('points at the visitor’s own scene for the night once geo resolves', () => {
      scenesState.scenes = [phoenix]
      geoState.geo = inPhoenix
      renderWithProviders(<GraphObservatory />)

      expect(tonightLink()).toHaveAttribute('href', '/scenes/phoenix-az/tonight')
    })

    // Placing the visitor is not enough. A scene that has been dark all week
    // has a nightly page that is correct and empty, which is a worse answer
    // than the listing the reader already had.
    it('keeps the listing when the visitor’s scene has been quiet all week', () => {
      scenesState.scenes = [{ ...phoenix, shows_this_week: 0 }]
      geoState.geo = inPhoenix
      renderWithProviders(<GraphObservatory />)

      expect(tonightLink()).toHaveAttribute('href', '/shows')
    })

    // CBSA membership, not a radius: Tempe is in the Phoenix metro, so the
    // member-slug resolve (tempe-az → Phoenix principal) is the scene we name.
    it('points at the metro tonight page for a CBSA suburb', () => {
      scenesState.scenes = [phoenix]
      geoState.geo = { city: 'Tempe', state: 'AZ' }
      cbsaState.bySlug['tempe-az'] = {
        city: 'Phoenix',
        state: 'AZ',
        slug: 'phoenix-az',
      }
      renderWithProviders(<GraphObservatory />)

      expect(tonightLink()).toHaveAttribute('href', '/scenes/phoenix-az/tonight')
    })

    it('keeps the listing for a city outside every scene CBSA', () => {
      scenesState.scenes = [phoenix]
      geoState.geo = { city: 'Honolulu', state: 'HI' }
      renderWithProviders(<GraphObservatory />)

      expect(tonightLink()).toHaveAttribute('href', '/shows')
    })

    it('keeps the listing when a CBSA suburb’s metro has been quiet all week', () => {
      scenesState.scenes = [{ ...phoenix, shows_this_week: 0 }]
      geoState.geo = { city: 'Tempe', state: 'AZ' }
      cbsaState.bySlug['tempe-az'] = {
        city: 'Phoenix',
        state: 'AZ',
        slug: 'phoenix-az',
      }
      renderWithProviders(<GraphObservatory />)

      expect(tonightLink()).toHaveAttribute('href', '/shows')
    })
  })
})
