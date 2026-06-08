import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { ArtistHops } from './ArtistHops'

vi.mock('next/link', () => ({
  default: ({ href, children, onClick, ...props }: { href: string; children: React.ReactNode; onClick?: () => void; [key: string]: unknown }) => (
    <a href={href} onClick={onClick} {...props}>{children}</a>
  ),
}))

describe('ArtistHops', () => {
  it('links artists that have a graph slug', () => {
    render(
      <ArtistHops
        hops={[
          { name: 'Sleater-Kinney', slug: 'sleater-kinney' },
          { name: 'Wipers', slug: 'wipers' },
        ]}
      />
    )
    expect(screen.getByRole('link', { name: 'Sleater-Kinney' })).toHaveAttribute(
      'href',
      '/artists/sleater-kinney'
    )
    expect(screen.getByRole('link', { name: 'Wipers' })).toHaveAttribute('href', '/artists/wipers')
  })

  it('renders unlinked artists as plain text (no dead link)', () => {
    render(<ArtistHops hops={[{ name: 'Some Unmatched Band', slug: null }]} />)
    expect(screen.getByText('Some Unmatched Band')).toBeInTheDocument()
    expect(screen.queryByRole('link', { name: 'Some Unmatched Band' })).not.toBeInTheDocument()
  })

  it('fires onNavigate when a hop is followed (closes the nav panel)', async () => {
    const onNavigate = vi.fn()
    const user = userEvent.setup()
    render(<ArtistHops hops={[{ name: 'Wipers', slug: 'wipers' }]} onNavigate={onNavigate} />)
    await user.click(screen.getByRole('link', { name: 'Wipers' }))
    expect(onNavigate).toHaveBeenCalled()
  })

  it('renders nothing for an empty hop list', () => {
    const { container } = render(<ArtistHops hops={[]} />)
    expect(container).toBeEmptyDOMElement()
  })
})
