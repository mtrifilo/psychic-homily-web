import { jsonldProvider } from './jsonld'
import { ticketwebProvider } from './ticketweb'
import type { DiscoveryProvider } from './types'

// Provider registry
const providers: Record<string, DiscoveryProvider> = {
  ticketweb: ticketwebProvider,
  jsonld: jsonldProvider,
}

// Get provider by type
export function getProvider(type: string): DiscoveryProvider | undefined {
  return providers[type]
}

// Export types
export type { DiscoveredEvent, PreviewEvent, DiscoveryProvider } from './types'
