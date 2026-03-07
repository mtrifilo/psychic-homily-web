'use client'

import { useState, useEffect, useCallback } from 'react'
import { TopBar } from './TopBar'
import { Sidebar } from './Sidebar'
import { CommandPalette } from './CommandPalette'
import { openCommandPalette } from '@/lib/hooks/useCommandPalette'

const STORAGE_KEY = 'sidebar-collapsed'

export function SidebarLayout({ children }: { children: React.ReactNode }) {
  const [collapsed, setCollapsed] = useState(false)
  const [mobileOpen, setMobileOpen] = useState(false)

  useEffect(() => {
    const stored = localStorage.getItem(STORAGE_KEY)
    if (stored === 'true') setCollapsed(true)
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
