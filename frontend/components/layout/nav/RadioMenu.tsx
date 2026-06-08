'use client'

import Link from 'next/link'
import { usePathname } from 'next/navigation'
import { ChevronDown, Radio } from 'lucide-react'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { isNavActive, navItemClassName } from './navData'

// Radio ▾ — PSY-1013 ships a minimal functional menu (the caret matches the
// Figma design and /radio stays reachable). PSY-1016 replaces the content with
// the rich Option-D2 "station overview" panel (now-playing + recent shows +
// entity hops). Real providers only: KEXP / WFMU / NTS.
export function RadioMenu() {
  const pathname = usePathname()
  const active = isNavActive(pathname, '/radio')

  return (
    <DropdownMenu>
      <DropdownMenuTrigger className={navItemClassName(active)} aria-label="Radio">
        Radio
        <ChevronDown className="size-3.5 opacity-70" aria-hidden />
      </DropdownMenuTrigger>
      <DropdownMenuContent align="start" className="w-56">
        <DropdownMenuLabel className="text-xs font-semibold uppercase tracking-wider text-muted-foreground/60">
          Freeform radio
        </DropdownMenuLabel>
        <DropdownMenuItem asChild>
          <Link href="/radio">
            <Radio aria-hidden />
            Stations &amp; playlists
          </Link>
        </DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  )
}
