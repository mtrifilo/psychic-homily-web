'use client'

import Link from 'next/link'
import { useCookieConsent } from '@/lib/context/CookieConsentContext'

/**
 * Footer (PSY-389) — the editorial site footer from the Navigation & Discovery
 * Redesign (Figma `491:29`). Rendered globally in `app/layout.tsx`, so it
 * applies to every page, not just home.
 *
 * Brand col + four link columns (Discover / Browse / Community / About) + a
 * bottom sign-off row. The legacy footer's legal affordances are preserved in
 * the About column (Privacy, Terms, Contact) plus the Cookie Preferences
 * trigger — cookie consent is a button (it opens the preferences dialog), never
 * a navigation link.
 *
 * Link destinations are real routes only; the Figma "About / FAQ / Vision"
 * labels reference pages that don't exist yet, so the About column ships the
 * meta/legal links we actually have (PSY-389 scope-adjacent note).
 */

interface FooterColumn {
  heading: string
  links: ReadonlyArray<{ href: string; label: string; external?: boolean }>
}

const FOOTER_COLUMNS: ReadonlyArray<FooterColumn> = [
  {
    heading: 'Discover',
    links: [
      { href: '/shows', label: 'Shows' },
      { href: '/artists', label: 'Artists' },
      { href: '/venues', label: 'Venues' },
      { href: '/releases', label: 'Releases' },
      { href: '/labels', label: 'Labels' },
      { href: '/festivals', label: 'Festivals' },
    ],
  },
  {
    heading: 'Browse',
    links: [
      { href: '/explore', label: 'Explore' },
      { href: '/collections', label: 'Collections' },
      { href: '/charts', label: 'Charts' },
      { href: '/tags', label: 'Tags' },
      { href: '/scenes', label: 'Scenes' },
      { href: '/radio', label: 'Radio' },
    ],
  },
  {
    heading: 'Community',
    links: [
      { href: '/contribute', label: 'Contribute' },
      { href: '/requests', label: 'Requests' },
      { href: '/community/leaderboard', label: 'Leaderboard' },
      { href: '/shows/submit', label: 'Submit a show' },
    ],
  },
]

export default function Footer() {
  const { openPreferences } = useCookieConsent()
  const currentYear = new Date().getFullYear()

  return (
    <footer className="mt-auto w-full border-t border-border/60">
      <div className="mx-auto max-w-7xl px-4 pb-10 pt-11 md:px-8">
        <div className="flex flex-col gap-10 md:flex-row md:items-start">
          {/* Brand column */}
          <div className="flex flex-col gap-2.5 md:w-80">
            <div className="flex items-center gap-2.5">
              <span
                className="size-5 shrink-0 rounded-full bg-primary"
                aria-hidden
              />
              <span className="text-sm font-semibold tracking-wide text-foreground">
                PSYCHIC HOMILY
              </span>
            </div>
            <p className="text-[13px] text-muted-foreground">
              Live shows + the knowledge graph. The underground, mapped — link by
              link.
            </p>
          </div>

          {/* Link columns */}
          {FOOTER_COLUMNS.map(column => (
            <nav
              key={column.heading}
              aria-label={column.heading}
              className="flex flex-1 flex-col gap-2.5"
            >
              <p className="font-mono text-[10px] font-bold tracking-[0.12em] text-muted-foreground">
                {column.heading.toUpperCase()}
              </p>
              {column.links.map(link => (
                <Link
                  key={link.href}
                  href={link.href}
                  className="text-[13px] font-medium text-muted-foreground transition-colors hover:text-foreground"
                >
                  {link.label}
                </Link>
              ))}
            </nav>
          ))}

          {/* About / meta column — real routes only (see header note). */}
          <nav aria-label="About" className="flex flex-1 flex-col gap-2.5">
            <p className="font-mono text-[10px] font-bold tracking-[0.12em] text-muted-foreground">
              ABOUT
            </p>
            <Link
              href="/privacy"
              className="text-[13px] font-medium text-muted-foreground transition-colors hover:text-foreground"
            >
              Privacy Policy
            </Link>
            <Link
              href="/terms"
              className="text-[13px] font-medium text-muted-foreground transition-colors hover:text-foreground"
            >
              Terms of Service
            </Link>
            <Link
              href="mailto:hello@psychichomily.com"
              className="text-[13px] font-medium text-muted-foreground transition-colors hover:text-foreground"
            >
              Contact
            </Link>
            <button
              type="button"
              onClick={openPreferences}
              className="text-left text-[13px] font-medium text-muted-foreground transition-colors hover:text-foreground"
            >
              Cookie Preferences
            </button>
            <a
              href="https://psychichomily.substack.com/"
              target="_blank"
              rel="noopener noreferrer"
              className="text-[13px] font-medium text-muted-foreground transition-colors hover:text-foreground"
            >
              Substack ↗
            </a>
          </nav>
        </div>

        <div className="mt-8 flex flex-col gap-2 text-xs text-muted-foreground sm:flex-row sm:items-center sm:justify-between">
          <p>&copy; {currentYear} Psychic Homily</p>
          <p>Made by the scene, for the scene.</p>
        </div>
      </div>
    </footer>
  )
}
