import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { act, fireEvent, screen, waitFor } from '@testing-library/react'
import { renderWithProviders } from '@/test/utils'
import type { SceneListItem } from '../types'

const mockPush = vi.fn()
vi.mock('next/navigation', () => ({
  useRouter: () => ({ push: mockPush }),
}))

import {
  AtlasSearch,
  listSideOffsetPx,
  visualViewportShrunk,
} from './AtlasSearch'

const scenes: SceneListItem[] = [
  {
    city: 'Phoenix',
    state: 'AZ',
    slug: 'phoenix-az',
    venue_count: 11,
    upcoming_show_count: 42,
    total_show_count: 180,
    shows_this_week: 0,
    shows_calendar_week: 0,
    latitude: 33.448,
    longitude: -112.074,
  },
  {
    city: 'Chicago',
    state: 'IL',
    slug: 'chicago-il',
    venue_count: 9,
    upcoming_show_count: 283,
    total_show_count: 337,
    shows_this_week: 0,
    shows_calendar_week: 0,
    latitude: 41.88,
    longitude: -87.63,
  },
  {
    // Unplaceable — no coords; selecting it must NAVIGATE, never "fly".
    city: 'Faketown',
    state: 'ZZ',
    slug: 'faketown-zz',
    venue_count: 2,
    upcoming_show_count: 3,
    total_show_count: 3,
    shows_this_week: 0,
    shows_calendar_week: 0,
  },
]

describe('AtlasSearch (PSY-1310)', () => {
  const onPick = vi.fn()

  beforeEach(() => {
    onPick.mockReset()
    mockPush.mockReset()
  })

  function openSearch() {
    renderWithProviders(<AtlasSearch scenes={scenes} onPick={onPick} />)
    fireEvent.click(screen.getByRole('combobox', { name: /search scenes/i }))
  }

  it('lists scenes most-active-first when opened', () => {
    openSearch()
    const options = screen.getAllByRole('option')
    expect(options[0]).toHaveTextContent('Chicago, IL')
    expect(options[1]).toHaveTextContent('Phoenix, AZ')
    expect(options[2]).toHaveTextContent('Faketown, ZZ')
  })

  it('sets the field in 16px text under a coarse pointer, 14px otherwise', () => {
    openSearch()
    expect(screen.getByPlaceholderText('City or state…')).toHaveClass(
      'text-sm',
      'pointer-coarse:text-base',
    )
  })

  it('filters as the user types', () => {
    openSearch()
    fireEvent.change(screen.getByPlaceholderText('City or state…'), {
      target: { value: 'phoe' },
    })
    expect(screen.getByRole('option', { name: /Phoenix/ })).toBeInTheDocument()
    expect(screen.queryByRole('option', { name: /Chicago/ })).not.toBeInTheDocument()
  })

  it('picking a placeable scene calls onPick (fly + preview), not navigation', () => {
    openSearch()
    fireEvent.click(screen.getByRole('option', { name: /Phoenix/ }))
    expect(onPick).toHaveBeenCalledTimes(1)
    expect(onPick.mock.calls[0][0]).toMatchObject({ slug: 'phoenix-az' })
    expect(mockPush).not.toHaveBeenCalled()
  })

  it('picking an unplaceable scene navigates to its scene page instead', () => {
    openSearch()
    fireEvent.click(screen.getByRole('option', { name: /Faketown/ }))
    expect(mockPush).toHaveBeenCalledWith('/scenes/faketown-zz')
    expect(onPick).not.toHaveBeenCalled()
  })

  it('parks focus on the trigger when a scene is picked (PSY-1313 focus-return seam)', () => {
    // The preview panel captures document.activeElement at mount as its
    // focus-return target — a pick must leave the trigger focused, not the
    // cmdk input (which unmounts with the popover's exit animation).
    openSearch()
    fireEvent.click(screen.getByRole('option', { name: /Phoenix/ }))
    expect(
      screen.getByRole('combobox', { name: /search scenes/i }),
    ).toHaveFocus()
  })

  it('"/" opens the search — but not while typing in another field', () => {
    renderWithProviders(
      <>
        <input aria-label="decoy" />
        <AtlasSearch scenes={scenes} onPick={onPick} />
      </>,
    )
    // Typing "/" inside another input must NOT hijack it.
    fireEvent.keyDown(screen.getByLabelText('decoy'), { key: '/' })
    expect(screen.queryByRole('option')).not.toBeInTheDocument()

    // "/" on the document opens the combobox list.
    fireEvent.keyDown(document, { key: '/' })
    expect(screen.getAllByRole('option').length).toBeGreaterThan(0)
  })
})

