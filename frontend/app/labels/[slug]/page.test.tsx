import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import type { ReactElement } from 'react'
import { renderToString } from 'react-dom/server'
import { render, waitFor } from '@testing-library/react'
import {
  HydrationBoundary,
  QueryClient,
  QueryClientProvider,
  hashKey,
  type DehydratedState,
} from '@tanstack/react-query'
import { errorResponse, jsonBodyResponse } from '@/lib/seo/test-helpers'
import { findElement, findElements } from '@/test/elementTree'

// `notFound()` in Next.js throws to halt rendering; mirrored so a test can tell
// that execution stopped.
const NOT_FOUND_SENTINEL = 'NEXT_NOT_FOUND'
vi.mock('next/navigation', () => ({
  notFound: vi.fn(() => {
    throw new Error(NOT_FOUND_SENTINEL)
  }),
  useRouter: () => ({ push: vi.fn(), replace: vi.fn(), back: vi.fn() }),
  usePathname: () => '/labels/4ad',
  useSearchParams: () => new URLSearchParams(),
}))

// `seedFirstScreen` awaits `connection()`, which marks a request-time render and
// has nothing to do outside Next.
vi.mock('next/server', () => ({
  connection: async () => {},
}))

// On the server `getQueryClient()` mints a fresh client per call. Under jsdom
// it would hand back the browser singleton instead.
vi.mock('@/lib/queryClient', async importOriginal => {
  const actual = await importOriginal<typeof import('@/lib/queryClient')>()
  const { QueryClient } = await import('@tanstack/react-query')
  return { ...actual, getQueryClient: () => new QueryClient() }
})

// The shared server-fetch reporter, observed rather than replaced: the seeds
// must reach it through fetchListPayload and have no reporting path of their own.
const mockReportServerFetchFailure = vi.fn()
vi.mock('@/lib/ssr/reportServerFetchFailure', async importOriginal => {
  const actual = await importOriginal<typeof import('@/lib/ssr/reportServerFetchFailure')>()
  return {
    ...actual,
    reportServerFetchFailure: (...args: Parameters<typeof actual.reportServerFetchFailure>) =>
      mockReportServerFetchFailure(...args),
  }
})

// The page renders the REAL `LabelDetail` and its real roster and catalog
// hooks, so a seed under the wrong key shows up as a loader in the HTML. What
// is stubbed is everything around those two sections that owns its own reads
// or needs a signed-in context: none of it renders roster or catalog rows.
vi.mock('@/features/auth', () => ({
  useIsAuthenticated: () => ({ user: null, isAuthenticated: false }),
}))
vi.mock('@/features/collections', () => ({ EntityCollections: (): null => null }))
vi.mock('@/features/comments', () => ({ CommentThread: (): null => null }))
vi.mock('@/features/tags', () => ({
  EntityTagList: (): null => null,
  AddTagDialog: (): null => null,
}))
vi.mock('@/features/notifications', () => ({ NotifyMeButton: (): null => null }))
vi.mock('@/features/contributions', () => ({
  AttributionLine: (): null => null,
  EntityEditDrawer: (): null => null,
  EntitySaveSuccessBanner: (): null => null,
  ReportEntityDialog: (): null => null,
  useEntitySaveSuccessBanner: () => ({ isVisible: false, handleSaveSuccess: vi.fn() }),
}))
vi.mock('@/components/shared', async importOriginal => ({
  ...(await importOriginal<typeof import('@/components/shared')>()),
  FollowButton: (): null => null,
  AddToCollectionButton: (): null => null,
  RevisionHistory: (): null => null,
}))

import LabelPage from './page'
import { LabelDetail } from '@/features/labels/components'
import { labelEndpoints } from '@/features/labels/api'
import { queryKeys } from '@/lib/queryClient'

const SLUG = '4ad'

function buildLabel(overrides: Record<string, unknown> = {}) {
  return {
    id: 12,
    name: '4AD',
    slug: SLUG,
    city: 'London',
    state: null,
    country: 'UK',
    founded_year: 1980,
    status: 'active',
    description: null,
    image_url: null,
    social: {},
    artist_count: 2,
    release_count: 3,
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z',
    ...overrides,
  }
}

