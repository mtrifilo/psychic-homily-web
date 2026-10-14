import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { act, fireEvent, render, screen } from '@testing-library/react'

import { installMatchMedia } from '@/test/mocks/matchMedia'
import { atlasItem } from './navData'
import { chromePrefetch } from './ChromeLink'

const pathname = vi.hoisted(() => ({ current: '' }))
const router = vi.hoisted(() => ({ prefetch: vi.fn() }))
vi.mock('next/navigation', () => ({
  usePathname: () => pathname.current,
  useRouter: () => router,
}))

// The literal query useCoarsePointer asks, so a drifted query fails here.
const COARSE_POINTER_QUERY = '(pointer: coarse)'

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

describe('chromePrefetch', () => {
  it("takes the caller's Atlas policy on a fine pointer once hydrated", () => {
    expect(chromePrefetch(true, false, 'after-map')).toBe('after-map')
    expect(chromePrefetch(true, false, 'never')).toBe('never')
  })

  it('prefetches on touch on a coarse pointer, whatever the Atlas policy', () => {
    expect(chromePrefetch(true, true, 'after-map')).toBe('on-touch')
    expect(chromePrefetch(true, true, 'never')).toBe('on-touch')
  })

  it('prefetches on touch through the hydration render, before the pointer is known', () => {
    expect(chromePrefetch(false, false, 'after-map')).toBe('on-touch')
    expect(chromePrefetch(false, false, 'never')).toBe('on-touch')
    expect(chromePrefetch(false, true, 'never')).toBe('on-touch')
  })
})

describe('ChromeLink on a coarse pointer', () => {
  let matchMedia: ReturnType<typeof installMatchMedia>

  beforeEach(() => {
    router.prefetch.mockClear()
    pathname.current = '/shows'
    matchMedia = installMatchMedia({ [COARSE_POINTER_QUERY]: true })
  })
  afterEach(() => {
    matchMedia.restore()
  })

  it('never prefetches on viewport entry, on any route', async () => {
    const { ChromeLink } = await load()
    render(
      <>
        <ChromeLink href="/venues">Venues</ChromeLink>
        <ChromeLink href="/" atlasPrefetch="after-map">
          Home
        </ChromeLink>
      </>
    )
    expect(screen.getByText('Venues')).toHaveAttribute('data-prefetch', 'false')
    expect(screen.getByText('Home')).toHaveAttribute('data-prefetch', 'false')
  })

  it('prefetches its route when a touch lands on it', async () => {
    const { ChromeLink } = await load()
    render(
      <ChromeLink href="/shows" atlasPrefetch="after-map">
        Shows
      </ChromeLink>
    )
    expect(router.prefetch).not.toHaveBeenCalled()

    fireEvent.touchStart(screen.getByText('Shows'))

    expect(router.prefetch).toHaveBeenCalledExactlyOnceWith('/shows')
  })

  it('leaves a finger pointerdown to touchstart', async () => {
    const { ChromeLink } = await load()
    render(<ChromeLink href="/radio">Radio</ChromeLink>)

    fireEvent.pointerDown(screen.getByText('Radio'), { pointerType: 'touch' })

    expect(router.prefetch).not.toHaveBeenCalled()
  })

  // A pen, or a mouse or trackpad on a coarse-pointer device (a tablet with a
  // trackpad), sends no touchstart, so its pointerdown is the intent.
  it.each(['pen', 'mouse'])('prefetches on a %s pointerdown', async pointerType => {
    const { ChromeLink } = await load()
    render(<ChromeLink href="/radio">Radio</ChromeLink>)

    fireEvent.pointerDown(screen.getByText('Radio'), { pointerType, button: 0 })

    expect(router.prefetch).toHaveBeenCalledExactlyOnceWith('/radio')
  })

  it('makes no router call on a secondary or middle mouse press', async () => {
    const { ChromeLink } = await load()
    render(<ChromeLink href="/radio">Radio</ChromeLink>)
    const link = screen.getByText('Radio')

    fireEvent.pointerDown(link, { pointerType: 'mouse', button: 2 })
    fireEvent.pointerDown(link, { pointerType: 'mouse', button: 1 })

    expect(router.prefetch).not.toHaveBeenCalled()
  })

  it('prefetches on touch on /atlas before the map is up', async () => {
    pathname.current = atlasItem.href
    const { ChromeLink } = await load()
    render(
      <ChromeLink href="/shows" atlasPrefetch="after-map">
        Shows
      </ChromeLink>
    )
    expect(screen.getByText('Shows')).toHaveAttribute('data-prefetch', 'false')

    fireEvent.touchStart(screen.getByText('Shows'))

    expect(router.prefetch).toHaveBeenCalledExactlyOnceWith('/shows')
  })

  it("keeps the viewport prefetch off after the map is up on /atlas", async () => {
    pathname.current = atlasItem.href
    const { signal, ChromeLink } = await load()
    render(
      <ChromeLink href="/shows" atlasPrefetch="after-map">
        Shows
      </ChromeLink>
    )
    act(() => signal.markAtlasMapReady())
    expect(screen.getByText('Shows')).toHaveAttribute('data-prefetch', 'false')
  })

  it("does not prefetch on touch when the caller passed prefetch={false}, and still runs the caller's handlers", async () => {
    const { ChromeLink } = await load()
    const onTouchStart = vi.fn()
    const onPointerDown = vi.fn()
    render(
      <ChromeLink
        href="/admin"
        prefetch={false}
        onTouchStart={onTouchStart}
        onPointerDown={onPointerDown}
      >
        Admin
      </ChromeLink>
    )
    fireEvent.touchStart(screen.getByText('Admin'))
    fireEvent.pointerDown(screen.getByText('Admin'), { pointerType: 'pen' })

    expect(router.prefetch).not.toHaveBeenCalled()
    expect(onTouchStart).toHaveBeenCalledOnce()
    expect(onPointerDown).toHaveBeenCalledOnce()
  })

  it('prefetches on touch with viewportPrefetch={false}, with no viewport prefetch', async () => {
    const { ChromeLink } = await load()
    render(
      <ChromeLink href="/graph" viewportPrefetch={false}>
        Graph
      </ChromeLink>
    )
    const link = screen.getByText('Graph')
    expect(link).toHaveAttribute('data-prefetch', 'false')

    fireEvent.touchStart(link)

    expect(router.prefetch).toHaveBeenCalledExactlyOnceWith('/graph')
  })

  it('takes the fine-pointer policy when the pointer turns fine mid-session', async () => {
    const { ChromeLink } = await load()
    render(<ChromeLink href="/venues">Venues</ChromeLink>)
    expect(screen.getByText('Venues')).toHaveAttribute('data-prefetch', 'false')

    matchMedia.set(COARSE_POINTER_QUERY, false)

    expect(screen.getByText('Venues')).toHaveAttribute('data-prefetch', 'default')
  })
})

