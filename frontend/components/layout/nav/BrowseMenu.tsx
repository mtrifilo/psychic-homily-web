'use client'

import { useCallback, useEffect, useRef, useState } from 'react'
import Link from 'next/link'
import { usePathname } from 'next/navigation'
import { ChevronDown } from 'lucide-react'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { browseGroups, browseHrefs, isNavActive, navItemClassName } from './navData'

// Hover-intent timing. Kept short so the menu feels snappy on enter and leave
// (PSY-1089). Open after a brief dwell so a pointer merely passing over the
// trigger doesn't pop the panel; close is longer than open so the diagonal
// travel from the trigger into the panel doesn't dismiss it mid-move.
const OPEN_DELAY_MS = 100
const CLOSE_DELAY_MS = 200

// Browse ▾ — the wide three-column mega-menu (Catalog / Curation / Scenes) per
// Figma `455:5`. Built on Radix DropdownMenu so it keeps the W3C APG menu
// pattern for free: arrow-key roving focus across `menuitem`s, type-ahead, and
// Escape closing + returning focus to the trigger. The Fresh / Trending rail in
// the mock is intentionally DEFERRED for v1 — there is no recently-added /
// trending data source yet.
//
// DropdownMenu alone is click-only, so this layers NN/G hover-intent on top via
// a controlled `open` state with open/close timers (Radix still owns click +
// keyboard). Pointer parity: hovering EITHER the trigger or the panel keeps it
// open; leaving both closes it after the delay.
export function BrowseMenu() {
  const pathname = usePathname()
  const active = browseHrefs.some(href => isNavActive(pathname, href))

  const [open, setOpen] = useState(false)
  const timerRef = useRef<ReturnType<typeof setTimeout> | null>(null)

  const clearTimer = useCallback(() => {
    if (timerRef.current) {
      clearTimeout(timerRef.current)
      timerRef.current = null
    }
  }, [])

  // Avoid leaking a pending open/close timer if the component unmounts mid-dwell.
  useEffect(() => clearTimer, [clearTimer])

  const scheduleOpen = useCallback(() => {
    clearTimer()
    timerRef.current = setTimeout(() => setOpen(true), OPEN_DELAY_MS)
  }, [clearTimer])

  const scheduleClose = useCallback(() => {
    clearTimer()
    timerRef.current = setTimeout(() => setOpen(false), CLOSE_DELAY_MS)
  }, [clearTimer])

  // Click / keyboard go through Radix's own open state; cancel any pending hover
  // timer so a click doesn't get clobbered by a late open/close fire.
  const handleOpenChange = useCallback(
    (next: boolean) => {
      clearTimer()
      setOpen(next)
    },
    [clearTimer]
  )

  return (
    // modal={false} is REQUIRED for the hover-intent model: Radix's default
    // modal DropdownMenu sets `pointer-events: none` on <body> while open, which
    // strips the trigger's pointer events the instant the menu opens → the
    // browser fires pointerleave on the trigger → scheduleClose → close →
    // pointer is over the trigger again → scheduleOpen → an endless open/close
    // flicker. Non-modal keeps the page (and trigger) pointer-interactive;
    // outside-click + Escape dismissal still work via Radix's dismissable layer.
    <DropdownMenu open={open} onOpenChange={handleOpenChange} modal={false}>
      <DropdownMenuTrigger
        className={navItemClassName(active)}
        aria-label="Browse the catalog"
        onPointerEnter={scheduleOpen}
        onPointerLeave={scheduleClose}
      >
        Browse
        <ChevronDown className="size-3.5 opacity-70" aria-hidden />
      </DropdownMenuTrigger>
      <DropdownMenuContent
        align="start"
        sideOffset={10}
        className="flex w-auto gap-12 rounded-[10px] border-border p-0 px-7 py-6 shadow-[0px_2px_8px_0px_rgba(0,0,0,0.08)]"
        onPointerEnter={clearTimer}
        onPointerLeave={scheduleClose}
      >
        {browseGroups.map(group => (
          <DropdownMenuGroup key={group.label} className="flex flex-col gap-3">
            <DropdownMenuLabel className="px-0 py-0 font-mono text-[11px] font-bold uppercase tracking-[1.2px] text-muted-foreground">
              {group.label}
            </DropdownMenuLabel>
            {group.items.map(item => (
              <DropdownMenuItem
                key={item.href}
                asChild
                className="px-0 py-0 text-[15px] font-medium text-foreground focus:bg-transparent focus:text-foreground focus:underline data-[highlighted]:bg-transparent data-[highlighted]:underline"
              >
                <Link href={item.href}>{item.label}</Link>
              </DropdownMenuItem>
            ))}
          </DropdownMenuGroup>
        ))}
      </DropdownMenuContent>
    </DropdownMenu>
  )
}
