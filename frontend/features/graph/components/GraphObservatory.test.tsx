import { beforeEach, describe, expect, it, vi } from 'vitest'
import { screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'

import { renderWithProviders } from '@/test/utils'
import type { ArtistGraph } from '@/features/artists/types'

const { graphs, reviewState, shuffleRefetch, shuffleTarget } = vi.hoisted(() => {
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
    graphs: new Map<number, ArtistGraph>([
      [1, dinersGraph],
      [2, playboyGraph],
    ]),
    reviewState: { throwGraph: false },
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
  ArtistSearch: ({ onSelect }: { onSelect: (artist: Record<string, unknown>) => void }) => (
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
    isError: false,
    refetch: vi.fn(),
  }),
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

vi.mock('@/features/explore/hooks', () => ({
  useShuffleTarget: () => ({
    refetch: shuffleRefetch,
    isFetching: false,
  }),
}))

import { GraphObservatory } from './GraphObservatory'

describe('GraphObservatory', () => {
  beforeEach(() => {
    reviewState.throwGraph = false
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
    expect(screen.getByLabelText('Graph centered on Diners')).toBeInTheDocument()
    expect(screen.queryByRole('navigation', { name: 'Graph traversal history' })).not.toBeInTheDocument()
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
  })

  it('does not reuse a stale shuffle target after a failed refresh', async () => {
    const user = userEvent.setup()
    renderWithProviders(<GraphObservatory />)

    await user.click(screen.getByRole('button', { name: /A random rabbit hole/i }))
    shuffleRefetch.mockResolvedValueOnce({ data: shuffleTarget, isError: true })
    await user.click(screen.getByRole('button', { name: /A random rabbit hole/i }))

    expect(screen.getByRole('status')).toHaveTextContent('No rabbit hole is available')
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
