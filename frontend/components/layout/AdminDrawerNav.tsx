'use client'

import Link from 'next/link'
import { usePathname, useSearchParams } from 'next/navigation'
import { ArrowLeft } from 'lucide-react'
import { cn } from '@/lib/utils'
import {
  adminNavGroups, adminTabHref, isAdminTabActive, ADMIN_BADGE_CLASS,
} from './adminNav'
import { useAdminNavCounts } from '@/lib/hooks/admin/useAdminNavCounts'

/**
 * Mobile drawer admin nav content (PSY-933). Dynamically imported by TopBar and
 * mounted ONLY on the /admin tab-shell for admins (the desktop Sidebar is
 * hidden < md, so this is the sole admin nav on mobile). Same chunk-splitting
 * rationale as AdminSidebarNav: keeps admin-only code out of the public bundle.
 *
 * `onNavigate` closes the drawer on selection. Default export for `next/dynamic`.
 */
export default function AdminDrawerNav({ onNavigate }: { onNavigate: () => void }) {
  const pathname = usePathname()
  const tabParam = useSearchParams().get('tab')
  const counts = useAdminNavCounts({ enabled: true })

  return (
    <>
      <Link
        href="/"
        onClick={onNavigate}
        className="mb-2 flex items-center gap-3 rounded-md px-3 py-2.5 text-sm font-medium text-foreground/70 transition-colors hover:bg-accent/50 hover:text-accent-foreground"
      >
        <ArrowLeft className="h-4 w-4" />
        <span>Back to site</span>
      </Link>
      {adminNavGroups.map(group => (
        <div key={group.label} className="mb-4">
          <p className="mb-2 px-3 text-xs font-semibold uppercase tracking-wider text-muted-foreground/50">
            {group.label}
          </p>
          {group.items.map(item => {
            const Icon = item.icon
            const active = isAdminTabActive(item.tab, pathname, tabParam)
            const count = item.badgeKey ? counts[item.badgeKey] : 0
            return (
              <Link
                key={item.tab}
                href={adminTabHref(item.tab)}
                onClick={onNavigate}
                className={cn(
                  'flex items-center gap-3 rounded-md px-3 py-2.5 text-sm font-medium transition-colors',
                  active
                    ? 'bg-accent text-accent-foreground'
                    : 'text-foreground/70 hover:bg-accent/50 hover:text-accent-foreground'
                )}
              >
                <Icon className="h-4 w-4" />
                <span>{item.label}</span>
                {item.badgeKey && count > 0 && (
                  <span className={cn('ml-auto rounded-full px-2 py-0.5 text-xs font-medium text-white', ADMIN_BADGE_CLASS[item.badgeKey])}>
                    {count}
                  </span>
                )}
              </Link>
            )
          })}
        </div>
      ))}
    </>
  )
}
