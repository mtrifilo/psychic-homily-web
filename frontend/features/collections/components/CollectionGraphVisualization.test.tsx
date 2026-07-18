import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/react'
import type { GraphNode } from '@/components/graph/ForceGraphView'
import type {
  CollectionGraphLink,
  CollectionGraphNode,
} from '../types'

interface CapturedProps {
  nodes: Array<{ id: number; name: string; slug: string }>
  ariaLabel: string
  onNodeClick: (node: GraphNode) => void
  onBackgroundClick?: () => void
  focusNodeId?: number | null
  showAccessibleNodeControls?: boolean
}
let lastProps: CapturedProps | null = null

vi.mock('@/components/graph/ForceGraphView', () => ({
  ForceGraphView: (props: CapturedProps) => {
    lastProps = props
    return (
      <div data-testid="force-graph-view" aria-label={props.ariaLabel} role="img">
        {props.nodes.map(n => (
          <button
            key={n.id}
            type="button"
            onClick={() => props.onNodeClick(n as GraphNode)}
          >
            {`node-${n.slug}`}
          </button>
        ))}
        <button type="button" onClick={() => props.onBackgroundClick?.()}>
          canvas-background
        </button>
      </div>
    )
  },
}))

const useArtistGraphCard = vi.fn()
vi.mock('@/features/artists/hooks/useArtistGraphCard', () => ({
  useArtistGraphCard: (opts: {
    artistId: number | string | null
    enabled?: boolean
  }) => useArtistGraphCard(opts),
}))

const useCollectionEntityPanelModel = vi.fn()
vi.mock('../hooks/useCollectionEntityPanelModel', () => ({
  useCollectionEntityPanelModel: (opts: unknown) =>
    useCollectionEntityPanelModel(opts),
}))

vi.mock('next/link', () => ({
  default: ({
    href,
    children,
    ...props
  }: {
    href: string
    children: React.ReactNode
    [key: string]: unknown
  }) => (
    <a href={href} {...props}>
      {children}
    </a>
  ),
}))

vi.mock('@/components/shared/MusicEmbed', () => ({
  MusicEmbed: () => <div data-testid="music-embed" />,
}))

import { CollectionGraphVisualization } from './CollectionGraphVisualization'
import { graphEntitySelectGestureHint } from '@/components/graph/EntityContextPanel'

const sourceNodes: CollectionGraphNode[] = [
  {
    id: 101,
    entity_type: 'artist',
    name: 'Gatecreeper',
    slug: 'gatecreeper',
    upcoming_show_count: 0,
    is_isolate: false,
  },
  {
    id: 102,
    entity_type: 'venue',
    name: 'Valley Bar',
    slug: 'valley-bar',
    city: 'Phoenix',
    state: 'AZ',
    upcoming_show_count: 0,
    is_isolate: false,
  },
]

const renderNodes: GraphNode[] = sourceNodes.map(n => ({
  id: n.id,
  name: n.name,
  slug: n.slug,
  city: n.city,
  state: n.state,
  upcoming_show_count: n.upcoming_show_count,
  is_isolate: n.is_isolate,
  cluster_id: n.entity_type,
}))

const links: CollectionGraphLink[] = []

