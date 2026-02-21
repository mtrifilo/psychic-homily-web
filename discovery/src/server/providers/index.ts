import { emptybottleProvider } from './emptybottle'
import { jsonldProvider } from './jsonld'
import { seeticketsProvider } from './seetickets'
import { ticketwebProvider } from './ticketweb'
import { wixProvider } from './wix'
import type { DiscoveryProvider } from './types'

// Provider registry
const providers: Record<string, DiscoveryProvider> = {
  ticketweb: ticketwebProvider,
  jsonld: jsonldProvider,
  wix: wixProvider,
  seetickets: seeticketsProvider,
  emptybottle: emptybottleProvider,
}

// Get provider by type
export function getProvider(type: string): DiscoveryProvider | undefined {
  return providers[type]
}

// Export types
export type { DiscoveredEvent, PreviewEvent, DiscoveryProvider, ScrapeProgress, OnScrapeProgress } from './types'
