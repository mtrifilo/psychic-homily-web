import { describe, it, expect } from 'vitest'
import { render, screen } from '@testing-library/react'

import { GraphSkeleton } from './GraphSkeleton'

describe('GraphSkeleton', () => {
  it('default: an aria-hidden pulsing CLS placeholder (no content)', () => {
    const { container } = render(<GraphSkeleton className="aspect-[16/9]" />)
    const box = container.firstElementChild as HTMLElement
    expect(box).toHaveAttribute('aria-hidden', 'true')
    expect(box.className).toContain('animate-pulse')
    expect(box.className).toContain('aspect-[16/9]')
  })

  it('with children: shows the content, not hidden and not pulsing (ArtistGraph modal variant)', () => {
    const { container } = render(
      <GraphSkeleton>
        <span>Loading graph...</span>
      </GraphSkeleton>,
    )
    const box = container.firstElementChild as HTMLElement
    expect(box).not.toHaveAttribute('aria-hidden')
    expect(box.className).not.toContain('animate-pulse')
    expect(screen.getByText('Loading graph...')).toBeInTheDocument()
  })
})
