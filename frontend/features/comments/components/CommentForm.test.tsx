import { describe, it, expect, vi } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/react'
import { CommentForm } from './CommentForm'

describe('CommentForm', () => {
  it('renders textarea and submit button', () => {
    render(<CommentForm onSubmit={vi.fn()} />)

    expect(screen.getByTestId('comment-textarea')).toBeInTheDocument()
    expect(screen.getByTestId('comment-submit')).toBeInTheDocument()
  })

  it('submit button is disabled when textarea is empty', () => {
    render(<CommentForm onSubmit={vi.fn()} />)

    expect(screen.getByTestId('comment-submit')).toBeDisabled()
  })

  it('submit button is enabled when textarea has content', () => {
    render(<CommentForm onSubmit={vi.fn()} />)

    fireEvent.change(screen.getByTestId('comment-textarea'), {
      target: { value: 'Hello world' },
    })

    expect(screen.getByTestId('comment-submit')).not.toBeDisabled()
  })

  it('calls onSubmit with trimmed body but does NOT clear textarea (PSY-589 — clear is parent-driven via resetSignal)', () => {
    const handleSubmit = vi.fn()
    render(<CommentForm onSubmit={handleSubmit} />)

    fireEvent.change(screen.getByTestId('comment-textarea'), {
      target: { value: '  Great show!  ' },
    })
    fireEvent.click(screen.getByTestId('comment-submit'))

    expect(handleSubmit).toHaveBeenCalledWith('Great show!', undefined)
    // PSY-589: form keeps the draft. Parent clears via resetSignal on
    // mutation success so 4xx errors don't discard typed text.
    expect(screen.getByTestId('comment-textarea')).toHaveValue('  Great show!  ')
  })

  it('clears textarea when parent bumps resetSignal (PSY-589)', () => {
    const handleSubmit = vi.fn()
    const { rerender } = render(
      <CommentForm onSubmit={handleSubmit} resetSignal={0} />
    )

    fireEvent.change(screen.getByTestId('comment-textarea'), {
      target: { value: 'Great show!' },
    })
    fireEvent.click(screen.getByTestId('comment-submit'))

    expect(handleSubmit).toHaveBeenCalledWith('Great show!', undefined)
    // Pre-bump: draft preserved.
    expect(screen.getByTestId('comment-textarea')).toHaveValue('Great show!')

    // Parent signals success.
    rerender(<CommentForm onSubmit={handleSubmit} resetSignal={1} />)
    expect(screen.getByTestId('comment-textarea')).toHaveValue('')
  })

  it('does not clear textarea when initialBody is provided (edit mode)', () => {
    const handleSubmit = vi.fn()
    render(
      <CommentForm onSubmit={handleSubmit} initialBody="Original text" />
    )

    expect(screen.getByTestId('comment-textarea')).toHaveValue('Original text')

    fireEvent.click(screen.getByTestId('comment-submit'))

    expect(handleSubmit).toHaveBeenCalledWith('Original text', undefined)
    // In edit mode, should NOT clear the textarea
    expect(screen.getByTestId('comment-textarea')).toHaveValue('Original text')
  })

  it('renders an inline error banner when errorMessage is set (PSY-589)', () => {
    render(
      <CommentForm
        onSubmit={vi.fn()}
        errorMessage="Please wait 60s before commenting again."
      />
    )
    const banner = screen.getByTestId('comment-form-error')
    expect(banner).toBeInTheDocument()
    expect(banner).toHaveAttribute('role', 'alert')
    expect(banner).toHaveTextContent(
      'Please wait 60s before commenting again.'
    )
  })

  it('does not clear the textarea when an error is present so the draft is preserved (PSY-589)', () => {
    const handleSubmit = vi.fn()
    const { rerender } = render(
      <CommentForm onSubmit={handleSubmit} resetSignal={0} />
    )

    fireEvent.change(screen.getByTestId('comment-textarea'), {
      target: { value: 'first try' },
    })
    fireEvent.click(screen.getByTestId('comment-submit'))

    // Mutation comes back 429 — parent renders an errorMessage but does NOT
    // bump resetSignal. The draft must survive.
    rerender(
      <CommentForm
        onSubmit={handleSubmit}
        resetSignal={0}
        errorMessage="Please wait 60s before commenting again."
      />
    )

    expect(screen.getByTestId('comment-form-error')).toBeInTheDocument()
    expect(screen.getByTestId('comment-textarea')).toHaveValue('first try')
  })

  it('renders cancel button when onCancel is provided', () => {
    const handleCancel = vi.fn()
    render(<CommentForm onSubmit={vi.fn()} onCancel={handleCancel} />)

    const cancelButton = screen.getByText('Cancel')
    expect(cancelButton).toBeInTheDocument()

    fireEvent.click(cancelButton)
    expect(handleCancel).toHaveBeenCalledTimes(1)
  })

  it('does not render cancel button when onCancel is not provided', () => {
    render(<CommentForm onSubmit={vi.fn()} />)

    expect(screen.queryByText('Cancel')).not.toBeInTheDocument()
  })

  it('shows custom placeholder and submit label', () => {
    render(
      <CommentForm
        onSubmit={vi.fn()}
        placeholder="Reply to someone..."
        submitLabel="Reply"
      />
    )

    expect(screen.getByPlaceholderText('Reply to someone...')).toBeInTheDocument()
    expect(screen.getByText('Reply')).toBeInTheDocument()
  })

  it('disables form when isPending', () => {
    render(<CommentForm onSubmit={vi.fn()} isPending />)

    expect(screen.getByTestId('comment-textarea')).toBeDisabled()
    expect(screen.getByTestId('comment-submit')).toBeDisabled()
  })

  it('does not call onSubmit when body is only whitespace', () => {
    const handleSubmit = vi.fn()
    render(<CommentForm onSubmit={handleSubmit} />)

    fireEvent.change(screen.getByTestId('comment-textarea'), {
      target: { value: '   ' },
    })

    // Submit button should be disabled
    expect(screen.getByTestId('comment-submit')).toBeDisabled()
  })
})
