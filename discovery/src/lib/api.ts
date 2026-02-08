import type {
  ScrapedEvent,
  PreviewEvent,
  ImportResult,
  ImportStatusMap,
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
import { DISCOVERY_API_URL } from './config'

// Get settings from localStorage
export function getSettings(): AppSettings {
  const defaults: AppSettings = {
    apiToken: '',
    stageToken: '',
    productionToken: '',
    localToken: '',
    targetEnvironment: 'stage',
    stageUrl: 'https://stage.api.psychichomily.com',
    productionUrl: 'https://api.psychichomily.com',
  }

  if (typeof window === 'undefined') {
    return defaults
  }

  // Try new key first, fall back to old key for migration
  const stored = localStorage.getItem('discovery-settings') || localStorage.getItem('scraper-settings')
  if (stored) {
    // Save under new key if migrating from old key
    localStorage.setItem('discovery-settings', stored)
    const parsed = JSON.parse(stored)
    // Migrate old single apiToken to new format
    if (parsed.apiToken && !parsed.stageToken && !parsed.productionToken) {
      // If they had a single token, copy it to the current target environment
      if (parsed.targetEnvironment === 'production') {
        parsed.productionToken = parsed.apiToken
      } else {
        parsed.stageToken = parsed.apiToken
      }
    }
    return { ...defaults, ...parsed }
  }
  return defaults
}

// Get the API token for the current target environment
export function getTargetToken(): string {
  const settings = getSettings()
  return settings.targetEnvironment === 'production'
    ? settings.productionToken
    : settings.stageToken
}

// Get the API token for local development
export function getLocalToken(): string {
  const settings = getSettings()
  return settings.localToken
}

// Save settings to localStorage
export function saveSettings(settings: AppSettings): void {
  localStorage.setItem('discovery-settings', JSON.stringify(settings))
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
  const response = await fetch(`${DISCOVERY_API_URL}/preview/${venueSlug}`)
  if (!response.ok) {
    const error = await response.json()
    throw new Error(error.error || 'Failed to preview events')
  }
  return response.json()
}

// Preview events from multiple venues in parallel
export async function previewVenueEventsBatch(venueSlugs: string[]): Promise<BatchPreviewResult[]> {
  const response = await fetch(`${DISCOVERY_API_URL}/preview-batch`, {
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
  const response = await fetch(`${DISCOVERY_API_URL}/scrape/${venueSlug}`, {
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
  const token = getTargetToken()
  if (!token) {
    const settings = getSettings()
    const env = settings.targetEnvironment === 'production' ? 'Production' : 'Stage'
    throw new Error(`${env} API token not configured. Go to Settings to add your token.`)
  }

  const baseUrl = getApiBaseUrl()
  const response = await fetch(`${baseUrl}/admin/discovery/import`, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      'Authorization': `Bearer ${token}`,
    },
    body: JSON.stringify({ events, dryRun }),
  })

  if (!response.ok) {
    const error = await response.json()
    throw new Error(error.detail || error.message || 'Failed to import events')
  }

  return response.json()
}

// Check import status of events against the target environment
export async function checkImportStatus(
  events: { id: string; venueSlug: string }[]
): Promise<ImportStatusMap> {
  const token = getTargetToken()
  if (!token) {
    // Graceful degradation — no token means no badges
    return {}
  }

  const baseUrl = getApiBaseUrl()
  const response = await fetch(`${baseUrl}/admin/discovery/check`, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      'Authorization': `Bearer ${token}`,
    },
    body: JSON.stringify({ events }),
  })

  if (!response.ok) {
    // Silently fail — badges are non-critical
    console.warn('Failed to check import status:', response.status)
    return {}
  }

  const data = await response.json()
  return data.events || {}
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
  const token = getLocalToken()
  if (!token) {
    throw new Error('Local API token not configured. Go to Settings to add your local token.')
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
      'Authorization': `Bearer ${token}`,
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
  const token = getLocalToken()
  if (!token) {
    throw new Error('Local API token not configured. Go to Settings to add your local token.')
  }

  const searchParams = new URLSearchParams()
  if (params.limit) searchParams.set('limit', params.limit.toString())
  if (params.offset) searchParams.set('offset', params.offset.toString())
  if (params.search) searchParams.set('search', params.search)

  const url = `${getSourceApiUrl()}/admin/export/artists?${searchParams.toString()}`
  const response = await fetch(url, {
    headers: {
      'Authorization': `Bearer ${token}`,
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
  const token = getLocalToken()
  if (!token) {
    throw new Error('Local API token not configured. Go to Settings to add your local token.')
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
      'Authorization': `Bearer ${token}`,
    },
  })

  if (!response.ok) {
    const error = await response.json()
    throw new Error(error.detail || error.message || 'Failed to export venues')
  }

  return response.json()
}

// Fetch shows from a remote environment (for existence checks)
export async function fetchRemoteShows(
  baseUrl: string,
  token: string,
  params: { limit?: number; offset?: number; status?: string }
): Promise<ExportShowsResult> {
  try {
    const searchParams = new URLSearchParams()
    if (params.limit) searchParams.set('limit', params.limit.toString())
    if (params.offset) searchParams.set('offset', params.offset.toString())
    if (params.status) searchParams.set('status', params.status)

    const url = `${baseUrl}/admin/export/shows?${searchParams.toString()}`
    const response = await fetch(url, {
      headers: { 'Authorization': `Bearer ${token}` },
    })

    if (!response.ok) {
      return { shows: [], total: 0 }
    }

    return response.json()
  } catch {
    return { shows: [], total: 0 }
  }
}

// Import data to a specific remote environment
export async function importDataToEnv(
  data: {
    shows?: ExportedShow[]
    artists?: ExportedArtist[]
    venues?: ExportedVenue[]
  },
  dryRun: boolean,
  env: 'stage' | 'production'
): Promise<DataImportResult> {
  const settings = getSettings()
  const token = env === 'production' ? settings.productionToken : settings.stageToken
  const baseUrl = env === 'production' ? settings.productionUrl : settings.stageUrl
  const envLabel = env === 'production' ? 'Production' : 'Stage'

  if (!token) {
    throw new Error(`${envLabel} API token not configured. Go to Settings to add your token.`)
  }

  const response = await fetch(`${baseUrl}/admin/data/import`, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      'Authorization': `Bearer ${token}`,
    },
    body: JSON.stringify({ ...data, dryRun }),
  })

  if (!response.ok) {
    const error = await response.json()
    throw new Error(error.detail || error.message || `Failed to import data to ${envLabel}`)
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
  const token = getTargetToken()
  if (!token) {
    const settings = getSettings()
    const env = settings.targetEnvironment === 'production' ? 'Production' : 'Stage'
    throw new Error(`${env} API token not configured. Go to Settings to add your token.`)
  }

  const baseUrl = getApiBaseUrl() // This gets Stage or Production URL
  const response = await fetch(`${baseUrl}/admin/data/import`, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      'Authorization': `Bearer ${token}`,
    },
    body: JSON.stringify({ ...data, dryRun }),
  })

  if (!response.ok) {
    const error = await response.json()
    throw new Error(error.detail || error.message || 'Failed to import data')
  }

  return response.json()
}
