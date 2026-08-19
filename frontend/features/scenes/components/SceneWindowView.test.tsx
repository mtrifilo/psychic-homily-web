import { describe, expect, it } from 'vitest'
import { render, screen, within } from '@testing-library/react'
import { SceneWindowView, type SceneWindowData } from './SceneWindowView'
import type { SceneWeekDay, SceneWeekShow } from '../sceneWeek'

const show = (over: Partial<SceneWeekShow> = {}): SceneWeekShow =>
  ({
    id: 1,
    title: '',
    event_date: '2026-08-21',
    starts_at: '2026-08-22T02:00:00Z',
    venue_name: 'Valley Bar',
    artist_names: ['Ovlov'],
    is_sold_out: false,
    is_cancelled: false,
    ...over,
  }) as SceneWeekShow

const day = (date: string, shows: SceneWeekShow[] = []): SceneWeekDay =>
  ({ date, shows }) as SceneWeekDay

const data = (over: Partial<SceneWindowData> = {}): SceneWindowData => ({
  window: 'this-weekend',
  slug: 'phoenix-az',
  sceneName: 'Phoenix, AZ',
  city: 'Phoenix',
  state: 'AZ',
  timezone: 'America/Phoenix',
  days: [day('2026-08-21', [show()]), day('2026-08-22'), day('2026-08-23')],
  rendered: 1,
  truncated: false,
  trackedVenues: [{ name: 'Valley Bar', slug: 'valley-bar' }],
  widerWindow: 'next-4-weeks',
  ...over,
})

describe('SceneWindowView — window nav', () => {
  // The bug this ticket exists to fix: two chips, one destination.
  it('offers the other three windows at three distinct hrefs', () => {
    render(<SceneWindowView data={data()} />)

    const nav = screen.getByRole('navigation', { name: 'Show windows' })
    const hrefs = within(nav)
      .getAllByRole('link')
      .map(a => a.getAttribute('href'))

    expect(hrefs).toEqual([
      '/scenes/phoenix-az/tonight',
      '/scenes/phoenix-az/week',
      '/scenes/phoenix-az/next-4-weeks',
    ])
    expect(new Set(hrefs).size).toBe(3)
  })

  // The strip is a set of places to GO, matching the sibling pages — neither
  // `/week` nor `/tonight` restates where the reader already is.
  it('does not link the window the reader is already on', () => {
    render(<SceneWindowView data={data({ window: 'next-4-weeks' })} />)

    const nav = screen.getByRole('navigation', { name: 'Show windows' })
    expect(within(nav).queryByRole('link', { name: 'Next 4 weeks' })).toBeNull()
    expect(within(nav).getByRole('link', { name: 'This weekend' })).toHaveAttribute(
      'href',
      '/scenes/phoenix-az/this-weekend'
    )
  })
})

describe('SceneWindowView — header', () => {
  it('names the window and the span it actually rendered', () => {
    render(<SceneWindowView data={data()} />)
    expect(screen.getByText(/This weekend/)).toBeInTheDocument()
    expect(screen.getByText(/Fri, Aug 21 – Sun, Aug 23, 2026/)).toBeInTheDocument()
  })

  it('counts the shows it rendered', () => {
    render(<SceneWindowView data={data()} />)
    expect(screen.getByText(/1 show(?!s)/)).toBeInTheDocument()
  })

  // A capped list must not print a total the page never verified.
  it('says "first N shows" rather than stating a total it did not check', () => {
    render(<SceneWindowView data={data({ rendered: 60, truncated: true })} />)
    expect(screen.getByText(/first 60 shows/)).toBeInTheDocument()
  })
})

describe('SceneWindowView — day rows', () => {
  it('renders a heading per day, quiet nights included', () => {
    render(<SceneWindowView data={data()} />)
    expect(screen.getByText('FRI AUG 21')).toBeInTheDocument()
    expect(screen.getByText('SAT AUG 22')).toBeInTheDocument()
    expect(screen.getByText('SUN AUG 23')).toBeInTheDocument()
  })

  it('links each show', () => {
    render(<SceneWindowView data={data({ days: [day('2026-08-21', [show({ slug: 'ovlov' })])] })} />)
    expect(screen.getByRole('link', { name: /Ovlov/ })).toHaveAttribute('href', '/shows/ovlov')
  })

  // The count on the day the cap landed inside is suppressed, not qualified:
  // the rows are real but their number is not that date's total.
  it('suppresses the count on a day the cap cut', () => {
    render(
      <SceneWindowView
        data={data({
          days: [day('2026-08-21', [show()])],
          rendered: 1,
          truncated: true,
        })}
      />
    )
    const heading = screen.getByText('FRI AUG 21')
    expect(within(heading.parentElement as HTMLElement).queryByText('1')).toBeNull()
  })

  it('keeps the count on a day the cap did not reach', () => {
    render(<SceneWindowView data={data({ days: [day('2026-08-21', [show()])] })} />)
    const heading = screen.getByText('FRI AUG 21')
    expect(within(heading.parentElement as HTMLElement).getByText('1')).toBeInTheDocument()
  })
})

describe('SceneWindowView — quiet window', () => {
  const quiet = data({
    days: [day('2026-08-21'), day('2026-08-22'), day('2026-08-23')],
    rendered: 0,
  })

  // Never asserts the CITY is quiet — only that our calendar is. Coverage is a
  // curated slice of each city's rooms.
  it('speaks only for the rooms we track', () => {
    render(<SceneWindowView data={quiet} />)
    expect(
      screen.getByText(/Nothing on our calendar for the Phoenix rooms we track this weekend/)
    ).toBeInTheDocument()
  })

  // An empty window is still an answer, and it owes the reader one way onward.
  it('offers exactly one step wider', () => {
    render(<SceneWindowView data={quiet} />)
    expect(screen.getByRole('link', { name: /Try next 4 weeks in Phoenix/ })).toHaveAttribute(
      'href',
      '/scenes/phoenix-az/next-4-weeks'
    )
  })

  it('offers no wider window when nothing in the family is wider', () => {
    render(<SceneWindowView data={data({ ...quiet, window: 'next-4-weeks', widerWindow: null })} />)
    expect(screen.queryByRole('link', { name: /^Try / })).toBeNull()
  })

  // The rooms footer is the coverage disclosure for a POPULATED list; the quiet
  // copy already makes the same disclosure in its own words.
  it('does not repeat the rooms footer under the quiet copy', () => {
    render(<SceneWindowView data={quiet} />)
    expect(screen.queryByText(/ROOMS WE TRACK IN PHOENIX/)).toBeNull()
  })
})
