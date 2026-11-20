import { describe, it, expect } from 'vitest'
import { render, screen } from '@testing-library/react'
import { renderToStaticMarkup } from 'react-dom/server'
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

  // Google may read the attribute before or after rendering, so it must be on
  // the element the moment the teaser is added to the page, on a span.
  it('keeps the whole sentence out of search snippets from its first render', () => {
    const markup = renderToStaticMarkup(
      <MobileGraphTeaser href="/graph">Sentence</MobileGraphTeaser>,
    )
    expect(markup).toMatch(/^<p[^>]*><span data-nosnippet="">\s*<a [^>]*>Sentence /)
    render(<MobileGraphTeaser href="/graph">Sentence</MobileGraphTeaser>)
    expect(screen.getByRole('link').closest('span[data-nosnippet]')).not.toBeNull()
  })

  // A line, not a module: a border or a reserved height would make this a card.
  it('is one line of text, not a box', () => {
    const { container } = render(
      <MobileGraphTeaser href="/graph">Sentence</MobileGraphTeaser>,
    )
    const line = container.firstElementChild!
    expect(line.tagName).toBe('P')
    expect(line.className).not.toContain('border')
    expect(line.className).not.toContain('h-[')
  })

  // The treatment is the locked half of the design, and this component exists
  // to be its single source, so pin it here rather than at six call sites.
  it('renders the locked type and link treatment', () => {
    const { container } = render(
      <MobileGraphTeaser href="/graph">Sentence</MobileGraphTeaser>,
    )
    const line = container.firstElementChild!
    expect(line.className).toContain('text-xs')
    expect(line.className).toContain('text-muted-foreground')
    const link = screen.getByRole('link')
    expect(link.className).toContain('underline')
    expect(link.className).toContain('underline-offset-4')
    expect(link.className).toContain('hover:text-foreground')
  })
})
