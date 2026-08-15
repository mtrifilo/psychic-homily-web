'use client'

import { usePathname } from 'next/navigation'
import { PanelLeftClose, PanelLeft } from 'lucide-react'
import { cn } from '@/lib/utils'
import { useAuthContext } from '@/lib/context/AuthContext'
import {
  Tooltip, TooltipContent, TooltipProvider, TooltipTrigger,
} from '@/components/ui/tooltip'
import { SidebarNavLink } from './SidebarNavLink'
import { sidebarGroups, sidebarAccountItems } from './nav/navData'
import type { NavDestination } from './nav/navData'

interface SidebarProps {
  collapsed: boolean
  onToggleCollapse: () => void
}

export function Sidebar({ collapsed, onToggleCollapse }: SidebarProps) {
  const pathname = usePathname()
  const { user, isAuthenticated } = useAuthContext()

  // The global sidebar is purely the public Discover/Community nav. The admin
  // area's own rail is owned by AdminSidebar (app/admin/layout.tsx, PSY-1114) —
  // and in side-nav mode SideNavShell suppresses this sidebar under /admin — so
  // the PSY-933 context-swap that used to render admin nav here is retired
  // (PSY-1116) to avoid a double rail.
  const isActive = (href: string) => {
    if (href === '/') return pathname === '/'
    return pathname === href || pathname.startsWith(href + '/')
  }

  const renderItem = (item: NavDestination) => (
    <SidebarNavLink
      key={item.href}
      href={item.href}
      label={item.label}
      icon={item.icon}
      active={!item.external && isActive(item.href)}
      collapsed={collapsed}
      external={item.external}
    />
  )

  const renderGroupHeader = (label: string) =>
    !collapsed && (
      <p className="mb-2 px-3 text-xs font-semibold uppercase tracking-wider text-sidebar-foreground/50">
        {label}
      </p>
    )

  return (
    <TooltipProvider delayDuration={0}>
      <aside
        className={cn(
          'sticky top-[var(--topbar-height)] z-40 hidden h-[calc(100vh-var(--topbar-height)-var(--bottom-tab-bar-height)-env(safe-area-inset-bottom))] shrink-0 xl:h-[calc(100vh-var(--topbar-height))] flex-col overflow-hidden border-r border-sidebar-border bg-sidebar transition-[width] duration-200 md:flex',
          collapsed ? 'w-[var(--sidebar-width-collapsed)]' : 'w-[var(--sidebar-width)]'
        )}
      >
        <nav className="flex-1 space-y-6 overflow-y-auto px-2 py-4">
          {sidebarGroups.map(group => (
            <div key={group.label}>
              {renderGroupHeader(group.label)}
              <div className="space-y-0.5">
                {group.items.map(renderItem)}
              </div>
            </div>
          ))}

          {isAuthenticated && (
            <div>
              <div className={cn('mb-2 border-t border-sidebar-border', collapsed ? 'mx-2' : 'mx-3')} />
              <div className="space-y-0.5">
                {sidebarAccountItems(user).map(renderItem)}
              </div>
            </div>
          )}
        </nav>

        <div className="border-t border-sidebar-border p-2">
          <Tooltip>
            <TooltipTrigger asChild>
              <button
                onClick={onToggleCollapse}
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
