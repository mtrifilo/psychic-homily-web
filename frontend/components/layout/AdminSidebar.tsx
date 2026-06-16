'use client'

import { useCallback } from 'react'
import dynamic from 'next/dynamic'
import { PanelLeftClose, PanelLeft } from 'lucide-react'
import { cn } from '@/lib/utils'
import {
  Tooltip, TooltipContent, TooltipProvider, TooltipTrigger,
} from '@/components/ui/tooltip'
import { useLocalStorageEnum } from '@/lib/hooks/common/useLocalStorageEnum'

// The admin rail content (config + the queue-count hooks) loads client-side
// only — it reads `useSearchParams()` for the active `?tab=`, and `ssr: false`
// keeps that out of the prerendered shell (mirrors how the retired global
// Sidebar mounted it; avoids a useSearchParams Suspense requirement under
// cacheComponents). This component only ever mounts under /admin, so the chunk
// is already admin-scoped.
const AdminSidebarNav = dynamic(() => import('./AdminSidebarNav'), { ssr: false })

// PSY-1114: the always-on admin left rail. Mounted by app/admin/layout.tsx so
// the admin area keeps its dense 18-section nav on desktop regardless of the
// global nav-style preference (the upcoming top-bar vs side-nav toggle,
// PSY-1116). It is the sole owner of admin desktop nav — the prior admin nav
// rode inside the global Sidebar's context-swap (PSY-933), which was orphaned
// when PSY-1013 retired that Sidebar from the shell.
//
// The <aside> chrome + collapse toggle mirror the retired global Sidebar
// (Sidebar.tsx) so the rail looks and behaves identically; the nav body is the
// existing AdminSidebarNav. Hidden below `md` — mobile keeps the hamburger
// drawer (MobileNav + AdminDrawerNav), unaffected by this rail.
//
// Module-level `as const` tuple per useLocalStorageEnum's contract: a stable
// `allowed` reference keeps the snapshot getter from churning each render.
const COLLAPSE_STATES = ['expanded', 'collapsed'] as const

export function AdminSidebar() {
  // SSR-safe persisted collapse state via the shared hook (useSyncExternalStore
  // hydration + cross-tab sync) — the same primitive useDensity and the
  // collections/contributions surfaces use, rather than a hand-rolled
  // localStorage effect.
  const [collapseState, setCollapseState] = useLocalStorageEnum(
    'admin-sidebar-collapsed',
    'expanded',
    COLLAPSE_STATES
  )
  const collapsed = collapseState === 'collapsed'

  const toggleCollapse = useCallback(
    () => setCollapseState(collapsed ? 'expanded' : 'collapsed'),
    [collapsed, setCollapseState]
  )

  return (
    <TooltipProvider delayDuration={0}>
      <aside
        className={cn(
          'sticky top-[var(--topbar-height)] z-40 hidden h-[calc(100vh-var(--topbar-height))] shrink-0 flex-col overflow-hidden border-r border-sidebar-border bg-sidebar transition-[width] duration-200 md:flex',
          collapsed ? 'w-[var(--sidebar-width-collapsed)]' : 'w-[var(--sidebar-width)]'
        )}
        aria-label="Admin navigation"
      >
        <nav className="flex-1 space-y-6 overflow-y-auto px-2 py-4">
          <AdminSidebarNav collapsed={collapsed} />
        </nav>

        <div className="border-t border-sidebar-border p-2">
          <Tooltip>
            <TooltipTrigger asChild>
              <button
                onClick={toggleCollapse}
                className={cn(
                  'flex w-full items-center gap-3 rounded-md px-3 py-2 text-sm font-medium text-sidebar-foreground/70 transition-colors hover:bg-sidebar-accent/50 hover:text-sidebar-accent-foreground',
                  collapsed && 'justify-center px-2'
                )}
                aria-label={collapsed ? 'Expand sidebar' : 'Collapse sidebar'}
              >
                {collapsed ? (
                  <PanelLeft className="h-4 w-4" />
                ) : (
                  <>
                    <PanelLeftClose className="h-4 w-4" />
                    <span>Collapse</span>
                  </>
                )}
              </button>
            </TooltipTrigger>
            {collapsed && <TooltipContent side="right">Expand sidebar</TooltipContent>}
          </Tooltip>
        </div>
      </aside>
    </TooltipProvider>
  )
}
