import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import { LatestRadioShows } from './LatestRadioShows'

vi.mock('next/link', () => ({
  default: ({ href, children, ...props }: { href: string; children: React.ReactNode; [key: string]: unknown }) => (
    <a href={href} {...props}>{children}</a>
  ),
}))

describe('LatestRadioShows', () => {
  it('renders the section heading and a "Browse all stations" link to /radio', () => {
    render(<LatestRadioShows />)
    expect(
      screen.getByRole('heading', { name: /latest radio shows/i })
    ).toBeInTheDocument()
    expect(
      screen.getByRole('link', { name: /browse all stations/i })
    ).toHaveAttribute('href', '/radio')
  })

  it('renders three station preview cards (KEXP / WFMU / NTS)', () => {
    render(<LatestRadioShows />)
    expect(screen.getByText('KEXP')).toBeInTheDocument()
    expect(screen.getByText('WFMU')).toBeInTheDocument()
    expect(screen.getByText('NTS')).toBeInTheDocument()
  })

  it('links every station card to /radio (PSY-389 decision: no D2-panel coupling)', () => {
    render(<LatestRadioShows />)
    const cardLinks = screen
      .getAllByRole('link')
      .filter(a => a.getAttribute('aria-label')?.includes('Browse radio'))
    expect(cardLinks).toHaveLength(3)
    for (const link of cardLinks) {
      expect(link).toHaveAttribute('href', '/radio')
    }
  })
})
