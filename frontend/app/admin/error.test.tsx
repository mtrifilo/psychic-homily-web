import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import * as Sentry from '@sentry/nextjs'
import AdminError from './error'


describe('Admin route error boundary (app/admin/error.tsx)', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('renders the error message and a retry button', () => {
    render(<AdminError error={new Error('boom')} reset={vi.fn()} />)

    expect(
      screen.getByRole('heading', { name: 'Something went wrong' })
    ).toBeInTheDocument()
    expect(
      screen.getByText(/admin console encountered an error/i)
    ).toBeInTheDocument()
    expect(
      screen.getByRole('button', { name: 'Try again' })
    ).toBeInTheDocument()
  })

  it('calls reset when the retry button is clicked', async () => {
    const user = userEvent.setup()
    const reset = vi.fn()
    render(<AdminError error={new Error('boom')} reset={reset} />)

    await user.click(screen.getByRole('button', { name: 'Try again' }))

    expect(reset).toHaveBeenCalledTimes(1)
  })

  it('reports the error to Sentry with its digest', () => {
    const error = Object.assign(new Error('boom'), { digest: 'abc123' })
    render(<AdminError error={error} reset={vi.fn()} />)

    expect(Sentry.captureException).toHaveBeenCalledTimes(1)
    expect(Sentry.captureException).toHaveBeenCalledWith(error)
    expect(vi.mocked(Sentry.captureException).mock.calls[0][0]).toHaveProperty(
      'digest',
      'abc123'
    )
  })
})
