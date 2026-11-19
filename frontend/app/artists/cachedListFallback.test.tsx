import { vi } from 'vitest'
import { describeCachedListFallback } from '@/test/cachedListFallbackSuite'
import { suspenseFallbackOf } from '@/test/firstScreenRender'

// The page's ItemList read; no artists, so no JSON-LD block.
const { fetchSeoList } = vi.hoisted(() => ({ fetchSeoList: vi.fn(async () => []) }))
vi.mock('@/lib/seo/fetchSeoList', () => ({ fetchSeoList }))

// The browser requests a mounted list issues on a cold cache. They never
// answer, so the list stays on its first request.
const { apiRequest } = vi.hoisted(() => ({ apiRequest: vi.fn() }))
vi.mock('@/lib/api', async importOriginal => ({
  ...(await importOriginal<typeof import('@/lib/api')>()),
  apiRequest,
}))

// The URL a mounted list reads. Reset per test; a test may swap in params.
const nav = vi.hoisted(() => ({ params: new URLSearchParams() }))
vi.mock('next/navigation', () => ({
  useRouter: () => ({ push: vi.fn(), replace: vi.fn() }),
  useSearchParams: () => nav.params,
  usePathname: () => '/artists',
}))

// nuqs `useQueryState` reads the same mocked URL; the real parsers run.
vi.mock('nuqs', async importOriginal => {
  const actual = await importOriginal<typeof import('nuqs')>()
  return {
    ...actual,
    useQueryState: (
      key: string,
      parser: { parse: (v: string) => unknown; defaultValue?: unknown }
    ) => {
      const raw = nav.params.get(key)
      const value = raw != null ? parser.parse(raw) : (parser.defaultValue ?? null)
      return [value, vi.fn()]
    },
  }
})

import ArtistsPage from './page'
import { ArtistList } from '@/features/artists'
import {
  ARTIST_LIST_FIRST_SCREEN_KEY,
  ARTIST_MISSING_PARAM,
  artistQueryKeys,
} from '@/features/artists/api'
import type { ArtistListItem } from '@/features/artists/types'

const artist = (id: number, name: string, slug: string): ArtistListItem => ({
  id,
  slug,
  name,
  city: 'Phoenix',
  state: 'AZ',
  bandcamp_embed_url: null,
  upcoming_show_count: 2,
  social: {
    instagram: null,
    facebook: null,
    twitter: null,
    youtube: null,
    spotify: null,
    soundcloud: null,
    bandcamp: null,
    website: null,
  },
  created_at: '2024-01-01T00:00:00Z',
  updated_at: '2024-01-01T00:00:00Z',
})

const citiesPayload = { cities: [{ city: 'Phoenix', state: 'AZ', artist_count: 2 }] }

describeCachedListFallback({
  route: '/artists',
  fallback: async () =>
    suspenseFallbackOf(await ArtistsPage({ searchParams: Promise.resolve({}) })),
  list: ArtistList,
  seeded: [
    [
      ARTIST_LIST_FIRST_SCREEN_KEY,
      { artists: [artist(1, 'Gatecreeper', 'gatecreeper')], total: 1, limit: 50, offset: 0 },
    ],
    [artistQueryKeys.cities, citiesPayload],
  ],
  defeatingParams: ['page', 'cities', 'tags', 'tag_match', ARTIST_MISSING_PARAM],
  seededRow: 'href="/artists/gatecreeper"',
  // The route's skeleton, and the list's own first-load spinner.
  loaders: ['w-full max-w-6xl animate-pulse', 'aria-label="Loading"'],
  otherQuery: {
    // Page 2: the first-screen key with the next offset.
    entry: [
      artistQueryKeys.list({
        ...(ARTIST_LIST_FIRST_SCREEN_KEY[2] as Record<string, unknown>),
        offset: 50,
      }),
      { artists: [artist(51, 'Snailmate', 'snailmate')], total: 51, limit: 50, offset: 50 },
    ],
    row: 'href="/artists/snailmate"',
    search: 'page=2',
  },
  setSearch: search => {
    nav.params = new URLSearchParams(search)
  },
  reset: () => {
    vi.clearAllMocks()
    apiRequest.mockReturnValue(new Promise(() => {}))
  },
})
