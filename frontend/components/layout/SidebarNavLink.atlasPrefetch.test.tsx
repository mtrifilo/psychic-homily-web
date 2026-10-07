import React from 'react'
import { describe, it, expect, vi } from 'vitest'
import { act, render, screen } from '@testing-library/react'
import { Home } from 'lucide-react'
import { TooltipProvider } from '@/components/ui/tooltip'
import { SidebarNavLink } from './SidebarNavLink'
import { PrimaryNav } from './nav/PrimaryNav'
import { markAtlasMapReady } from '@/lib/atlasMapReady'

vi.mock('next/navigation', () => ({
  usePathname: () => '/atlas',
}))

vi.mock('@/lib/context/AuthContext', () => ({
  useAuthContext: () => ({ isAuthenticated: false, user: null }),
}))

// A plain anchor that surfaces the prefetch prop the link was rendered with.
vi.mock('next/link', () => {
  const MockLink = React.forwardRef<
    HTMLAnchorElement,
    React.AnchorHTMLAttributes<HTMLAnchorElement> & { href: string; prefetch?: boolean }
  >(({ href, children, prefetch, ...props }, ref) => (
    <a href={href} ref={ref} data-prefetch={String(prefetch)} {...props}>
      {children}
    </a>
  ))
  MockLink.displayName = 'MockLink'
  return { default: MockLink }
})

function DesktopNav() {
  return (
    <TooltipProvider>
      {/* top-nav mode */}
      <PrimaryNav />
      {/* side-nav mode, expanded and collapsed rows */}
      <SidebarNavLink href="/shows" label="Side shows" icon={Home} active={false} collapsed={false} />
      <SidebarNavLink href="/venues" label="Side venues" icon={Home} active={false} collapsed />
    </TooltipProvider>
  )
}

// Whichever nav mode the visitor chose, the desktop primary navigation on
// /atlas holds its links until the map is up, then re-arms them.
describe('desktop primary navigation on /atlas, both nav modes', () => {
  it('holds PrimaryNav and the side rail until the map is up, then re-arms both', () => {
    const { rerender } = render(<DesktopNav />)
    const links = () => screen.getAllByRole('link')
    expect(links().length).toBeGreaterThan(2)
    for (const link of links()) expect(link).toHaveAttribute('data-prefetch', 'false')

    act(() => markAtlasMapReady())
    rerender(<DesktopNav />)

    for (const link of links()) expect(link).toHaveAttribute('data-prefetch', 'undefined')
    expect(screen.getByText('Side shows').closest('a')).toHaveAttribute('data-prefetch', 'undefined')
  })
})
