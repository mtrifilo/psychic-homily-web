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
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { browseGroups, browseHrefs, isNavActive, navItemClassName } from './navData'

// Browse ▾ — the full faceted catalog. PSY-1013 ships it as a grouped dropdown
// so every catalog/curation/scene destination the retired sidebar exposed stays
// reachable on desktop; PSY-1014 replaces this with the wide three-column
// mega-menu (+ optional Fresh rail). Radix gives the trigger aria-haspopup /
// aria-expanded and full keyboard operation.
export function BrowseMenu() {
  const pathname = usePathname()
  const active = browseHrefs.some(href => isNavActive(pathname, href))

  return (
    <DropdownMenu>
      <DropdownMenuTrigger className={navItemClassName(active)} aria-label="Browse the catalog">
        Browse
        <ChevronDown className="size-3.5 opacity-70" aria-hidden />
      </DropdownMenuTrigger>
      <DropdownMenuContent align="start" className="w-56">
        {browseGroups.map((group, i) => (
          <DropdownMenuGroup key={group.label}>
            {i > 0 && <DropdownMenuSeparator />}
            <DropdownMenuLabel className="text-xs font-semibold uppercase tracking-wider text-muted-foreground/60">
              {group.label}
            </DropdownMenuLabel>
            {group.items.map(item => {
              const Icon = item.icon
              return (
                <DropdownMenuItem key={item.href} asChild>
                  <Link href={item.href}>
                    {Icon && <Icon aria-hidden />}
                    {item.label}
                  </Link>
                </DropdownMenuItem>
              )
            })}
          </DropdownMenuGroup>
        ))}
      </DropdownMenuContent>
    </DropdownMenu>
  )
}
