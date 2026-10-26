import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { ReactElement } from 'react'
import { renderToString } from 'react-dom/server'
import {
  HydrationBoundary,
  QueryClient,
  QueryClientProvider,
  type DehydratedState,
} from '@tanstack/react-query'

const { fetchListPayload } = vi.hoisted(() => ({ fetchListPayload: vi.fn() }))
vi.mock('@/lib/ssr/fetchListPayload', () => ({ fetchListPayload }))

// `seedFirstScreen` awaits `connection()`, which marks a request-time render and
// has nothing to do outside Next.
vi.mock('next/server', () => ({ connection: async () => {} }))

// On the server `getQueryClient()` mints a fresh client per call. Under jsdom it
// would hand back the browser singleton, so the server behaviour is restored.
vi.mock('@/lib/queryClient', async importOriginal => {
  const actual = await importOriginal<typeof import('@/lib/queryClient')>()
  const { QueryClient } = await import('@tanstack/react-query')
  return { ...actual, getQueryClient: () => new QueryClient() }
})

vi.mock('next/navigation', () => ({
  useRouter: () => ({ push: vi.fn(), replace: vi.fn() }),
  useSearchParams: () => new URLSearchParams(),
  usePathname: () => '/labels',
}))

import LabelsPage, { HydratedLabelList } from './page'
import {
  LABEL_LIST_FIRST_SCREEN_KEY,
  LABEL_LIST_FIRST_SCREEN_URL,
} from '@/features/labels/listFirstScreen'

const LOADER = 'animate-spin'

const labelsPayload = {
  labels: [
    {
      id: 1,
      name: 'Sub Pop',
      slug: 'sub-pop',
      city: 'Seattle',
      state: 'WA',
      status: 'active',
      artist_count: 12,
      release_count: 340,
    },
    {
      id: 2,
      name: 'Merge Records',
      slug: 'merge-records',
      city: 'Durham',
      state: 'NC',
      status: 'active',
      artist_count: 8,
      release_count: 120,
    },
  ],
  count: 2,
}

const renderBoundary = (
  params: Record<string, string | string[] | undefined> = {}
) => HydratedLabelList({ searchParams: Promise.resolve(params) })

/**
 * The route's HTML for the list: no client fetch runs inside `renderToString`,
 * so whatever this prints came from the seed.
 */
function serverRender(boundary: ReactElement): string {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return renderToString(
    <QueryClientProvider client={client}>{boundary}</QueryClientProvider>
  )
}

const stateOf = (boundary: ReactElement) =>
  (boundary.props as { state: DehydratedState }).state

describe('/labels server-seeded first screen', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    fetchListPayload.mockResolvedValue(labelsPayload)
  })

  it('reads the paired first-screen URL', async () => {
    await renderBoundary()

    expect(fetchListPayload).toHaveBeenCalledTimes(1)
    expect(fetchListPayload).toHaveBeenCalledWith(
      expect.objectContaining({
        url: LABEL_LIST_FIRST_SCREEN_URL,
        collection: 'labels',
      })
    )
  })

  it('seeds exactly the first-screen key', async () => {
    const state = stateOf(await renderBoundary())

    expect(state.queries.map(q => q.queryKey)).toEqual([
      LABEL_LIST_FIRST_SCREEN_KEY,
    ])
  })

  // The real list over the seed: a key the hook does not register would leave
  // this on the loader.
  it('server-renders the labels as links, with no loader', async () => {
    const html = serverRender(await renderBoundary())

    expect(html).toContain('href="/labels/sub-pop"')
    expect(html).toContain('href="/labels/merge-records"')
    expect(html).not.toContain(LOADER)
  })

  it('renders the list with its loader when the read fails, rather than throwing', async () => {
    fetchListPayload.mockResolvedValue(null)

    const boundary = await renderBoundary()

    expect(boundary.type).toBe(HydrationBoundary)
    expect(stateOf(boundary)).toEqual({ mutations: [], queries: [] })
    expect(serverRender(boundary)).toContain(LOADER)
  })

  it.each([
    ['status', 'active'],
    ['tags', 'shoegaze'],
    ['tag_match', 'any'],
  ])('skips the read when ?%s= is present', async (key, value) => {
    const boundary = await renderBoundary({ [key]: value })

    expect(fetchListPayload).not.toHaveBeenCalled()
    expect(boundary.type).toBe(HydrationBoundary)
    expect(stateOf(boundary)).toEqual({ mutations: [], queries: [] })
  })

  it('renders the heading and the boundary fallback in the page body', () => {
    const html = renderToString(
      LabelsPage({ searchParams: Promise.resolve({}) })
    )

    expect(html).toContain('>Labels</h1>')
    expect(html).toContain(LOADER)
  })
})
