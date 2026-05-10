import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import { EntityCardTitle } from './EntityCardTitle'

// Mock next/link so the test environment doesn't need the Next router.
vi.mock('next/link', () => ({
  default: ({
    href,
    children,
    ...props
  }: {
    href: string
    children: React.ReactNode
    [key: string]: unknown
  }) => (
    <a href={href} {...props}>
      {children}
    </a>
  ),
}))

describe('EntityCardTitle', () => {
  it('renders the name as a heading inside a link', () => {
    render(<EntityCardTitle name="The National" href="/artists/the-national" />)

    const link = screen.getByRole('link', { name: 'The National' })
    expect(link).toHaveAttribute('href', '/artists/the-national')

    const heading = screen.getByRole('heading', {
      level: 3,
      name: 'The National',
    })
    expect(heading).toBeInTheDocument()
    // The heading must live inside the link, not beside it — single
    // outer-Link contract is load-bearing for Playwright strict-mode.
    expect(link).toContainElement(heading)
  })

  it('emits exactly one link element (Playwright strict-mode safe)', () => {
    render(<EntityCardTitle name="Test" href="/foo" />)

    expect(screen.getAllByRole('link')).toHaveLength(1)
  })

  it('applies a `title=` tooltip with the full name', () => {
    render(<EntityCardTitle name="Peter Hook and the Light" href="/artists/x" />)

    const heading = screen.getByRole('heading', {
      level: 3,
      name: 'Peter Hook and the Light',
    })
    expect(heading).toHaveAttribute('title', 'Peter Hook and the Light')
  })

  it('uses line-clamp-2 for omitted density (comfortable-equivalent default)', () => {
    render(<EntityCardTitle name="Foo" href="/foo" />)

    const heading = screen.getByRole('heading', { level: 3, name: 'Foo' })
    expect(heading.className).toContain('line-clamp-2')
    expect(heading.className).not.toContain('truncate')
    expect(heading.className).toContain('font-bold')
    expect(heading.className).toContain('text-base')
  })

  it('uses line-clamp-2 for density="comfortable"', () => {
    render(<EntityCardTitle name="Foo" href="/foo" density="comfortable" />)

    const heading = screen.getByRole('heading', { level: 3, name: 'Foo' })
    expect(heading.className).toContain('line-clamp-2')
    expect(heading.className).toContain('font-bold')
    expect(heading.className).toContain('text-base')
  })

  it('uses line-clamp-2 for density="expanded" with text-xl', () => {
    render(<EntityCardTitle name="Foo" href="/foo" density="expanded" />)

    const heading = screen.getByRole('heading', { level: 3, name: 'Foo' })
    expect(heading.className).toContain('line-clamp-2')
    expect(heading.className).toContain('font-bold')
    expect(heading.className).toContain('text-xl')
  })

  it('uses truncate for density="compact"', () => {
    render(<EntityCardTitle name="Foo" href="/foo" density="compact" />)

    const heading = screen.getByRole('heading', { level: 3, name: 'Foo' })
    expect(heading.className).toContain('truncate')
    expect(heading.className).not.toContain('line-clamp-2')
    expect(heading.className).toContain('font-medium')
    expect(heading.className).toContain('text-sm')
  })

  it('falls back to name for the link aria-label when ariaLabel is omitted', () => {
    render(<EntityCardTitle name="Foo" href="/foo" />)

    const link = screen.getByRole('link', { name: 'Foo' })
    expect(link).toHaveAttribute('aria-label', 'Foo')
  })

  it('uses ariaLabel when provided', () => {
    render(
      <EntityCardTitle
        name="Test Album"
        href="/releases/x"
        ariaLabel="Test Album (album)"
      />
    )

    const link = screen.getByRole('link', { name: 'Test Album (album)' })
    expect(link).toHaveAttribute('aria-label', 'Test Album (album)')
    // Visible heading text remains the bare name.
    expect(
      screen.getByRole('heading', { level: 3, name: 'Test Album' })
    ).toBeInTheDocument()
  })

  it('applies group-hover styling so caller wrappers can pair with `group`', () => {
    render(<EntityCardTitle name="Foo" href="/foo" />)

    const heading = screen.getByRole('heading', { level: 3, name: 'Foo' })
    expect(heading.className).toContain('group-hover:text-primary')
  })

  it('keeps the link className stable across density values', () => {
    const { rerender } = render(
      <EntityCardTitle name="Foo" href="/foo" density="compact" />
    )
    const compactLink = screen.getByRole('link')
    expect(compactLink.className).toContain('block')
    expect(compactLink.className).toContain('group')

    rerender(<EntityCardTitle name="Foo" href="/foo" density="expanded" />)
    const expandedLink = screen.getByRole('link')
    expect(expandedLink.className).toContain('block')
    expect(expandedLink.className).toContain('group')
  })
})
