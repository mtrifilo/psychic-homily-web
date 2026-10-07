import { describe, it, expect, vi, beforeEach } from 'vitest'
import { act, render, screen } from '@testing-library/react'

import { atlasItem } from './navData'

const pathname = vi.hoisted(() => ({ current: '' }))
vi.mock('next/navigation', () => ({
  usePathname: () => pathname.current,
}))

// The real Link's prefetch is internal; the stub surfaces the prop it was
// rendered with, which is what decides whether Next arms its prefetch.
vi.mock('next/link', () => ({
  default: ({
    prefetch,
    href,
    children,
  }: {
    prefetch?: boolean | null
    href: string
    children: React.ReactNode
  }) => (
    <a href={href} data-prefetch={prefetch === undefined ? 'default' : String(prefetch)}>
      {children}
    </a>
  ),
}))

// The signal is page-load state, so each test gets a fresh copy of it and of
// the component that reads it.
async function load() {
  vi.resetModules()
  const signal = await import('@/lib/atlasMapReady')
  const { ChromeLink } = await import('./ChromeLink')
  return { signal, ChromeLink }
}

describe('ChromeLink', () => {
  beforeEach(() => {
    pathname.current = atlasItem.href
  })

  it('holds the prefetch on /atlas until the map is ready, then re-arms it', async () => {
    const { signal, ChromeLink } = await load()
    render(<ChromeLink href="/shows">Shows</ChromeLink>)
    expect(screen.getByText('Shows')).toHaveAttribute('data-prefetch', 'false')

    act(() => signal.markAtlasMapReady())

    expect(screen.getByText('Shows')).toHaveAttribute('data-prefetch', 'default')
  })

  it('prefetches normally on /shows', async () => {
    pathname.current = '/shows'
    const { ChromeLink } = await load()
    render(<ChromeLink href="/venues">Venues</ChromeLink>)
    expect(screen.getByText('Venues')).toHaveAttribute('data-prefetch', 'default')
  })

  it('prefetches normally on a later /atlas visit once the signal has released', async () => {
    const { signal, ChromeLink } = await load()
    signal.markAtlasMapReady()
    render(<ChromeLink href="/">Home</ChromeLink>)
    expect(screen.getByText('Home')).toHaveAttribute('data-prefetch', 'default')
  })

  // The hold keys on the nav registry's Atlas entry, while the cap and input
  // release are armed by app/atlas/page.tsx; both must name the same route.
  it('holds on the route app/atlas serves', () => {
    expect(atlasItem.href).toBe('/atlas')
  })

  it("keeps a caller's own prefetch={false} after the release", async () => {
    const { signal, ChromeLink } = await load()
    render(
      <ChromeLink href="/admin" prefetch={false}>
        Admin
      </ChromeLink>
    )
    act(() => signal.markAtlasMapReady())
    expect(screen.getByText('Admin')).toHaveAttribute('data-prefetch', 'false')
  })
})
