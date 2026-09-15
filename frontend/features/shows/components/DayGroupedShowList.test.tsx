import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import { renderToString } from 'react-dom/server'
import { DayGroupedShowList } from './DayGroupedShowList'
import type { ShowResponse } from '../types'

// The ROW is mocked: this file tests the GROUPING (runs, anchors, headings,
// the tonight gate). The row's own columns are pinned by
// `DayGroupedShowRow.test.tsx`.
vi.mock('./DayGroupedShowRow', () => ({
  DayGroupedShowRow: ({
    show,
    density,
    isAdmin,
    userId,
    showCity,
  }: {
    show: ShowResponse
    density: string
    isAdmin: boolean
    userId?: string
    showCity: boolean
  }) => (
    <article
      data-testid={`show-card-${show.id}`}
      data-density={density}
      data-is-admin={String(isAdmin)}
      data-user-id={userId ?? ''}
      data-show-city={String(showCity)}
    >
      {show.title}
    </article>
  ),
  DayGroupedShowListHeader: () => <div data-testid="show-list-header" />,
}))

function makeShow(
  id: number,
  eventDate: string,
  timezone = 'America/Phoenix',
  state = 'AZ'
): ShowResponse {
  return {
    id,
    slug: `show-${id}`,
    title: `Show ${id}`,
    event_date: eventDate,
    status: 'approved',
    city: 'Phoenix',
    state,
    venues: [{ id, name: 'Room', timezone } as never],
    artists: [],
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z',
    is_sold_out: false,
    is_cancelled: false,
  } as ShowResponse
}

const twoDays = [
  makeShow(1, '2026-09-12T02:00:00Z'), // Sep 11 in Phoenix
  makeShow(2, '2026-09-12T03:00:00Z'), // Sep 11 in Phoenix
  makeShow(3, '2026-09-13T02:00:00Z'), // Sep 12 in Phoenix
]

function renderList(
  shows: ShowResponse[] = twoDays,
  density: 'compact' | 'comfortable' | 'expanded' = 'comfortable'
) {
  return render(
    <DayGroupedShowList
      shows={shows}
      density={density}
      isAdmin={false}
      showCity={false}
    />
  )
}

