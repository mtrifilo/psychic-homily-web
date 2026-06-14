'use client'

import Link from 'next/link'
import { Search } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { openCommandPalette } from '@/lib/hooks/common/useCommandPalette'

/**
 * HomeHero (PSY-389) — the centered discovery-landing hero for the logged-out
 * homepage (Figma `491:29`).
 *
 * Pure mystique by design: an oblique What.cd call-back headline + evocative
 * two-line subtext, then a dominant search field and a "Find a show" primary
 * CTA. The concrete "what is this" work rests on the search placeholder, the
 * Discover quick-links row, and the sections below — the dominant actions stay
 * value-first (value-before-contribution). The only contribution prompt is the
 * quiet, secondary sign-up nudge at the bottom of the section.
 *
 * The search field reuses the existing command-palette mechanism
 * (`openCommandPalette`, same as the nav `SearchTrigger`); it is presented
 * wider here as the page's dominant action.
 */

/** Discover quick-links — acclimation row helping a newcomer find an entry. */
const DISCOVER_LINKS: ReadonlyArray<{ href: string; label: string }> = [
  { href: '/shows', label: 'Shows in any city' },
  { href: '/artists', label: 'Artists' },
  { href: '/radio', label: 'Freeform Radio' },
  { href: '/labels', label: 'Record Labels' },
  { href: '/explore', label: 'and more' },
]

export function HomeHero() {
  return (
    <section
      aria-labelledby="home-hero-heading"
      className="flex w-full flex-col items-center gap-4 pb-3.5 pt-6"
    >
      <h1
        id="home-hero-heading"
        className="text-center text-4xl font-bold tracking-tight text-foreground sm:text-5xl"
      >
        This is not a mirage.
      </h1>

      <div className="max-w-[720px] text-center text-base leading-relaxed text-muted-foreground sm:text-lg">
        <p>You&rsquo;ve stumbled upon a door where your imagination is the limit.</p>
        <p>Bring your true self without compromise, and you&rsquo;ll find your answers.</p>
      </div>

      {/* Dominant action: search + Find a show */}
      <div className="flex w-full max-w-[600px] flex-col items-stretch gap-2.5 pt-1.5 sm:flex-row sm:items-center sm:justify-center">
        <button
          type="button"
          onClick={() => openCommandPalette()}
          aria-label="Search shows, artists, labels"
          aria-keyshortcuts="Meta+K Control+K"
          className="flex h-[52px] flex-1 items-center gap-2.5 rounded-[10px] border border-input bg-muted px-4 text-left text-base text-muted-foreground transition-colors hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring/50 sm:w-[480px] sm:flex-none"
        >
          <Search className="size-5 shrink-0" aria-hidden />
          <span className="flex-1 truncate">Search shows, artists, labels…</span>
          <kbd className="pointer-events-none hidden shrink-0 items-center rounded border border-input bg-background px-1.5 py-0.5 font-mono text-[11px] text-muted-foreground sm:inline-flex">
            ⌘K
          </kbd>
        </button>
        <Button asChild size="lg" className="h-[52px] shrink-0 text-base">
          <Link href="/shows">Find a show</Link>
        </Button>
      </div>

      {/* Discover quick-links row */}
      <nav
        aria-label="Discover"
        className="flex flex-wrap items-center justify-center gap-x-1.5 gap-y-1 pt-0.5 text-sm"
      >
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

      {/* Quiet sign-up nudge */}
      <p className="pt-1 text-sm text-muted-foreground">
        <Link
          href="/auth"
          className="font-semibold text-primary transition-colors hover:underline underline-offset-4"
        >
          Sign up
        </Link>{' '}
        to contribute, and never miss a show again.
      </p>
    </section>
  )
}
