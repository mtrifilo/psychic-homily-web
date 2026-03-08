import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { DeleteShowDialog } from './DeleteShowDialog'
import type { ShowResponse } from '@/lib/types/show'

const mockMutate = vi.fn()
const mockDeleteHook = vi.fn(() => ({
  mutate: mockMutate,
  isPending: false,
  isError: false,
  error: null,
}))

vi.mock('@/lib/hooks/useShowDelete', () => ({
  useShowDelete: () => mockDeleteHook(),
}))

function makeShow(overrides: Partial<ShowResponse> = {}): ShowResponse {
  return {
    id: 1,
    slug: 'test-show',
    title: 'Test Show Title',
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

describe('DeleteShowDialog', () => {
  const onOpenChange = vi.fn()

  beforeEach(() => {
    vi.clearAllMocks()
    mockDeleteHook.mockReturnValue({
      mutate: mockMutate,
      isPending: false,
      isError: false,
      error: null,
    })
  })

  it('renders nothing when closed', () => {
    render(
      <DeleteShowDialog
        show={makeShow()}
        open={false}
        onOpenChange={onOpenChange}
      />
    )
    expect(screen.queryByText('Delete Show')).not.toBeInTheDocument()
  })

  it('renders dialog title and description when open', () => {
    render(
      <DeleteShowDialog
        show={makeShow()}
        open={true}
        onOpenChange={onOpenChange}
      />
    )
    // "Delete Show" appears in the title and the button
    expect(screen.getAllByText('Delete Show').length).toBeGreaterThanOrEqual(1)
    expect(screen.getByText(/Test Show Title/)).toBeInTheDocument()
    expect(screen.getByText(/cannot be undone/)).toBeInTheDocument()
  })

  it('uses artist names when title is empty', () => {
    render(
      <DeleteShowDialog
        show={makeShow({ title: '' })}
        open={true}
        onOpenChange={onOpenChange}
      />
    )
    expect(screen.getByText(/Some Artist/)).toBeInTheDocument()
  })

  it('uses "Untitled Show" when no title and no artists', () => {
    render(
      <DeleteShowDialog
        show={makeShow({ title: '', artists: [] })}
        open={true}
        onOpenChange={onOpenChange}
      />
    )
    expect(screen.getByText(/Untitled Show/)).toBeInTheDocument()
  })

  it('renders Cancel and Delete buttons', () => {
    render(
      <DeleteShowDialog
        show={makeShow()}
        open={true}
        onOpenChange={onOpenChange}
      />
    )
    expect(screen.getByRole('button', { name: 'Cancel' })).toBeInTheDocument()
    // The Delete Show button (with icon)
    const deleteButtons = screen.getAllByText(/Delete Show/)
    expect(deleteButtons.length).toBeGreaterThanOrEqual(1)
  })

  it('calls onOpenChange(false) when Cancel is clicked', async () => {
    const user = userEvent.setup()
    render(
      <DeleteShowDialog
        show={makeShow()}
        open={true}
        onOpenChange={onOpenChange}
      />
    )

    await user.click(screen.getByRole('button', { name: 'Cancel' }))
    expect(onOpenChange).toHaveBeenCalledWith(false)
  })

  it('calls deleteMutation.mutate with show ID on confirm', async () => {
    const user = userEvent.setup()
    render(
      <DeleteShowDialog
        show={makeShow({ id: 42 })}
        open={true}
        onOpenChange={onOpenChange}
      />
    )

    // Click the destructive "Delete Show" button (second one, inside footer)
    const deleteButtons = screen.getAllByRole('button').filter(b =>
      b.textContent?.includes('Delete Show')
    )
    await user.click(deleteButtons[deleteButtons.length - 1])
    expect(mockMutate).toHaveBeenCalledWith(42, expect.any(Object))
  })

  it('disables buttons when mutation is pending', () => {
    mockDeleteHook.mockReturnValue({
      mutate: mockMutate,
      isPending: true,
      isError: false,
      error: null,
    })
    render(
      <DeleteShowDialog
        show={makeShow()}
        open={true}
        onOpenChange={onOpenChange}
      />
    )

    expect(screen.getByRole('button', { name: 'Cancel' })).toBeDisabled()
    expect(screen.getByText('Deleting...')).toBeInTheDocument()
  })

  it('shows error message when mutation fails', () => {
    mockDeleteHook.mockReturnValue({
      mutate: mockMutate,
      isPending: false,
      isError: true,
      error: { message: 'Server error' },
    })
    render(
      <DeleteShowDialog
        show={makeShow()}
        open={true}
        onOpenChange={onOpenChange}
      />
    )

    expect(screen.getByText('Server error')).toBeInTheDocument()
  })

  it('shows default error message when error has no message', () => {
    mockDeleteHook.mockReturnValue({
      mutate: mockMutate,
      isPending: false,
      isError: true,
      error: {},
    })
    render(
      <DeleteShowDialog
        show={makeShow()}
        open={true}
        onOpenChange={onOpenChange}
      />
    )

    expect(screen.getByText('Failed to delete show. Please try again.')).toBeInTheDocument()
  })
})
