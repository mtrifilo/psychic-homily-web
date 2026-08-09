import { describe, it, expect, vi } from 'vitest'
import { render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { YearStrip, type YearStripEntry } from './YearStrip'

const years: YearStripEntry[] = [
  { year: 2026, count: 34, href: '/venues/rebel?year=2026#past-shows' },
  { year: 2025, count: 161, href: '/venues/rebel?year=2025#past-shows' },
  { year: 2024, count: 98, href: '/venues/rebel?year=2024#past-shows' },
]

function renderStrip(props: Partial<React.ComponentProps<typeof YearStrip>> = {}) {
  return render(
    <YearStrip
      years={years}
      allYearsHref="/venues/rebel#past-shows"
      ariaLabel="Filter shows by year"
      {...props}
    />
  )
}

describe('YearStrip', () => {
  it('renders nothing when no year has rows', () => {
    const { container } = renderStrip({
      years: [{ year: 2026, count: 0, href: '/x' }],
    })
    expect(container).toBeEmptyDOMElement()
  })

  it('renders nothing when the years list is empty', () => {
    const { container } = renderStrip({ years: [] })
    expect(container).toBeEmptyDOMElement()
  })

  it('labels the nav landmark with the consumer-supplied name', () => {
    renderStrip()
    expect(
      screen.getByRole('navigation', { name: 'Filter shows by year' })
    ).toBeInTheDocument()
  })

  it('renders each year as a real <a href> with its count', () => {
    renderStrip()
    const link = screen.getByRole('link', { name: '2025 (161)' })
    expect(link).toHaveAttribute('href', '/venues/rebel?year=2025#past-shows')
  })

  it('leads with an all-years link', () => {
    renderStrip()
    expect(screen.getByRole('link', { name: 'All years' })).toHaveAttribute(
      'href',
      '/venues/rebel#past-shows'
    )
  })

  it('honors a custom all-years label', () => {
    renderStrip({ allYearsLabel: 'Everything' })
    expect(screen.getByRole('link', { name: 'Everything' })).toBeInTheDocument()
  })

  it('never renders a zero-count year', () => {
    renderStrip({
      years: [...years, { year: 2023, count: 0, href: '/venues/rebel?year=2023' }],
    })
    expect(screen.queryByRole('link', { name: /2023/ })).not.toBeInTheDocument()
  })

  it('marks the current year with aria-current and keeps it a link', () => {
    renderStrip({ currentYear: 2025 })
    const current = screen.getByRole('link', { name: '2025 (161)' })
    expect(current).toHaveAttribute('aria-current', 'page')
    expect(current).toHaveAttribute('href', '/venues/rebel?year=2025#past-shows')
    expect(current.className).toContain('font-bold')
    expect(current.className).toContain('underline')
  })

  it('marks all-years current when no year is selected', () => {
    renderStrip()
    expect(screen.getByRole('link', { name: 'All years' })).toHaveAttribute(
      'aria-current',
      'page'
    )
    expect(
      screen.getByRole('link', { name: '2025 (161)' })
    ).not.toHaveAttribute('aria-current')
  })

  it('drops the all-years aria-current once a year is selected', () => {
    renderStrip({ currentYear: 2024 })
    expect(
      screen.getByRole('link', { name: 'All years' })
    ).not.toHaveAttribute('aria-current')
  })

  it('preserves the consumer ordering rather than sorting', () => {
    renderStrip({ years: [...years].reverse() })
    const rendered = within(screen.getByRole('list'))
      .getAllByRole('link')
      .map((link) => link.textContent)
    expect(rendered).toEqual(['All years', '2024 (98)', '2025 (161)', '2026 (34)'])
  })

  it('structures the years as a list', () => {
    renderStrip()
    expect(
      within(screen.getByRole('list')).getAllByRole('listitem')
    ).toHaveLength(4)
  })

  it('separates years with a decorative dot', () => {
    renderStrip()
    screen.getAllByText('·').forEach((dot) => {
      expect(dot).toHaveAttribute('aria-hidden', 'true')
    })
  })

  it('fires onNavigate with the year, and null for all years', async () => {
    const user = userEvent.setup()
    const onNavigate = vi.fn()
    renderStrip({ onNavigate })
    await user.click(screen.getByRole('link', { name: '2024 (98)' }))
    expect(onNavigate).toHaveBeenCalledWith(2024)
    await user.click(screen.getByRole('link', { name: 'All years' }))
    expect(onNavigate).toHaveBeenCalledWith(null)
  })

  it('renders no disclosure when collapseAfter is unset', () => {
    renderStrip()
    expect(screen.queryByRole('button')).not.toBeInTheDocument()
  })

  it('renders no disclosure when it would hide a single year', () => {
    // 3 years with collapseAfter=2 would hide exactly one; the toggle costs
    // more than it saves.
    renderStrip({ collapseAfter: 2 })
    expect(screen.queryByRole('button')).not.toBeInTheDocument()
  })

  it('hides the tail behind a disclosure past collapseAfter', () => {
    renderStrip({ collapseAfter: 1 })
    const toggle = screen.getByRole('button', { name: /older/ })
    expect(toggle).toHaveAttribute('aria-expanded', 'false')
    expect(screen.getByRole('link', { name: '2026 (34)' })).toBeVisible()
    // Collapsed years leave the accessibility tree entirely, so they are only
    // reachable with `hidden: true`.
    expect(
      screen.queryByRole('link', { name: '2025 (161)' })
    ).not.toBeInTheDocument()
    expect(
      screen.getByRole('link', { name: '2025 (161)', hidden: true })
    ).not.toBeVisible()
  })

  it('keeps collapsed years in the DOM as crawlable links', () => {
    const { container } = renderStrip({ collapseAfter: 1 })
    expect(
      container.querySelector('a[href="/venues/rebel?year=2024#past-shows"]')
    ).toBeInTheDocument()
  })

  it('expands and collapses the tail on toggle', async () => {
    const user = userEvent.setup()
    renderStrip({ collapseAfter: 1 })
    await user.click(screen.getByRole('button', { name: /older/ }))
    const toggle = screen.getByRole('button', { name: /fewer/ })
    expect(toggle).toHaveAttribute('aria-expanded', 'true')
    expect(screen.getByRole('link', { name: '2025 (161)' })).toBeVisible()
    await user.click(toggle)
    expect(
      screen.getByRole('link', { name: '2025 (161)', hidden: true })
    ).not.toBeVisible()
  })

  it('points the disclosure at the year list it controls', () => {
    renderStrip({ collapseAfter: 1 })
    const toggle = screen.getByRole('button', { name: /older/ })
    const controls = toggle.getAttribute('aria-controls')
    expect(controls).toBeTruthy()
    expect(screen.getByRole('list')).toHaveAttribute('id', controls as string)
  })

  it('starts expanded when the selected year sits in the hidden tail', () => {
    renderStrip({ collapseAfter: 1, currentYear: 2024 })
    expect(
      screen.getByRole('button', { name: /fewer/ })
    ).toHaveAttribute('aria-expanded', 'true')
    expect(screen.getByRole('link', { name: '2024 (98)' })).toBeVisible()
  })

  it('starts collapsed when the selected year is already visible', () => {
    renderStrip({ collapseAfter: 1, currentYear: 2026 })
    expect(
      screen.getByRole('button', { name: /older/ })
    ).toHaveAttribute('aria-expanded', 'false')
  })

  it('forwards custom className onto the nav', () => {
    renderStrip({ className: 'mb-4' })
    expect(screen.getByTestId('year-strip').className).toContain('mb-4')
  })
})
