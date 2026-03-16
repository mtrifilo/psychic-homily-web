'use client'

import Link from 'next/link'
import { useNavigationBreadcrumbs, type BreadcrumbEntry } from '@/lib/context/NavigationBreadcrumbContext'

interface BreadcrumbProps {
  /** Fallback breadcrumb shown when there is no navigation history (direct landing).
   *  For example: { href: '/artists', label: 'Artists' } */
  fallback: BreadcrumbEntry
  /** Label for the current page (last crumb, rendered as plain text) */
  currentPage: string
}

/**
 * Renders a breadcrumb trail reflecting the user's navigation path.
 *
 * When the user has navigated across entity pages, it shows:
 *   Shows > Jeff Tweedy at Van Buren > Macie Stewart
 *
 * When landed directly (no history), it shows a sensible fallback:
 *   Artists > Macie Stewart
 */
export function Breadcrumb({ fallback, currentPage }: BreadcrumbProps) {
  const { breadcrumbs } = useNavigationBreadcrumbs()

  // Build the trail: either navigation history or the fallback
  // Exclude the current page from the clickable trail (it will be appended as plain text)
  const trail: BreadcrumbEntry[] = breadcrumbs.length > 0
    ? breadcrumbs.filter(b => b.label !== currentPage || b.href !== fallback.href)
    : [fallback]

  return (
    <nav aria-label="Breadcrumb" className="mb-6">
      <ol className="flex items-center gap-1 text-sm text-muted-foreground">
        {trail.map((crumb, index) => (
          <li key={crumb.href + index} className="flex items-center gap-1">
            {index > 0 && (
              <span className="text-muted-foreground/50" aria-hidden="true">&rsaquo;</span>
            )}
            <Link
              href={crumb.href}
              className="hover:text-foreground transition-colors truncate max-w-[200px]"
              title={crumb.label}
            >
              {crumb.label}
            </Link>
          </li>
        ))}
        <li className="flex items-center gap-1">
          <span className="text-muted-foreground/50" aria-hidden="true">&rsaquo;</span>
          <span className="text-foreground font-medium truncate max-w-[200px]" title={currentPage}>
            {currentPage}
          </span>
        </li>
      </ol>
    </nav>
  )
}
