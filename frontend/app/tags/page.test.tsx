import { beforeEach, describe, expect, it, vi } from 'vitest'
import { renderToString } from 'react-dom/server'
import { HydrationBoundary, hashKey } from '@tanstack/react-query'
import {
  LOADER,
  boundaryStateOf as stateOf,
  registeredQueryHashes,
  serverRenderFirstScreen as serverRender,
} from '@/test/firstScreenRender'

const { fetchListPayload } = vi.hoisted(() => ({ fetchListPayload: vi.fn() }))
vi.mock('@/lib/ssr/fetchListPayload', () => ({ fetchListPayload }))

// The browser request a mounted list issues on a cold cache. It never
// answers, so the list stays on its first request.
const { apiRequest } = vi.hoisted(() => ({ apiRequest: vi.fn() }))
vi.mock('@/lib/api', async importOriginal => ({
  ...(await importOriginal<typeof import('@/lib/api')>()),
  apiRequest,
}))

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
  usePathname: () => '/tags',
}))

import TagsPage, { HydratedTagBrowse } from './page'
import { TagBrowse } from '@/features/tags/components/TagBrowse'
import { TAG_BROWSE_FIRST_SCREEN_READS } from '@/features/tags/browseFirstScreen'
import { TAG_CATEGORIES } from '@/features/tags/types'


const tag = (id: number, name: string, category: string) => ({
  id,
  name,
  slug: name,
  category,
  is_official: false,
  usage_count: 10 - id,
  created_at: '2025-01-01T00:00:00Z',
})

const CATEGORY_TOTALS: Record<string, number> = {
  genre: 115,
  locale: 31,
  other: 20,
  crew: 7,
}

/** Answers each first-screen read the way the backend would. */
function answerReads(url: string) {
  const category = new URL(url).searchParams.get('category')
  if (category) return { tags: [], total: CATEGORY_TOTALS[category] }
  return {
    tags: [tag(1, 'shoegaze', 'genre'), tag(2, 'phoenix', 'locale')],
    total: 173,
  }
}

const renderBoundary = (
  params: Record<string, string | string[] | undefined> = {}
) => HydratedTagBrowse({ searchParams: Promise.resolve(params) })

/** The count a facet chip prints, by its test id. */
function chipCount(html: string, label: string): string | undefined {
  const container = document.createElement('div')
  container.innerHTML = html
  const chip = container.querySelector(`[data-testid="facet-${label}"]`)
  return chip?.querySelectorAll('span')[1]?.textContent ?? undefined
}

describe('/tags server-seeded first screen', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    fetchListPayload.mockImplementation(async ({ url }: { url: string }) =>
      answerReads(url)
    )
  })

  it('reads the directory page and one count per category', async () => {
    await renderBoundary()

    const urls = fetchListPayload.mock.calls.map(([options]) => options.url)
    expect(urls).toEqual(TAG_BROWSE_FIRST_SCREEN_READS.map(read => read.url))
    expect(urls).toHaveLength(1 + TAG_CATEGORIES.length)
    expect(
      fetchListPayload.mock.calls.every(([options]) => options.collection === 'tags')
    ).toBe(true)
  })

  it('seeds exactly the first-screen keys', async () => {
    const state = stateOf(await renderBoundary())

    expect(state.queries.map(q => q.queryKey)).toEqual(
      TAG_BROWSE_FIRST_SCREEN_READS.map(read => read.queryKey)
    )
  })

  // The real directory over the seed: a key the hooks do not register would
  // leave the table on its loader and the chips at zero.
  it('server-renders the directory as links and the chips with their counts', async () => {
    const html = serverRender(await renderBoundary())

    expect(html).toContain('href="/tags/shoegaze"')
    expect(html).toContain('href="/tags/phoenix"')
    expect(html).not.toContain(LOADER)
    expect(chipCount(html, 'genre')).toBe('115')
    expect(chipCount(html, 'crew')).toBe('7')
    expect(chipCount(html, 'all')).toBe('173')
  })

  it('seeds nothing when any read fails, so the directory renders its loader', async () => {
    fetchListPayload.mockImplementation(async ({ url }: { url: string }) =>
      new URL(url).searchParams.get('category') === 'crew' ? null : answerReads(url)
    )

    const boundary = await renderBoundary()

    expect(boundary.type).toBe(HydrationBoundary)
    expect(stateOf(boundary)).toEqual({ mutations: [], queries: [] })
    expect(serverRender(boundary)).toContain(LOADER)
  })

  it('skips the reads when ?sort= is present', async () => {
    const boundary = await renderBoundary({ sort: 'alphabetical' })

    expect(fetchListPayload).not.toHaveBeenCalled()
    expect(boundary.type).toBe(HydrationBoundary)
    expect(stateOf(boundary)).toEqual({ mutations: [], queries: [] })
  })

  it('renders the heading and the boundary fallback in the page body', () => {
    const html = renderToString(TagsPage({ searchParams: Promise.resolve({}) }))

    expect(html).toContain('>Tags</h1>')
    expect(html).toContain(LOADER)
  })
})

/**
 * The client half of the pair: mounted on a cold cache, the real list requests
 * the URL the server reads and registers the key the server seeds. A drift in
 * either leaves the seed unread or seeds the wrong payload.
 */
describe('/tags first-screen request pairing', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    apiRequest.mockReturnValue(new Promise(() => {}))
  })

  it('TagBrowse requests exactly the first-screen URLs and keys', () => {
    const hashes = registeredQueryHashes(<TagBrowse />)

    expect([...hashes].sort()).toEqual(
      TAG_BROWSE_FIRST_SCREEN_READS.map(read => hashKey(read.queryKey)).sort()
    )
    expect(apiRequest.mock.calls.map(([url]) => url).sort()).toEqual(
      TAG_BROWSE_FIRST_SCREEN_READS.map(read => read.url).sort()
    )
  })
})
