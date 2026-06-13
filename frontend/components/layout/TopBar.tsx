'use client'

import Image from 'next/image'
import Link from 'next/link'
import { useTheme } from 'next-themes'
import { Moon, Search, Sun } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { openCommandPalette } from '@/lib/hooks/common/useCommandPalette'
import { PrimaryNav } from './nav/PrimaryNav'
import { SearchTrigger } from './nav/SearchTrigger'
import { UserMenu } from './nav/UserMenu'
import { MobileNav } from './nav/MobileNav'

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
//   • the admin-sections drawer trigger (admins on /admin, below `lg`) — the
//     public hamburger sheet was retired by PSY-1020's bottom tab bar
// The Browse / Contribute menus, the authenticated bar, and the palette re-skin
// are each elaborated by their own follow-up tickets (Radio became a plain
// /radio link in PSY-1057); this file just assembles the seams.
export function TopBar() {
  // resolvedTheme (not theme) so the first click always flips the *visible*
  // theme — with theme==='system' a `theme === 'dark'` check would set explicit
  // 'dark' and appear to do nothing. Matches the canonical ModeToggle.
  const { resolvedTheme, setTheme } = useTheme()

  return (
    <>
      {glitchFilter}

      <header className="sticky top-0 z-50 flex h-[var(--topbar-height)] w-full items-center justify-between border-b border-border/50 bg-background/95 px-4 backdrop-blur-sm supports-[backdrop-filter]:bg-background/60 sm:px-6">
        {/* Left: admin drawer (admins on /admin only) + brand + primary nav */}
        <div className="flex items-center gap-3 lg:gap-[30px]">
          <div className="flex items-center gap-3">
            <MobileNav />
            <Link href="/" aria-label="Psychic Homily — home" className="flex items-center gap-2 transition-opacity hover:opacity-80">
              <div className="relative size-[36px] overflow-hidden rounded-md">
                <Image
                  src="/PsychicHomilyLogov2.svg"
                  alt=""
                  width={36}
                  height={36}
                  priority
                  className="rounded-md"
                  style={{ filter: 'url(#glitch)' }}
                />
              </div>
              <span className="hidden text-[15px] font-semibold uppercase tracking-[0.04em] text-foreground sm:inline">
                Psychic Homily
              </span>
            </Link>
          </div>
          <PrimaryNav />
        </div>

        {/* Right: search + theme + account */}
        <div className="flex items-center gap-[14px]">
          <div role="search" className="hidden sm:block">
            <SearchTrigger />
          </div>

          {/* Icon-only search tap target below `sm` (PSY-1020) — same
              CommandPalette, just without the field chrome the phone top bar
              has no room for. */}
          <Button
            variant="ghost"
            size="icon"
            className="sm:hidden"
            aria-label="Search"
            aria-keyshortcuts="Meta+K Control+K"
            onClick={() => openCommandPalette()}
          >
            <Search className="size-5" />
          </Button>

          <Button
            variant="ghost"
            size="icon"
            className="hidden cursor-pointer sm:flex"
            onClick={() => setTheme(resolvedTheme === 'dark' ? 'light' : 'dark')}
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
