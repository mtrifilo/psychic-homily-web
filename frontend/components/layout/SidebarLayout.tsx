'use client'

import { useState, useEffect, useCallback } from 'react'
import { TopBar } from './TopBar'
import { Sidebar } from './Sidebar'
import { CommandPalette } from './CommandPalette'
import { openCommandPalette } from '@/lib/hooks/common/useCommandPalette'

const STORAGE_KEY = 'sidebar-collapsed'

export function SidebarLayout({ children }: { children: React.ReactNode }) {
  const [collapsed, setCollapsed] = useState(false)
  const [mobileOpen, setMobileOpen] = useState(false)

  // Hydrate the collapsed preference from localStorage AFTER mount. The first
  // render is intentionally the default (expanded) on both server and client
  // so hydration matches; the stored value (client-only) is applied a beat
  // later. React 19.2: defer the setState to a microtask so it lands after the
  // effect returns instead of synchronously in the effect body (which trips
  // set-state-in-effect / cascading render). A lazy useState initializer is
  // unsafe here — it would read localStorage during SSR and mismatch hydration.
  useEffect(() => {
    const stored = localStorage.getItem(STORAGE_KEY)
    if (stored !== 'true') return
    let cancelled = false
    Promise.resolve().then(() => {
      if (!cancelled) setCollapsed(true)
    })
    return () => {
      cancelled = true
    }
  }, [])

  const toggleCollapse = useCallback(() => {
    setCollapsed(prev => {
      const next = !prev
      localStorage.setItem(STORAGE_KEY, String(next))
      return next
    })
  }, [])

  const handleSearchClick = useCallback(() => {
    openCommandPalette()
  }, [])

  return (
    <div className="flex min-h-screen flex-col">
      <TopBar
        mobileOpen={mobileOpen}
        onMobileOpenChange={setMobileOpen}
        onSearchClick={handleSearchClick}
      />
      <div className="flex flex-1">
        <Sidebar collapsed={collapsed} onToggleCollapse={toggleCollapse} />
        <div className="flex min-w-0 flex-1 flex-col">
          {children}
        </div>
      </div>
      <CommandPalette />
    </div>
  )
}
