'use client'

import Link from 'next/link'
import { usePathname } from 'next/navigation'
import { ChevronDown, ExternalLink } from 'lucide-react'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { useAuthContext } from '@/lib/context/AuthContext'
import {
  contributeItems,
  contributeHrefs,
  editorialItems,
  isNavActive,
  navItemClassName,
  type NavLink as NavLinkData,
} from './navData'

// Contribute ▾ — the call-to-action menu. PSY-1013 ships the functional menu;
// PSY-1015 refines presentation and resolves whether Editorial (Blog / DJ Sets /
// Substack) lives here or under Browse → Curation.
export function ContributeMenu() {
  const pathname = usePathname()
  const { isAuthenticated } = useAuthContext()
  const active = contributeHrefs.some(href => isNavActive(pathname, href))

  const renderItem = (item: NavLinkData) => {
    const Icon = item.icon
    return (
      <DropdownMenuItem key={item.href} asChild>
        <Link
          href={item.href}
          target={item.external ? '_blank' : undefined}
          rel={item.external ? 'noopener noreferrer' : undefined}
        >
          {Icon && <Icon aria-hidden />}
          {item.label}
          {item.external && <ExternalLink className="ml-auto size-3 opacity-50" aria-hidden />}
        </Link>
      </DropdownMenuItem>
    )
  }

  return (
    <DropdownMenu>
      <DropdownMenuTrigger className={navItemClassName(active)} aria-label="Contribute">
        Contribute
        <ChevronDown className="size-3.5 opacity-70" aria-hidden />
      </DropdownMenuTrigger>
      <DropdownMenuContent align="start" className="w-56">
        {contributeItems
          .filter(item => isAuthenticated || !item.authOnly)
          .map(renderItem)}
        <DropdownMenuSeparator />
        <DropdownMenuLabel className="text-xs font-semibold uppercase tracking-wider text-muted-foreground/60">
          Editorial
        </DropdownMenuLabel>
        {editorialItems.map(renderItem)}
      </DropdownMenuContent>
    </DropdownMenu>
  )
}
