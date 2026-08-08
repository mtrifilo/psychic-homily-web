import { beforeEach, describe, expect, it, vi } from 'vitest'
import { render, screen } from '@testing-library/react'

const fetchGraphOverview = vi.hoisted(() => vi.fn())

vi.mock('./graphOverviewApi', () => ({
  fetchGraphOverview,
  GRAPH_OVERVIEW_REVALIDATE: 3600,
}))

const QUANT = 32767
const EPOCH = '2020-01-01T00:00:00Z'

function appearAt(iso: string): number {
  return Math.floor((new Date(iso).getTime() - new Date(EPOCH).getTime()) / 1000)
}

function encodeBytes(bytes: number[]): string {
  return Buffer.from(bytes).toString('base64')
}

/** Two artists, one of which arrived inside the JUL 27 - AUG 2 window. */
function overviewFixture(overrides: Record<string, unknown> = {}) {
  return {
    version: 1,
    last_mapped: '2026-08-02T04:30:00Z',
    epoch: EPOCH,
    extent: 500,
    node_count: 2,
    edge_count: 1,
    isolate_count: 0,
    rank_metric: 'betweenness',
    hull_kind: 'convex',
    nodes: {
      id: [1, 2],
      kind: encodeBytes([0, 0]),
      name: ['Alpha', 'Beta'],
      slug: ['alpha', 'beta'],
      x: [-QUANT, QUANT],
      y: [0, 0],
      community: [0, 0],
      degree: [1, 1],
      rank: [0, 1],
      flags: encodeBytes([0, 0]),
      appear: [appearAt('2021-01-01T00:00:00Z'), appearAt('2026-07-30T00:00:00Z')],
    },
    edges: {
      offsets: [0, 1, 2],
      targets: [1, 0],
      kind: encodeBytes([0, 0]),
      appear: [appearAt('2026-07-30T00:00:00Z'), appearAt('2026-07-30T00:00:00Z')],
    },
    regions: [],
    ...overrides,
  }
}

/**
 * A fresh module per case. `getGraphWeek` is wrapped in React's `cache`, so a
 * second call inside one module instance would return the first case's answer.
 */
async function loadPage() {
  vi.resetModules()
  return import('./graphWeekPage')
}

beforeEach(() => {
  fetchGraphOverview.mockReset()
})

describe('buildGraphWeekMetadata', () => {
  it('describes the week and keys the share image on it', async () => {
    fetchGraphOverview.mockResolvedValue(overviewFixture())
    const { buildGraphWeekMetadata } = await loadPage()

    const metadata = await buildGraphWeekMetadata()

    expect(metadata.title).toBe('This week in the graph — JUL 27 - AUG 2 2026')
    expect(metadata.description).toBe(
      '1 new artist and 1 new connection joined the map, JUL 27 - AUG 2 2026.'
    )
    expect(metadata.alternates?.canonical).toBe('https://psychichomily.com/graph/this-week')

    // THE non-obvious requirement. A file-convention OG route's URL is a
    // constant while this card changes nightly, and unfurl caches key on the
    // URL — so the week has to ride in the query string or Discord pins
    // whichever week it saw first, forever.
    const image = metadata.openGraph?.images
    expect(image).toEqual([
      {
        url: 'https://psychichomily.com/graph/this-week/opengraph-image?w=2026-08-02',
        width: 1200,
        height: 630,
        type: 'image/png',
        alt: '1 new artist and 1 new connection joined the map, JUL 27 - AUG 2 2026.',
      },
    ])
  })

  it('is noindex, follow — a share URL, not an index target', async () => {
    fetchGraphOverview.mockResolvedValue(overviewFixture())
    const { buildGraphWeekMetadata } = await loadPage()

    const metadata = await buildGraphWeekMetadata()

    // The content changes every night, so an indexed snippet is stale by
    // definition; `follow` keeps the link equity flowing to /graph, which is the
    // surface that should rank.
    expect(metadata.robots).toEqual({ index: false, follow: true })
  })

  it('carries no twitter images, so Next inherits the full openGraph descriptor', async () => {
    fetchGraphOverview.mockResolvedValue(overviewFixture())
    const { buildGraphWeekMetadata } = await loadPage()

    const metadata = await buildGraphWeekMetadata()

    expect(metadata.twitter).toEqual({
      card: 'summary_large_image',
      title: 'This week in the graph — JUL 27 - AUG 2 2026',
      description: '1 new artist and 1 new connection joined the map, JUL 27 - AUG 2 2026.',
    })
    expect(metadata.twitter).not.toHaveProperty('images')
  })

  it('advertises nothing when there is no snapshot to describe', async () => {
    fetchGraphOverview.mockResolvedValue(null)
    const { buildGraphWeekMetadata } = await loadPage()

    const metadata = await buildGraphWeekMetadata()

    expect(metadata.robots).toEqual({ index: false, follow: false })
    // No OG image URL at all: the page 404s, and advertising a card for a page
    // that does not exist is how an unfurler caches a preview of a dead link.
    expect(metadata.openGraph).toBeUndefined()
  })

  it('advertises nothing when the snapshot cannot be dated', async () => {
    const undated = overviewFixture()
    undated.nodes.appear = [0, 0]
    undated.edges.appear = [0, 0]
    fetchGraphOverview.mockResolvedValue(undated)
    const { buildGraphWeekMetadata } = await loadPage()

    expect((await buildGraphWeekMetadata()).robots).toEqual({ index: false, follow: false })
  })
})

