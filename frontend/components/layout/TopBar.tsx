'use client'

import Image from 'next/image'
import Link from 'next/link'
import { Moon, Sun } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { useThemeToggle } from './mode-toggle'
import { PrimaryNav } from './nav/PrimaryNav'
import { SearchTrigger } from './nav/SearchTrigger'
import { UserMenu } from './nav/UserMenu'

// Static SVG filter for the logo glitch effect. Hoisted to module scope so the
// (animated, non-trivial) filter tree is created once rather than rebuilt on
// every TopBar re-render (theme toggle, route change, auth change).
const glitchFilter = (
  <svg width="0" height="0" className="absolute">
    <defs>
      <filter id="glitch">
        <feTurbulence type="fractalNoise" baseFrequency="0.003 0.002" numOctaves="1" seed="1" result="noise1">
          <animate attributeName="seed" dur="2s" values="1;2;3;4;5;6;7;8;1" repeatCount="indefinite" />
        </feTurbulence>
        <feDisplacementMap in="SourceGraphic" in2="noise1" scale="3" result="base" />
        <feTurbulence type="fractalNoise" baseFrequency="0.09" numOctaves="1" seed="1" result="noise2">
          <animate attributeName="seed" dur="0.3s" values="1;5;1" repeatCount="indefinite" calcMode="discrete" />
        </feTurbulence>
        <feDisplacementMap in="base" in2="noise2" scale="1" />
      </filter>
    </defs>
  </svg>
)

// The global top bar (PSY-1013) — the primary navigation chrome that replaces
// the retired left sidebar. It is a thin shell that composes:
//   • brand (clickable logo, left-aligned) + explicit labelled PrimaryNav
//   • the dominant search field (→ CommandPalette); below `sm` it condenses to
//     an icon-only tap target (PSY-1020 — search stays reachable on phones)
//   • a bare sun/moon theme toggle + the account cluster / login link
// It is public chrome only: the mobile admin drawer that used to sit in the
// left cluster moved into app/admin/layout.tsx (PSY-1817), and the public
// hamburger sheet was retired by PSY-1020's bottom tab bar.
// The Browse / Contribute menus, the authenticated bar, and the palette re-skin
// are each elaborated by their own follow-up tickets (Radio became a plain
// /radio link in PSY-1057); this file just assembles the seams.
//
// `variant` (PSY-1116): in side-nav mode the global nav lives in the left
// Sidebar, so the top bar drops its PrimaryNav and becomes a slim brand +
// search + theme + account bar. 'full' (default) is the top-nav-mode chrome.
export function TopBar({ variant = 'full' }: { variant?: 'full' | 'slim' } = {}) {
  const { toggle: toggleTheme } = useThemeToggle()

  return (
    <>
      {glitchFilter}

      <header className="sticky top-0 z-50 flex h-[var(--topbar-height)] w-full items-center justify-between border-b border-border/50 bg-background/95 px-4 backdrop-blur-sm supports-[backdrop-filter]:bg-background/60 sm:px-6">
        {/* Left: brand + primary nav.
            `shrink-0`: the brand and the nav labels are the bar's fixed frame.
            Left to itself the group would absorb part of any width shortfall by
            wrapping the wordmark onto two lines (measured at 1280px before
            PSY-1638) — and it still would not free enough room. The search
            field on the right is the designated slack absorber instead. */}
        <div className="flex shrink-0 items-center gap-3 xl:gap-[30px]">
          <Link href="/" aria-label="Psychic Homily — home" className="flex items-center gap-2 transition-opacity hover:opacity-80">
            <div className="relative size-[36px] overflow-hidden rounded-md">
              {/* Raster, not vector, on purpose (PSY-1771): the predecessor
                  `/PsychicHomilyLogov2.svg` put 166 KB on the wire of every
                  route because `next/image` cannot optimize SVG. Same artwork
                  at 144x144, i.e. 4x the 36px display size, so it holds up
                  to a 4x DPR. Regenerate from the vector source in git
                  history.

                  `unoptimized` spends ~3 KB per fetch to buy cacheability;
                  next.config.ts has the measurement and pins the header. It
                  also means one file serves every client with no `Accept`
                  negotiation, which is why this is PNG and not the smaller
                  WebP. */}
              <Image
                src="/psychic-homily-logo-v1.png"
                alt=""
                width={36}
                height={36}
                priority
                unoptimized
                className="rounded-md"
                style={{ filter: 'url(#glitch)' }}
              />
            </div>
            <span className="hidden text-[15px] font-semibold uppercase tracking-[0.04em] text-foreground sm:inline">
              Psychic Homily
            </span>
          </Link>
          {variant === 'full' && <PrimaryNav />}
        </div>

        {/* Right: search + theme + account.
            PSY-1638. Every control in here is `shrink-0` (the Button base
            variant sets it), so the row used to be rigid: at exactly `xl` the
            8-item PrimaryNav appears AND the search jumps 220→320px in the same
            breakpoint, putting 1277px of content in a 1232px box at a 1280px
            viewport. `justify-between` lays out from the start when free space
            is negative, so the surplus fell off the right edge and took the
            avatar with it — its centre was not even hittable.

            The search field is the one element here with an arbitrary width, so
            it is the one that gives: the two `min-w-0`s below let the flex
            algorithm take the shortfall out of the only item that still has a
            shrink factor. It keeps its full 220/320px whenever the row fits and
            narrows (label truncates, ⌘K hint stays) when it does not — so
            adding a nav item now costs search width rather than pushing the
            account cluster off-screen again. */}
        <div className="flex min-w-0 items-center gap-[14px]">
          {/* ONE search trigger at every width (PSY-1818): the box is 36px —
              an icon tap target — below `sm` and the full field above it, and
              SearchTrigger adapts its own chrome to fit. The `sm` here and the
              `sm` in SearchTrigger's class list are one contract; SearchTrigger
              documents what breaks if they diverge. `shrink-0` below `sm`
              because a 36px icon has nothing to give; at `sm`+ it goes back to
              being the row's designated slack absorber per the note above. */}
          <div role="search" className="w-9 min-w-0 shrink-0 sm:w-[220px] sm:shrink xl:w-[320px]">
            <SearchTrigger />
          </div>

          <Button
            variant="ghost"
            size="icon"
            className="hidden cursor-pointer sm:flex"
            onClick={toggleTheme}
          >
            <Sun className="size-[1.2rem] rotate-0 scale-100 transition-all dark:-rotate-90 dark:scale-0" />
            <Moon className="absolute size-[1.2rem] rotate-90 scale-0 transition-all dark:rotate-0 dark:scale-100" />
            <span className="sr-only">Toggle theme</span>
          </Button>

          <div className="hidden items-center sm:flex">
            <UserMenu />
          </div>
        </div>
      </header>
    </>
  )
}
