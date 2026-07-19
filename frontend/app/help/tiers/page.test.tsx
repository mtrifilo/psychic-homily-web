import React from 'react'
import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import TiersHelpPage from './page'

vi.mock('next/link', () => ({
  default: ({
    href,
    children,
    ...props
  }: {
    href: string
    children: React.ReactNode
  }) => (
    <a href={href} {...props}>
      {children}
    </a>
  ),
}))

describe('Tiers help page (/help/tiers)', () => {
  it('renders all four tiers with their labels', () => {
    render(<TiersHelpPage />)

    expect(screen.getByText('New User')).toBeInTheDocument()
    expect(screen.getByText('Contributor')).toBeInTheDocument()
    expect(screen.getByText('Trusted Contributor')).toBeInTheDocument()
    expect(screen.getByText('Local Ambassador')).toBeInTheDocument()
  })

  it('renders advancement criteria pulled from backend auto_promotion constants', () => {
    render(<TiersHelpPage />)

    expect(screen.getByText(/^5 approved edits$/)).toBeInTheDocument()
    expect(screen.getByText(/^25 approved edits$/)).toBeInTheDocument()
    expect(screen.getByText(/At least 95% approval rate/i)).toBeInTheDocument()
    expect(screen.getByText(/^50 approved edits$/)).toBeInTheDocument()
    expect(screen.getByText(/Account age at least 180 days/i)).toBeInTheDocument()
  })

  // PSY-1486: "View your profile" → public identity view (/users/me redirects
  // to /users/<username> when one is set), not the /profile editor.
  it('links to the public identity view', () => {
    render(<TiersHelpPage />)

    const profileLink = screen.getByRole('link', { name: /View your profile/i })
    expect(profileLink).toHaveAttribute('href', '/users/me')
  })
})
