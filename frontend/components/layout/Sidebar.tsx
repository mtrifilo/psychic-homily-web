'use client'

import { usePathname } from 'next/navigation'
import {
  Calendar, Mic2, MapPin, Disc3, Tag, Tags, Tent, BookOpen, Headphones, Newspaper,
  Send, Library, LayoutList, MessageSquarePlus, UserCircle, Shield, PanelLeftClose, PanelLeft,
  Globe, Orbit, TrendingUp, Bell, HeartHandshake, Trophy, Radio, Music, Palette,
  ClipboardList,
} from 'lucide-react'
import type { LucideIcon } from 'lucide-react'
import { cn } from '@/lib/utils'
import { useAuthContext } from '@/lib/context/AuthContext'
import {
  Tooltip, TooltipContent, TooltipProvider, TooltipTrigger,
} from '@/components/ui/tooltip'
import { SidebarNavLink } from './SidebarNavLink'

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
      { href: '/graph', label: 'Graph', icon: Orbit },
      { href: '/releases', label: 'Releases', icon: Disc3 },
      { href: '/labels', label: 'Labels', icon: Tag },
      { href: '/tags', label: 'Tags', icon: Tags },
      { href: '/scenes', label: 'Scenes', icon: Globe },
      { href: '/atlas', label: 'Atlas', icon: Orbit },
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

  const renderItem = (item: SidebarNavItem) => (
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
          'sticky top-[var(--topbar-height)] z-40 hidden h-[calc(100vh-var(--topbar-height))] shrink-0 flex-col overflow-hidden border-r border-sidebar-border bg-sidebar transition-[width] duration-200 md:flex',
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
                {renderItem({ href: '/library', label: 'Library', icon: Library })}
                {renderItem({ href: '/contribute/submissions', label: 'Show Submissions', icon: ClipboardList })}
                {renderItem({ href: '/settings/notification-filters', label: 'Notification Filters', icon: Bell })}
                {renderItem({ href: '/settings/appearance', label: 'Appearance', icon: Palette })}
                {/* PSY-1486 / PSY-1025: Profile → public identity view, not the
                    /profile editor. Same profileHref pattern as UserMenu. */}
                {renderItem({
                  href: user?.username ? `/users/${user.username}` : '/users/me',
                  label: 'Profile',
                  icon: UserCircle,
                })}
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
