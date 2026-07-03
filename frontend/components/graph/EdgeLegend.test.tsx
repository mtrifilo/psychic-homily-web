import { describe, it, expect, vi } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/react'
import { EdgeLegend } from './EdgeLegend'

// PSY-1083: interactive edge legend — type rows with swatch + label +
// live count, and show/hide toggles that generalize the artist graph's
// activeTypes mechanic. fireEvent (not userEvent) per the project's
// blur-timer flake guidance — no focus management needed here.

describe('EdgeLegend', () => {
  it('renders one row per type with label and live count', () => {
    render(
      <EdgeLegend
        types={['shared_bills', 'similar']}
        counts={new Map([['shared_bills', 12], ['similar', 3]])}
      />,
    )
    expect(screen.getByText('Shared Bills')).toBeInTheDocument()
    expect(screen.getByText('12')).toBeInTheDocument()
    expect(screen.getByText('Similar')).toBeInTheDocument()
    expect(screen.getByText('3')).toBeInTheDocument()
  })

  it('orders rows canonically with unknown types after the grammar set', () => {
    render(
      <EdgeLegend
        types={['played_at', 'member_of', 'similar']}
        counts={new Map()}
        onToggleType={() => {}}
      />,
    )
    const labels = screen.getAllByRole('button').map(b => b.textContent)
    expect(labels).toEqual(['Similar0', 'Member Of0', 'Played at0'])
  })

  it('humanizes unknown collection-derived types (PSY-555 tolerance)', () => {
    render(<EdgeLegend types={['show_lineup']} />)
    expect(screen.getByText('Show lineup')).toBeInTheDocument()
  })

  it('fires onToggleType with the clicked type', () => {
    const onToggle = vi.fn()
    render(
      <EdgeLegend
        types={['shared_bills', 'shared_label']}
        counts={new Map([['shared_bills', 2], ['shared_label', 1]])}
        onToggleType={onToggle}
      />,
    )
    const button = screen.getByRole('button', { name: /shared label/i })
    // The hover affordance explains the toggle action.
    expect(button).toHaveAttribute('title', 'Hide Shared Label connections')
    fireEvent.click(button)
    expect(onToggle).toHaveBeenCalledTimes(1)
    expect(onToggle).toHaveBeenCalledWith('shared_label')
  })

  it('marks hidden types as un-pressed toggles with a dimmed row', () => {
    render(
      <EdgeLegend
        types={['shared_bills', 'similar']}
        hiddenTypes={new Set(['similar'])}
        onToggleType={() => {}}
      />,
    )
    const hiddenRow = screen.getByRole('button', { name: /similar/i })
    expect(hiddenRow).toHaveAttribute('aria-pressed', 'false')
    expect(hiddenRow).toHaveAttribute('title', 'Show Similar connections')
    expect(hiddenRow.className).toContain('opacity-40')
    const visibleRow = screen.getByRole('button', { name: /shared bills/i })
    expect(visibleRow).toHaveAttribute('aria-pressed', 'true')
  })

  it('renders static (non-button) rows when no toggle handler is provided', () => {
    render(<EdgeLegend types={['similar']} counts={new Map([['similar', 4]])} />)
    expect(screen.queryByRole('button')).not.toBeInTheDocument()
    expect(screen.getByText('Similar')).toBeInTheDocument()
  })

  it('omits counts entirely when no counts map is provided (artist-graph parity mode)', () => {
    render(<EdgeLegend types={['similar']} />)
    expect(screen.queryByText('0')).not.toBeInTheDocument()
  })

  it('encodes the dash pattern in the swatch (WCAG 1.4.1 redundancy)', () => {
    const { container } = render(<EdgeLegend types={['shared_label', 'similar']} />)
    const lines = container.querySelectorAll('svg line')
    expect(lines).toHaveLength(2)
    // Canonical order: similar (solid) first, shared_label (dashed 5 5) second.
    expect(lines[0]).not.toHaveAttribute('stroke-dasharray')
    expect(lines[1]).toHaveAttribute('stroke-dasharray', '5 5')
    // Swatch color is the theme var() expression — theme-reactive in DOM.
    expect(lines[1]).toHaveAttribute('stroke', 'var(--edge-shared-label, #c084fc)')
  })

  it('shows the weight-scale affordance by default and hides it on request', () => {
    const { rerender } = render(<EdgeLegend types={['similar']} />)
    expect(screen.getByText('Thicker = stronger signal')).toBeInTheDocument()
    rerender(<EdgeLegend types={['similar']} showWeightHint={false} />)
    expect(screen.queryByText('Thicker = stronger signal')).not.toBeInTheDocument()
  })

  it('renders nothing for an empty type set', () => {
    const { container } = render(<EdgeLegend types={[]} />)
    expect(container).toBeEmptyDOMElement()
  })
})

