'use client'

import { createContext, useContext, useState, useCallback, useEffect, type ReactNode } from 'react'

export interface BreadcrumbEntry {
  label: string
  href: string
}

interface NavigationBreadcrumbContextValue {
  breadcrumbs: BreadcrumbEntry[]
  pushBreadcrumb: (label: string, href: string) => void
}

const STORAGE_KEY = 'ph-breadcrumbs'
const MAX_ENTRIES = 4

const NavigationBreadcrumbContext = createContext<NavigationBreadcrumbContextValue | null>(null)

function loadFromSession(): BreadcrumbEntry[] {
  if (typeof window === 'undefined') return []
  try {
    const stored = sessionStorage.getItem(STORAGE_KEY)
    if (stored) {
      const parsed = JSON.parse(stored)
      if (Array.isArray(parsed)) return parsed.slice(0, MAX_ENTRIES)
    }
  } catch {
    // Ignore parse errors
  }
  return []
}

function saveToSession(entries: BreadcrumbEntry[]) {
  if (typeof window === 'undefined') return
  try {
    sessionStorage.setItem(STORAGE_KEY, JSON.stringify(entries))
  } catch {
    // Ignore storage errors
  }
}

export function NavigationBreadcrumbProvider({ children }: { children: ReactNode }) {
  const [breadcrumbs, setBreadcrumbs] = useState<BreadcrumbEntry[]>([])

  // Load from sessionStorage on mount
  useEffect(() => {
    setBreadcrumbs(loadFromSession())
  }, [])

  const pushBreadcrumb = useCallback((label: string, href: string) => {
    setBreadcrumbs(prev => {
      // If the href already exists in the stack, pop back to it
      const existingIndex = prev.findIndex(entry => entry.href === href)
      if (existingIndex !== -1) {
        // Update the label at that position (entity name may differ) and truncate
        const updated = prev.slice(0, existingIndex + 1)
        updated[existingIndex] = { label, href }
        saveToSession(updated)
        return updated
      }

      // Push new entry, keeping max entries
      const next = [...prev, { label, href }].slice(-MAX_ENTRIES)
      saveToSession(next)
      return next
    })
  }, [])

  return (
    <NavigationBreadcrumbContext.Provider value={{ breadcrumbs, pushBreadcrumb }}>
      {children}
    </NavigationBreadcrumbContext.Provider>
  )
}

export function useNavigationBreadcrumbs() {
  const context = useContext(NavigationBreadcrumbContext)
  if (!context) {
    throw new Error('useNavigationBreadcrumbs must be used within NavigationBreadcrumbProvider')
  }
  return context
}