describe('CollectionGraphVisualization', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    lastProps = null
    useArtistGraphCard.mockReturnValue({ data: undefined, isError: false })
    useCollectionEntityPanelModel.mockReturnValue(null)
  })

  it('appends the mixed-type select-gesture hint to the aria-label', () => {
    render(
      <CollectionGraphVisualization
        nodes={renderNodes}
        sourceNodes={sourceNodes}
        links={links}
        clusters={[]}
        containerWidth={1024}
        collectionTitle="Desert Doom"
        countPhrase="2 items"
        edgeCount={0}
      />,
    )
    expect(lastProps?.ariaLabel).toContain(graphEntitySelectGestureHint)
  })

  it('carries the truncation cue from countPhrase into the aria-label (PSY-1476)', () => {
    render(
      <CollectionGraphVisualization
        nodes={renderNodes}
        sourceNodes={sourceNodes}
        links={links}
        clusters={[]}
        containerWidth={1024}
        collectionTitle="Desert Doom"
        countPhrase="top 150 of 312 items"
        edgeCount={0}
      />,
    )
    expect(lastProps?.ariaLabel).toContain('top 150 of 312 items')
  })

  it('selects an artist into ArtistContextPanel and fetches by slug', () => {
    useArtistGraphCard.mockReturnValue({
      data: {
        id: 7,
        name: 'Gatecreeper',
        slug: 'gatecreeper',
        city: 'Phoenix',
        state: 'AZ',
        bandcamp_embed_url: null,
        spotify: null,
        next_show: null,
        labels: [],
        radio: null,
        connections: {
          bills: 0,
          similar: 0,
          members: 0,
          radio: 0,
          shared_labels: 0,
        },
      },
      isError: false,
    })

    render(
      <CollectionGraphVisualization
        nodes={renderNodes}
        sourceNodes={sourceNodes}
        links={links}
        clusters={[]}
        containerWidth={1024}
        collectionTitle="Desert Doom"
        countPhrase="2 items"
        edgeCount={0}
      />,
    )

    fireEvent.click(screen.getByRole('button', { name: 'node-gatecreeper' }))
    expect(useArtistGraphCard).toHaveBeenLastCalledWith({
      artistId: 'gatecreeper',
      enabled: true,
    })
    expect(
      screen.getByRole('link', { name: '[ Open page → ]' }),
    ).toHaveAttribute('href', '/artists/gatecreeper')
    expect(screen.getByText('Selected')).toBeInTheDocument()
  })

  it('selects a venue into EntityContextPanel (type-tag dispatch)', () => {
    useCollectionEntityPanelModel.mockReturnValue({
      entityType: 'venue',
      name: 'Valley Bar',
      slug: 'valley-bar',
      meta: 'Phoenix, AZ',
      primary: {
        kind: 'labeled',
        label: 'Next show',
        text: 'Fri Jul 24 · Diners',
      },
      facts: ['2 artists in this graph have played here'],
      isLoading: false,
      isError: false,
    })

    render(
      <CollectionGraphVisualization
        nodes={renderNodes}
        sourceNodes={sourceNodes}
        links={links}
        clusters={[]}
        containerWidth={1024}
        collectionTitle="Desert Doom"
        countPhrase="2 items"
        edgeCount={0}
      />,
    )

    fireEvent.click(screen.getByRole('button', { name: 'node-valley-bar' }))
    expect(screen.getByText('VENUE')).toBeInTheDocument()
    expect(screen.getByText('Next show')).toBeInTheDocument()
    expect(
      screen.getByRole('link', { name: '[ Open page → ]' }),
    ).toHaveAttribute('href', '/venues/valley-bar')
    expect(screen.queryByText('Selected')).not.toBeInTheDocument()
    expect(useArtistGraphCard).toHaveBeenLastCalledWith({
      artistId: null,
      enabled: false,
    })
  })

  it('deselects on a second click of the same node', () => {
    useCollectionEntityPanelModel.mockReturnValue({
      entityType: 'venue',
      name: 'Valley Bar',
      slug: 'valley-bar',
      meta: null,
      primary: null,
      facts: [],
      isLoading: false,
      isError: false,
    })

    render(
      <CollectionGraphVisualization
        nodes={renderNodes}
        sourceNodes={sourceNodes}
        links={links}
        clusters={[]}
        containerWidth={1024}
        collectionTitle="Desert Doom"
        countPhrase="2 items"
        edgeCount={0}
      />,
    )

    fireEvent.click(screen.getByRole('button', { name: 'node-valley-bar' }))
    expect(screen.getByText('VENUE')).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: 'node-valley-bar' }))
    expect(screen.queryByText('VENUE')).not.toBeInTheDocument()
  })
})
