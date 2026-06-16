'use client'

import { useCallback } from 'react'
import { usePathname } from 'next/navigation'
import { Sidebar } from './Sidebar'
import { useLocalStorageEnum } from '@/lib/hooks/common/useLocalStorageEnum'

// Module-level `as const` tuple per useLocalStorageEnum's contract (stable
// `allowed` reference). Distinct key from the admin rail's
// `admin-sidebar-collapsed` so the two collapse states are independent.
const COLLAPSE_STATES = ['expanded', 'collapsed'] as const

// PSY-1116: the side-nav composition. Rendered by AppShell only when the
// nav-mode cookie resolves to 'side' — it revives the pre-PSY-1013
// SidebarLayout shape (the global left Sidebar to the left of the content
// column). Client-side because the Sidebar's collapse state is local + browser
// persisted; `children` is the server-rendered page subtree passed through as a
// prop.
export function SideNavShell({ children }: { children: React.ReactNode }) {
  const pathname = usePathname()
  const [collapseState, setCollapseState] = useLocalStorageEnum(
    'sidebar-collapsed',
    'expanded',
    COLLAPSE_STATES
  )
  const collapsed = collapseState === 'collapsed'
  const toggleCollapse = useCallback(
    () => setCollapseState(collapsed ? 'expanded' : 'collapsed'),
    [collapsed, setCollapseState]
  )

  // The admin area renders its OWN always-on rail (AdminSidebar, PSY-1114), so
  // suppress the global public sidebar under /admin — otherwise side-nav mode
  // would stack two left rails. usePathname() strips the ?tab= query, so this
  // still matches the /admin tab-shell and its sub-routes.
  const showGlobalSidebar = !pathname.startsWith('/admin')

  return (
    <div className="flex flex-1">
      {showGlobalSidebar && (
        <Sidebar collapsed={collapsed} onToggleCollapse={toggleCollapse} />
      )}
      <div className="flex min-w-0 flex-1 flex-col">{children}</div>
    </div>
  )
}
