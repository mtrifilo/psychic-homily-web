'use client'

import Link from 'next/link'
import { usePathname } from 'next/navigation'
import {
  Calendar, CalendarCheck, Mic2, MapPin, Disc3, Tag, Tags, Tent, BookOpen, Headphones, Newspaper,
  Send, Library, LayoutList, MessageSquarePlus, Settings, Shield, PanelLeftClose, PanelLeft,
  ExternalLink, Globe, UserCheck,
} from 'lucide-react'
import type { LucideIcon } from 'lucide-react'
import { cn } from '@/lib/utils'
import { useAuthContext } from '@/lib/context/AuthContext'
import {
  Tooltip, TooltipContent, TooltipProvider, TooltipTrigger,
} from '@/components/ui/tooltip'

export interface SidebarNavItem {
  href: string
  label: string
  icon: LucideIcon
  external?: boolean
}

export interface SidebarGroup {
  label: string
  items: SidebarNavItem[]
}

export const sidebarGroups: SidebarGroup[] = [
  {
    label: 'Discover',
    items: [
      { href: '/shows', label: 'Shows', icon: Calendar },
      { href: '/festivals', label: 'Festivals', icon: Tent },
      { href: '/artists', label: 'Artists', icon: Mic2 },
      { href: '/venues', label: 'Venues', icon: MapPin },
      { href: '/releases', label: 'Releases', icon: Disc3 },
      { href: '/labels', label: 'Labels', icon: Tag },
      { href: '/tags', label: 'Tags', icon: Tags },
      { href: '/scenes', label: 'Scenes', icon: Globe },
      { href: '/collections', label: 'Collections', icon: LayoutList },
    ],
  },
  {
    label: 'Community',
    items: [
      { href: '/requests', label: 'Requests', icon: MessageSquarePlus },
      { href: '/blog', label: 'Blog', icon: BookOpen },
      { href: '/dj-sets', label: 'DJ Sets', icon: Headphones },
      { href: 'https://psychichomily.substack.com/', label: 'Substack', icon: Newspaper, external: true },
      { href: '/submissions', label: 'Submissions', icon: Send },
    ],
  },
]

interface SidebarProps {
  collapsed: boolean
  onToggleCollapse: () => void
}

export function Sidebar({ collapsed, onToggleCollapse }: SidebarProps) {
  const pathname = usePathname()
  const { user, isAuthenticated } = useAuthContext()

  const isActive = (href: string) => {
    if (href === '/') return pathname === '/'
    return pathname === href || pathname.startsWith(href + '/')
  }

  const renderItem = (item: SidebarNavItem) => {
    const Icon = item.icon
    const active = !item.external && isActive(item.href)

    const link = (
      <Link
        href={item.href}
        target={item.external ? '_blank' : undefined}
        rel={item.external ? 'noopener noreferrer' : undefined}
        className={cn(
          'flex items-center gap-3 rounded-md px-3 py-2 text-sm font-medium transition-colors',
          active
            ? 'bg-sidebar-accent text-sidebar-accent-foreground'
            : 'text-sidebar-foreground/70 hover:bg-sidebar-accent/50 hover:text-sidebar-accent-foreground',
          collapsed && 'justify-center px-2'
        )}
      >
        <Icon className="h-4 w-4 shrink-0" />
        {!collapsed && <span>{item.label}</span>}
        {!collapsed && item.external && (
          <ExternalLink className="ml-auto h-3 w-3 opacity-50" />
        )}
      </Link>
    )

    if (collapsed) {
      return (
        <Tooltip key={item.href}>
          <TooltipTrigger asChild>{link}</TooltipTrigger>
          <TooltipContent side="right">{item.label}</TooltipContent>
        </Tooltip>
      )
    }

    return <div key={item.href}>{link}</div>
  }

  return (
    <TooltipProvider delayDuration={0}>
      <aside
        className={cn(
          'sticky top-[var(--topbar-height)] z-40 hidden h-[calc(100vh-var(--topbar-height))] shrink-0 flex-col overflow-hidden border-r border-sidebar-border bg-sidebar transition-[width] duration-200 md:flex',
          collapsed ? 'w-[var(--sidebar-width-collapsed)]' : 'w-[var(--sidebar-width)]'
        )}
      >
        <nav className="flex-1 space-y-6 overflow-y-auto px-2 py-4">
          {sidebarGroups.map(group => (
            <div key={group.label}>
              {!collapsed && (
                <p className="mb-2 px-3 text-xs font-semibold uppercase tracking-wider text-sidebar-foreground/50">
                  {group.label}
                </p>
              )}
              <div className="space-y-0.5">
                {group.items.map(renderItem)}
              </div>
            </div>
          ))}

          {isAuthenticated && (
            <div>
              <div className={cn('mb-2 border-t border-sidebar-border', collapsed ? 'mx-2' : 'mx-3')} />
              <div className="space-y-0.5">
                {renderItem({ href: '/my-shows', label: 'My Shows', icon: CalendarCheck })}
                {renderItem({ href: '/following', label: 'Following', icon: UserCheck })}
                {renderItem({ href: '/collection', label: 'Collection', icon: Library })}
                {renderItem({ href: '/profile', label: 'Settings', icon: Settings })}
                {user?.is_admin && renderItem({ href: '/admin', label: 'Admin', icon: Shield })}
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
