import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { HomeHero } from './HomeHero'

const mockOpenCommandPalette = vi.fn()

vi.mock('@/lib/hooks/common/useCommandPalette', () => ({
  openCommandPalette: () => mockOpenCommandPalette(),
}))

vi.mock('next/link', () => ({
  default: ({ href, children, ...props }: { href: string; children: React.ReactNode; [key: string]: unknown }) => (
    <a href={href} {...props}>{children}</a>
  ),
}))

describe('HomeHero', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('renders the "This is not a mirage." headline as the page h1', () => {
    render(<HomeHero />)
    const heading = screen.getByRole('heading', { name: 'This is not a mirage.' })
    expect(heading.tagName).toBe('H1')
  })

  it('opens the command palette when the hero search is clicked', async () => {
    const user = userEvent.setup()
    render(<HomeHero />)
    await user.click(
      screen.getByRole('button', { name: /search shows, artists, labels/i })
    )
    expect(mockOpenCommandPalette).toHaveBeenCalledOnce()
  })

  it('renders "Find a show" as a primary CTA linking to /shows (value-before-contribution)', () => {
    render(<HomeHero />)
    const cta = screen.getByRole('link', { name: 'Find a show' })
    expect(cta).toHaveAttribute('href', '/shows')
  })

  it('renders the Discover quick-links to their canonical destinations', () => {
    render(<HomeHero />)
    const expected: Array<[string, string]> = [
      ['Shows in any city', '/shows'],
      ['Artists', '/artists'],
      ['Freeform Radio', '/radio'],
      ['Record Labels', '/labels'],
      ['and more', '/explore'],
    ]
    for (const [label, href] of expected) {
      expect(screen.getByRole('link', { name: label })).toHaveAttribute(
        'href',
        href
      )
    }
  })

  it('renders a value-forward Sign up nudge linking to /auth', () => {
    render(<HomeHero />)
    expect(screen.getByRole('link', { name: 'Sign up' })).toHaveAttribute(
      'href',
      '/auth'
    )
    expect(
      screen.getByText(/to contribute, and never miss a show again\./)
    ).toBeInTheDocument()
  })
})
