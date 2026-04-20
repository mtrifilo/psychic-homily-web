import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { EntityDetailLayout } from './EntityDetailLayout'

// Mock next/link to render a regular anchor
vi.mock('next/link', () => ({
  default: ({ href, children, ...props }: { href: string; children: React.ReactNode; [key: string]: unknown }) => (
    <a href={href} {...props}>{children}</a>
  ),
}))

const defaultProps = {
  fallback: { href: '/releases', label: 'Releases' },
  entityName: 'Album Name',
  header: <div data-testid="test-header">Album Name</div>,
  tabs: [
    { value: 'overview', label: 'Overview' },
    { value: 'links', label: 'Listen/Buy' },
  ],
  activeTab: 'overview',
  onTabChange: vi.fn(),
  children: <div data-testid="tab-content">Tab content here</div>,
}

describe('EntityDetailLayout', () => {
  it('renders breadcrumb with fallback link and entity name', () => {
    render(<EntityDetailLayout {...defaultProps} />)
    const link = screen.getByRole('link', { name: /Releases/ })
    expect(link).toBeInTheDocument()
    expect(link).toHaveAttribute('href', '/releases')
    // Entity name appears in both breadcrumb and header
    const matches = screen.getAllByText('Album Name')
    expect(matches.length).toBeGreaterThanOrEqual(2)
  })

  it('renders breadcrumb navigation', () => {
    render(<EntityDetailLayout {...defaultProps} />)
    expect(screen.getByRole('navigation', { name: /Breadcrumb/ })).toBeInTheDocument()
  })

  it('renders header content', () => {
    render(<EntityDetailLayout {...defaultProps} />)
    expect(screen.getByTestId('test-header')).toBeInTheDocument()
  })

  it('renders tab triggers for each tab', () => {
    render(<EntityDetailLayout {...defaultProps} />)
    expect(screen.getByRole('tab', { name: 'Overview' })).toBeInTheDocument()
    expect(screen.getByRole('tab', { name: 'Listen/Buy' })).toBeInTheDocument()
  })

  it('renders children (tab content)', () => {
    render(<EntityDetailLayout {...defaultProps} />)
    expect(screen.getByTestId('tab-content')).toBeInTheDocument()
  })

  it('calls onTabChange when a tab is clicked', async () => {
    const user = userEvent.setup()
    const onTabChange = vi.fn()
    render(<EntityDetailLayout {...defaultProps} onTabChange={onTabChange} />)

    await user.click(screen.getByRole('tab', { name: 'Listen/Buy' }))
    expect(onTabChange).toHaveBeenCalledWith('links')
  })

  it('renders sidebar when provided', () => {
    render(
      <EntityDetailLayout
        {...defaultProps}
        sidebar={<div data-testid="sidebar-content">Sidebar info</div>}
      />
    )
    expect(screen.getByTestId('sidebar-content')).toBeInTheDocument()
    expect(screen.getByRole('complementary')).toBeInTheDocument()
  })

  it('does not render aside element when sidebar is not provided', () => {
    render(<EntityDetailLayout {...defaultProps} />)
    expect(screen.queryByRole('complementary')).not.toBeInTheDocument()
  })

  it('renders header inside a header element', () => {
    render(<EntityDetailLayout {...defaultProps} />)
    const headerEl = screen.getByTestId('test-header').closest('header')
    expect(headerEl).toBeInTheDocument()
  })

  it('renders tablist', () => {
    render(<EntityDetailLayout {...defaultProps} />)
    expect(screen.getByRole('tablist')).toBeInTheDocument()
  })

  it('applies custom className', () => {
    const { container } = render(
      <EntityDetailLayout {...defaultProps} className="bg-gray-100" />
    )
    expect(container.firstChild).toHaveClass('bg-gray-100')
  })

  it('renders with a single tab', () => {
    render(
      <EntityDetailLayout
        {...defaultProps}
        tabs={[{ value: 'overview', label: 'Overview' }]}
      />
    )
    const tabs = screen.getAllByRole('tab')
    expect(tabs).toHaveLength(1)
  })

  it('renders with many tabs', () => {
    render(
      <EntityDetailLayout
        {...defaultProps}
        tabs={[
          { value: 'overview', label: 'Overview' },
          { value: 'tracks', label: 'Tracks' },
          { value: 'links', label: 'Listen/Buy' },
          { value: 'credits', label: 'Credits' },
        ]}
        activeTab="overview"
      />
    )
    const tabs = screen.getAllByRole('tab')
    expect(tabs).toHaveLength(4)
  })

  describe('flat (no-tabs) shape', () => {
    const flatProps = {
      fallback: { href: '/shows', label: 'Shows' },
      entityName: 'Test Show',
      header: <div data-testid="test-header">Test Show</div>,
      children: <div data-testid="flat-content">Flat content</div>,
    }

    it('renders without a tablist when tabs are omitted', () => {
      render(<EntityDetailLayout {...flatProps} />)
      expect(screen.queryByRole('tablist')).not.toBeInTheDocument()
      expect(screen.queryByRole('tab')).not.toBeInTheDocument()
    })

    it('renders without a tablist when tabs is an empty array', () => {
      render(<EntityDetailLayout {...flatProps} tabs={[]} />)
      expect(screen.queryByRole('tablist')).not.toBeInTheDocument()
    })

    it('still renders header, breadcrumb, and children in flat mode', () => {
      render(<EntityDetailLayout {...flatProps} />)
      expect(screen.getByTestId('test-header')).toBeInTheDocument()
      expect(screen.getByRole('navigation', { name: /Breadcrumb/ })).toBeInTheDocument()
      expect(screen.getByTestId('flat-content')).toBeInTheDocument()
    })

    it('still renders sidebar in flat mode when provided', () => {
      render(
        <EntityDetailLayout
          {...flatProps}
          sidebar={<div data-testid="sidebar-content">Sidebar info</div>}
        />
      )
      expect(screen.getByTestId('sidebar-content')).toBeInTheDocument()
      expect(screen.getByRole('complementary')).toBeInTheDocument()
    })
  })
})
