'use client'

import Link from 'next/link'

export interface BreadcrumbEntry {
  label: string
  href: string
}

interface BreadcrumbProps {
  /** Parent category breadcrumb (e.g., { href: '/artists', label: 'Artists' }) */
  fallback: BreadcrumbEntry
  /** Label for the current page (last crumb, rendered as plain text) */
  currentPage: string
}

/**
 * Renders a simple two-level breadcrumb: Category > Entity Name
 *
 * Examples:
 *   Artists > Macie Stewart
 *   Shows > Jeff Tweedy at Van Buren
 *   Releases > Satori
 */
export function Breadcrumb({ fallback, currentPage }: BreadcrumbProps) {
  return (
    <nav aria-label="Breadcrumb" className="mb-6">
      <ol className="flex items-center gap-1 text-sm text-muted-foreground">
        <li className="flex items-center gap-1">
          <Link
            href={fallback.href}
            className="hover:text-foreground transition-colors truncate max-w-[200px]"
            title={fallback.label}
          >
            {fallback.label}
          </Link>
        </li>
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
