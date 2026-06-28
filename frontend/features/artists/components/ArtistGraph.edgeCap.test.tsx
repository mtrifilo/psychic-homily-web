import { describe, it, expect, afterEach } from 'vitest'
import { screen } from '@testing-library/react'
import { vi } from 'vitest'
import { renderWithProviders } from '@/test/utils'
import type { ArtistGraph, ArtistGraphLink, ArtistGraphNode } from '../types'

// PSY-1258: the per-node top-k edge cap must be DISCLOSED in the legend, not silent
// (CLAUDE.md "no silent caps"). The top-k selection itself is unit-tested in
// components/graph/edgeCap.test.ts; this verifies the component wires the cap's
// shown/total tallies into the EdgeLegend footnote — and omits it when nothing is capped.

// ForceGraph2D loads via next/dynamic (ssr:false). A synchronous, non-ref-forwarding
// stub is enough here: graphRef.current stays null so the force-config effect no-ops, and
// the EdgeLegend (the surface under test) renders independently of the canvas.
vi.mock('next/dynamic', () => ({
  default: () =>
    function ForceGraph2DStub() {
      return <div data-testid="force-graph" />
    },
}))

vi.mock('next/link', () => ({
  default: ({ href, children, className }: { href: string; children: React.ReactNode; className?: string }) => (
    <a href={href} className={className}>{children}</a>
  ),
}))

import { ArtistGraphVisualization } from './ArtistGraph'

const center: ArtistGraphNode = {
  id: 1,
  name: 'Cola',
  slug: 'cola',
  city: 'Phoenix',
  state: 'AZ',
  upcoming_show_count: 0,
}

const radioLink = (source: number, target: number, score: number): ArtistGraphLink => ({
  source_id: source,
  target_id: target,
  type: 'radio_cooccurrence',
  score,
  votes_up: 0,
  votes_down: 0,
})

// A center wired to many satellites that are themselves densely cross-connected by radio
// co-occurrence — so each satellite carries far more than k=5 radio edges and the cap must
// drop the weakest cross-ties (shown < total). Center↔satellite edges always survive
// (no-orphan invariant), so the dropped edges are satellite↔satellite ties.
function denseRadioGraph(): ArtistGraph {
  const nodes: ArtistGraphNode[] = []
  const links: ArtistGraphLink[] = []
  for (let i = 2; i <= 9; i++) {
    nodes.push({ id: i, name: `Sat ${i}`, slug: `sat-${i}`, upcoming_show_count: 0 })
    links.push(radioLink(1, i, 0.9)) // strong center tie → top-1 for the satellite
  }
  // Low, strictly-decreasing scores so each satellite has a clear weakest-few to trim.
  let score = 0.5
  for (let i = 2; i <= 9; i++) {
    for (let j = i + 1; j <= 9; j++) {
      score -= 0.001
      links.push(radioLink(i, j, score))
    }
  }
  return { center, nodes, links, user_votes: {} }
}

const sparseRadioGraph: ArtistGraph = {
  center,
  nodes: [{ id: 2, name: 'Sat 2', slug: 'sat-2', upcoming_show_count: 0 }],
  links: [radioLink(1, 2, 0.9)],
  user_votes: {},
}

const renderGraph = (data: ArtistGraph) =>
  renderWithProviders(
    <ArtistGraphVisualization
      data={data}
      activeTypes={new Set(['radio_cooccurrence'])}
      containerWidth={1024}
    />,
  )

describe('ArtistGraphVisualization — edge-cap disclosure (PSY-1258)', () => {
  afterEach(() => vi.clearAllTimers())

  it('discloses the per-node cap in the legend when dense radio edges are trimmed', () => {
    renderGraph(denseRadioGraph())
    // The footnote names the type, the k, and the honest shown-of-total tally. The plain
    // legend row is just "Radio Co-occurrence", so the longer phrase targets the footnote.
    expect(
      screen.getByText(/Radio Co-occurrence: each artist's 5 strongest \(\d+ of \d+\)/i),
    ).toBeInTheDocument()
  })

  it('omits the cap footnote when nothing is trimmed', () => {
    renderGraph(sparseRadioGraph)
    expect(screen.queryByText(/each artist's 5 strongest/i)).not.toBeInTheDocument()
  })
})