// Desktop guard: a fine pointer keeps Next's own prefetch (viewport, hover and
// touchstart) and the Atlas hold, and the chrome adds no router call of its own.
describe('ChromeLink on a fine pointer', () => {
  let matchMedia: ReturnType<typeof installMatchMedia>

  beforeEach(() => {
    router.prefetch.mockClear()
    pathname.current = '/shows'
    matchMedia = installMatchMedia({ [COARSE_POINTER_QUERY]: false })
  })
  afterEach(() => {
    matchMedia.restore()
  })

  it('prefetches on viewport entry off the Atlas and makes no router call on touch, pen or mouse', async () => {
    const { ChromeLink } = await load()
    render(
      <ChromeLink href="/venues" atlasPrefetch="after-map">
        Venues
      </ChromeLink>
    )
    const link = screen.getByText('Venues')
    expect(link).toHaveAttribute('data-prefetch', 'default')

    fireEvent.touchStart(link)
    fireEvent.pointerDown(link, { pointerType: 'pen' })
    fireEvent.pointerDown(link, { pointerType: 'mouse' })

    expect(router.prefetch).not.toHaveBeenCalled()
  })

  it('turns Next prefetch off with viewportPrefetch={false}, and makes no router call on touch', async () => {
    const { ChromeLink } = await load()
    render(
      <ChromeLink href="/graph" viewportPrefetch={false}>
        Graph
      </ChromeLink>
    )
    const link = screen.getByText('Graph')
    expect(link).toHaveAttribute('data-prefetch', 'false')

    fireEvent.touchStart(link)

    expect(router.prefetch).not.toHaveBeenCalled()
  })

  it('keeps the Atlas hold on /atlas', async () => {
    pathname.current = atlasItem.href
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
})