const ROSTER = {
  artists: [
    { id: 119, slug: 'alvvays', name: 'Alvvays' },
    { id: 19, slug: 'pixies', name: 'Pixies' },
  ],
  count: 2,
}

const CATALOG = {
  releases: [
    {
      id: 11,
      title: 'Blue Rev',
      slug: 'blue-rev',
      release_type: 'lp',
      release_year: 2022,
      cover_art_url: null,
      catalog_number: 'CAD 3X01',
    },
    {
      id: 9,
      title: 'Doolittle',
      slug: 'doolittle',
      release_type: 'lp',
      release_year: 1989,
      cover_art_url: null,
      catalog_number: null,
    },
    {
      id: 30,
      title: 'Gigantic',
      slug: 'gigantic',
      release_type: 'single',
      release_year: 1988,
      cover_art_url: null,
      catalog_number: null,
    },
  ],
  count: 3,
}

const labelFetch = vi.fn()
const rosterFetch = vi.fn()
const catalogFetch = vi.fn()

beforeEach(() => {
  vi.clearAllMocks()
  labelFetch.mockImplementation(async () => jsonBodyResponse(buildLabel()))
  rosterFetch.mockImplementation(async () => jsonBodyResponse(ROSTER))
  catalogFetch.mockImplementation(async () => jsonBodyResponse(CATALOG))
  vi.stubGlobal('fetch', (url: string, init?: RequestInit) => {
    const path = new URL(String(url), 'http://test.local').pathname
    if (path.endsWith('/artists')) return rosterFetch(url, init)
    if (path.endsWith('/releases')) return catalogFetch(url, init)
    if (/\/labels\/[^/]+$/.test(path)) return labelFetch(url, init)
    throw new Error(`unexpected fetch in this suite: ${String(url)}`)
  })
})

afterEach(() => {
  vi.unstubAllGlobals()
  vi.unstubAllEnvs()
})

async function renderPage(slug = SLUG): Promise<ReactElement> {
  return (await LabelPage({ params: Promise.resolve({ slug }) })) as ReactElement
}

function boundaryStatesOf(tree: ReactElement): DehydratedState[] {
  return findElements(tree, node => node.type === HydrationBoundary)
    .map(node => (node.props as { state?: DehydratedState }).state)
    .filter((state): state is DehydratedState => state !== undefined)
}

function seededQueryHashes(tree: ReactElement): string[] {
  return boundaryStatesOf(tree).flatMap(state => state.queries.map(q => q.queryHash))
}

const testClient = () =>
  new QueryClient({ defaultOptions: { queries: { retry: false } } })

/** The page's own `LabelDetail` element under its boundaries, nested as the page nests them. */
function detailUnder(tree: ReactElement, client: QueryClient): ReactElement {
  const detail = findElement(tree, node => node.type === LabelDetail)
  return (
    <QueryClientProvider client={client}>
      {boundaryStatesOf(tree).reduceRight<ReactElement>(
        (children, state) => <HydrationBoundary state={state}>{children}</HydrationBoundary>,
        detail,
      )}
    </QueryClientProvider>
  )
}

/**
 * Server-render the detail the way the route's HTML is produced: no client
 * fetch runs inside `renderToString`, so every row it prints came from a seed.
 */
function serverRender(tree: ReactElement, client = testClient()): string {
  return renderToString(detailUnder(tree, client))
}

const LOADER = 'animate-spin'
const ROSTER_KEY_HASH = hashKey(queryKeys.labels.roster(SLUG))
const CATALOG_KEY_HASH = hashKey(queryKeys.labels.catalog(SLUG))
const DETAIL_KEY_HASH = hashKey(queryKeys.labels.detail(SLUG))

