'use client'

import Link from 'next/link'

/** Discover quick-links — acclimation row helping a newcomer find an entry. */
const DISCOVER_LINKS: ReadonlyArray<{ href: string; label: string }> = [
  { href: '/shows', label: 'Shows in any city' },
  { href: '/artists', label: 'Artists' },
  { href: '/radio', label: 'Freeform Radio' },
  { href: '/labels', label: 'Record Labels' },
  { href: '/graph', label: 'Graph Observatory' },
]

/**
 * The quiet "Discover:" row. Shared verbatim by the logged-out hero and the
 * signed-in home (PSY-2103) so the same five entry points survive the hero's
 * replacement — one list, not two that drift.
 *
 * `className` positions the row for its surface; the row's own typography and
 * separators are fixed.
 */
export function HomeDiscoverLinks({ className }: { className?: string }) {
  return (
    <nav aria-label="Discover" className={className}>
      <span className="font-medium text-muted-foreground">Discover:</span>
      {DISCOVER_LINKS.map((link, i) => (
        <span key={link.href} className="inline-flex items-center">
          <Link
            href={link.href}
            className="font-medium text-foreground transition-colors hover:text-primary hover:underline underline-offset-4"
          >
            {link.label}
          </Link>
          {i < DISCOVER_LINKS.length - 1 && (
            <span className="ml-1.5 text-muted-foreground/60" aria-hidden>
              ·
            </span>
          )}
        </span>
      ))}
    </nav>
  )
}
