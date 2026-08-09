import { describe, it, expect, vi } from 'vitest'
import { render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { Pagination, paginationWindow } from './Pagination'

const pageHref = (page: number) =>
  page === 1 ? '/venues/rebel#past-shows' : `/venues/rebel?page=${page}#past-shows`

function renderPager(props: Partial<React.ComponentProps<typeof Pagination>> = {}) {
  return render(
    <Pagination
      currentPage={2}
      totalPages={4}
      pageHref={pageHref}
      ariaLabel="Past shows pagination"
      {...props}
    />
  )
}

/** The desktop row is the one carrying the numbered strip and caption. */
const desktop = () => within(screen.getByTestId('pagination-desktop'))
const mobile = () => within(screen.getByTestId('pagination-mobile'))

describe('paginationWindow', () => {
  it('lists every page when there are 7 or fewer', () => {
    expect(paginationWindow(1, 1)).toEqual([1])
    expect(paginationWindow(3, 4)).toEqual([1, 2, 3, 4])
    expect(paginationWindow(4, 7)).toEqual([1, 2, 3, 4, 5, 6, 7])
  })

  it('collapses to first, last and current±1 beyond 7 pages', () => {
    expect(paginationWindow(6, 12)).toEqual([
      1,
      'ellipsis',
      5,
      6,
      7,
      'ellipsis',
      12,
    ])
  })

  it('omits the leading ellipsis when the window touches the first page', () => {
    expect(paginationWindow(2, 12)).toEqual([1, 2, 3, 'ellipsis', 12])
    expect(paginationWindow(1, 12)).toEqual([1, 2, 'ellipsis', 12])
  })

  it('omits the trailing ellipsis when the window touches the last page', () => {
    expect(paginationWindow(11, 12)).toEqual([1, 'ellipsis', 10, 11, 12])
    expect(paginationWindow(12, 12)).toEqual([1, 'ellipsis', 11, 12])
  })

  it('never emits an ellipsis standing in for a single page', () => {
    // current=3 of 9 leaves only page 4 between the window and the tail, so the
    // gap after 4 is real but the one before 2 is not.
    expect(paginationWindow(3, 9)).toEqual([1, 2, 3, 4, 'ellipsis', 9])
  })

  it('clamps out-of-range and fractional input instead of throwing', () => {
    expect(paginationWindow(99, 4)).toEqual([1, 2, 3, 4])
    expect(paginationWindow(0, 4)).toEqual([1, 2, 3, 4])
    expect(paginationWindow(-5, 3)).toEqual([1, 2, 3])
    expect(paginationWindow(2.7, 4)).toEqual([1, 2, 3, 4])
    expect(paginationWindow(1, 0)).toEqual([1])
  })
})

describe('Pagination', () => {
  it('renders nothing at one page or fewer', () => {
    const { container } = renderPager({ totalPages: 1 })
    expect(container).toBeEmptyDOMElement()
  })

  it('renders every page as a real crawlable <a href>', () => {
    renderPager()
    const links = desktop().getAllByRole('link')
    const pageLinks = links.filter((link) => /^Page \d/.test(link.textContent ?? ''))
    expect(pageLinks.map((link) => link.getAttribute('href'))).toEqual([
      '/venues/rebel#past-shows',
      '/venues/rebel?page=2#past-shows',
      '/venues/rebel?page=3#past-shows',
      '/venues/rebel?page=4#past-shows',
    ])
  })

  it('passes anchor suffixes through untouched from the href builder', () => {
    renderPager()
    expect(desktop().getByRole('link', { name: /Page 3/ })).toHaveAttribute(
      'href',
      '/venues/rebel?page=3#past-shows'
    )
  })

  it('labels the nav landmark with the consumer-supplied name', () => {
    renderPager()
    expect(
      screen.getByRole('navigation', { name: 'Past shows pagination' })
    ).toBeInTheDocument()
  })

  it('structures the page links as a list', () => {
    renderPager()
    const list = desktop().getByRole('list')
    expect(within(list).getAllByRole('listitem')).toHaveLength(4)
  })

  it('marks the current page with aria-current while keeping it a link', () => {
    renderPager()
    const current = desktop().getByRole('link', { name: /Page 2/ })
    expect(current).toHaveAttribute('aria-current', 'page')
    expect(current).toHaveAttribute('href', '/venues/rebel?page=2#past-shows')
  })

  it('distinguishes the current page by weight and underline, not color alone', () => {
    renderPager()
    const current = desktop().getByRole('link', { name: /Page 2/ })
    expect(current.className).toContain('font-bold')
    expect(current.className).toContain('underline')
  })

  it('leaves aria-current off every other page', () => {
    renderPager()
    expect(desktop().getByRole('link', { name: /Page 3/ })).not.toHaveAttribute(
      'aria-current'
    )
  })

  it('gives numeric links visually-hidden "Page N" text', () => {
    renderPager()
    const link = desktop().getByRole('link', { name: /Page 3/ })
    const hidden = link.querySelector('.sr-only')
    expect(hidden).toHaveTextContent('Page 3')
  })

  it('renders range labels alongside the numerals when provided', () => {
    renderPager({
      rangeLabels: { 1: 'Sep–Dec', 2: 'Jun–Sep', 3: 'Mar–Jun', 4: 'Jan–Feb' },
    })
    expect(
      desktop().getByRole('link', { name: 'Page 2, Jun–Sep' })
    ).toBeInTheDocument()
    // Visible rendering is "3 · Mar–Jun" next to the numeral.
    expect(
      desktop().getByRole('link', { name: 'Page 3, Mar–Jun' })
    ).toHaveTextContent('3 · Mar–Jun')
  })

  it('falls back to plain numerals when no range labels are given', () => {
    renderPager()
    expect(desktop().getByRole('link', { name: 'Page 2' })).toBeInTheDocument()
    expect(desktop().queryByText('·')).not.toBeInTheDocument()
  })

  it('labels only the pages it has labels for', () => {
    renderPager({ rangeLabels: { 2: 'Jun–Sep' } })
    expect(
      desktop().getByRole('link', { name: 'Page 2, Jun–Sep' })
    ).toBeInTheDocument()
    expect(desktop().getByRole('link', { name: 'Page 3' })).toBeInTheDocument()
  })

  it('hides the ellipsis from assistive tech and keeps it unfocusable', () => {
    renderPager({ currentPage: 6, totalPages: 12 })
    const ellipses = screen
      .getAllByText('…')
      .filter((node) => node.closest('[data-testid="pagination-desktop"]'))
    expect(ellipses).toHaveLength(2)
    ellipses.forEach((node) => {
      expect(node).toHaveAttribute('aria-hidden', 'true')
      expect(node.querySelector('a, button')).toBeNull()
    })
  })

  it('keeps disabled boundary controls in the DOM on the first page', () => {
    renderPager({ currentPage: 1 })
    const previous = screen.getAllByTestId('pagination-previous-disabled')
    expect(previous.length).toBeGreaterThan(0)
    previous.forEach((node) => {
      expect(node).toHaveTextContent('Newer')
      expect(node.className).toContain('pointer-events-none')
      expect(node).toHaveAttribute('aria-hidden', 'true')
    })
    expect(desktop().getByRole('link', { name: /Older/ })).toBeInTheDocument()
  })

  it('keeps disabled boundary controls in the DOM on the last page', () => {
    renderPager({ currentPage: 4 })
    expect(
      screen.getAllByTestId('pagination-next-disabled').length
    ).toBeGreaterThan(0)
    expect(desktop().getByRole('link', { name: /Newer/ })).toBeInTheDocument()
  })

  it('points the boundary controls at the adjacent pages', () => {
    renderPager({ currentPage: 2 })
    expect(desktop().getByRole('link', { name: /Newer/ })).toHaveAttribute(
      'href',
      '/venues/rebel#past-shows'
    )
    expect(desktop().getByRole('link', { name: /Older/ })).toHaveAttribute(
      'href',
      '/venues/rebel?page=3#past-shows'
    )
  })

  it('honors custom boundary labels for non-chronological lists', () => {
    renderPager({ previousLabel: 'Previous', nextLabel: 'Next' })
    expect(
      desktop().getByRole('link', { name: /Previous/ })
    ).toBeInTheDocument()
    expect(desktop().getByRole('link', { name: /Next/ })).toBeInTheDocument()
  })

  it('captions with the row range and page position', () => {
    renderPager({ captionRange: { start: 51, end: 100, total: 161 } })
    expect(
      desktop().getByText('Showing 51–100 of 161 · Page 2 of 4')
    ).toBeInTheDocument()
  })

  it('captions with page position alone when no row range is given', () => {
    renderPager()
    expect(desktop().getByText('Page 2 of 4')).toBeInTheDocument()
  })

  it('collapses to position plus current range on mobile', () => {
    renderPager({ rangeLabels: { 2: 'Jun–Sep' } })
    const row = mobile()
    expect(row.getByText(/Page 2 of 4/)).toBeInTheDocument()
    expect(row.getByText('Jun–Sep')).toBeInTheDocument()
    expect(row.queryByRole('list')).not.toBeInTheDocument()
  })

  it('gives the mobile boundary controls 44px touch targets', () => {
    renderPager()
    mobile()
      .getAllByRole('link')
      .forEach((link) => {
        expect(link.className).toContain('min-h-11')
        expect(link.className).toContain('min-w-11')
      })
  })

  it('fires onNavigate with the target page alongside navigation', async () => {
    const user = userEvent.setup()
    const onNavigate = vi.fn()
    renderPager({ onNavigate })
    await user.click(desktop().getByRole('link', { name: /Page 3/ }))
    expect(onNavigate).toHaveBeenCalledWith(3)
  })

  it('fires onNavigate from the boundary controls too', async () => {
    const user = userEvent.setup()
    const onNavigate = vi.fn()
    renderPager({ onNavigate })
    await user.click(desktop().getByRole('link', { name: /Newer/ }))
    expect(onNavigate).toHaveBeenCalledWith(1)
  })

  it('ships a polite live region that stays silent on first render', () => {
    renderPager()
    const status = screen.getByRole('status')
    expect(status).toHaveAttribute('aria-live', 'polite')
    expect(status).toBeEmptyDOMElement()
  })

  it('announces the new position after a client-side page change', () => {
    const { rerender } = renderPager({ rangeLabels: { 3: 'Mar–Jun' } })
    rerender(
      <Pagination
        currentPage={3}
        totalPages={4}
        pageHref={pageHref}
        ariaLabel="Past shows pagination"
        rangeLabels={{ 3: 'Mar–Jun' }}
      />
    )
    expect(screen.getByRole('status')).toHaveTextContent('Page 3 of 4, Mar–Jun')
  })

  it('clamps an out-of-range current page onto the last page', () => {
    renderPager({ currentPage: 99 })
    expect(desktop().getByText('Page 4 of 4')).toBeInTheDocument()
    expect(
      desktop().getByRole('link', { name: /Page 4/ })
    ).toHaveAttribute('aria-current', 'page')
  })

  it('groups large caption numbers deterministically rather than by runtime locale', () => {
    // Locale-dependent grouping would hydrate into a mismatch for viewers whose
    // browser groups digits differently from the server.
    renderPager({ captionRange: { start: 1001, end: 2000, total: 12345 } })
    expect(
      desktop().getByText('Showing 1,001–2,000 of 12,345 · Page 2 of 4')
    ).toBeInTheDocument()
  })

  it('forwards custom className onto the nav', () => {
    renderPager({ className: 'mt-6' })
    expect(screen.getByTestId('pagination').className).toContain('mt-6')
  })
})
