import { API_BASE_URL } from '@/lib/api-base'
import type { ChartWindow } from './types'

export const chartEndpoints = {
  MOST_ACTIVE_ARTISTS: `${API_BASE_URL}/charts/most-active-artists`,
  ON_THE_RADIO: `${API_BASE_URL}/charts/on-the-radio`,
  MOST_ANTICIPATED: `${API_BASE_URL}/charts/most-anticipated`,
  BUSIEST_VENUES: `${API_BASE_URL}/charts/busiest-venues`,
  NEW_RELEASES: `${API_BASE_URL}/charts/new-releases`,
  OPENERS_TO_WATCH: `${API_BASE_URL}/charts/openers-to-watch`,
  TOP_TAGS: `${API_BASE_URL}/charts/top-tags`,
  SUMMARY: `${API_BASE_URL}/charts/summary`,
  FRESHLY_ADDED: `${API_BASE_URL}/charts/freshly-added`,
  SCENES: `${API_BASE_URL}/charts/scenes`,
  PERSONAL: `${API_BASE_URL}/charts/me`,
  RANK: `${API_BASE_URL}/charts/rank`,
  FEATURED_COLLECTION_HISTORY: `${API_BASE_URL}/charts/featured-collection/history`,
} as const

export const chartQueryKeys = {
  all: ['charts'] as const,
  mostActiveArtists: (
    window: ChartWindow,
    scene: string,
    limit: number,
    offset = 0
  ) => ['charts', 'most-active-artists', window, scene, limit, offset] as const,
  onTheRadio: (window: ChartWindow, scene: string, limit: number, offset = 0) =>
    ['charts', 'on-the-radio', window, scene, limit, offset] as const,
  mostAnticipated: (
    window: ChartWindow,
    scene: string,
    limit: number,
    offset = 0
  ) => ['charts', 'most-anticipated', window, scene, limit, offset] as const,
  busiestVenues: (
    window: ChartWindow,
    scene: string,
    limit: number,
    offset = 0
  ) => ['charts', 'busiest-venues', window, scene, limit, offset] as const,
  newReleases: (
    window: ChartWindow,
    scene: string,
    limit: number,
    offset = 0
  ) => ['charts', 'new-releases', window, scene, limit, offset] as const,
  openersToWatch: (
    window: ChartWindow,
    scene: string,
    limit: number,
    offset = 0
  ) => ['charts', 'openers-to-watch', window, scene, limit, offset] as const,
  topTags: (window: ChartWindow, scene: string, limit: number) =>
    ['charts', 'top-tags', window, scene, limit] as const,
  summary: (window: ChartWindow, scene: string) =>
    ['charts', 'summary', window, scene] as const,
  freshlyAdded: (scene: string, limit: number) =>
    ['charts', 'freshly-added', scene, limit] as const,
  scenes: (window: ChartWindow) => ['charts', 'scenes', window] as const,
  personalRoot: ['charts', 'personal'] as const,
  personal: (userId?: string | number) =>
    [
      ...chartQueryKeys.personalRoot,
      userId == null ? null : String(userId),
    ] as const,
  rank: (
    entityType: string,
    entityId: number,
    window: ChartWindow
  ) => ['charts', 'rank', entityType, entityId, window] as const,
  featuredCollectionHistory: (limit: number, offset = 0) =>
    ['charts', 'featured-collection', 'history', limit, offset] as const,
} as const
