import { describe, it, expect } from 'vitest'
import { render, screen } from '@testing-library/react'
import { InlineErrorBanner } from './InlineErrorBanner'

describe('InlineErrorBanner', () => {
  it('renders children content', () => {
    render(<InlineErrorBanner>Something went wrong</InlineErrorBanner>)

    expect(screen.getByText('Something went wrong')).toBeInTheDocument()
  })

  it('renders complex children (e.g. icon + span)', () => {
    render(
      <InlineErrorBanner>
        <span data-testid="icon">!</span>
        <span>Couldn&apos;t parse CSV</span>
      </InlineErrorBanner>
    )

    expect(screen.getByTestId('icon')).toBeInTheDocument()
    expect(screen.getByText("Couldn't parse CSV")).toBeInTheDocument()
  })

  it('bakes in role="alert" so screen readers announce the message', () => {
    render(<InlineErrorBanner>Failed</InlineErrorBanner>)

    expect(screen.getByRole('alert')).toBeInTheDocument()
    expect(screen.getByRole('alert')).toHaveTextContent('Failed')
  })

  it('applies the canonical destructive-tone classes by default', () => {
    render(<InlineErrorBanner>Failed</InlineErrorBanner>)

    const banner = screen.getByRole('alert')
    expect(banner).toHaveClass(
      'rounded-lg',
      'border',
      'border-destructive/50',
      'bg-destructive/10',
      'p-3',
      'text-sm',
      'text-destructive'
    )
  })

  it('merges className with the defaults rather than replacing them', () => {
    render(
      <InlineErrorBanner className="flex items-start gap-2">
        Failed
      </InlineErrorBanner>
    )

    const banner = screen.getByRole('alert')
    // Caller-supplied layout classes are present.
    expect(banner).toHaveClass('flex', 'items-start', 'gap-2')
    // Default tone classes survive the merge.
    expect(banner).toHaveClass('border-destructive/50', 'bg-destructive/10')
  })

  it('forwards testId as data-testid on the banner', () => {
    render(
      <InlineErrorBanner testId="alias-create-error">
        Slug already exists
      </InlineErrorBanner>
    )

    const banner = screen.getByTestId('alias-create-error')
    expect(banner).toHaveTextContent('Slug already exists')
    expect(banner).toHaveAttribute('role', 'alert')
  })

  it('omits data-testid when testId prop is not provided', () => {
    render(<InlineErrorBanner>Failed</InlineErrorBanner>)

    const banner = screen.getByRole('alert')
    expect(banner).not.toHaveAttribute('data-testid')
  })

  describe('variant="default"', () => {
    it('applies p-3 + text-sm shape classes when variant is omitted', () => {
      render(<InlineErrorBanner>Failed</InlineErrorBanner>)

      const banner = screen.getByRole('alert')
      expect(banner).toHaveClass('p-3', 'text-sm', 'text-destructive')
      // Should NOT pick up the queryFallback shape.
      expect(banner).not.toHaveClass('p-4', 'text-center')
    })

    it('applies the same shape classes when variant="default" is explicit', () => {
      render(<InlineErrorBanner variant="default">Failed</InlineErrorBanner>)

      const banner = screen.getByRole('alert')
      expect(banner).toHaveClass('p-3', 'text-sm', 'text-destructive')
      expect(banner).not.toHaveClass('p-4', 'text-center')
    })
  })

  describe('variant="queryFallback"', () => {
    it('applies p-4 + text-center shape classes for query-load fallbacks', () => {
      render(
        <InlineErrorBanner variant="queryFallback">
          Failed to load tags.
        </InlineErrorBanner>
      )

      const banner = screen.getByRole('alert')
      expect(banner).toHaveClass(
        'p-4',
        'text-center',
        'text-destructive'
      )
      // Should NOT pick up the default shape.
      expect(banner).not.toHaveClass('p-3', 'text-sm')
    })

    it('keeps the destructive tone tokens shared with default', () => {
      render(
        <InlineErrorBanner variant="queryFallback">
          Failed to load tags.
        </InlineErrorBanner>
      )

      const banner = screen.getByRole('alert')
      expect(banner).toHaveClass(
        'rounded-lg',
        'border',
        'border-destructive/50',
        'bg-destructive/10'
      )
    })

    it('still bakes in role="alert" so screen readers announce the fallback', () => {
      render(
        <InlineErrorBanner variant="queryFallback">
          Failed to load tags.
        </InlineErrorBanner>
      )

      expect(screen.getByRole('alert')).toHaveTextContent(
        'Failed to load tags.'
      )
    })

    it('merges className without dropping the variant shape', () => {
      render(
        <InlineErrorBanner variant="queryFallback" className="my-4">
          Failed
        </InlineErrorBanner>
      )

      const banner = screen.getByRole('alert')
      expect(banner).toHaveClass('my-4', 'p-4', 'text-center')
    })

    it('forwards testId on the queryFallback variant', () => {
      render(
        <InlineErrorBanner
          variant="queryFallback"
          testId="tags-load-error"
        >
          Failed to load tags.
        </InlineErrorBanner>
      )

      const banner = screen.getByTestId('tags-load-error')
      expect(banner).toHaveAttribute('role', 'alert')
      expect(banner).toHaveClass('p-4', 'text-center')
    })
  })
})
