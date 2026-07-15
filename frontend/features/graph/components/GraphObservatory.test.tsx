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
    reviewState: { graphError: false, throwGraph: false },
    shuffleRefetch: vi.fn(),
    shuffleTarget: {
      artist_id: 2,
      artist_slug: 'playboy-manbaby',
      artist_name: 'Playboy Manbaby',
    },
  }
})

vi.mock('@sentry/nextjs', () => ({
  captureException: vi.fn(),
}))

vi.mock('@/components/graph/useContainerWidth', () => ({
  GRAPH_BREAKPOINT_PX: 640,
  useContainerWidth: () => ({ refCallback: vi.fn(), containerWidth: 1024 }),
}))

vi.mock('@/features/artists/hooks/useReducedMotion', () => ({
  useReducedMotion: () => true,
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
  }: {
    data: ArtistGraph
    onSelect: (node: ArtistGraph['center']) => void
  }) => {
    if (reviewState.throwGraph) throw new Error('graph chunk failed')
    return (
      <div aria-label={`Graph centered on ${data.center.name}`}>
        <button type="button" onClick={() => onSelect(data.nodes[0])}>
          Select {data.nodes[0].name}
        </button>
      </div>
    )
  },
}))

vi.mock('@/features/artists/hooks/useArtistGraph', () => ({
  useArtistGraph: ({ artistId }: { artistId: number }) => ({
    data: graphs.get(artistId),
    isPending: false,
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

import { GraphObservatory } from './GraphObservatory'

describe('GraphObservatory', () => {
  beforeEach(() => {
    reviewState.graphError = false
    reviewState.throwGraph = false
    fetchGraph.mockReset()
    fetchGraph.mockImplementation(async (artistId: number) => graphs.get(artistId))
    shuffleRefetch.mockReset()
    shuffleRefetch.mockResolvedValue({ data: shuffleTarget, isError: false })
  })

  it('starts from search, opens context, hops with a trail, and resets', async () => {
    const user = userEvent.setup()
    renderWithProviders(<GraphObservatory />)

    expect(screen.getByRole('heading', { name: 'Pick a name. See what it touches.' })).toBeInTheDocument()
    expect(screen.getByText(/Try searching for/)).toHaveTextContent('Diners')

    await user.click(screen.getByRole('button', { name: 'Search Diners' }))
    expect(screen.getByLabelText('Graph centered on Diners')).toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: 'Select Playboy Manbaby' }))
    expect(screen.getByRole('region', { name: 'About Playboy Manbaby' })).toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: /Center here/i }))
    expect(screen.getByLabelText('Graph centered on Playboy Manbaby')).toBeInTheDocument()
    expect(screen.getByRole('navigation', { name: 'Graph traversal history' })).toHaveTextContent(
      'DinersPlayboy Manbaby',
    )

    await user.click(screen.getByRole('button', { name: 'Reset' }))
    expect(screen.getByRole('heading', { name: 'Pick a name. See what it touches.' })).toBeInTheDocument()
    expect(screen.queryByLabelText('Graph centered on Diners')).not.toBeInTheDocument()
    expect(screen.queryByRole('navigation', { name: 'Graph traversal history' })).not.toBeInTheDocument()
    await waitFor(() => expect(screen.getByRole('textbox', { name: 'Mock artist search' })).toHaveFocus())
  })

  it('uses the existing shuffle target as the random rabbit-hole root', async () => {
    const user = userEvent.setup()
    renderWithProviders(<GraphObservatory />)

    await user.click(screen.getByRole('button', { name: /A random rabbit hole/i }))
    expect(shuffleRefetch).toHaveBeenCalled()
    expect(screen.getByLabelText('Graph centered on Playboy Manbaby')).toBeInTheDocument()
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

  it('shows an honest zero state for an artist without mapped connections', async () => {
    const user = userEvent.setup()
    const dinersGraph = graphs.get(1)!
    graphs.set(1, { center: dinersGraph.center, nodes: [], links: [] })

    try {
      renderWithProviders(<GraphObservatory />)
      await user.click(screen.getByRole('button', { name: 'Search Diners' }))

      expect(screen.getByRole('status')).toHaveTextContent('No mapped connections yet')
      expect(screen.queryByRole('list', { name: /Artists connected/ })).not.toBeInTheDocument()
      expect(screen.getByRole('link', { name: /Open Diners/i })).toHaveAttribute(
        'href',
        '/artists/diners',
      )
    } finally {
      graphs.set(1, dinersGraph)
    }
  })

  it('does not reuse a stale shuffle target after a failed refresh', async () => {
    const user = userEvent.setup()
    renderWithProviders(<GraphObservatory />)

    await user.click(screen.getByRole('button', { name: /A random rabbit hole/i }))
    shuffleRefetch.mockResolvedValueOnce({ data: shuffleTarget, isError: true })
    await user.click(screen.getByRole('button', { name: /A random rabbit hole/i }))

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

    await user.click(screen.getByRole('button', { name: /A random rabbit hole/i }))

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
    await user.click(screen.getByRole('button', { name: /A random rabbit hole/i }))
    await user.click(screen.getByRole('button', { name: 'Reset' }))
    await act(async () => {
      resolveShuffle?.({ data: shuffleTarget, isError: false })
    })

    expect(screen.getByRole('heading', { name: 'Pick a name. See what it touches.' })).toBeInTheDocument()
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
})
