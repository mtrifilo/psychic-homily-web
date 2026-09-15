import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import { renderToString } from 'react-dom/server'
import { QuickWindowChips } from './QuickWindowChips'
import { QUICK_WINDOW_LABEL } from '../quickWindows'

// Friday 18 September 2026, 23:00 in Phoenix and already Saturday in Tokyo.
const FRIDAY_NIGHT = new Date('2026-09-19T06:00:00Z')

function renderChips(props: Partial<Parameters<typeof QuickWindowChips>[0]> = {}) {
  return render(
    <QuickWindowChips
      params={new URLSearchParams()}
      pathname="/shows"
      currentDays={undefined}
      {...props}
    />
  )
}

// Restored after every test: the worker running this file is reused, and a
// leaked TZ would move the dates in whatever suite runs next.
const runnerTimeZone = process.env.TZ

beforeEach(() => {
  vi.useFakeTimers()
  vi.setSystemTime(FRIDAY_NIGHT)
  // The viewer's own zone, for the All Cities case. The runtime takes the
  // runner's otherwise, which would make every date here depend on where CI is.
  process.env.TZ = 'America/Phoenix'
})

afterEach(() => {
  vi.useRealTimers()
  process.env.TZ = runnerTimeZone
})

describe('QuickWindowChips', () => {
  it('links the four windows to absolute dated URLs', () => {
    renderChips()

    expect(screen.getByRole('link', { name: QUICK_WINDOW_LABEL.tonight })).toHaveAttribute(
      'href',
      '/shows/2026/09/18'
    )
    expect(
      screen.getByRole('link', { name: QUICK_WINDOW_LABEL['this-weekend'] })
    ).toHaveAttribute('href', '/shows/2026/09/18?days=3')
    expect(
      screen.getByRole('link', { name: QUICK_WINDOW_LABEL['next-7-days'] })
    ).toHaveAttribute('href', '/shows/2026/09/18?days=7')
    expect(
      screen.getByRole('link', { name: QUICK_WINDOW_LABEL['this-month'] })
    ).toHaveAttribute('href', '/shows/2026/09')
  })

  // The filters on screen belong to the reader, not to the window they are
  // looking at, so every chip carries them across.
  it('carries the params already on screen', () => {
    renderChips({
      params: new URLSearchParams({ cities: 'all', tags: 'punk', page: '3' }),
    })

    const href = screen
      .getByRole('link', { name: QUICK_WINDOW_LABEL['this-month'] })
      .getAttribute('href')
    expect(href).toContain('cities=all')
    expect(href).toContain('tags=punk')
    // A different window is a different question, answered from page 1.
    expect(href).not.toContain('page=')
  })

  /**
   * The zone decides the date. A list filtered to one metro answers "tonight"
   * on that metro's clock, because that is where the shows are; an unfiltered
   * one answers on the reader's.
   */
  it('resolves tonight in the selected metro s zone', () => {
    // 23:00 Friday in Phoenix is 01:00 Saturday in Chicago.
    renderChips({ metroState: 'IL' })

    expect(screen.getByRole('link', { name: QUICK_WINDOW_LABEL.tonight })).toHaveAttribute(
      'href',
      '/shows/2026/09/19'
    )
    // And Saturday's weekend is two nights, not three.
    expect(
      screen.getByRole('link', { name: QUICK_WINDOW_LABEL['this-weekend'] })
    ).toHaveAttribute('href', '/shows/2026/09/19?days=2')
  })

  // A state the zone map does not know would resolve to the map's default,
  // which can be a calendar day out. The reader's own zone is the better wrong
  // answer: it is at least the day they are having.
  it('falls back to the viewer s zone for a state with no known zone', () => {
    renderChips({ metroState: 'ZZ' })

    expect(screen.getByRole('link', { name: QUICK_WINDOW_LABEL.tonight })).toHaveAttribute(
      'href',
      '/shows/2026/09/18'
    )
  })

  it('marks the chip the reader is already on, and only that one', () => {
    renderChips({ pathname: '/shows/2026/09/18', currentDays: 3 })

    expect(
      screen.getByRole('link', { name: QUICK_WINDOW_LABEL['this-weekend'] })
    ).toHaveAttribute('aria-current', 'page')
    expect(
      screen.getByRole('link', { name: QUICK_WINDOW_LABEL.tonight })
    ).not.toHaveAttribute('aria-current')
    expect(
      screen.getByRole('link', { name: QUICK_WINDOW_LABEL['next-7-days'] })
    ).not.toHaveAttribute('aria-current')
  })

  // Weight and border carry the state as well as colour does, because colour
  // alone is not a sufficient distinction (WCAG 1.4.1).
  it('marks the current chip with more than a colour', () => {
    renderChips({ pathname: '/shows/2026/09', currentDays: undefined })

    const current = screen.getByRole('link', {
      name: QUICK_WINDOW_LABEL['this-month'],
    })
    expect(current.className).toContain('font-bold')
    expect(current.className).toContain('border-primary')
  })

  /**
   * The server has no viewer: no clock worth reading and no zone to read it in.
   * So the pre-hydration row is the four words and no links, and the commit
   * after hydration replaces them. Both renders agree by construction, which is
   * what keeps a date computed from the wrong clock off the page entirely.
   *
   * The words are in the server HTML, so the row occupies its full height there
   * and nothing below it moves when the links arrive.
   */
  it('renders the row without links in the server HTML', () => {
    const html = renderToString(
      <QuickWindowChips
        params={new URLSearchParams()}
        pathname="/shows"
        currentDays={undefined}
      />
    )

    for (const label of Object.values(QUICK_WINDOW_LABEL)) {
      expect(html).toContain(label)
    }
    expect(html).toContain('Jump to')
    expect(html).not.toContain('href')
  })
})
