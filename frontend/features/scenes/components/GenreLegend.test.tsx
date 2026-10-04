import { describe, it, expect, vi, afterEach } from 'vitest'
import { fireEvent, render, screen } from '@testing-library/react'
import { GenreLegend } from './GenreLegend'
import { ATLAS_COMPACT_VIEWPORT_QUERY } from '../atlasViewport'
import { installMatchMedia } from '@/test/mocks/matchMedia'

describe('GenreLegend', () => {
  let matchMedia: ReturnType<typeof installMatchMedia> | null = null
  afterEach(() => {
    matchMedia?.restore()
    matchMedia = null
  })

  const toggle = () => screen.getByRole('button', { name: 'Genres' })
  const key = () => document.getElementById('atlas-genre-legend')

  it('is a chip as tall as Drift below lg and the fixed-width key from lg', () => {
    const { container } = render(<GenreLegend openChoice={false} onOpenChange={() => {}} />)
    expect(toggle()).toHaveClass('min-h-9', 'text-sm', 'lg:min-h-0', 'lg:text-xs')
    expect(container.firstElementChild).toHaveClass('lg:w-44')
    expect(container.firstElementChild).not.toHaveClass('w-44')
  })

  it('opens upward below lg, the toggle held at the bottom', () => {
    const { container } = render(<GenreLegend openChoice onOpenChange={() => {}} />)
    // The toggle stays first in the DOM (aria-controls order); the reversed
    // column draws the key above it until lg, where the key drops below.
    expect(container.firstElementChild?.firstElementChild).toBe(toggle())
    expect(container.firstElementChild).toHaveClass('flex', 'flex-col-reverse', 'lg:block')
  })

  it('takes its placement from the caller', () => {
    const { container } = render(
      <GenreLegend
        openChoice={false}
        onOpenChange={() => {}}
        className="lg:absolute lg:right-4"
      />,
    )
    expect(container.firstElementChild).toHaveClass('lg:absolute', 'lg:right-4')
    expect(container.firstElementChild).not.toHaveClass('absolute')
  })

  it('starts collapsed on a compact viewport and open on a wide one', () => {
    matchMedia = installMatchMedia({ [ATLAS_COMPACT_VIEWPORT_QUERY]: true })
    render(<GenreLegend openChoice={null} onOpenChange={() => {}} />)
    expect(toggle()).toHaveAttribute('aria-expanded', 'false')
    expect(key()).not.toBeVisible()
    matchMedia.set(ATLAS_COMPACT_VIEWPORT_QUERY, false)
    expect(toggle()).toHaveAttribute('aria-expanded', 'true')
    expect(key()).toBeVisible()
  })

  it('follows the user’s choice over the viewport', () => {
    matchMedia = installMatchMedia({ [ATLAS_COMPACT_VIEWPORT_QUERY]: true })
    render(<GenreLegend openChoice onOpenChange={() => {}} />)
    expect(toggle()).toHaveAttribute('aria-expanded', 'true')
    expect(screen.getByText('Mixed / no data')).toBeVisible()
  })

  it('reports the opposite of what it shows when toggled', () => {
    const onOpenChange = vi.fn()
    render(<GenreLegend openChoice={false} onOpenChange={onOpenChange} />)
    fireEvent.click(toggle())
    expect(onOpenChange).toHaveBeenCalledWith(true)
  })
})
