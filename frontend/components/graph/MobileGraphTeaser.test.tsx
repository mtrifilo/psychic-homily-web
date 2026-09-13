import { describe, it, expect } from 'vitest'
import { render, screen } from '@testing-library/react'
import { MobileGraphTeaser } from './MobileGraphTeaser'

describe('MobileGraphTeaser', () => {
  it('makes the whole sentence the link to the given href', () => {
    render(
      <MobileGraphTeaser href="/graph?artist=sundressed">
        See how Phoenix artists connect on the music map
      </MobileGraphTeaser>,
    )
    const link = screen.getByRole('link')
    expect(link).toHaveAccessibleName(
      'See how Phoenix artists connect on the music map',
    )
    expect(link).toHaveAttribute('href', '/graph?artist=sundressed')
  })

  it('renders the arrow as decoration, outside the accessible name', () => {
    const { container } = render(
      <MobileGraphTeaser href="/graph">Sentence</MobileGraphTeaser>,
    )
    expect(screen.getByRole('link')).toHaveAccessibleName('Sentence')
    expect(container.textContent).toContain('→')
    expect(container.querySelector('[aria-hidden="true"]')?.textContent).toBe('→')
  })

  it('is one line of muted body text, not a box', () => {
    const { container } = render(
      <MobileGraphTeaser href="/graph">Sentence</MobileGraphTeaser>,
    )
    const line = container.firstElementChild!
    expect(line.tagName).toBe('P')
    expect(line.className).toContain('text-xs')
    expect(line.className).toContain('text-muted-foreground')
    expect(line.className).not.toContain('border')
    // The height contract that made the retired card a 240px module.
    expect(line.className).not.toContain('h-[')
    expect(screen.getByRole('link').className).toContain('underline-offset-4')
  })
})
