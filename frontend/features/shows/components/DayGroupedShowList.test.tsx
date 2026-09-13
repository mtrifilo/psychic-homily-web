import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import { renderToString } from 'react-dom/server'
import { DayGroupedShowList } from './DayGroupedShowList'
import type { ShowResponse } from '../types'

vi.mock('./ShowCard', () => ({
  ShowCard: ({
    show,
    density,
  }: {
    show: ShowResponse
    density: string
  }) => (
    <article data-testid={`show-card-${show.id}`} data-density={density}>
      {show.title}
    </article>
  ),
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
    <DayGroupedShowList shows={shows} density={density} isAdmin={false} />
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
        />
      )

      expect(html).toContain('FRI · SEP 11')
      expect(html).not.toContain('TONIGHT')
    })

    it('carries the heading s emphasis on the rule beneath it too', () => {
      const { container } = renderList()

      const groups = container.querySelectorAll(
        '[data-testid="show-day-group"]'
      )
      expect(groups[0].querySelector('.border-primary')).not.toBeNull()
      expect(groups[1].querySelector('.border-primary')).toBeNull()
    })
  })
})
