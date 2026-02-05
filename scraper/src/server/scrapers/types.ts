// Scraped event interface
export interface ScrapedEvent {
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
}

// Preview event interface (quick scan)
export interface PreviewEvent {
  id: string
  title: string
  date: string
  venue: string
}

// Scraper interface
export interface Scraper {
  preview(venueSlug: string): Promise<PreviewEvent[]>
  scrape(venueSlug: string, eventIds: string[]): Promise<ScrapedEvent[]>
}
