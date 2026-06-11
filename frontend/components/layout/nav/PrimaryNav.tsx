'use client'

import Link from 'next/link'
import { usePathname } from 'next/navigation'
import { BrowseMenu } from './BrowseMenu'
import { ContributeMenu } from './ContributeMenu'
import { isNavActive, navItemClassName } from './navData'

// The explicit, labelled primary destinations (NN/G: a labelled "Home" link in
// addition to the clickable logo — left-aligned logo alone is not discoverable).
// Explore sits next to Home as the deep graph-traversal entry; Shows is our
// unique advantage. Radio links straight to the Dial hub (PSY-1057 retired the
// D2 popover once /radio itself became the dial, PSY-1049). Browse / Contribute
// carry menus (own components).
const primaryLinks = [
  { href: '/', label: 'Home' },
  { href: '/explore', label: 'Explore' },
  { href: '/shows', label: 'Shows' },
  { href: '/artists', label: 'Artists' },
  { href: '/radio', label: 'Radio' },
]

// Desktop primary navigation. Condenses into the mobile hamburger sheet below
// `lg` (the dense 7-item row + wide search needs the width); PSY-1020 replaces
// mobile with the bottom tab bar.
export function PrimaryNav() {
  const pathname = usePathname()

  return (
    <nav aria-label="Primary" className="hidden items-center gap-[22px] lg:flex">
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
