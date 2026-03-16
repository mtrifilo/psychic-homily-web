import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import { Breadcrumb } from './Breadcrumb'

// Mock next/link
vi.mock('next/link', () => ({
  default: ({ href, children, ...props }: { href: string; children: React.ReactNode; [key: string]: unknown }) => (
    <a href={href} {...props}>{children}</a>
  ),
}))

// Track mock breadcrumbs
let mockBreadcrumbs: Array<{ label: string; href: string }> = []

vi.mock('@/lib/context/NavigationBreadcrumbContext', () => ({
  useNavigationBreadcrumbs: () => ({
    breadcrumbs: mockBreadcrumbs,
    pushBreadcrumb: vi.fn(),
  }),
}))

describe('Breadcrumb', () => {
  it('renders fallback when no navigation history', () => {
    mockBreadcrumbs = []
    render(
      <Breadcrumb
        fallback={{ href: '/artists', label: 'Artists' }}
        currentPage="Macie Stewart"
      />
    )

    const nav = screen.getByRole('navigation', { name: /Breadcrumb/ })
    expect(nav).toBeInTheDocument()

    const link = screen.getByRole('link', { name: 'Artists' })
    expect(link).toHaveAttribute('href', '/artists')

    expect(screen.getByText('Macie Stewart')).toBeInTheDocument()
  })

  it('renders navigation history when breadcrumbs exist', () => {
    mockBreadcrumbs = [
      { label: 'Shows', href: '/shows' },
      { label: 'Jeff Tweedy at Van Buren', href: '/shows/jeff-tweedy' },
    ]
    render(
      <Breadcrumb
        fallback={{ href: '/artists', label: 'Artists' }}
        currentPage="Macie Stewart"
      />
    )

    expect(screen.getByRole('link', { name: 'Shows' })).toHaveAttribute('href', '/shows')
    expect(screen.getByRole('link', { name: 'Jeff Tweedy at Van Buren' })).toHaveAttribute('href', '/shows/jeff-tweedy')
    expect(screen.getByText('Macie Stewart')).toBeInTheDocument()
  })

  it('current page is not a link', () => {
    mockBreadcrumbs = []
    render(
      <Breadcrumb
        fallback={{ href: '/artists', label: 'Artists' }}
        currentPage="Macie Stewart"
      />
    )

    const currentPageElement = screen.getByText('Macie Stewart')
    expect(currentPageElement.tagName).toBe('SPAN')
    expect(currentPageElement.closest('a')).toBeNull()
  })

  it('renders separator between crumbs', () => {
    mockBreadcrumbs = [
      { label: 'Shows', href: '/shows' },
    ]
    render(
      <Breadcrumb
        fallback={{ href: '/artists', label: 'Artists' }}
        currentPage="Artist Name"
      />
    )

    // Check for separator characters (rsaquo)
    const separators = screen.getAllByText((_, element) =>
      element?.getAttribute('aria-hidden') === 'true' && element?.textContent === '\u203A'
    )
    expect(separators.length).toBeGreaterThanOrEqual(1)
  })

  it('renders as an ordered list for accessibility', () => {
    mockBreadcrumbs = []
    render(
      <Breadcrumb
        fallback={{ href: '/artists', label: 'Artists' }}
        currentPage="Test"
      />
    )

    const list = screen.getByRole('list')
    expect(list).toBeInTheDocument()
    const items = screen.getAllByRole('listitem')
    expect(items).toHaveLength(2) // fallback + current page
  })
})
