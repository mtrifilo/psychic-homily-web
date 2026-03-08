import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MakePrivateDialog } from './MakePrivateDialog'
import type { ShowResponse } from '@/lib/types/show'

const mockMutate = vi.fn()
const mockMakePrivateHook = vi.fn(() => ({
  mutate: mockMutate,
  isPending: false,
  isError: false,
  error: null,
}))

vi.mock('@/lib/hooks/useShowMakePrivate', () => ({
  useShowMakePrivate: () => mockMakePrivateHook(),
}))

function makeShow(overrides: Partial<ShowResponse> = {}): ShowResponse {
  return {
    id: 1,
    slug: 'test-show',
    title: 'My Private Show',
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

describe('MakePrivateDialog', () => {
  const onOpenChange = vi.fn()

  beforeEach(() => {
    vi.clearAllMocks()
    mockMakePrivateHook.mockReturnValue({
      mutate: mockMutate,
      isPending: false,
      isError: false,
      error: null,
    })
  })

  it('renders nothing when closed', () => {
    render(
      <MakePrivateDialog show={makeShow()} open={false} onOpenChange={onOpenChange} />
    )
    expect(screen.queryByText('Make Private')).not.toBeInTheDocument()
  })

  it('renders dialog title and description when open', () => {
    render(
      <MakePrivateDialog show={makeShow()} open={true} onOpenChange={onOpenChange} />
    )
    // Title appears twice: once in the dialog header, once in the button
    expect(screen.getAllByText('Make Private').length).toBeGreaterThanOrEqual(1)
    expect(screen.getByText(/My Private Show/)).toBeInTheDocument()
    expect(screen.getByText(/only be visible to you/)).toBeInTheDocument()
  })

  it('calls mutate with show ID on confirm', async () => {
    const user = userEvent.setup()
    render(
      <MakePrivateDialog show={makeShow({ id: 10 })} open={true} onOpenChange={onOpenChange} />
    )

    // Click the Make Private action button
    const buttons = screen.getAllByRole('button').filter(b =>
      b.textContent?.includes('Make Private')
    )
    await user.click(buttons[buttons.length - 1])
    expect(mockMutate).toHaveBeenCalledWith(10, expect.any(Object))
  })

  it('calls onOpenChange(false) on cancel', async () => {
    const user = userEvent.setup()
    render(
      <MakePrivateDialog show={makeShow()} open={true} onOpenChange={onOpenChange} />
    )

    await user.click(screen.getByRole('button', { name: 'Cancel' }))
    expect(onOpenChange).toHaveBeenCalledWith(false)
  })

  it('shows pending state during mutation', () => {
    mockMakePrivateHook.mockReturnValue({
      mutate: mockMutate,
      isPending: true,
      isError: false,
      error: null,
    })
    render(
      <MakePrivateDialog show={makeShow()} open={true} onOpenChange={onOpenChange} />
    )
    expect(screen.getByText('Making Private...')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Cancel' })).toBeDisabled()
  })

  it('shows error message on failure', () => {
    mockMakePrivateHook.mockReturnValue({
      mutate: mockMutate,
      isPending: false,
      isError: true,
      error: { message: 'Permission denied' },
    })
    render(
      <MakePrivateDialog show={makeShow()} open={true} onOpenChange={onOpenChange} />
    )
    expect(screen.getByText('Permission denied')).toBeInTheDocument()
  })

  it('shows default error message when error has no message', () => {
    mockMakePrivateHook.mockReturnValue({
      mutate: mockMutate,
      isPending: false,
      isError: true,
      error: {},
    })
    render(
      <MakePrivateDialog show={makeShow()} open={true} onOpenChange={onOpenChange} />
    )
    expect(screen.getByText('Failed to make show private. Please try again.')).toBeInTheDocument()
  })

  it('uses artist names when title is empty', () => {
    render(
      <MakePrivateDialog
        show={makeShow({ title: '' })}
        open={true}
        onOpenChange={onOpenChange}
      />
    )
    expect(screen.getByText(/Some Artist/)).toBeInTheDocument()
  })
})
