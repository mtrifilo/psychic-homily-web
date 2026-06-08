'use client'

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
import { cn } from '@/lib/utils'
import { useAuthContext } from '@/lib/context/AuthContext'
import {
  contributeItems,
  contributeHrefs,
  editorialItems,
  isNavActive,
  navItemClassName,
  type NavLink as NavLinkData,
} from './navData'

// Contribute ▾ — the What.cd "request system as call-to-action". PSY-1015
// refines the PSY-1013 functional menu into the two-column Participate /
// Editorial panel from Figma (frame 460:3): "+ Submit a show" is the primary
// (orange) action and lives in the menu, not as a standalone bar CTA (OQ-2).
// Text-only links per the design — no item icons. Radix gives the trigger
// aria-haspopup / aria-expanded and full APG keyboard operation; arrow keys
// traverse every item across both columns in DOM order, and Escape returns
// focus to the trigger.

// Per-item color: primary (orange + semibold) for the "+ Submit a show"
// call-to-action, muted for the trailing "hub" / external links, foreground
// for the rest — matching the Figma panel. The color is pinned on the focus
// state too so the item keeps its own color (the default item chrome would
// otherwise swap to accent-foreground on hover/keyboard focus).
function itemClassName(item: NavLinkData): string {
  if (item.submitPrimary) {
    return 'font-semibold text-primary focus:text-primary'
  }
  const trailing = item.external || item.href === '/contribute'
  return cn(
    'font-medium',
    trailing
      ? 'text-muted-foreground focus:text-muted-foreground'
      : 'text-foreground focus:text-foreground'
  )
}

function renderItem(item: NavLinkData) {
  // Submit's label is "Submit a Show" in shared nav data; the Contribute panel
  // presents it as the "+ Submit a show" call-to-action per Figma.
  const label = item.submitPrimary ? '+ Submit a show' : item.label
  // Trailing affordance: ↗ for external (Substack), → for the internal hub.
  const trailingGlyph = item.external ? ' ↗' : item.href === '/contribute' ? ' →' : ''

  return (
    <DropdownMenuItem
      key={item.href}
      asChild
      // Override the default item chrome: text-only editorial links with no
      // background highlight box; hover/keyboard focus shows an underline and
      // keeps the item's own color instead of the accent-foreground default.
      className="px-0 py-0 text-[15px] focus:bg-transparent focus:underline"
    >
      <Link
        href={item.href}
        target={item.external ? '_blank' : undefined}
        rel={item.external ? 'noopener noreferrer' : undefined}
        className={cn('rounded-sm', itemClassName(item))}
      >
        {label}
        {trailingGlyph}
      </Link>
    </DropdownMenuItem>
  )
}

const groupLabelClassName =
  'px-0 py-0 font-mono text-[11px] font-bold uppercase tracking-[1.2px] text-muted-foreground'

export function ContributeMenu() {
  const pathname = usePathname()
  const { isAuthenticated } = useAuthContext()
  const active = contributeHrefs.some(href => isNavActive(pathname, href))

  return (
    <DropdownMenu>
      <DropdownMenuTrigger className={navItemClassName(active)} aria-label="Contribute">
        Contribute
        <ChevronDown className="size-3.5 opacity-70" aria-hidden />
      </DropdownMenuTrigger>
      <DropdownMenuContent
        align="start"
        className="flex w-auto items-start gap-12 rounded-[10px] px-7 py-6"
      >
        <DropdownMenuGroup className="flex flex-col gap-3">
          <DropdownMenuLabel className={groupLabelClassName}>Participate</DropdownMenuLabel>
          {contributeItems
            .filter(item => isAuthenticated || !item.authOnly)
            .map(renderItem)}
        </DropdownMenuGroup>
        <DropdownMenuGroup className="flex flex-col gap-3">
          <DropdownMenuLabel className={groupLabelClassName}>Editorial</DropdownMenuLabel>
          {editorialItems.map(renderItem)}
        </DropdownMenuGroup>
      </DropdownMenuContent>
    </DropdownMenu>
  )
}