describe('DayGroupedShowList', () => {
  afterEach(() => {
    vi.useRealTimers()
  })

  it('puts each day s rows under their own section heading', () => {
    renderList()

    const headings = screen.getAllByRole('heading', { level: 2 })
    expect(headings.map(heading => heading.textContent)).toEqual([
      'FRI · SEP 11',
      'SAT · SEP 12',
    ])

    const groups = screen.getAllByTestId('show-day-group')
    expect(groups).toHaveLength(2)
    expect(groups[0].querySelectorAll('article')).toHaveLength(2)
    expect(groups[1].querySelectorAll('article')).toHaveLength(1)
  })

  it('anchors each heading at its venue-local date', () => {
    renderList()

    expect(screen.getByRole('heading', { name: 'FRI · SEP 11' })).toHaveAttribute(
      'id',
      'd-2026-09-11'
    )
    expect(screen.getByRole('heading', { name: 'SAT · SEP 12' })).toHaveAttribute(
      'id',
      'd-2026-09-12'
    )
  })

  it('renders every row, in list order, at the requested density', () => {
    renderList(twoDays, 'compact')

    const rows = screen.getAllByRole('article')
    expect(rows.map(row => row.getAttribute('data-testid'))).toEqual([
      'show-card-1',
      'show-card-2',
      'show-card-3',
    ])
    for (const row of rows) {
      expect(row).toHaveAttribute('data-density', 'compact')
    }
  })

  it('keeps the grouping in every density', () => {
    for (const density of ['compact', 'comfortable', 'expanded'] as const) {
      const { unmount } = renderList(twoDays, density)
      expect(screen.getAllByTestId('show-day-group')).toHaveLength(2)
      unmount()
    }
  })

  // The formatters answer an unparseable date with the literal "INVALID DATE",
  // and a heading is a much louder place to print that than a row's date tile.
  it('renders no heading over a run whose date cannot be read', () => {
    const undated = makeShow(9, 'not-a-date')

    renderList([undated])

    expect(screen.queryByRole('heading', { level: 2 })).toBeNull()
    // The row is still listed: an unreadable date is not a reason to hide it.
    expect(screen.getByTestId('show-card-9')).toBeInTheDocument()
  })

  it('still heads the readable runs on a page that also has an unreadable one', () => {
    renderList([makeShow(9, 'not-a-date'), ...twoDays])

    const headings = screen.getAllByRole('heading', { level: 2 })
    expect(headings.map(heading => heading.textContent)).toEqual([
      'FRI · SEP 11',
      'SAT · SEP 12',
    ])
  })

  describe('the TONIGHT heading', () => {
    beforeEach(() => {
      vi.useFakeTimers()
      // 12:00 on Sep 11 in Phoenix.
      vi.setSystemTime(new Date('2026-09-11T19:00:00Z'))
    })

    it('marks only today s group once hydrated', () => {
      renderList()

      const headings = screen.getAllByRole('heading', { level: 2 })
      expect(headings.map(heading => heading.textContent)).toEqual([
        'TONIGHT · FRI SEP 11',
        'SAT · SEP 12',
      ])
    })

    // Naming today means reading a clock, and this list renders inside the
    // route's prerendered shell. A clock read there moves the whole subtree
    // into the dynamic resume, which is a route-mode change, so the server
    // render must not make the claim at all.
    it('makes no TONIGHT claim in the server render', () => {
      const html = renderToString(
        <DayGroupedShowList
          shows={twoDays}
          density="comfortable"
          isAdmin={false}
          showCity={false}
        />
      )

      expect(html).toContain('FRI · SEP 11')
      expect(html).not.toContain('TONIGHT')
    })

    // Every heading carries the same primary treatment, so the label is the
    // ONLY thing that separates tonight from any other day.
    it('separates tonight from the other days by label alone', () => {
      renderList()

      const [tonight, other] = screen.getAllByRole('heading', { level: 2 })
      expect(tonight.textContent).toBe('TONIGHT · FRI SEP 11')
      expect(other.textContent).toBe('SAT · SEP 12')
      expect(tonight.className).toBe(other.className)
    })
  })

  // The heading is the one thing a reader scanning a long list navigates by,
  // so its size, its colour and its stuck position are a contract rather than
  // styling: `/shows`, the month pages and the day pages all render this
  // component, and there is no second place to set them.
  describe('the day heading treatment', () => {
    // `pt-3.5`, `text-sm` and `pb-1.5` are the three classes
    // `--shows-day-header-height` is the sum of (globals.css), so they are
    // pinned here: editing one without the token would shorten the clearance
    // the rows below rely on.
    it('renders every heading stuck, primary, at Space Mono bold 14', () => {
      renderList()

      const headings = screen.getAllByRole('heading', { level: 2 })
      expect(headings.length).toBeGreaterThan(0)
      for (const heading of headings) {
        expect(heading).toHaveClass(
          'font-mono',
          'text-sm',
          'font-bold',
          'tracking-[1px]',
          'uppercase',
          'text-primary',
          'pt-3.5',
          'pb-1.5',
          'sticky',
          'top-[var(--topbar-height)]',
          'z-20',
          'bg-background',
          // The hook globals.css keys the heading link's clearance, the
          // forced-colors rule and the short-viewport unsticking on.
          'shows-day-heading'
        )
        const rule = heading.querySelector('[aria-hidden="true"]')
        expect(rule).toHaveClass('h-px', 'bg-primary', 'shows-day-heading-rule')
      }
    })

    it('gives every anchored heading the top bar as its scroll margin', () => {
      renderList()

      const anchored: Array<[string, string]> = [
        ['FRI · SEP 11', 'd-2026-09-11'],
        ['SAT · SEP 12', 'd-2026-09-12'],
      ]
      for (const [name, id] of anchored) {
        const heading = screen.getByRole('heading', { name })
        expect(heading).toHaveAttribute('id', id)
        expect(heading).toHaveClass('scroll-mt-[var(--topbar-height)]')
      }
    })

    // The clearance itself is a stylesheet rule keyed on this class
    // (globals.css), so losing the class loses the guarantee silently.
    it('marks rows that sit under a heading, and only those', () => {
      const { container } = renderList()

      const groups = container.querySelectorAll(
        '[data-testid="show-day-group"]'
      )
      expect(groups.length).toBeGreaterThan(0)
      for (const group of groups) {
        const hasHeading = group.querySelector('h2') !== null
        expect(group.querySelector('.shows-day-rows') !== null).toBe(hasHeading)
      }
    })

    // jsdom loads no stylesheet, so the class assertions above prove only that
    // the component asks for the clearance. The rule that grants it lives in
    // globals.css, where renaming or deleting it would leave every one of them
    // green. This reads the stylesheet as text so that it does not.
    it('keeps the stylesheet rule the marker classes depend on', () => {
      const css = readFileSync(
        resolve(__dirname, '../../../app/globals.css'),
        'utf8'
      )

      expect(css).toContain('--shows-day-header-height')
      expect(css).toMatch(
        /\.shows-day-rows\s+:where\([^)]*\)\s*\{\s*scroll-margin-top: calc\(\s*var\(--topbar-height\) \+ var\(--shows-day-header-height\)/
      )
      // The expanded row mounts a music embed, so the clearance has to reach an
      // iframe as well as the links and buttons of a collapsed row.
      expect(css).toMatch(/\.shows-day-rows\s+:where\([^)]*\biframe\b[^)]*\)/)
      expect(css).toMatch(
        /\.shows-day-heading :where\(a\)\s*\{\s*scroll-margin-top: calc\(/
      )
      expect(css).toMatch(/\.shows-day-heading \{\s*position: static/)
      expect(css).toContain('.shows-day-heading-rule')
    })

    it('leaves an undated run s rows unmarked, having no heading to clear', () => {
      const { container } = renderList([makeShow(9, 'not-a-date')])

      expect(container.querySelector('h2')).toBeNull()
      expect(container.querySelector('.shows-day-rows')).toBeNull()
    })

    // Scoped to this component's own wrappers, which is all it renders: a
    // non-visible `overflow` on the section or the list box would make
    // `position: sticky` inert. An `overflow` introduced by the route shell
    // above would do the same and is NOT covered here.
    it('adds no scroll container of its own around the heading', () => {
      const { container } = renderList()

      const heading = screen.getByRole('heading', { name: 'FRI · SEP 11' })
      const offenders: string[] = []
      for (
        let node = heading.parentElement;
        node !== null && node !== container.parentElement;
        node = node.parentElement
      ) {
        const style = node.getAttribute('style') ?? ''
        const className = node.getAttribute('class') ?? ''
        if (/overflow(-[xy])?\s*:\s*(?!visible)/.test(style)) {
          offenders.push(`style="${style}"`)
        }
        if (/(^|\s|:)overflow-(x-|y-)?(auto|hidden|scroll|clip)(\s|$)/.test(className)) {
          offenders.push(`class="${className}"`)
        }
      }
      expect(offenders).toEqual([])
    })

    it('keeps the same heading in the compact density', () => {
      const comfortable = renderList(twoDays, 'comfortable')
      const comfortableClass = screen
        .getAllByRole('heading', { level: 2 })[0]
        .className
      comfortable.unmount()

      renderList(twoDays, 'compact')
      expect(
        screen.getAllByRole('heading', { level: 2 })[0].className
      ).toBe(comfortableClass)
    })
  })

  // The list is the only thing between `ShowList` and the row for these, and a
  // mock that ignored them would let a dropped prop pass every test here.
  describe('what it hands each row', () => {
    it('forwards the viewer identity and the city flag', () => {
      render(
        <DayGroupedShowList
          shows={twoDays}
          density="comfortable"
          isAdmin
          userId="42"
          showCity
        />
      )

      const row = screen.getByTestId('show-card-1')
      expect(row).toHaveAttribute('data-is-admin', 'true')
      expect(row).toHaveAttribute('data-user-id', '42')
      expect(row).toHaveAttribute('data-show-city', 'true')
    })

    it('forwards the absence of one too', () => {
      render(
        <DayGroupedShowList
          shows={twoDays}
          density="comfortable"
          isAdmin={false}
          showCity={false}
        />
      )

      const row = screen.getByTestId('show-card-1')
      expect(row).toHaveAttribute('data-is-admin', 'false')
      expect(row).toHaveAttribute('data-user-id', '')
      expect(row).toHaveAttribute('data-show-city', 'false')
    })
  })

  /**
   * Each heading is the entry point to that day's own page (PSY-2061). The link
   * is INSIDE the heading rather than around it, so the heading keeps its
   * anchor id and its role: a reader jumping to `#d-2026-09-11` lands on the
   * heading, and a reader following the text lands on the day.
   */
  describe('day headings link to their day page', () => {
    it('links each heading at its venue-local date', () => {
      renderList()

      expect(screen.getByRole('link', { name: 'FRI · SEP 11' })).toHaveAttribute(
        'href',
        '/shows/2026/09/11'
      )
      expect(screen.getByRole('link', { name: 'SAT · SEP 12' })).toHaveAttribute(
        'href',
        '/shows/2026/09/12'
      )
    })

    it('keeps the anchor on the heading rather than moving it to the link', () => {
      renderList()

      const heading = screen.getByRole('heading', { name: 'FRI · SEP 11' })
      expect(heading).toHaveAttribute('id', 'd-2026-09-11')
      expect(heading.querySelector('a')).toHaveAttribute(
        'href',
        '/shows/2026/09/11'
      )
    })

    // A run whose date cannot be read gets no heading at all, so there is
    // nothing to link; the guard is what keeps an unreadable date from becoming
    // a link to a URL the route would refuse.
    it('renders no heading, and so no link, for an unreadable date', () => {
      renderList([makeShow(9, 'not-a-date')])

      expect(screen.queryByRole('heading', { level: 2 })).not.toBeInTheDocument()
      expect(screen.queryByRole('link')).not.toBeInTheDocument()
    })
  })
})
