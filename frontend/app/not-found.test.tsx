import React from 'react'
import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import NotFound from './not-found'

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

describe('Root not-found boundary (app/not-found.tsx)', () => {
  it('renders the 404 message', () => {
    render(<NotFound />)

    expect(screen.getByRole('heading', { name: '404' })).toBeInTheDocument()
    expect(screen.getByText('Page not found')).toBeInTheDocument()
  })

  it('renders a link home', () => {
    render(<NotFound />)

    expect(screen.getByRole('link', { name: 'Go home' })).toHaveAttribute(
      'href',
      '/'
    )
  })

  it('renders a link to browse shows', () => {
    render(<NotFound />)

    expect(
      screen.getByRole('link', { name: 'Browse shows' })
    ).toHaveAttribute('href', '/shows')
  })
})
