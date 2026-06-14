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
import { browseGroups, browseHrefs, isNavActive, navItemClassName } from './navData'
import { useHoverIntentMenu } from './useHoverIntentMenu'

// Browse ▾ — the wide three-column mega-menu (Catalog / Curation / Scenes) per
// Figma `455:5`. Built on Radix DropdownMenu so it keeps the W3C APG menu
// pattern for free: arrow-key roving focus across `menuitem`s, type-ahead, and
// Escape closing + returning focus to the trigger. The Fresh / Trending rail in
// the mock is intentionally DEFERRED for v1 — there is no recently-added /
// trending data source yet.
//
// NN/G hover-intent (open on hover) is layered on via `useHoverIntentMenu`,
// shared with ContributeMenu so the two menus behave identically. The hook
// requires `modal={false}` below — see its docblock for the rationale.
export function BrowseMenu() {
  const pathname = usePathname()
  const active = browseHrefs.some(href => isNavActive(pathname, href))

  const { open, onOpenChange, triggerHoverProps, contentHoverProps } = useHoverIntentMenu()

  return (
    <DropdownMenu open={open} onOpenChange={onOpenChange} modal={false}>
      <DropdownMenuTrigger
        className={navItemClassName(active)}
        aria-label="Browse the catalog"
        {...triggerHoverProps}
      >
        Browse
        <ChevronDown className="size-3.5 opacity-70" aria-hidden />
      </DropdownMenuTrigger>
      <DropdownMenuContent
        align="start"
        sideOffset={10}
        className="flex w-auto gap-12 rounded-[10px] border-border p-0 px-7 py-6 shadow-[0px_2px_8px_0px_rgba(0,0,0,0.08)] duration-100"
        {...contentHoverProps}
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
