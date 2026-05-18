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
 *
 * Usage:
 *   const { density, setDensity } = useDensity('shows')
 */
export function useDensity(storageKeySuffix?: string) {
  const key = getStorageKey(storageKeySuffix)
  const [density, setDensity] = useLocalStorageEnum<Density>(
    key,
    DEFAULT_DENSITY,
    VALID_DENSITIES
  )
  return { density, setDensity }
}
