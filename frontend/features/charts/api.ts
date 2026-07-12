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
} as const

export const chartQueryKeys = {
  all: ['charts'] as const,
  mostActiveArtists: (window: ChartWindow, limit: number) =>
    ['charts', 'most-active-artists', window, limit] as const,
  onTheRadio: (window: ChartWindow, limit: number) =>
    ['charts', 'on-the-radio', window, limit] as const,
  mostAnticipated: (limit: number) =>
    ['charts', 'most-anticipated', limit] as const,
  busiestVenues: (window: ChartWindow, limit: number) =>
    ['charts', 'busiest-venues', window, limit] as const,
  newReleases: (window: ChartWindow, limit: number) =>
    ['charts', 'new-releases', window, limit] as const,
  openersToWatch: (window: ChartWindow, limit: number) =>
    ['charts', 'openers-to-watch', window, limit] as const,
  summary: (window: ChartWindow) => ['charts', 'summary', window] as const,
  freshlyAdded: (limit: number) => ['charts', 'freshly-added', limit] as const,
} as const
