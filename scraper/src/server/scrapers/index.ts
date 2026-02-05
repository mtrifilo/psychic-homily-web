import { ticketwebScraper } from './ticketweb'
import type { Scraper } from './types'

// Scraper registry
const scrapers: Record<string, Scraper> = {
  ticketweb: ticketwebScraper,
}

// Get scraper by type
export function getScraper(type: string): Scraper | undefined {
  return scrapers[type]
}

// Export types
export type { ScrapedEvent, PreviewEvent, Scraper } from './types'
