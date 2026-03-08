import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { UnpublishShowDialog } from './UnpublishShowDialog'
import type { ShowResponse } from '@/lib/types/show'

const mockMutate = vi.fn()
const mockUnpublishHook = vi.fn(() => ({
  mutate: mockMutate,
  isPending: false,
  isError: false,
  error: null,
}))

vi.mock('@/lib/hooks/useShowUnpublish', () => ({
  useShowUnpublish: () => mockUnpublishHook(),
}))

function makeShow(overrides: Partial<ShowResponse> = {}): ShowResponse {
  return {
    id: 1,
    slug: 'test-show',
    title: 'Test Show',
    event_date: '2026-04-15T20:00:00Z',
    status: 'approved',
    venues: [],
    artists: [
      { id: 1, slug: 'artist', name: 'Some Artist', socials: {} },
    ],
    created_at: '2024-01-01T00:00:00Z',
    updated_at: '2024-01-01T00:00:00Z',
    is_sold_out: false,
    is_cancelled: false,
    ...overrides,
  }
}

describe('UnpublishShowDialog', () => {
  const onOpenChange = vi.fn()

  beforeEach(() => {
    vi.clearAllMocks()
    mockUnpublishHook.mockReturnValue({
      mutate: mockMutate,
      isPending: false,
      isError: false,
      error: null,
    })
  })

  it('renders nothing when closed', () => {
    render(
      <UnpublishShowDialog show={makeShow()} open={false} onOpenChange={onOpenChange} />
    )
    expect(screen.queryByText('Unpublish Show')).not.toBeInTheDocument()
  })

  it('renders dialog title and description when open', () => {
    render(
      <UnpublishShowDialog show={makeShow()} open={true} onOpenChange={onOpenChange} />
    )
    expect(screen.getByText('Unpublish Show')).toBeInTheDocument()
    expect(screen.getByText(/Test Show/)).toBeInTheDocument()
    expect(screen.getByText(/become private/)).toBeInTheDocument()
  })

  it('calls mutate with show ID on unpublish', async () => {
    const user = userEvent.setup()
    render(
      <UnpublishShowDialog show={makeShow({ id: 7 })} open={true} onOpenChange={onOpenChange} />
    )

    await user.click(screen.getByRole('button', { name: 'Unpublish' }))
    expect(mockMutate).toHaveBeenCalledWith(7, expect.any(Object))
  })

  it('calls onOpenChange(false) on cancel', async () => {
    const user = userEvent.setup()
    render(
      <UnpublishShowDialog show={makeShow()} open={true} onOpenChange={onOpenChange} />
    )

    await user.click(screen.getByRole('button', { name: 'Cancel' }))
    expect(onOpenChange).toHaveBeenCalledWith(false)
  })

  it('shows pending state during mutation', () => {
    mockUnpublishHook.mockReturnValue({
      mutate: mockMutate,
      isPending: true,
      isError: false,
      error: null,
    })
    render(
      <UnpublishShowDialog show={makeShow()} open={true} onOpenChange={onOpenChange} />
    )
    expect(screen.getByText('Unpublishing...')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Cancel' })).toBeDisabled()
  })

  it('shows error message on failure', () => {
    mockUnpublishHook.mockReturnValue({
      mutate: mockMutate,
      isPending: false,
      isError: true,
      error: { message: 'Not authorized' },
    })
    render(
      <UnpublishShowDialog show={makeShow()} open={true} onOpenChange={onOpenChange} />
    )
    expect(screen.getByText('Not authorized')).toBeInTheDocument()
  })

  it('shows default error message when error has no message', () => {
    mockUnpublishHook.mockReturnValue({
      mutate: mockMutate,
      isPending: false,
      isError: true,
      error: {},
    })
    render(
      <UnpublishShowDialog show={makeShow()} open={true} onOpenChange={onOpenChange} />
    )
    expect(screen.getByText('Failed to unpublish show. Please try again.')).toBeInTheDocument()
  })

  it('uses artist names when title is empty', () => {
    render(
      <UnpublishShowDialog
        show={makeShow({ title: '' })}
        open={true}
        onOpenChange={onOpenChange}
      />
    )
    expect(screen.getByText(/Some Artist/)).toBeInTheDocument()
  })

  it('uses "Untitled Show" when no title and no artists', () => {
    render(
      <UnpublishShowDialog
        show={makeShow({ title: '', artists: [] })}
        open={true}
        onOpenChange={onOpenChange}
      />
    )
    expect(screen.getByText(/Untitled Show/)).toBeInTheDocument()
  })
})
