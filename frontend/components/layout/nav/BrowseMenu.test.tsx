import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { act, fireEvent, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { BrowseMenu } from './BrowseMenu'
import { browseGroups } from './navData'

let mockPathname = '/'
vi.mock('next/navigation', () => ({
  usePathname: () => mockPathname,
}))

// Must stay in lockstep with useHoverIntentMenu.ts open/close delays.
const OPEN_DELAY_MS = 50
const CLOSE_DELAY_MS = 200

const browseTrigger = () => screen.getByRole('button', { name: 'Browse the catalog' })
const menuOpen = () => screen.queryByRole('menuitem', { name: 'Artists' }) !== null

describe('BrowseMenu', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockPathname = '/'
  })

  it('opens on click and renders the three scent-rich column headers', async () => {
    const user = userEvent.setup()
    render(<BrowseMenu />)
    await user.click(screen.getByRole('button', { name: 'Browse the catalog' }))
    // Column headers carry no role; assert their text is present once open.
    expect(await screen.findByText('Catalog')).toBeInTheDocument()
    expect(screen.getByText('Curation')).toBeInTheDocument()
    expect(screen.getByText('Scenes')).toBeInTheDocument()
  })

  it('every catalog + curation item is a real link to its route', async () => {
    const user = userEvent.setup()
    render(<BrowseMenu />)
    await user.click(screen.getByRole('button', { name: 'Browse the catalog' }))

    const cases: Array<[string, string]> = [
      ['Artists', '/artists'],
      ['Venues', '/venues'],
      ['Releases', '/releases'],
      ['Labels', '/labels'],
      ['Festivals', '/festivals'],
      ['Collections', '/collections'],
      ['Charts', '/charts'],
      ['Tags', '/tags'],
      ['Leaderboard', '/community/leaderboard'],
    ]
    for (const [name, href] of cases) {
      expect(await screen.findByRole('menuitem', { name })).toHaveAttribute('href', href)
    }
  })

  it('renders Browse → Scenes shortcuts from navData (href wiring)', async () => {
    const user = userEvent.setup()
    render(<BrowseMenu />)
    await user.click(screen.getByRole('button', { name: 'Browse the catalog' }))

    // Wiring only — HTTP 200 resolution was verified manually against stage
    // (2026-07-22), not by this unit test. Lock the curated set so a silent
    // navData edit still fails here.
    const scenes = browseGroups.find(g => g.label === 'Scenes')!.items
    expect(scenes.map(i => i.href)).toEqual([
      '/scenes/phoenix-az',
      '/scenes/tucson-az',
      '/scenes/los-angeles-ca',
      '/scenes/denver-co',
      '/scenes',
    ])
    for (const item of scenes) {
      expect(await screen.findByRole('menuitem', { name: item.label })).toHaveAttribute(
        'href',
        item.href
      )
    }
  })

  it('opens via keyboard (Enter on the focused trigger) — APG menu pattern', async () => {
    const user = userEvent.setup()
    render(<BrowseMenu />)
    const triggerEl = screen.getByRole('button', { name: 'Browse the catalog' })
    triggerEl.focus()
    await user.keyboard('{Enter}')
    expect(await screen.findByRole('menuitem', { name: 'Artists' })).toBeInTheDocument()
  })

  it('Escape closes the menu and returns focus to the trigger', async () => {
    const user = userEvent.setup()
    render(<BrowseMenu />)
    const triggerEl = screen.getByRole('button', { name: 'Browse the catalog' })
    await user.click(triggerEl)
    expect(await screen.findByRole('menuitem', { name: 'Artists' })).toBeInTheDocument()

    await user.keyboard('{Escape}')
    expect(screen.queryByRole('menuitem', { name: 'Artists' })).not.toBeInTheDocument()
    expect(triggerEl).toHaveFocus()
  })

  it('marks the trigger active when on a destination route', () => {
    mockPathname = '/venues'
    render(<BrowseMenu />)
    // navItemClassName applies the active (foreground) treatment; assert the
    // active class is present so the trigger reads as selected.
    expect(screen.getByRole('button', { name: 'Browse the catalog' }).className).toContain(
      'text-foreground'
    )
  })

  // Hover-intent path: fireEvent + fake timers only — userEvent + fake timers
  // can deadlock (see pattern_userevent_blur_timer_flake).
  describe('hover-intent timing', () => {
    beforeEach(() => {
      vi.useFakeTimers()
    })

    afterEach(() => {
      vi.useRealTimers()
    })

    it('opens after the open delay, not before', () => {
      render(<BrowseMenu />)
      fireEvent.pointerEnter(browseTrigger())

      act(() => {
        vi.advanceTimersByTime(OPEN_DELAY_MS - 1)
      })
      expect(menuOpen()).toBe(false)

      act(() => {
        vi.advanceTimersByTime(1)
      })
      expect(menuOpen()).toBe(true)
    })

    it('closes after the close delay when leaving the trigger without entering the panel', () => {
      render(<BrowseMenu />)
      fireEvent.pointerEnter(browseTrigger())
      act(() => {
        vi.advanceTimersByTime(OPEN_DELAY_MS)
      })
      expect(menuOpen()).toBe(true)

      fireEvent.pointerLeave(browseTrigger())
      act(() => {
        vi.advanceTimersByTime(CLOSE_DELAY_MS - 1)
      })
      expect(menuOpen()).toBe(true)

      act(() => {
        vi.advanceTimersByTime(1)
      })
      expect(menuOpen()).toBe(false)
    })

    it('cancels a pending close when the pointer enters the panel (diagonal-travel grace)', () => {
      render(<BrowseMenu />)
      fireEvent.pointerEnter(browseTrigger())
      act(() => {
        vi.advanceTimersByTime(OPEN_DELAY_MS)
      })
      expect(menuOpen()).toBe(true)

      // Leaving the trigger arms the close timer; entering the panel clears it.
      fireEvent.pointerLeave(browseTrigger())
      fireEvent.pointerEnter(screen.getByRole('menu'))
      act(() => {
        vi.advanceTimersByTime(CLOSE_DELAY_MS)
      })
      expect(menuOpen()).toBe(true)
    })

    it('clears a pending open timer on unmount (no state update after unmount)', () => {
      const clearTimeoutSpy = vi.spyOn(globalThis, 'clearTimeout')
      const { unmount } = render(<BrowseMenu />)
      fireEvent.pointerEnter(browseTrigger())
      // Unmount effect must clearTimeout the pending open so setOpen never fires.
      const clearsBeforeUnmount = clearTimeoutSpy.mock.calls.length
      unmount()
      expect(clearTimeoutSpy.mock.calls.length).toBeGreaterThan(clearsBeforeUnmount)

      expect(() => {
        act(() => {
          vi.runOnlyPendingTimers()
        })
      }).not.toThrow()
      expect(screen.queryByRole('menuitem', { name: 'Artists' })).not.toBeInTheDocument()
      clearTimeoutSpy.mockRestore()
    })
  })
})
