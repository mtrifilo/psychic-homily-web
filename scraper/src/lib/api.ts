import type {
  ScrapedEvent,
  PreviewEvent,
  ImportResult,
  AppSettings,
  BatchPreviewResult,
  ExportShowsResult,
  ExportArtistsResult,
  ExportVenuesResult,
  DataImportRequest,
  DataImportResult,
  ExportedShow,
  ExportedArtist,
  ExportedVenue,
} from './types'
import { SCRAPER_API_URL } from './config'

// Get settings from localStorage
export function getSettings(): AppSettings {
  if (typeof window === 'undefined') {
    return {
      apiToken: '',
      targetEnvironment: 'stage',
      stageUrl: 'https://api-stage.psychichomily.com',
      productionUrl: 'https://api.psychichomily.com',
    }
  }
  const stored = localStorage.getItem('scraper-settings')
  if (stored) {
    return JSON.parse(stored)
  }
  return {
    apiToken: '',
    targetEnvironment: 'stage',
    stageUrl: 'https://api-stage.psychichomily.com',
    productionUrl: 'https://api.psychichomily.com',
  }
}

// Save settings to localStorage
export function saveSettings(settings: AppSettings): void {
  localStorage.setItem('scraper-settings', JSON.stringify(settings))
}

// Get the current API base URL based on settings
function getApiBaseUrl(): string {
  const settings = getSettings()
  return settings.targetEnvironment === 'production'
    ? settings.productionUrl
    : settings.stageUrl
}

// Preview events from a venue (quick scan)
export async function previewVenueEvents(venueSlug: string): Promise<PreviewEvent[]> {
  const response = await fetch(`${SCRAPER_API_URL}/preview/${venueSlug}`)
  if (!response.ok) {
    const error = await response.json()
    throw new Error(error.error || 'Failed to preview events')
  }
  return response.json()
}

// Preview events from multiple venues in parallel
export async function previewVenueEventsBatch(venueSlugs: string[]): Promise<BatchPreviewResult[]> {
  const response = await fetch(`${SCRAPER_API_URL}/preview-batch`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ venueSlugs }),
  })
  if (!response.ok) {
    const error = await response.json()
    throw new Error(error.error || 'Failed to batch preview events')
  }
  return response.json()
}

// Scrape selected events from a venue (full details)
export async function scrapeVenueEvents(
  venueSlug: string,
  eventIds: string[]
): Promise<ScrapedEvent[]> {
  const response = await fetch(`${SCRAPER_API_URL}/scrape/${venueSlug}`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ eventIds }),
  })
  if (!response.ok) {
    const error = await response.json()
    throw new Error(error.error || 'Failed to scrape events')
  }
  return response.json()
}

// Import events to the backend
export async function importEvents(
  events: ScrapedEvent[],
  dryRun: boolean = false
): Promise<ImportResult> {
  const settings = getSettings()
  if (!settings.apiToken) {
    throw new Error('API token not configured. Go to Settings to add your token.')
  }

  const baseUrl = getApiBaseUrl()
  const response = await fetch(`${baseUrl}/admin/scraper/import`, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      'Authorization': `Bearer ${settings.apiToken}`,
    },
    body: JSON.stringify({ events, dryRun }),
  })

  if (!response.ok) {
    const error = await response.json()
    throw new Error(error.detail || error.message || 'Failed to import events')
  }

  return response.json()
}

// ============================================================================
// Data Export/Import API (for syncing local data to Stage/Production)
// ============================================================================

// Local API URL - the local Go backend
const LOCAL_API_URL = 'http://localhost:8080'

// Get the source API URL (local for export, remote for import)
function getSourceApiUrl(): string {
  return LOCAL_API_URL
}

// Export shows from local database
export async function exportShows(params: {
  limit?: number
  offset?: number
  status?: string
  fromDate?: string
  city?: string
  state?: string
}): Promise<ExportShowsResult> {
  const settings = getSettings()
  if (!settings.apiToken) {
    throw new Error('API token not configured. Go to Settings to add your token.')
  }

  const searchParams = new URLSearchParams()
  if (params.limit) searchParams.set('limit', params.limit.toString())
  if (params.offset) searchParams.set('offset', params.offset.toString())
  if (params.status) searchParams.set('status', params.status)
  if (params.fromDate) searchParams.set('from_date', params.fromDate)
  if (params.city) searchParams.set('city', params.city)
  if (params.state) searchParams.set('state', params.state)

  const url = `${getSourceApiUrl()}/admin/export/shows?${searchParams.toString()}`
  const response = await fetch(url, {
    headers: {
      'Authorization': `Bearer ${settings.apiToken}`,
    },
  })

  if (!response.ok) {
    const error = await response.json()
    throw new Error(error.detail || error.message || 'Failed to export shows')
  }

  return response.json()
}

// Export artists from local database
export async function exportArtists(params: {
  limit?: number
  offset?: number
  search?: string
}): Promise<ExportArtistsResult> {
  const settings = getSettings()
  if (!settings.apiToken) {
    throw new Error('API token not configured. Go to Settings to add your token.')
  }

  const searchParams = new URLSearchParams()
  if (params.limit) searchParams.set('limit', params.limit.toString())
  if (params.offset) searchParams.set('offset', params.offset.toString())
  if (params.search) searchParams.set('search', params.search)

  const url = `${getSourceApiUrl()}/admin/export/artists?${searchParams.toString()}`
  const response = await fetch(url, {
    headers: {
      'Authorization': `Bearer ${settings.apiToken}`,
    },
  })

  if (!response.ok) {
    const error = await response.json()
    throw new Error(error.detail || error.message || 'Failed to export artists')
  }

  return response.json()
}

// Export venues from local database
export async function exportVenues(params: {
  limit?: number
  offset?: number
  search?: string
  verified?: string
  city?: string
  state?: string
}): Promise<ExportVenuesResult> {
  const settings = getSettings()
  if (!settings.apiToken) {
    throw new Error('API token not configured. Go to Settings to add your token.')
  }

  const searchParams = new URLSearchParams()
  if (params.limit) searchParams.set('limit', params.limit.toString())
  if (params.offset) searchParams.set('offset', params.offset.toString())
  if (params.search) searchParams.set('search', params.search)
  if (params.verified) searchParams.set('verified', params.verified)
  if (params.city) searchParams.set('city', params.city)
  if (params.state) searchParams.set('state', params.state)

  const url = `${getSourceApiUrl()}/admin/export/venues?${searchParams.toString()}`
  const response = await fetch(url, {
    headers: {
      'Authorization': `Bearer ${settings.apiToken}`,
    },
  })

  if (!response.ok) {
    const error = await response.json()
    throw new Error(error.detail || error.message || 'Failed to export venues')
  }

  return response.json()
}

// Import data to remote backend (Stage or Production)
export async function importData(
  data: {
    shows?: ExportedShow[]
    artists?: ExportedArtist[]
    venues?: ExportedVenue[]
  },
  dryRun: boolean = false
): Promise<DataImportResult> {
  const settings = getSettings()
  if (!settings.apiToken) {
    throw new Error('API token not configured. Go to Settings to add your token.')
  }

  const baseUrl = getApiBaseUrl() // This gets Stage or Production URL
  const response = await fetch(`${baseUrl}/admin/data/import`, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      'Authorization': `Bearer ${settings.apiToken}`,
    },
    body: JSON.stringify({ ...data, dryRun }),
  })

  if (!response.ok) {
    const error = await response.json()
    throw new Error(error.detail || error.message || 'Failed to import data')
  }

  return response.json()
}
