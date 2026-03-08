import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { SubmissionSuccessDialog } from './SubmissionSuccessDialog'

describe('SubmissionSuccessDialog', () => {
  it('renders dialog content when open', () => {
    render(<SubmissionSuccessDialog open={true} onOpenChange={vi.fn()} />)
    expect(screen.getByText('Private Show Added')).toBeInTheDocument()
  })

  it('renders description text about private show', () => {
    render(<SubmissionSuccessDialog open={true} onOpenChange={vi.fn()} />)
    expect(
      screen.getByText(/saved to your personal list/)
    ).toBeInTheDocument()
    expect(
      screen.getByText(/won't appear in public listings/)
    ).toBeInTheDocument()
  })

  it('renders "Got it" button', () => {
    render(<SubmissionSuccessDialog open={true} onOpenChange={vi.fn()} />)
    expect(screen.getByRole('button', { name: /Got it/ })).toBeInTheDocument()
  })

  it('calls onOpenChange with false when "Got it" is clicked', async () => {
    const user = userEvent.setup()
    const onOpenChange = vi.fn()
    render(<SubmissionSuccessDialog open={true} onOpenChange={onOpenChange} />)

    await user.click(screen.getByRole('button', { name: /Got it/ }))
    expect(onOpenChange).toHaveBeenCalledWith(false)
  })

  it('does not render content when closed', () => {
    render(<SubmissionSuccessDialog open={false} onOpenChange={vi.fn()} />)
    expect(screen.queryByText('Private Show Added')).not.toBeInTheDocument()
  })

  it('renders dialog with heading role', () => {
    render(<SubmissionSuccessDialog open={true} onOpenChange={vi.fn()} />)
    expect(screen.getByRole('heading', { name: 'Private Show Added' })).toBeInTheDocument()
  })

  it('renders a dialog element', () => {
    render(<SubmissionSuccessDialog open={true} onOpenChange={vi.fn()} />)
    expect(screen.getByRole('dialog')).toBeInTheDocument()
  })
})
