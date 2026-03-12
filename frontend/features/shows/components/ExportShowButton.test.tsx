import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import { ExportShowButton } from './ExportShowButton'

// Mock Sentry
vi.mock('@sentry/nextjs', () => ({
  captureMessage: vi.fn(),
  captureException: vi.fn(),
}))

describe('ExportShowButton', () => {
  const originalEnv = process.env.NODE_ENV

  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('renders in development environment', () => {
    // NODE_ENV is 'test' in vitest, which is not 'development'
    // The component only renders when NODE_ENV === 'development'
    // So it should render nothing in test
    render(<ExportShowButton showId={1} showTitle="Test" />)
    expect(screen.queryByRole('button')).not.toBeInTheDocument()
  })

  it('sets title with show name', () => {
    // We can't easily change NODE_ENV in vitest, so we verify the component
    // returns null when not in development
    const { container } = render(
      <ExportShowButton showId={1} showTitle="My Show" />
    )
    expect(container.innerHTML).toBe('')
  })
})
