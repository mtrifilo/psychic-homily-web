// Scraped event that matches the backend's ScrapedEvent type
export interface ScrapedEvent {
  id: string
  title: string
  date: string // ISO format (YYYY-MM-DD)
  venue: string
  venueSlug: string
  imageUrl?: string
  doorsTime?: string
  showTime?: string
  ticketUrl?: string
  artists: string[]
  scrapedAt: string // ISO timestamp
}

// Preview event (quick scan without details)
export interface PreviewEvent {
  id: string
  title: string
  date: string
  venue: string
}

// Venue configuration for scraping
export interface VenueConfig {
  slug: string
  name: string
  scraperType: 'ticketweb' | 'seetickets' | 'other'
  url: string
  city: string
  state: string
}

// Batch preview result from parallel scraping
export interface BatchPreviewResult {
  venueSlug: string
  events: PreviewEvent[]
  error?: string
}

// Import result from the backend
export interface ImportResult {
  total: number
  imported: number
  duplicates: number
  rejected: number
  errors: number
  messages: string[]
}

// App settings stored in localStorage
export interface AppSettings {
  apiToken: string
  targetEnvironment: 'stage' | 'production'
  stageUrl: string
  productionUrl: string
}

// Default settings
export const DEFAULT_SETTINGS: AppSettings = {
  apiToken: '',
  targetEnvironment: 'stage',
  stageUrl: 'https://api-stage.psychichomily.com',
  productionUrl: 'https://api.psychichomily.com',
}

// ============================================================================
// Data Export/Import Types (for syncing local data to Stage/Production)
// ============================================================================

// Exported artist from local database
export interface ExportedArtist {
  name: string
  city?: string
  state?: string
  bandcampEmbedUrl?: string
  instagram?: string
  facebook?: string
  twitter?: string
  youtube?: string
  spotify?: string
  soundcloud?: string
  bandcamp?: string
  website?: string
}

// Exported venue from local database
export interface ExportedVenue {
  name: string
  address?: string
  city: string
  state: string
  zipcode?: string
  verified: boolean
  instagram?: string
  facebook?: string
  twitter?: string
  youtube?: string
  spotify?: string
  soundcloud?: string
  bandcamp?: string
  website?: string
}

// Show artist with position info
export interface ExportedShowArtist {
  name: string
  position: number
  setType: string
}

// Exported show from local database
export interface ExportedShow {
  title: string
  eventDate: string
  city?: string
  state?: string
  price?: number
  ageRequirement?: string
  description?: string
  status: string
  isSoldOut: boolean
  isCancelled: boolean
  venues: ExportedVenue[]
  artists: ExportedShowArtist[]
}

// Export results with pagination
export interface ExportShowsResult {
  shows: ExportedShow[]
  total: number
}

export interface ExportArtistsResult {
  artists: ExportedArtist[]
  total: number
}

export interface ExportVenuesResult {
  venues: ExportedVenue[]
  total: number
}

// Data import request
export interface DataImportRequest {
  shows?: ExportedShow[]
  artists?: ExportedArtist[]
  venues?: ExportedVenue[]
  dryRun: boolean
}

// Import statistics for a single entity type
export interface EntityImportStats {
  total: number
  imported: number
  duplicates: number
  updated?: number
  errors: number
  messages: string[]
}

// Data import result
export interface DataImportResult {
  shows: EntityImportStats
  artists: EntityImportStats
  venues: EntityImportStats
}
