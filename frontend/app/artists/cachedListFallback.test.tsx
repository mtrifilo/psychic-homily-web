// Separate from `page.test.tsx`, which stubs `ArtistList` and `seedFirstScreen`:
// this suite needs the real list and the real seed.
import { vi } from 'vitest'
import { describeCachedListFallback } from '@/test/cachedListFallbackSuite'
import { cachedListFallbackOf } from '@/test/firstScreenRender'

// The page's ItemList read. It returns an artist, so the page also renders its
// ItemList `Suspense` ahead of the list's, which the fallback lookup must skip.
const { fetchSeoList } = vi.hoisted(() => ({
  fetchSeoList: vi.fn(async () => [{ slug: 'gatecreeper', name: 'Gatecreeper' }]),
}))
vi.mock('@/lib/seo/fetchSeoList', () => ({ fetchSeoList }))

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

import ArtistsPage, { HydratedArtistList } from './page'
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

const artistsPayload = {
  artists: [artist(1, 'Gatecreeper', 'gatecreeper')],
  total: 1,
  limit: 50,
  offset: 0,
}

describeCachedListFallback({
  route: '/artists',
  fallback: async () =>
    cachedListFallbackOf(await ArtistsPage({ searchParams: Promise.resolve({}) })),
  seededBoundary: () => {
    fetchListPayload.mockImplementation(async ({ url }: { url: string }) =>
      url.includes('/cities') ? citiesPayload : artistsPayload
    )
    return HydratedArtistList({ searchParams: Promise.resolve({}) })
  },
  list: ArtistList,
  seeded: [
    [ARTIST_LIST_FIRST_SCREEN_KEY, artistsPayload],
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
  nav,
  apiRequest,
})
