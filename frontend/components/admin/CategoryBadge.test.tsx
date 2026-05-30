import { describe, it, expect } from 'vitest'
import { render, screen } from '@testing-library/react'

import { CategoryBadge } from './CategoryBadge'

describe('CategoryBadge', () => {
  it('renders the Edit kind with its blue tone', () => {
    render(<CategoryBadge kind="edit" />)

    const badge = screen.getByText('Edit').closest('div')
    expect(badge).toHaveClass('bg-blue-500/10', 'text-blue-700')
  })

  it('renders the Report kind with its amber tone', () => {
    render(<CategoryBadge kind="report" />)

    const badge = screen.getByText('Report').closest('div')
    expect(badge).toHaveClass('bg-amber-500/10', 'text-amber-700')
  })

  it('renders the Comment kind with its violet tone', () => {
    render(<CategoryBadge kind="comment" />)

    const badge = screen.getByText('Comment').closest('div')
    expect(badge).toHaveClass('bg-violet-500/10', 'text-violet-700')
  })

  it('renders an icon alongside the label', () => {
    const { container } = render(<CategoryBadge kind="report" />)

    expect(container.querySelector('svg')).toBeInTheDocument()
  })

  it('keeps shrink-0 and merges a caller className', () => {
    render(<CategoryBadge kind="edit" className="ml-2" />)

    const badge = screen.getByText('Edit').closest('div')
    expect(badge).toHaveClass('shrink-0', 'ml-2')
  })
})
