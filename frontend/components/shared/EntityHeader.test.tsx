import { describe, it, expect } from 'vitest'
import { render, screen } from '@testing-library/react'
import { EntityHeader } from './EntityHeader'

describe('EntityHeader', () => {
  it('renders title as an h1 heading', () => {
    render(<EntityHeader title="Test Album" />)
    const heading = screen.getByRole('heading', { level: 1 })
    expect(heading).toHaveTextContent('Test Album')
  })

  it('renders subtitle when provided', () => {
    render(<EntityHeader title="Test Album" subtitle="2024" />)
    expect(screen.getByText('2024')).toBeInTheDocument()
  })

  it('does not render subtitle container when subtitle is not provided', () => {
    const { container } = render(<EntityHeader title="Test Album" />)
    // The subtitle wrapper div should not exist
    const subtitleDiv = container.querySelector('.text-muted-foreground')
    expect(subtitleDiv).not.toBeInTheDocument()
  })

  it('renders actions when provided', () => {
    render(
      <EntityHeader
        title="Test Album"
        actions={<button data-testid="save-btn">Save</button>}
      />
    )
    expect(screen.getByTestId('save-btn')).toBeInTheDocument()
  })

  it('does not render actions container when actions not provided', () => {
    const { container } = render(<EntityHeader title="Test Album" />)
    const actionsDiv = container.querySelector('.sm\\:shrink-0')
    expect(actionsDiv).not.toBeInTheDocument()
  })

  it('stacks header column vertically on mobile and side-by-side at sm breakpoint', () => {
    // Structural check: at <sm the outer row is flex-col so the action cluster
    // drops below the title; at sm+ it becomes flex-row (action cluster sits to the right).
    const { container } = render(
      <EntityHeader title="Test Album" actions={<button>Follow</button>} />
    )
    const row = container.querySelector('.flex.flex-col.sm\\:flex-row')
    expect(row).toBeInTheDocument()
  })

  it('applies sm:shrink-0 (not shrink-0) on actions wrapper so narrow viewports wrap naturally', () => {
    // Regression guard for PSY-467: shrink-0 at all widths made the action
    // cluster push the h1 to 0 width on <640px viewports, clipping the title.
    const { container } = render(
      <EntityHeader title="Test Album" actions={<button>Follow</button>} />
    )
    const actionsDiv = container.querySelector('.sm\\:shrink-0')
    expect(actionsDiv).toBeInTheDocument()
    expect(actionsDiv?.className).not.toMatch(/(^|\s)shrink-0(\s|$)/)
  })

  it('renders subtitle as ReactNode (JSX)', () => {
    render(
      <EntityHeader
        title="Test Album"
        subtitle={<span data-testid="subtitle-span">LP - 2024</span>}
      />
    )
    expect(screen.getByTestId('subtitle-span')).toBeInTheDocument()
    expect(screen.getByTestId('subtitle-span')).toHaveTextContent('LP - 2024')
  })

  it('renders both subtitle and actions together', () => {
    render(
      <EntityHeader
        title="Test Album"
        subtitle="2024"
        actions={<button>Follow</button>}
      />
    )
    expect(screen.getByText('2024')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Follow' })).toBeInTheDocument()
  })

  it('applies custom className', () => {
    const { container } = render(
      <EntityHeader title="Test Album" className="mt-8" />
    )
    expect(container.firstChild).toHaveClass('mt-8')
  })
})
