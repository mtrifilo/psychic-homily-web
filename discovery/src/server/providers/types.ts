// Discovered event interface
export interface DiscoveredEvent {
  id: string
  title: string
  date: string
  venue: string
  venueSlug: string
  imageUrl?: string
  doorsTime?: string
  showTime?: string
  ticketUrl?: string
  artists: string[]
  scrapedAt: string
  price?: string
  ageRestriction?: string
  isSoldOut?: boolean
  isCancelled?: boolean
}

// Preview event interface (quick scan)
export interface PreviewEvent {
  id: string
  title: string
  date: string
  venue: string
}

// Scrape progress update
export interface ScrapeProgress {
  current: number
  total: number
  eventTitle: string
  phase?: string
}

// Callback type for scrape progress
export type OnScrapeProgress = (progress: ScrapeProgress) => void

// Discovery provider interface
export interface DiscoveryProvider {
  preview(venueSlug: string): Promise<PreviewEvent[]>
  scrape(venueSlug: string, eventIds: string[], onProgress?: OnScrapeProgress): Promise<DiscoveredEvent[]>
}
