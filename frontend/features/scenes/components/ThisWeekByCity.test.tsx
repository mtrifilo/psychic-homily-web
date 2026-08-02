import { describe, expect, it } from 'vitest'
import { render, screen, within } from '@testing-library/react'
import type { SceneListItem } from '../types'
import { ThisWeekByCity } from './ThisWeekByCity'

// PSY-1623: this block is the ONLY inbound link `/shows` gives the scene-week
// pages, so the assertions that matter are about the anchors — that every
// scene gets one, that a quiet scene still gets one, and that the rank order a
// reader sees is the DOM order a crawler walks.

// Fixtures carry BOTH week fields, and disagreeing values on purpose: the block
// reads `shows_calendar_week` (its rows' destinations serve that window) and a
// regression to `shows_this_week` has to show up as a wrong number rather than
// as an identical one.
function scene(over: Partial<SceneListItem> & { slug: string }): SceneListItem {
  return {
    city: 'Phoenix',
    state: 'AZ',
    venue_count: 11,
    upcoming_show_count: 334,
    total_show_count: 500,
    shows_this_week: 999,
    shows_calendar_week: 0,
    ...over,
  }
}

const scenes: SceneListItem[] = [
  scene({ slug: 'chicago-il', city: 'Chicago', state: 'IL', shows_calendar_week: 96 }),
  // Two scenes tied on 12 and listed in this order by the API — the block must
  // not reorder them relative to each other.
  scene({ slug: 'minneapolis-mn', city: 'Minneapolis', state: 'MN', shows_calendar_week: 12 }),
  scene({ slug: 'phoenix-az', shows_calendar_week: 22 }),
  scene({ slug: 'dallas-tx', city: 'Dallas', state: 'TX', shows_calendar_week: 12 }),
  scene({ slug: 'seattle-wa', city: 'Seattle', state: 'WA', shows_calendar_week: 0 }),
]

function renderBlock(rows: SceneListItem[] = scenes) {
  return render(
    <ThisWeekByCity scenes={rows} weekStart="2026-07-27" weekEnd="2026-08-02" />
  )
}

describe('ThisWeekByCity', () => {
  it('links every scene to its week page, including the quiet ones', () => {
    renderBlock()

    const hrefs = screen
      .getAllByRole('link')
      .map(link => link.getAttribute('href'))

    expect(hrefs).toHaveLength(scenes.length)
    for (const row of scenes) {
      expect(hrefs).toContain(`/scenes/${row.slug}/week`)
    }
  })

  // The whole point of the block is that a JS-less fetcher can walk it, so the
  // rows must be real anchors carrying a real href rather than anything that
  // needs a router to resolve.
  it('renders plain anchors carrying the href', () => {
    const { container } = renderBlock()

    expect(container.querySelectorAll('a[href^="/scenes/"]')).toHaveLength(
      scenes.length
    )
  })

  it('orders rows by shows this week, keeping the payload order for ties', () => {
    renderBlock()

    const order = screen
      .getAllByRole('link')
      .map(link => link.getAttribute('href'))

    expect(order).toEqual([
      '/scenes/chicago-il/week',
      '/scenes/phoenix-az/week',
      '/scenes/minneapolis-mn/week',
      '/scenes/dallas-tx/week',
      '/scenes/seattle-wa/week',
    ])
  })

  it('names each row for assistive tech, count included', () => {
    renderBlock()

    expect(
      screen.getByRole('link', { name: 'Phoenix, AZ, 22 shows this week' })
    ).toBeInTheDocument()
    expect(
      screen.getByRole('link', { name: 'Seattle, WA, No shows this week' })
    ).toBeInTheDocument()
  })

  // A locked decision, not a style detail: quiet scenes stay in the block but
  // must not read as equals of a busy one, which is what makes always linking
  // them cost nothing.
  it('mutes a scene with no shows and leaves a busy one at full weight', () => {
    renderBlock()

    expect(screen.getByText('Seattle')).toHaveClass('text-muted-foreground')
    expect(screen.getByText('Chicago')).not.toHaveClass('text-muted-foreground')
  })

  it('shows the city, its state and its count', () => {
    renderBlock()

    const row = screen.getByRole('link', { name: /^Chicago/ })
    expect(within(row).getByText('Chicago')).toBeInTheDocument()
    expect(within(row).getByText('IL')).toBeInTheDocument()
    expect(within(row).getByText('96')).toBeInTheDocument()
  })

  // The reason `shows_calendar_week` was added to the API (PSY-1623): a row's
  // number is read as the size of the page the row opens, so it has to be that
  // page's own total. Chicago's fixture carries the two production values that
  // exposed the lie — 96 on the week page, 76 in the rolling field.
  it('counts the destination page’s week, not the rolling seven days', () => {
    renderBlock()

    const row = screen.getByRole('link', { name: /^Chicago/ })
    expect(within(row).getByText('96')).toBeInTheDocument()
    expect(within(row).queryByText('999')).not.toBeInTheDocument()
  })

  // Both spellings ship; CSS picks one by viewport, so both are in the HTML.
  it('labels the week the links open, at both widths', () => {
    renderBlock()

    expect(screen.getByText('MON JUL 27 – SUN AUG 2')).toBeInTheDocument()
    expect(screen.getByText('JUL 27 – AUG 2')).toBeInTheDocument()
    expect(screen.getByRole('heading', { name: 'THIS WEEK, BY CITY' })).toBeInTheDocument()
  })

  it('renders nothing when there are no scenes', () => {
    const { container } = render(
      <ThisWeekByCity scenes={[]} weekStart="2026-07-27" weekEnd="2026-08-02" />
    )

    expect(container).toBeEmptyDOMElement()
  })
})
