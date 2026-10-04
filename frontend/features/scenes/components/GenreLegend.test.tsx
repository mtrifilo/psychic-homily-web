import { describe, it, expect, vi } from 'vitest'
import { fireEvent, render, screen } from '@testing-library/react'
import { GenreLegend } from './GenreLegend'

describe('GenreLegend', () => {
  const toggle = () => screen.getByRole('button', { name: 'Genres' })

  it('is a 36px chip below lg and the fixed-width key from lg', () => {
    const { container } = render(<GenreLegend open={false} onToggle={() => {}} />)
    expect(toggle()).toHaveClass('min-h-9', 'text-sm', 'lg:min-h-0', 'lg:text-xs')
    expect(container.firstElementChild).toHaveClass('lg:w-44')
    expect(container.firstElementChild).not.toHaveClass('w-44')
  })

  it('takes its placement from the caller', () => {
    const { container } = render(
      <GenreLegend open={false} onToggle={() => {}} className="lg:absolute lg:right-4" />,
    )
    expect(container.firstElementChild).toHaveClass('lg:absolute', 'lg:right-4')
    expect(container.firstElementChild).not.toHaveClass('absolute')
  })

  it('hides the key while collapsed and reports the toggle', () => {
    const onToggle = vi.fn()
    render(<GenreLegend open={false} onToggle={onToggle} />)
    expect(toggle()).toHaveAttribute('aria-expanded', 'false')
    expect(document.getElementById('atlas-genre-legend')).not.toBeVisible()
    fireEvent.click(toggle())
    expect(onToggle).toHaveBeenCalledTimes(1)
  })

  it('shows every family while open', () => {
    render(<GenreLegend open onToggle={() => {}} />)
    expect(toggle()).toHaveAttribute('aria-expanded', 'true')
    expect(document.getElementById('atlas-genre-legend')).toBeVisible()
    expect(screen.getByText('Mixed / no data')).toBeInTheDocument()
  })
})
