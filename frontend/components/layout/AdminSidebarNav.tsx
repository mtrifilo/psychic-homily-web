'use client'

import { usePathname, useSearchParams } from 'next/navigation'
import { ArrowLeft } from 'lucide-react'
import { cn } from '@/lib/utils'
import {
  adminNavGroups, adminTabHref, isAdminTabActive, ADMIN_BADGE_CLASS,
} from './adminNav'
import { useAdminNavCounts } from '@/lib/hooks/admin/useAdminNavCounts'
import { SidebarNavLink } from './SidebarNavLink'

/**
 * Desktop admin rail content (PSY-933). Dynamically imported by Sidebar and
 * mounted ONLY on the /admin tab-shell for admins, so this module — the admin
 * nav config + the 7 queue-count hooks — is a separate chunk that public pages
 * (e.g. /explore) never download. Because it only mounts under that gate, the
 * count queries are unconditionally enabled here.
 *
 * Default export so `next/dynamic` can load it.
 */
export default function AdminSidebarNav({ collapsed }: { collapsed: boolean }) {
  const pathname = usePathname()
  const tabParam = useSearchParams().get('tab')
  const counts = useAdminNavCounts({ enabled: true })

  return (
    <>
      <div>
        <div className="space-y-0.5">
          <SidebarNavLink
            href="/"
            label="Back to site"
            icon={ArrowLeft}
            active={false}
            collapsed={collapsed}
          />
        </div>
        <div className={cn('mt-2 border-t border-sidebar-border', collapsed ? 'mx-2' : 'mx-3')} />
      </div>
      {adminNavGroups.map(group => (
        <div key={group.label}>
          {!collapsed && (
            <p className="mb-2 px-3 text-xs font-semibold uppercase tracking-wider text-sidebar-foreground/50">
              {group.label}
            </p>
          )}
          <div className="space-y-0.5">
            {group.items.map(item => (
              <SidebarNavLink
                key={item.tab}
                href={adminTabHref(item.tab)}
                label={item.label}
                icon={item.icon}
                active={isAdminTabActive(item.tab, pathname, tabParam)}
                collapsed={collapsed}
                badge={item.badgeKey
                  ? { count: counts[item.badgeKey], className: ADMIN_BADGE_CLASS[item.badgeKey] }
                  : null}
              />
            ))}
          </div>
        </div>
      ))}
    </>
  )
}
