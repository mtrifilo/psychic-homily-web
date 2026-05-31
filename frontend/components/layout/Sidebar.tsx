'use client'

import Link from 'next/link'
import { usePathname, useSearchParams } from 'next/navigation'
import {
  Calendar, Mic2, MapPin, Disc3, Tag, Tags, Tent, BookOpen, Headphones, Newspaper,
  Send, Library, LayoutList, MessageSquarePlus, UserCircle, Shield, PanelLeftClose, PanelLeft,
  ExternalLink, Globe, TrendingUp, Bell, HeartHandshake, Trophy, Radio, Music, Compass, ArrowLeft,
} from 'lucide-react'
import type { LucideIcon } from 'lucide-react'
import { cn } from '@/lib/utils'
import { useAuthContext } from '@/lib/context/AuthContext'
import {
  Tooltip, TooltipContent, TooltipProvider, TooltipTrigger,
} from '@/components/ui/tooltip'
import {
  adminNavGroups, adminTabHref, isAdminTabActive, ADMIN_BADGE_CLASS,
} from './adminNav'
import { useAdminNavCounts } from '@/lib/hooks/admin/useAdminNavCounts'

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
      { href: '/explore', label: 'Explore', icon: Compass },
      { href: '/releases', label: 'Releases', icon: Disc3 },
      { href: '/labels', label: 'Labels', icon: Tag },
      { href: '/tags', label: 'Tags', icon: Tags },
      { href: '/scenes', label: 'Scenes', icon: Globe },
      { href: '/collections', label: 'Collections', icon: LayoutList },
      { href: '/charts', label: 'Charts', icon: TrendingUp },
      { href: '/radio', label: 'Radio', icon: Radio },
    ],
  },
  {
    label: 'Community',
    items: [
      { href: '/contribute', label: 'Contribute', icon: HeartHandshake },
      { href: '/community/leaderboard', label: 'Leaderboard', icon: Trophy },
      { href: '/requests', label: 'Requests', icon: MessageSquarePlus },
      { href: '/blog', label: 'Blog', icon: BookOpen },
      { href: '/dj-sets', label: 'DJ Sets', icon: Headphones },
      { href: 'https://psychichomily.substack.com/', label: 'Substack', icon: Newspaper, external: true },
      { href: '/shows/submit', label: 'Submit a Show', icon: Music },
      { href: '/submissions', label: 'My Submissions', icon: Send },
    ],
  },
]

interface SidebarProps {
  collapsed: boolean
  onToggleCollapse: () => void
}

export function Sidebar({ collapsed, onToggleCollapse }: SidebarProps) {
  const pathname = usePathname()
  const searchParams = useSearchParams()
  const { user, isAuthenticated } = useAuthContext()

  // In the admin area the rail becomes context-aware (PSY-933): it swaps the
  // public Discover/Community groups for the grouped admin sections. Gated on
  // isAdmin so a non-admin mid-redirect at /admin still sees the public nav.
  const isAdmin = !!user?.is_admin
  const showAdminNav = isAdmin && pathname.startsWith('/admin')
  const tabParam = searchParams.get('tab')
  // Gated by showAdminNav: these admin-only count queries must not fire on
  // public routes or for non-admins (they'd 403 / waste requests).
  const counts = useAdminNavCounts({ enabled: showAdminNav })

  const isActive = (href: string) => {
    if (href === '/') return pathname === '/'
    return pathname === href || pathname.startsWith(href + '/')
  }

  // Unified link row used by both the public and admin nav. `badge`, when set,
  // renders a count pill (expanded) or a corner dot (collapsed).
  const renderNavLink = ({
    keyVal, href, label, icon: Icon, active, external = false,
    badge = null,
  }: {
    keyVal: string
    href: string
    label: string
    icon: LucideIcon
    active: boolean
    external?: boolean
    badge?: { count: number; className: string } | null
  }) => {
    const showBadge = badge != null && badge.count > 0
    const link = (
      <Link
        href={href}
        target={external ? '_blank' : undefined}
        rel={external ? 'noopener noreferrer' : undefined}
        className={cn(
          'flex items-center gap-3 rounded-md px-3 py-2 text-sm font-medium transition-colors',
          active
            ? 'bg-sidebar-accent text-sidebar-accent-foreground'
            : 'text-sidebar-foreground/70 hover:bg-sidebar-accent/50 hover:text-sidebar-accent-foreground',
          collapsed && 'relative justify-center px-2'
        )}
      >
        <Icon className="h-4 w-4 shrink-0" />
        {!collapsed && <span>{label}</span>}
        {!collapsed && external && (
          <ExternalLink className="ml-auto h-3 w-3 opacity-50" />
        )}
        {!collapsed && showBadge && (
          <span className={cn('ml-auto rounded-full px-2 py-0.5 text-xs font-medium text-white', badge!.className)}>
            {badge!.count}
          </span>
        )}
        {collapsed && showBadge && (
          <span className={cn('absolute right-1.5 top-1.5 h-2 w-2 rounded-full', badge!.className)} aria-hidden />
        )}
      </Link>
    )

    if (collapsed) {
      return (
        <Tooltip key={keyVal}>
          <TooltipTrigger asChild>{link}</TooltipTrigger>
          <TooltipContent side="right">
            {label}{showBadge ? ` (${badge!.count})` : ''}
          </TooltipContent>
        </Tooltip>
      )
    }
    return <div key={keyVal}>{link}</div>
  }

  const renderItem = (item: SidebarNavItem) =>
    renderNavLink({
      keyVal: item.href,
      href: item.href,
      label: item.label,
      icon: item.icon,
      active: !item.external && isActive(item.href),
      external: item.external,
    })

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
          'sticky top-[var(--topbar-height)] z-40 hidden h-[calc(100vh-var(--topbar-height))] shrink-0 flex-col overflow-hidden border-r border-sidebar-border bg-sidebar transition-[width] duration-200 md:flex',
          collapsed ? 'w-[var(--sidebar-width-collapsed)]' : 'w-[var(--sidebar-width)]'
        )}
      >
        <nav className="flex-1 space-y-6 overflow-y-auto px-2 py-4">
          {showAdminNav ? (
            <>
              <div>
                <div className="space-y-0.5">
                  {renderNavLink({
                    keyVal: 'back-to-site',
                    href: '/',
                    label: 'Back to site',
                    icon: ArrowLeft,
                    active: false,
                  })}
                </div>
                <div className={cn('mt-2 border-t border-sidebar-border', collapsed ? 'mx-2' : 'mx-3')} />
              </div>
              {adminNavGroups.map(group => (
                <div key={group.label}>
                  {renderGroupHeader(group.label)}
                  <div className="space-y-0.5">
                    {group.items.map(item =>
                      renderNavLink({
                        keyVal: item.tab,
                        href: adminTabHref(item.tab),
                        label: item.label,
                        icon: item.icon,
                        active: isAdminTabActive(item.tab, pathname, tabParam),
                        badge: item.badgeKey
                          ? { count: counts[item.badgeKey], className: ADMIN_BADGE_CLASS[item.badgeKey] }
                          : null,
                      })
                    )}
                  </div>
                </div>
              ))}
            </>
          ) : (
            <>
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
                    {renderItem({ href: '/library', label: 'Library', icon: Library })}
                    {renderItem({ href: '/settings/notification-filters', label: 'Notification Filters', icon: Bell })}
                    {renderItem({ href: '/profile', label: 'Profile', icon: UserCircle })}
                    {user?.is_admin && renderItem({ href: '/admin', label: 'Admin', icon: Shield })}
                  </div>
                </div>
              )}
            </>
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
