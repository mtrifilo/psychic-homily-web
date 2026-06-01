'use client'

import Link from 'next/link'
import { ExternalLink } from 'lucide-react'
import type { LucideIcon } from 'lucide-react'
import { cn } from '@/lib/utils'
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip'

export interface SidebarNavLinkProps {
  href: string
  label: string
  icon: LucideIcon
  active: boolean
  collapsed: boolean
  external?: boolean
  /** Count badge: a pill (expanded) or a corner dot (collapsed). */
  badge?: { count: number; className: string } | null
}

/**
 * The canonical sidebar link row — shared by the public Sidebar and the
 * (dynamically-loaded) admin rail so the active/badge/collapse/tooltip treatment
 * stays identical. Must render inside a `TooltipProvider` (the Sidebar's aside
 * provides one) for the collapsed-mode label tooltip.
 */
export function SidebarNavLink({
  href, label, icon: Icon, active, collapsed, external = false, badge = null,
}: SidebarNavLinkProps) {
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
      <Tooltip>
        <TooltipTrigger asChild>{link}</TooltipTrigger>
        <TooltipContent side="right">
          {label}{showBadge ? ` (${badge!.count})` : ''}
        </TooltipContent>
      </Tooltip>
    )
  }
  return link
}