describe('labels/[slug] page body', () => {
  it('server-renders the roster and catalog links with no loader', async () => {
    const html = serverRender(await renderPage())

    expect(html).toContain('href="/artists/alvvays"')
    expect(html).toContain('href="/artists/pixies"')
    expect(html).toContain('href="/releases/blue-rev"')
    expect(html).toContain('href="/releases/doolittle"')
    expect(html).toContain('href="/releases/gigantic"')
    expect(html).not.toContain(LOADER)
  })

  it('keeps the payload order and the client-side release-type grouping', async () => {
    const html = serverRender(await renderPage())

    expect(html.indexOf('/artists/alvvays')).toBeLessThan(html.indexOf('/artists/pixies'))
    expect(html.indexOf('/releases/blue-rev')).toBeLessThan(html.indexOf('/releases/doolittle'))
    // The single is grouped after the albums even though it is not last by year.
    expect(html.indexOf('Albums')).toBeLessThan(html.indexOf('/releases/blue-rev'))
    expect(html.indexOf('/releases/doolittle')).toBeLessThan(html.indexOf('Singles'))
    expect(html.indexOf('Singles')).toBeLessThan(html.indexOf('/releases/gigantic'))
  })

  it('fills every key the real hooks read, the lists under their own stale-stamped state', async () => {
    const tree = await renderPage()
    const client = testClient()

    serverRender(tree, client)

    const labelQueries = client.getQueryCache().findAll({ queryKey: ['labels'] })
    expect(labelQueries.map(q => q.queryHash).sort()).toEqual(
      [DETAIL_KEY_HASH, ROSTER_KEY_HASH, CATALOG_KEY_HASH].sort(),
    )
    for (const query of labelQueries) {
      expect(query.state.data).toBeDefined()
    }
    // The detail keeps its own boundary; the lists are stamped stale, so the
    // browser re-asks for them on mount.
    const [detailState, listsState] = boundaryStatesOf(tree)
    expect(detailState.queries.map(q => q.queryHash)).toEqual([DETAIL_KEY_HASH])
    expect(listsState.queries.map(q => q.queryHash)).toEqual([
      '["labels","roster","4ad"]',
      '["labels","catalog","4ad"]',
    ])
    for (const query of listsState.queries) {
      expect(query.state.dataUpdatedAt).toBe(0)
    }
  })

  it('keeps the server rows on screen while the browser re-asks for both lists on mount', async () => {
    const tree = await renderPage()
    rosterFetch.mockClear()
    catalogFetch.mockClear()

    render(detailUnder(tree, testClient()))

    expect(document.body.innerHTML).toContain('href="/artists/alvvays"')
    expect(document.body.innerHTML).toContain('href="/releases/blue-rev"')
    expect(document.body.innerHTML).not.toContain(LOADER)
    await waitFor(() => {
      expect(rosterFetch).toHaveBeenCalledTimes(1)
      expect(catalogFetch).toHaveBeenCalledTimes(1)
    })
  })

  it('asks for the URLs the real hooks ask for', async () => {
    await renderPage()
    const serverUrls = [rosterFetch.mock.calls[0][0], catalogFetch.mock.calls[0][0]].map(String)
    rosterFetch.mockClear()
    catalogFetch.mockClear()

    // The hooks mounted with only the label seeded, so they fetch for themselves.
    const client = testClient()
    client.setQueryData(queryKeys.labels.detail(SLUG), buildLabel())
    render(
      <QueryClientProvider client={client}>
        <LabelDetail idOrSlug={SLUG} />
      </QueryClientProvider>,
    )
    await waitFor(() => {
      expect(rosterFetch).toHaveBeenCalledTimes(1)
      expect(catalogFetch).toHaveBeenCalledTimes(1)
    })

    expect(serverUrls).toEqual([
      String(rosterFetch.mock.calls[0][0]),
      String(catalogFetch.mock.calls[0][0]),
    ])
  })

  it('encodes the slug in the server-side read', async () => {
    labelFetch.mockImplementation(async () => jsonBodyResponse(buildLabel({ slug: 'a/b' })))

    await renderPage('a/b')

    expect(String(rosterFetch.mock.calls[0][0])).toBe(labelEndpoints.ARTISTS('a%2Fb'))
    expect(String(catalogFetch.mock.calls[0][0])).toBe(labelEndpoints.RELEASES('a%2Fb'))
  })

  it('unpads a padded numeric id in the list reads and keys the seeds on the route slug', async () => {
    const tree = await renderPage('0012')

    expect(String(rosterFetch.mock.calls[0][0])).toBe(labelEndpoints.ARTISTS('12'))
    expect(String(catalogFetch.mock.calls[0][0])).toBe(labelEndpoints.RELEASES('12'))
    expect(seededQueryHashes(tree)).toEqual(
      expect.arrayContaining([
        hashKey(queryKeys.labels.roster('0012')),
        hashKey(queryKeys.labels.catalog('0012')),
      ]),
    )
  })

  it('sends both seed reads with the server-read header', async () => {
    vi.stubEnv('SERVER_READ_SECRET', 'a'.repeat(40))

    await renderPage()

    for (const call of [rosterFetch.mock.calls[0], catalogFetch.mock.calls[0]]) {
      const headers = new Headers((call[1] as RequestInit | undefined)?.headers)
      expect(headers.get('X-Server-Read-Secret')).toBe('a'.repeat(40))
    }
  })

  it('renders an empty roster and catalog server-side as no section and no loader', async () => {
    rosterFetch.mockImplementation(async () => jsonBodyResponse({ artists: [], count: 0 }))
    catalogFetch.mockImplementation(async () => jsonBodyResponse({ releases: [], count: 0 }))

    const tree = await renderPage()
    const html = serverRender(tree)

    expect(seededQueryHashes(tree)).toEqual(
      expect.arrayContaining([ROSTER_KEY_HASH, CATALOG_KEY_HASH]),
    )
    expect(html).not.toContain(LOADER)
    expect(html).not.toContain('href="/artists/')
    expect(html).not.toContain('href="/releases/')
    expect(html).not.toContain('aria-label="Catalog"')
  })

  it.each([
    ['a rate-limited roster read', 'roster', 429],
    ['a failing catalog read', 'catalog', 503],
    ['a not-found roster read', 'roster', 404],
  ] as const)(
    'reports %s through the shared reporter, skips that seed and keeps the other',
    async (_name, which, status) => {
      const failing = which === 'roster' ? rosterFetch : catalogFetch
      failing.mockImplementation(async () => errorResponse(status))

      const tree = await renderPage()
      const html = serverRender(tree)

      expect(mockReportServerFetchFailure).toHaveBeenCalledTimes(1)
      const report = mockReportServerFetchFailure.mock.calls[0][0]
      expect(report).toMatchObject({
        service: which === 'roster' ? 'label-page-roster' : 'label-page-catalog',
        pathFamily:
          which === 'roster' ? '/labels/{slug}/artists' : '/labels/{slug}/releases',
        notFoundIsAnswer: true,
      })
      expect(report.response.status).toBe(status)
      expect(seededQueryHashes(tree).sort()).toEqual(
        [
          DETAIL_KEY_HASH,
          which === 'roster' ? CATALOG_KEY_HASH : ROSTER_KEY_HASH,
        ].sort(),
      )
      // The failed section keeps its loader; the other one is in the HTML.
      expect(html).toContain(LOADER)
      if (which === 'roster') {
        expect(html).not.toContain('href="/artists/')
        expect(html).toContain('href="/releases/blue-rev"')
      } else {
        expect(html).toContain('href="/artists/alvvays"')
        expect(html).not.toContain('href="/releases/')
      }
    },
  )

  it('still renders the label with both loaders when both seed reads throw', async () => {
    rosterFetch.mockImplementation(async () => {
      throw new TypeError('fetch failed')
    })
    catalogFetch.mockImplementation(async () => {
      throw new TypeError('fetch failed')
    })

    const tree = await renderPage()
    const html = serverRender(tree)

    expect(seededQueryHashes(tree)).toEqual([DETAIL_KEY_HASH])
    expect(html).toContain('4AD')
    expect(html.match(/animate-spin/g)).toHaveLength(2)
  })

  it('starts both list reads without waiting for the label read', async () => {
    let releaseLabel: () => void = () => {}
    labelFetch.mockImplementation(
      () =>
        new Promise<Response>(resolve => {
          releaseLabel = () => resolve(jsonBodyResponse(buildLabel()))
        }),
    )

    const page = renderPage()
    await waitFor(() => {
      expect(rosterFetch).toHaveBeenCalledTimes(1)
      expect(catalogFetch).toHaveBeenCalledTimes(1)
    })
    releaseLabel()

    expect(seededQueryHashes(await page)).toEqual(
      expect.arrayContaining([DETAIL_KEY_HASH, ROSTER_KEY_HASH, CATALOG_KEY_HASH]),
    )
  })

  it('renders not-found for a label that does not exist, whatever the lists answered', async () => {
    labelFetch.mockImplementation(async () => errorResponse(404))

    await expect(renderPage('no-such-label')).rejects.toThrow(NOT_FOUND_SENTINEL)
  })
})
