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

  describe('actionsPlacement', () => {
    it('defaults to inline: title keeps flex-1 min-w-0 and sits in a flex-row with actions', () => {
      // Default placement must reproduce the historical side-by-side layout
      // exactly so artist/release/label/festival headers are visually unchanged.
      const { container } = render(
        <EntityHeader title="Test Album" actions={<button>Follow</button>} />
      )
      // The sm:flex-row justify-between row is the inline-layout signature.
      const row = container.querySelector(
        '.flex.flex-col.sm\\:flex-row.sm\\:justify-between'
      )
      expect(row).toBeInTheDocument()
      // Title block constrains itself with flex-1 min-w-0 so the narrow action
      // cluster never squeezes it in inline mode.
      const titleBlock = container.querySelector('.flex-1.min-w-0')
      expect(titleBlock).toBeInTheDocument()
      expect(titleBlock).toContainElement(
        screen.getByRole('heading', { level: 1 })
      )
    })

    it('inline placement matches the explicit default (no flex-row regression)', () => {
      const { container } = render(
        <EntityHeader
          title="Test Album"
          actionsPlacement="inline"
          actions={<button>Follow</button>}
        />
      )
      const row = container.querySelector(
        '.flex.flex-col.sm\\:flex-row.sm\\:justify-between'
      )
      expect(row).toBeInTheDocument()
    })

    it('below placement: title renders full width (no flex-1 min-w-0) with no side-by-side row', () => {
      // PSY-959: VenueDetail's wide button cluster squeezed the title in the
      // inline layout. In 'below' mode the title block must NOT be constrained
      // by flex-1 min-w-0, and the side-by-side justify-between row must be gone.
      const { container } = render(
        <EntityHeader
          title="The Rebel Lounge"
          actionsPlacement="below"
          actions={<button>Follow</button>}
        />
      )
      expect(
        container.querySelector('.sm\\:justify-between')
      ).not.toBeInTheDocument()
      expect(container.querySelector('.flex-1.min-w-0')).not.toBeInTheDocument()
      // Both the title and the actions still render.
      expect(screen.getByRole('heading', { level: 1 })).toHaveTextContent(
        'The Rebel Lounge'
      )
      expect(
        screen.getByRole('button', { name: 'Follow' })
      ).toBeInTheDocument()
    })

    it('below placement keeps the actions wrapper (flex-wrap, sm:shrink-0) for its own row', () => {
      const { container } = render(
        <EntityHeader
          title="The Rebel Lounge"
          actionsPlacement="below"
          actions={<button>Follow</button>}
        />
      )
      const actionsDiv = container.querySelector('.flex-wrap.sm\\:shrink-0')
      expect(actionsDiv).toBeInTheDocument()
      expect(actionsDiv).toContainElement(
        screen.getByRole('button', { name: 'Follow' })
      )
    })
  })
})
