import { describe, it, expect, vi, afterEach } from 'vitest'
import { render, screen, act } from '@testing-library/react'
import { Flag } from 'lucide-react'
import { StatusBanner } from './StatusBanner'

describe('StatusBanner', () => {
  afterEach(() => {
    vi.useRealTimers()
  })

  describe('rendering', () => {
    it('renders children content', () => {
      render(<StatusBanner variant="success">Changes saved</StatusBanner>)

      expect(screen.getByText('Changes saved')).toBeInTheDocument()
    })

    it('uses role="status" and aria-live="polite" for non-interrupting announcements', () => {
      render(<StatusBanner variant="success">Saved</StatusBanner>)

      const banner = screen.getByRole('status')
      expect(banner).toBeInTheDocument()
      expect(banner).toHaveAttribute('aria-live', 'polite')
    })

    it('forwards testId as data-testid on the banner', () => {
      render(
        <StatusBanner variant="pending" testId="pending-review-banner">
          Awaiting moderation
        </StatusBanner>
      )

      const banner = screen.getByTestId('pending-review-banner')
      expect(banner).toBeInTheDocument()
      expect(banner).toHaveAttribute('role', 'status')
    })

    it('omits data-testid when testId prop is not provided', () => {
      render(<StatusBanner variant="success">Saved</StatusBanner>)

      expect(screen.getByRole('status')).not.toHaveAttribute('data-testid')
    })

    it('merges className with the variant defaults rather than replacing them', () => {
      render(
        <StatusBanner variant="success" className="mb-4">
          Saved
        </StatusBanner>
      )

      const banner = screen.getByRole('status')
      expect(banner).toHaveClass('mb-4')
      // Variant chrome survives the merge.
      expect(banner).toHaveClass('border-green-800', 'bg-green-950/50')
    })
  })

  describe('variant: success', () => {
    it('applies the green Tailwind chrome', () => {
      render(<StatusBanner variant="success">Saved</StatusBanner>)

      const banner = screen.getByRole('status')
      expect(banner).toHaveClass(
        'rounded-md',
        'border',
        'border-green-800',
        'bg-green-950/50',
        'p-4'
      )
    })

    it('renders the default Check icon when no icon prop is supplied', () => {
      const { container } = render(
        <StatusBanner variant="success">Saved</StatusBanner>
      )

      // Lucide renders as <svg class="lucide lucide-check ...">
      const icon = container.querySelector('svg.lucide-check')
      expect(icon).toBeInTheDocument()
      expect(icon).toHaveClass('text-green-400')
    })
  })

  describe('variant: pending', () => {
    it('applies the amber Tailwind chrome', () => {
      render(<StatusBanner variant="pending">Awaiting moderation</StatusBanner>)

      const banner = screen.getByRole('status')
      expect(banner).toHaveClass(
        'rounded-md',
        'border',
        'border-amber-700/50',
        'bg-amber-950/40',
        'p-3'
      )
    })

    it('renders the default Clock icon when no icon prop is supplied', () => {
      const { container } = render(
        <StatusBanner variant="pending">Pending</StatusBanner>
      )

      const icon = container.querySelector('svg.lucide-clock')
      expect(icon).toBeInTheDocument()
      expect(icon).toHaveClass('text-amber-500')
    })
  })

  describe('icon override', () => {
    it('replaces the default icon when an `icon` prop is supplied', () => {
      const { container } = render(
        <StatusBanner
          variant="success"
          icon={<Flag data-testid="custom-icon" className="h-4 w-4" />}
        >
          Reported
        </StatusBanner>
      )

      // Custom icon is rendered.
      expect(screen.getByTestId('custom-icon')).toBeInTheDocument()
      // Default Check is not.
      expect(container.querySelector('svg.lucide-check')).not.toBeInTheDocument()
    })

    it('suppresses the leading icon when `icon={null}` is passed', () => {
      const { container } = render(
        <StatusBanner variant="success" icon={null}>
          Saved
        </StatusBanner>
      )

      expect(container.querySelector('svg.lucide-check')).not.toBeInTheDocument()
      expect(container.querySelector('svg.lucide-clock')).not.toBeInTheDocument()
    })
  })

  describe('auto-dismiss', () => {
    it('hides the banner after the configured timeout', () => {
      vi.useFakeTimers()

      render(
        <StatusBanner variant="success" dismissAfterMs={5000}>
          Saved
        </StatusBanner>
      )

      // Visible before the timer elapses.
      expect(screen.getByRole('status')).toBeInTheDocument()

      act(() => {
        vi.advanceTimersByTime(4999)
      })
      expect(screen.queryByRole('status')).toBeInTheDocument()

      // After the full delay, the banner unmounts itself.
      act(() => {
        vi.advanceTimersByTime(1)
      })
      expect(screen.queryByRole('status')).not.toBeInTheDocument()
    })

    it('fires onDismiss when the timer elapses', () => {
      vi.useFakeTimers()
      const onDismiss = vi.fn()

      render(
        <StatusBanner
          variant="success"
          dismissAfterMs={3000}
          onDismiss={onDismiss}
        >
          Saved
        </StatusBanner>
      )

      expect(onDismiss).not.toHaveBeenCalled()

      act(() => {
        vi.advanceTimersByTime(3000)
      })

      expect(onDismiss).toHaveBeenCalledTimes(1)
    })

    it('clears the pending timer on unmount (no setState on unmounted component)', () => {
      vi.useFakeTimers()
      const onDismiss = vi.fn()

      const { unmount } = render(
        <StatusBanner
          variant="success"
          dismissAfterMs={5000}
          onDismiss={onDismiss}
        >
          Saved
        </StatusBanner>
      )

      unmount()

      act(() => {
        vi.advanceTimersByTime(5000)
      })

      expect(onDismiss).not.toHaveBeenCalled()
    })

    it('stays visible indefinitely when dismissAfterMs is unset', () => {
      vi.useFakeTimers()

      render(<StatusBanner variant="pending">Pending</StatusBanner>)

      act(() => {
        vi.advanceTimersByTime(60_000)
      })

      expect(screen.getByRole('status')).toBeInTheDocument()
    })
  })
})
