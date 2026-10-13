import { describe, it, expect, vi } from 'vitest'
import { fireEvent, render, screen } from '@testing-library/react'
import { GenreLegend } from './GenreLegend'

describe('GenreLegend', () => {
  const toggle = () => screen.getByRole('button', { name: 'Genres' })
  const key = () => document.getElementById('atlas-genre-legend')

  it('is a chip as tall as Drift when compact and the fixed-width key otherwise', () => {
    const { container, rerender } = render(
      <GenreLegend compact openChoice={false} onOpenChange={() => {}} />,
    )
    expect(toggle()).toHaveClass('min-h-9', 'text-sm')
    expect(toggle()).not.toHaveClass('text-xs')
    expect(container.firstElementChild).not.toHaveClass('w-44')
    rerender(<GenreLegend compact={false} openChoice={false} onOpenChange={() => {}} />)
    expect(toggle()).toHaveClass('min-h-0', 'text-xs')
    expect(toggle()).not.toHaveClass('min-h-9', 'text-sm')
    expect(container.firstElementChild).toHaveClass('w-44')
  })

  it('opens upward when compact, the toggle held at the bottom', () => {
    const { container, rerender } = render(
      <GenreLegend compact openChoice onOpenChange={() => {}} />,
    )
    // The toggle stays first in the DOM (aria-controls order); the reversed
    // column draws the key above it when compact, below it otherwise.
    expect(container.firstElementChild?.firstElementChild).toBe(toggle())
    expect(container.firstElementChild).toHaveClass('flex', 'flex-col-reverse')
    rerender(<GenreLegend compact={false} openChoice onOpenChange={() => {}} />)
    expect(container.firstElementChild).toHaveClass('flex', 'flex-col')
    expect(container.firstElementChild).not.toHaveClass('flex-col-reverse')
  })

  it('takes its placement from the caller', () => {
    const { container } = render(
      <GenreLegend
        compact={false}
        openChoice={false}
        onOpenChange={() => {}}
        className="absolute right-4"
      />,
    )
    expect(container.firstElementChild).toHaveClass('absolute', 'right-4')
  })

  it('starts collapsed when compact and open otherwise', () => {
    const { rerender } = render(<GenreLegend compact openChoice={null} onOpenChange={() => {}} />)
    expect(toggle()).toHaveAttribute('aria-expanded', 'false')
    expect(key()).not.toBeVisible()
    rerender(<GenreLegend compact={false} openChoice={null} onOpenChange={() => {}} />)
    expect(toggle()).toHaveAttribute('aria-expanded', 'true')
    expect(key()).toBeVisible()
  })

  it('follows the user’s choice over its compact default', () => {
    render(<GenreLegend compact openChoice onOpenChange={() => {}} />)
    expect(toggle()).toHaveAttribute('aria-expanded', 'true')
    expect(screen.getByText('Mixed / no data')).toBeVisible()
  })

  it('scrolls the open key inside a height the caller bounds, at every width', () => {
    const { container } = render(
      <GenreLegend compact openChoice onOpenChange={() => {}} className="max-h-full min-h-0" />,
    )
    expect(container.firstElementChild).toHaveClass('max-h-full', 'min-h-0')
    expect(toggle()).toHaveClass('shrink-0')
    const list = document.getElementById('atlas-genre-legend')
    expect(list).toHaveClass('min-h-0', 'overflow-y-auto')
  })

  it('reports the opposite of what it shows when toggled', () => {
    const onOpenChange = vi.fn()
    render(<GenreLegend compact openChoice={false} onOpenChange={onOpenChange} />)
    fireEvent.click(toggle())
    expect(onOpenChange).toHaveBeenCalledWith(true)
  })
})