// PSY-1334: the solo ("only") affordance — isolate one edge type without
// mutating the hidden set underneath.
describe('EdgeLegend solo', () => {
  const soloProps = {
    types: ['shared_bills', 'shared_label'],
    counts: new Map<string, number>(),
    onToggleType: () => {},
  }

  it('renders an "only" button per row only when onSoloType is provided', () => {
    const { rerender } = render(<EdgeLegend {...soloProps} />)
    expect(screen.queryByRole('button', { name: /Show only/ })).not.toBeInTheDocument()
    rerender(<EdgeLegend {...soloProps} onSoloType={() => {}} />)
    expect(
      screen.getByRole('button', { name: 'Show only Shared Bills connections' }),
    ).toBeInTheDocument()
    expect(
      screen.getByRole('button', { name: 'Show only Shared Label connections' }),
    ).toBeInTheDocument()
  })

  it('solos on click and clears when the soloed row is clicked again', () => {
    const onSoloType = vi.fn()
    const { rerender } = render(<EdgeLegend {...soloProps} onSoloType={onSoloType} />)
    fireEvent.click(screen.getByRole('button', { name: 'Show only Shared Bills connections' }))
    expect(onSoloType).toHaveBeenCalledWith('shared_bills')

    rerender(<EdgeLegend {...soloProps} soloType="shared_bills" onSoloType={onSoloType} />)
    // The soloed row's button flips to the clear action.
    fireEvent.click(screen.getByRole('button', { name: 'Show all connection types' }))
    expect(onSoloType).toHaveBeenCalledWith(null)
  })

  it('dims every non-soloed row while solo is active, overriding hidden state', () => {
    render(
      <EdgeLegend
        {...soloProps}
        // shared_bills is hidden — but soloed, so it renders FULL opacity
        // (the solo view is what the simulation shows).
        hiddenTypes={new Set(['shared_bills'])}
        soloType="shared_bills"
        onSoloType={() => {}}
      />,
    )
    // Row toggle buttons are named by their visible content (swatch + label
    // + count); the "only" buttons carry aria-labels, so exclude them.
    const rowButtons = screen
      .getAllByRole('button')
      .filter(b => !(b.getAttribute('aria-label') ?? '').includes('only'))
    const bills = rowButtons.find(b => b.textContent?.includes('Shared Bills'))!
    const label = rowButtons.find(b => b.textContent?.includes('Shared Label'))!
    expect(bills.className).toContain('opacity-100')
    expect(label.className).toContain('opacity-40')
  })

  it('discloses the active solo filter (never silent)', () => {
    const { rerender } = render(<EdgeLegend {...soloProps} onSoloType={() => {}} />)
    expect(screen.queryByText(/Showing only/)).not.toBeInTheDocument()
    rerender(<EdgeLegend {...soloProps} soloType="shared_label" onSoloType={() => {}} />)
    expect(screen.getByText('Showing only Shared Label connections')).toBeInTheDocument()
  })
})