describe('getGraphWeek', () => {
  it('resolves the map and the week together', async () => {
    fetchGraphOverview.mockResolvedValue(overviewFixture())
    const { getGraphWeek } = await loadPage()

    const view = await getGraphWeek()

    expect(view?.week.newArtistCount).toBe(1)
    expect(view?.map.artistCount).toBe(2)
  })

  // NOT TESTED HERE: that `cache` deduplicates the metadata call and the body
  // call. React's `cache` memoises within a REQUEST SCOPE, and vitest has none,
  // so both calls miss and the assertion would be `2` — which says nothing about
  // production either way. Asserting it would mean pinning the miss as if it
  // were the contract.

  it('is null for an undecodable payload rather than throwing', async () => {
    fetchGraphOverview.mockResolvedValue(overviewFixture({ version: 99 }))
    const { getGraphWeek } = await loadPage()

    expect(await getGraphWeek()).toBeNull()
  })
})

describe('page bodies', () => {
  it('reports the week and offers the map', async () => {
    fetchGraphOverview.mockResolvedValue(overviewFixture())
    const { getGraphWeek, GraphWeekContent } = await loadPage()
    const view = (await getGraphWeek())!

    render(<GraphWeekContent view={view} />)

    expect(screen.getByRole('heading', { name: 'This week in the graph' })).toBeInTheDocument()
    expect(screen.getByText('+1 ARTIST · +1 CONNECTION')).toBeInTheDocument()
    expect(screen.getByText('JUL 27 - AUG 2 2026')).toBeInTheDocument()
    // The teaser is a picture of a snapshot; without a label it says nothing at
    // all to anyone not looking at it.
    expect(screen.getByRole('img')).toHaveAccessibleName(
      '1 new artist and 1 new connection joined the map, JUL 27 - AUG 2 2026.'
    )
    expect(screen.getByRole('link', { name: /Open the map of the scene/ })).toHaveAttribute(
      'href',
      '/graph'
    )
  })

  it('says when the numbers will exist rather than apologising', async () => {
    // The state before the first nightly build. It answers 200 by design — see
    // the page component for why a 404 is not available to this route — so the
    // body has to be a real answer rather than an error.
    const { GraphWeekUnbuilt } = await loadPage()

    render(<GraphWeekUnbuilt />)

    expect(screen.getByRole('heading', { name: 'This week in the graph' })).toBeInTheDocument()
    expect(screen.getByText(/built once a night/)).toBeInTheDocument()
    expect(screen.getByRole('link', { name: /Open the map of the scene/ })).toHaveAttribute(
      'href',
      '/graph'
    )
  })
})
