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
}

// Preview event interface (quick scan)
export interface PreviewEvent {
  id: string
  title: string
  date: string
  venue: string
}

// Discovery provider interface
export interface DiscoveryProvider {
  preview(venueSlug: string): Promise<PreviewEvent[]>
  scrape(venueSlug: string, eventIds: string[]): Promise<DiscoveredEvent[]>
}
