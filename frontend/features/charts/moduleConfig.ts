export const CHART_MODULE_SLUGS = [
  'most-active-artists',
  'on-the-radio',
  'most-anticipated',
  'busiest-venues',
  'new-releases',
  'openers-to-watch',
] as const

export type ChartModuleSlug = (typeof CHART_MODULE_SLUGS)[number]

export type ChartColumnKey =
  | 'artist'
  | 'shows'
  | 'headline'
  | 'last-show'
  | 'plays'
  | 'stations'
  | 'rotation'
  | 'show'
  | 'date'
  | 'venue'
  | 'saves'
  | 'location'
  | 'release'
  | 'artists'
  | 'labels'
  | 'released'
  | 'added'
  | 'slots'

export interface ChartColumnConfig {
  key: ChartColumnKey
  label: string
  className?: string
}

export interface ChartModuleConfig {
  title: string
  qualifyingNoun: string
  columns: ChartColumnConfig[]
}

export const CHART_MODULE_CONFIG: Record<ChartModuleSlug, ChartModuleConfig> = {
  'most-active-artists': {
    title: 'Hardest-Working Artists',
    qualifyingNoun: 'artists',
    columns: [
      { key: 'artist', label: 'Artist' },
      { key: 'shows', label: 'Shows', className: 'text-right' },
      { key: 'headline', label: 'Headline %', className: 'text-right' },
      { key: 'last-show', label: 'Last show' },
    ],
  },
  'on-the-radio': {
    title: 'On the Radio',
    qualifyingNoun: 'artists',
    columns: [
      { key: 'artist', label: 'Artist' },
      { key: 'plays', label: 'Plays', className: 'text-right' },
      { key: 'stations', label: 'Stations', className: 'text-right' },
      { key: 'rotation', label: 'Rotation' },
    ],
  },
  'most-anticipated': {
    title: 'Most Anticipated Shows',
    qualifyingNoun: 'shows',
    columns: [
      { key: 'show', label: 'Show' },
      { key: 'date', label: 'Date' },
      { key: 'venue', label: 'Venue' },
      { key: 'saves', label: 'Saves', className: 'text-right' },
    ],
  },
  'busiest-venues': {
    title: 'Busiest Venues',
    qualifyingNoun: 'venues',
    columns: [
      { key: 'venue', label: 'Venue' },
      { key: 'location', label: 'Location' },
      { key: 'shows', label: 'Shows', className: 'text-right' },
    ],
  },
  'new-releases': {
    title: 'New Releases',
    qualifyingNoun: 'releases',
    columns: [
      { key: 'release', label: 'Release' },
      { key: 'artists', label: 'Artists' },
      { key: 'labels', label: 'Labels' },
      { key: 'released', label: 'Released' },
      { key: 'added', label: 'Added' },
    ],
  },
  'openers-to-watch': {
    title: 'Openers to Watch',
    qualifyingNoun: 'artists',
    columns: [
      { key: 'artist', label: 'Artist' },
      { key: 'location', label: 'Location' },
      { key: 'slots', label: 'Support slots', className: 'text-right' },
    ],
  },
}

export function isChartModuleSlug(value: string): value is ChartModuleSlug {
  return CHART_MODULE_SLUGS.includes(value as ChartModuleSlug)
}
