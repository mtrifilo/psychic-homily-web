'use client'

import Link from 'next/link'
import { usePathname } from 'next/navigation'
import { BrowseMenu } from './BrowseMenu'
import { ContributeMenu } from './ContributeMenu'
import {
  atlasItem, graphItem, isNavActive, navDestination, navItemClassName, primaryTabs,
} from './navData'

// The explicit, labelled primary destinations (NN/G: a labelled "Home" link in
// addition to the clickable logo — left-aligned logo alone is not discoverable).
// Shows is our unique advantage. Graph occupies the discovery slot that
// PSY-1337 held open for the Observatory rebuild. Radio links
// straight to the Dial hub (PSY-1057 retired the D2 popover once /radio itself
// became the dial, PSY-1049). Browse / Contribute carry menus (own components).
// Composed from navData's canonical entries (PSY-1821) — this list only
// decides WHICH destinations are top-bar primaries and their order; Atlas is
// the spin-to-discover globe promoted as a flagship discovery surface
// (PSY-1219).
// Exported for BottomTabBar's mobile-reachability guard test: every desktop
// primary destination must stay reachable from the mobile tab bar or its
// Browse sheet (PSY-1020). Adding a link here without a mobile home fails
// that test instead of silently stranding phone users.
export const primaryLinks = [
  ...primaryTabs,
  navDestination('/artists'),
  graphItem,
  atlasItem,
]

// Desktop primary navigation. Renders only at `xl` and up — below that the
// BottomTabBar is the primary nav (PSY-1020), so its xl:hidden and this
// xl:flex are two ends of one contract: keep them in sync.
export function PrimaryNav() {
  const pathname = usePathname()

  return (
    <nav aria-label="Primary" className="hidden items-center gap-[22px] xl:flex">
      {primaryLinks.map(link => {
        const active = isNavActive(pathname, link.href)
        return (
          <Link
            key={link.href}
            href={link.href}
            aria-current={active ? 'page' : undefined}
            className={navItemClassName(active)}
          >
            {link.label}
          </Link>
        )
      })}
      <BrowseMenu />
      <ContributeMenu />
    </nav>
  )
}
