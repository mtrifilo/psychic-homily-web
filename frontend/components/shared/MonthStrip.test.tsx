import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MonthStrip, type MonthStripEntry } from './MonthStrip'

vi.mock('next/link', () => ({
  default: ({
    href,
    children,
    ...props
  }: {
    href: string
    children: React.ReactNode
  }) => (
    <a href={href} {...props}>
      {children}
    </a>
  ),
}))

/** The shape the Phoenix histogram actually has: one year boundary. */
const months: MonthStripEntry[] = [
  { year: 2026, month: 9, count: 64 },
  { year: 2026, month: 10, count: 112 },
  { year: 2026, month: 11, count: 68 },
  { year: 2026, month: 12, count: 13 },
  { year: 2027, month: 1, count: 2 },
  { year: 2027, month: 2, count: 5 },
  { year: 2027, month: 3, count: 4 },
]

const hrefFor = (year: number, month: number) =>
  `/shows/${year}/${String(month).padStart(2, '0')}`

function renderStrip(props: Partial<React.ComponentProps<typeof MonthStrip>> = {}) {
  return render(
    <MonthStrip
      months={months}
      hrefFor={hrefFor}
      allHref="/shows"
      allCount={268}
      ariaLabel="Filter shows by month"
      {...props}
    />
  )
}

describe('MonthStrip', () => {
  it('leads with the unscoped link and its total', () => {
    renderStrip()

    const all = screen.getByRole('link', { name: /All upcoming/ })
    expect(all).toHaveAttribute('href', '/shows')
    expect(all).toHaveTextContent('All upcoming (268)')
    expect(all).toHaveAttribute('aria-current', 'page')
  })

  it('builds every month href through the supplied builder', () => {
    renderStrip()

    expect(screen.getByRole('link', { name: 'Sep (64)' })).toHaveAttribute(
      'href',
      '/shows/2026/09'
    )
    expect(screen.getByRole('link', { name: 'Oct (112)' })).toHaveAttribute(
      'href',
      '/shows/2026/10'
    )
  })

  it('marks the month in view as current', () => {
    renderStrip({ current: { year: 2026, month: 11 } })

    expect(screen.getByRole('link', { name: 'Nov (68)' })).toHaveAttribute(
      'aria-current',
      'page'
    )
    expect(screen.getByRole('link', { name: /All upcoming/ })).not.toHaveAttribute(
      'aria-current'
    )
  })

  it('drops months with no rows: an empty month is a dead end', () => {
    renderStrip({
      months: [
        { year: 2026, month: 9, count: 64 },
        { year: 2026, month: 10, count: 0 },
      ],
    })

    expect(screen.queryByRole('link', { name: /^Oct/ })).toBeNull()
  })

  it('renders nothing when no month has rows', () => {
    const { container } = renderStrip({ months: [] })

    expect(container.firstChild).toBeNull()
  })

  describe('the next-year fold', () => {
    it('folds later years behind a year token carrying their total', () => {
      renderStrip()

      const token = screen.getByTestId('month-strip-year-2027')
      expect(token).toHaveTextContent('2027 ▸ (11)')
      expect(token).toHaveAttribute('aria-expanded', 'false')
    })

    // Hidden for a reader, present for a crawler: the hrefs stay in the DOM.
    it('keeps the folded months in the DOM as real links', () => {
      const { container } = renderStrip()

      const janLink = container.querySelector('a[href="/shows/2027/01"]')
      expect(janLink).not.toBeNull()
      expect(janLink?.closest('li')).toHaveAttribute('hidden')
    })

    it('unfolds them when the token is activated', async () => {
      const user = userEvent.setup()
      const { container } = renderStrip()

      await user.click(screen.getByTestId('month-strip-year-2027'))

      expect(screen.getByTestId('month-strip-year-2027')).toHaveAttribute(
        'aria-expanded',
        'true'
      )
      expect(
        container.querySelector('a[href="/shows/2027/01"]')?.closest('li')
      ).not.toHaveAttribute('hidden')
    })

    it('leaves the leading year s months unfolded', () => {
      const { container } = renderStrip()

      expect(
        container.querySelector('a[href="/shows/2026/09"]')?.closest('li')
      ).not.toHaveAttribute('hidden')
    })

    // A reader must be able to see where they are without opening a disclosure
    // first.
    it('lands open when the month in view is inside a folded year', () => {
      renderStrip({ current: { year: 2027, month: 2 } })

      expect(screen.getByTestId('month-strip-year-2027')).toHaveAttribute(
        'aria-expanded',
        'true'
      )
      expect(screen.getByRole('link', { name: 'Feb (5)' })).toHaveAttribute(
        'aria-current',
        'page'
      )
    })

    it('folds nothing when every month shares one year', () => {
      renderStrip({ months: months.filter(entry => entry.year === 2026) })

      expect(screen.queryByTestId('month-strip-year-2027')).toBeNull()
    })

    it('gives each later year its own token', () => {
      renderStrip({
        months: [
          { year: 2026, month: 12, count: 13 },
          { year: 2027, month: 1, count: 2 },
          { year: 2028, month: 1, count: 1 },
        ],
      })

      expect(screen.getByTestId('month-strip-year-2027')).toHaveTextContent(
        '2027 ▸ (2)'
      )
      expect(screen.getByTestId('month-strip-year-2028')).toHaveTextContent(
        '2028 ▸ (1)'
      )
    })
  })

  it('reports the month a reader picked', async () => {
    const user = userEvent.setup()
    const onNavigate = vi.fn()
    renderStrip({ onNavigate })

    await user.click(screen.getByRole('link', { name: 'Oct (112)' }))

    expect(onNavigate).toHaveBeenCalledWith({ year: 2026, month: 10 })
  })

  it('reports null for the unscoped link', async () => {
    const user = userEvent.setup()
    const onNavigate = vi.fn()
    renderStrip({ onNavigate, current: { year: 2026, month: 10 } })

    await user.click(screen.getByRole('link', { name: /All upcoming/ }))

    expect(onNavigate).toHaveBeenCalledWith(null)
  })
})
