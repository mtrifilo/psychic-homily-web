import React from 'react'
import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import TagNotFound from './not-found'

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

describe('Tag not-found boundary (app/tags/[slug]/not-found.tsx)', () => {
  it('renders the not-found message', () => {
    render(<TagNotFound />)

    expect(
      screen.getByRole('heading', { name: 'Tag Not Found' })
    ).toBeInTheDocument()
    expect(screen.getByText(/doesn.t exist/i)).toBeInTheDocument()
  })

  it('renders a link back to the tags index', () => {
    render(<TagNotFound />)

    expect(
      screen.getByRole('link', { name: /back to tags/i })
    ).toHaveAttribute('href', '/tags')
  })
})
