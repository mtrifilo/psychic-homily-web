'use client'

import { useLocalStorageEnum } from './useLocalStorageEnum'

const VALID_DENSITIES = ['compact', 'comfortable', 'expanded'] as const

export type Density = (typeof VALID_DENSITIES)[number]

const DENSITY_STORAGE_PREFIX = 'ph-density'
const DEFAULT_DENSITY: Density = 'comfortable'

function getStorageKey(suffix?: string): string {
  return suffix ? `${DENSITY_STORAGE_PREFIX}-${suffix}` : DENSITY_STORAGE_PREFIX
}

/**
 * Hook for managing display density preference.
 * Persists the selection in localStorage under a configurable key.
 *
 * @param storageKeySuffix - Optional suffix appended to the storage key (e.g., 'shows', 'artists')
 * @param defaultDensity - Density used until the user picks one (default 'comfortable').
 *   Collections pass 'compact' per PSY-892 D3 — collection viewers are
 *   already-curated audiences scanning a list, not first-time browsers.
 *
 * Usage:
 *   const { density, setDensity } = useDensity('shows')
 *   const { density, setDensity } = useDensity('collections', 'compact')
 */
export function useDensity(
  storageKeySuffix?: string,
  defaultDensity: Density = DEFAULT_DENSITY
) {
  const key = getStorageKey(storageKeySuffix)
  const [density, setDensity] = useLocalStorageEnum<Density>(
    key,
    defaultDensity,
    VALID_DENSITIES
  )
  return { density, setDensity }
}
