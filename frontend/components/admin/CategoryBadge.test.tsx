import { describe, it, expect } from 'vitest'
import { render, screen } from '@testing-library/react'

import { CategoryBadge } from './CategoryBadge'

describe('CategoryBadge', () => {
  // PSY-943: tones bound to the DS categorical chart palette, not raw hues.
  it('renders the Edit kind with the chart-6 (denim) tone', () => {
    render(<CategoryBadge kind="edit" />)

    const badge = screen.getByText('Edit').closest('div')
    expect(badge).toHaveClass('bg-chart-6/10', 'text-chart-6')
  })

  it('renders the Report kind with the chart-3 (gold) tone', () => {
    render(<CategoryBadge kind="report" />)

    const badge = screen.getByText('Report').closest('div')
    expect(badge).toHaveClass('bg-chart-3/10', 'text-chart-3')
  })

  it('renders the Comment kind with the chart-7 (plum) tone', () => {
    render(<CategoryBadge kind="comment" />)

    const badge = screen.getByText('Comment').closest('div')
    expect(badge).toHaveClass('bg-chart-7/10', 'text-chart-7')
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

  // PSY-871: the Request kind honors the locked purple (#a855f7); the chart
  // palette has no second purple besides comment's plum, so this stays a raw
  // tint until PSY-872 token-ifies the moderation palette.
  it('renders the Request kind with the purple tint and an icon', () => {
    const { container } = render(<CategoryBadge kind="request" />)

    const badge = screen.getByText('Request').closest('div')
    expect(badge?.className).toContain('#a855f7')
    expect(badge?.className).toContain('text-[#7e22ce]')
    expect(container.querySelector('svg')).toBeInTheDocument()
  })
})
