import { describe, it, expect, vi, beforeEach } from 'vitest'
import { act, render, screen } from '@testing-library/react'

import { atlasItem } from './navData'

const pathname = vi.hoisted(() => ({ current: '' }))
vi.mock('next/navigation', () => ({
  usePathname: () => pathname.current,
}))

// The real Link's prefetch is internal; the stub surfaces the prop it was
// rendered with, which is what decides whether Next arms its prefetch.
vi.mock('next/link', () => import('@/test/mocks/nextLink'))

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

  it('holds an after-map link on /atlas until the map is ready, then re-arms it', async () => {
    const { signal, ChromeLink } = await load()
    render(
      <ChromeLink href="/shows" atlasPrefetch="after-map">
        Shows
      </ChromeLink>
    )
    expect(screen.getByText('Shows')).toHaveAttribute('data-prefetch', 'false')

    act(() => signal.markAtlasMapReady())

    expect(screen.getByText('Shows')).toHaveAttribute('data-prefetch', 'default')
  })

  it('keeps a default link unprefetched on /atlas, after the map too', async () => {
    const { signal, ChromeLink } = await load()
    render(<ChromeLink href="/privacy">Privacy</ChromeLink>)
    expect(screen.getByText('Privacy')).toHaveAttribute('data-prefetch', 'false')

    act(() => signal.markAtlasMapReady())

    expect(screen.getByText('Privacy')).toHaveAttribute('data-prefetch', 'false')
  })

  it('prefetches normally on /shows, whatever its Atlas group', async () => {
    pathname.current = '/shows'
    const { ChromeLink } = await load()
    render(
      <>
        <ChromeLink href="/venues">Venues</ChromeLink>
        <ChromeLink href="/artists" atlasPrefetch="after-map">
          Artists
        </ChromeLink>
      </>
    )
    expect(screen.getByText('Venues')).toHaveAttribute('data-prefetch', 'default')
    expect(screen.getByText('Artists')).toHaveAttribute('data-prefetch', 'default')
  })

  it('prefetches an after-map link at once on a later /atlas visit once the signal has released', async () => {
    const { signal, ChromeLink } = await load()
    signal.markAtlasMapReady()
    render(
      <ChromeLink href="/" atlasPrefetch="after-map">
        Home
      </ChromeLink>
    )
    expect(screen.getByText('Home')).toHaveAttribute('data-prefetch', 'default')
  })

  // The hold and the nav registry's Atlas entry both read ATLAS_PATHNAME
  // (lib/atlasMapReady.ts), while the cap and input release are armed by
  // app/atlas/page.tsx; the constant must be the route that page serves.
  it('holds on the route app/atlas serves', () => {
    expect(atlasItem.href).toBe('/atlas')
  })

  it("keeps a caller's own prefetch={false} after the release", async () => {
    const { signal, ChromeLink } = await load()
    render(
      <ChromeLink href="/admin" prefetch={false} atlasPrefetch="after-map">
        Admin
      </ChromeLink>
    )
    act(() => signal.markAtlasMapReady())
    expect(screen.getByText('Admin')).toHaveAttribute('data-prefetch', 'false')
  })
})
