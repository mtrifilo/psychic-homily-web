'use client'

import { useState, useCallback, useEffect } from 'react'

export type Density = 'compact' | 'comfortable' | 'expanded'

const DENSITY_STORAGE_PREFIX = 'ph-density'
const DEFAULT_DENSITY: Density = 'comfortable'
const VALID_DENSITIES: Density[] = ['compact', 'comfortable', 'expanded']

function getStorageKey(suffix?: string): string {
  return suffix ? `${DENSITY_STORAGE_PREFIX}-${suffix}` : DENSITY_STORAGE_PREFIX
}

function readDensity(key: string): Density {
  if (typeof window === 'undefined') return DEFAULT_DENSITY
  try {
    const stored = localStorage.getItem(key)
    if (stored && VALID_DENSITIES.includes(stored as Density)) {
      return stored as Density
    }
  } catch {
    // localStorage not available
  }
  return DEFAULT_DENSITY
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
  const [density, setDensityState] = useState<Density>(DEFAULT_DENSITY)

  // Read from localStorage on mount (client-side only)
  useEffect(() => {
    setDensityState(readDensity(key))
  }, [key])

  const setDensity = useCallback(
    (value: Density) => {
      setDensityState(value)
      try {
        localStorage.setItem(key, value)
      } catch {
        // localStorage not available
      }
    },
    [key]
  )

  return { density, setDensity }
}
