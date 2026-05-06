import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import { UserAttribution } from './UserAttribution'

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

describe('UserAttribution', () => {
  it('renders a profile link when username is set', () => {
    render(<UserAttribution name="alice" username="alice" />)

    const link = screen.getByRole('link', { name: 'alice' })
    expect(link).toHaveAttribute('href', '/users/alice')
  })

  it('renders plain text when username is null', () => {
    render(<UserAttribution name="asdf" username={null} />)

    expect(screen.getByText('asdf')).toBeInTheDocument()
    expect(screen.queryByRole('link')).not.toBeInTheDocument()
  })

  it('renders plain text when username is undefined', () => {
    render(<UserAttribution name="asdf" />)

    expect(screen.getByText('asdf')).toBeInTheDocument()
    expect(screen.queryByRole('link')).not.toBeInTheDocument()
  })

  it('renders plain text when username is empty string', () => {
    render(<UserAttribution name="asdf" username="" />)

    expect(screen.getByText('asdf')).toBeInTheDocument()
    expect(screen.queryByRole('link')).not.toBeInTheDocument()
  })

  it('falls back to "Anonymous" when name is missing', () => {
    render(<UserAttribution />)

    expect(screen.getByText('Anonymous')).toBeInTheDocument()
    expect(screen.queryByRole('link')).not.toBeInTheDocument()
  })

  it('falls back to "Anonymous" when name is empty string', () => {
    render(<UserAttribution name="" />)

    expect(screen.getByText('Anonymous')).toBeInTheDocument()
  })

  it('respects custom fallback', () => {
    render(<UserAttribution name={null} fallback="Unknown editor" />)

    expect(screen.getByText('Unknown editor')).toBeInTheDocument()
  })

  it('still renders the link when both name and username are set', () => {
    render(<UserAttribution name="Alice Smith" username="alice" />)

    const link = screen.getByRole('link', { name: 'Alice Smith' })
    expect(link).toHaveAttribute('href', '/users/alice')
  })

  // The linkable={false} opt-out is intentionally available for future
  // callsites (e.g. cards already wrapped in an outer Link); this PR doesn't
  // adopt it, but the primitive must support it.
  it('renders plain text when linkable is false even with username set', () => {
    render(
      <UserAttribution name="alice" username="alice" linkable={false} />
    )

    expect(screen.getByText('alice')).toBeInTheDocument()
    expect(screen.queryByRole('link')).not.toBeInTheDocument()
  })

  it('forwards className to the rendered link', () => {
    render(
      <UserAttribution
        name="alice"
        username="alice"
        className="custom-class"
      />
    )

    const link = screen.getByRole('link', { name: 'alice' })
    expect(link).toHaveClass('custom-class')
  })

  it('forwards className to the rendered span', () => {
    render(<UserAttribution name="asdf" className="custom-class" />)

    const span = screen.getByText('asdf')
    expect(span.tagName).toBe('SPAN')
    expect(span).toHaveClass('custom-class')
  })

  it('forwards testId as data-testid on the link', () => {
    render(
      <UserAttribution
        name="alice"
        username="alice"
        testId="byline-link"
      />
    )

    expect(screen.getByTestId('byline-link')).toBeInTheDocument()
    expect(screen.getByTestId('byline-link').tagName).toBe('A')
  })

  it('forwards testId as data-testid on the span', () => {
    render(<UserAttribution name="asdf" testId="byline-text" />)

    expect(screen.getByTestId('byline-text')).toBeInTheDocument()
    expect(screen.getByTestId('byline-text').tagName).toBe('SPAN')
  })

  // Verify the primitive never produces "User #N" — that string is the
  // anti-pattern this PR exists to delete.
  it('never emits "User #" debug strings', () => {
    render(<UserAttribution name={null} username={null} />)
    expect(screen.queryByText(/User #/)).not.toBeInTheDocument()
    expect(screen.queryByText(/user #/)).not.toBeInTheDocument()
  })
})
