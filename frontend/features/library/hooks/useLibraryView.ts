'use client'

import { useLocalStorageEnum } from '@/lib/hooks/common/useLocalStorageEnum'

const VALID_VIEWS = ['table', 'wall'] as const

export type LibraryView = (typeof VALID_VIEWS)[number]

const LIBRARY_VIEW_STORAGE_KEY = 'ph-library-view'
const DEFAULT_LIBRARY_VIEW: LibraryView = 'table'

/**
 * Persisted library content view preference (PSY-1429).
 * Mirrors useDensity — SSR-safe enum via useLocalStorageEnum.
 * Table is the default; wall is the alternate collage grid.
 */
export function useLibraryView() {
  const [view, setView] = useLocalStorageEnum<LibraryView>(
    LIBRARY_VIEW_STORAGE_KEY,
    DEFAULT_LIBRARY_VIEW,
    VALID_VIEWS
  )
  return { view, setView }
}
