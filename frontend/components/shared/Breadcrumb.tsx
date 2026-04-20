'use client'

import Link from 'next/link'

export interface BreadcrumbEntry {
  label: string
  href: string
}

interface BreadcrumbProps {
  /** Parent category breadcrumb (e.g., { href: '/artists', label: 'Artists' }) */
  fallback: BreadcrumbEntry
  /**
   * Optional intermediate crumbs rendered between the fallback and the
   * current page. Used for nested hierarchies like
   * `Tags > post-punk > shoegaze` where `post-punk` is the genre tag's
   * parent. Each entry renders as a link. Order is closest-to-root first
   * (so an ancestor chain should be passed root→leaf, not leaf→root).
   * Omit or pass an empty array to fall back to the original two-level
   * `Category > Entity` rendering.
   */
  intermediate?: BreadcrumbEntry[]
  /** Label for the current page (last crumb, rendered as plain text) */
  currentPage: string
}

/**
 * Renders a breadcrumb of arbitrary depth: Category > [Ancestor...] > Entity Name
 *
 * Examples:
 *   Artists > Macie Stewart
 *   Shows > Jeff Tweedy at Van Buren
 *   Tags > post-punk > shoegaze
 */
export function Breadcrumb({ fallback, intermediate, currentPage }: BreadcrumbProps) {
  const middle = intermediate ?? []
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
        {middle.map((entry) => (
          <li key={`${entry.href}-${entry.label}`} className="flex items-center gap-1">
            <span className="text-muted-foreground/50" aria-hidden="true">&rsaquo;</span>
            <Link
              href={entry.href}
              className="hover:text-foreground transition-colors truncate max-w-[200px]"
              title={entry.label}
            >
              {entry.label}
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