describe('visualViewportShrunk', () => {
  it('is true while the visual viewport is shorter than the layout viewport', () => {
    expect(visualViewportShrunk({ height: 190, scale: 1 }, 390)).toBe(true)
  })

  it('is false at full height, at a fractional full height, and with no visual viewport', () => {
    expect(visualViewportShrunk({ height: 390, scale: 1 }, 390)).toBe(false)
    expect(visualViewportShrunk({ height: 389.6, scale: 1 }, 390)).toBe(false)
    expect(visualViewportShrunk(null, 390)).toBe(false)
    expect(visualViewportShrunk(undefined, 390)).toBe(false)
  })

  it('reads a zoomed page with no keyboard as no keyboard', () => {
    expect(visualViewportShrunk({ height: 195, scale: 2 }, 390)).toBe(false)
  })

  it('reads a keyboard on a zoomed page as a keyboard', () => {
    expect(visualViewportShrunk({ height: 95, scale: 2 }, 390)).toBe(true)
  })
})

describe('listSideOffsetPx', () => {
  it('drops the list to the line below the trigger', () => {
    // Trigger at top 16 and 34 tall, line at 112: 62px below its bottom edge.
    expect(listSideOffsetPx(112, 50, false)).toBe(62)
  })

  it('adds nothing when the trigger already reaches past the line', () => {
    expect(listSideOffsetPx(112, 112, false)).toBe(0)
    expect(listSideOffsetPx(112, 130, false)).toBe(0)
  })

  it('keeps the default gap when there is no line to clear', () => {
    expect(listSideOffsetPx(undefined, 50, false)).toBeUndefined()
  })

  it('gives the line up while the keyboard is up', () => {
    expect(listSideOffsetPx(112, 50, true)).toBeUndefined()
  })
})

describe('AtlasSearch getListMinTopPx', () => {
  /**
   * The list's vertical translate once Radix has placed it. jsdom lays the
   * trigger out at 0, so this is the popover's side offset: 4 is its default
   * gap. `data-radix-popper-content-wrapper` and the `translate(Xpx, Ypx)`
   * form are Radix Popper details.
   */
  async function expectListTranslateY(px: number) {
    await waitFor(() => {
      const wrapper = document.querySelector<HTMLElement>(
        '[data-radix-popper-content-wrapper]',
      )
      expect(wrapper?.style.transform).toBe(`translate(0px, ${px}px)`)
    })
  }

  function renderWithLine(getListMinTopPx: () => number | undefined) {
    renderWithProviders(
      <AtlasSearch
        scenes={scenes}
        onPick={vi.fn()}
        getListMinTopPx={getListMinTopPx}
      />,
    )
    return screen.getByRole('combobox', { name: /search scenes/i })
  }

  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('opens at the line, read afresh on every open', async () => {
    let line: number | undefined = 112
    const trigger = renderWithLine(() => line)
    fireEvent.click(trigger)
    await expectListTranslateY(112)
    fireEvent.keyDown(document.activeElement ?? document.body, { key: 'Escape' })
    await waitFor(() =>
      expect(screen.queryByRole('option')).not.toBeInTheDocument(),
    )
    line = undefined
    fireEvent.click(trigger)
    await expectListTranslateY(4)
  })

  it('clears a line that appears beside the trigger after the list opened', async () => {
    let lineDrawn = false
    const trigger = renderWithLine(() => (lineDrawn ? 112 : undefined))
    fireEvent.click(trigger)
    await expectListTranslateY(4)
    lineDrawn = true
    act(() => {
      const credit = document.createElement('div')
      credit.className = 'credit'
      trigger.parentElement!.appendChild(credit)
    })
    await expectListTranslateY(112)
  })

  it('follows a docked element drawn and emptied by class while the list is open', async () => {
    let credit: HTMLElement | null = null
    const trigger = renderWithLine(() =>
      credit && !credit.classList.contains('empty') ? 112 : undefined,
    )
    act(() => {
      credit = document.createElement('div')
      credit.className = 'empty'
      trigger.parentElement!.appendChild(credit)
    })
    fireEvent.click(trigger)
    await expectListTranslateY(4)
    act(() => credit!.classList.remove('empty'))
    await expectListTranslateY(112)
    act(() => credit!.classList.add('empty'))
    await expectListTranslateY(4)
  })

  it('gives the line up while the keyboard is up, and takes it back when it falls', async () => {
    // Floating UI positions against the visual viewport too, so the stand-in
    // carries every field it reads.
    const viewport = Object.assign(new EventTarget(), {
      width: window.innerWidth,
      height: window.innerHeight - 200,
      offsetLeft: 0,
      offsetTop: 0,
      pageLeft: 0,
      pageTop: 0,
      scale: 1,
    })
    vi.stubGlobal('visualViewport', viewport)
    const trigger = renderWithLine(() => 112)
    fireEvent.click(trigger)
    await expectListTranslateY(4)
    act(() => {
      viewport.height = window.innerHeight
      viewport.dispatchEvent(new Event('resize'))
    })
    await expectListTranslateY(112)
    act(() => {
      viewport.height = window.innerHeight - 200
      viewport.dispatchEvent(new Event('resize'))
    })
    await expectListTranslateY(4)
  })
})
