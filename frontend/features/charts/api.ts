import { API_BASE_URL } from '@/lib/api-base'
import type { ChartWindow } from './types'

export const chartEndpoints = {
  MOST_ACTIVE_ARTISTS: `${API_BASE_URL}/charts/most-active-artists`,
  ON_THE_RADIO: `${API_BASE_URL}/charts/on-the-radio`,
  MOST_ANTICIPATED: `${API_BASE_URL}/charts/most-anticipated`,
  BUSIEST_VENUES: `${API_BASE_URL}/charts/busiest-venues`,
  NEW_RELEASES: `${API_BASE_URL}/charts/new-releases`,
  OPENERS_TO_WATCH: `${API_BASE_URL}/charts/openers-to-watch`,
  SUMMARY: `${API_BASE_URL}/charts/summary`,
  FRESHLY_ADDED: `${API_BASE_URL}/charts/freshly-added`,
  SCENES: `${API_BASE_URL}/charts/scenes`,
  PERSONAL: `${API_BASE_URL}/charts/me`,
} as const

export const chartQueryKeys = {
  all: ['charts'] as const,
  mostActiveArtists: (window: ChartWindow, scene: string, limit: number) =>
    ['charts', 'most-active-artists', window, scene, limit] as const,
  onTheRadio: (window: ChartWindow, scene: string, limit: number) =>
    ['charts', 'on-the-radio', window, scene, limit] as const,
  mostAnticipated: (scene: string, limit: number) =>
    ['charts', 'most-anticipated', scene, limit] as const,
  busiestVenues: (window: ChartWindow, scene: string, limit: number) =>
    ['charts', 'busiest-venues', window, scene, limit] as const,
  newReleases: (window: ChartWindow, scene: string, limit: number) =>
    ['charts', 'new-releases', window, scene, limit] as const,
  openersToWatch: (window: ChartWindow, scene: string, limit: number) =>
    ['charts', 'openers-to-watch', window, scene, limit] as const,
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
} as const
