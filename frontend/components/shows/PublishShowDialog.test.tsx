import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { PublishShowDialog } from './PublishShowDialog'
import type { ShowResponse } from '@/lib/types/show'

const mockMutate = vi.fn()
const mockPublishHook = vi.fn(() => ({
  mutate: mockMutate,
  isPending: false,
  isError: false,
  error: null,
}))

vi.mock('@/lib/hooks/shows/useShowPublish', () => ({
  useShowPublish: () => mockPublishHook(),
}))

function makeShow(overrides: Partial<ShowResponse> = {}): ShowResponse {
  return {
    id: 1,
    slug: 'test-show',
    title: 'Test Show',
    event_date: '2026-04-15T20:00:00Z',
    status: 'private',
    venues: [
      { id: 1, slug: 'venue', name: 'The Venue', city: 'Phoenix', state: 'AZ', verified: true },
    ],
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

describe('PublishShowDialog', () => {
  const onOpenChange = vi.fn()

  beforeEach(() => {
    vi.clearAllMocks()
    mockPublishHook.mockReturnValue({
      mutate: mockMutate,
      isPending: false,
      isError: false,
      error: null,
    })
  })

  it('renders nothing when closed', () => {
    render(
      <PublishShowDialog show={makeShow()} open={false} onOpenChange={onOpenChange} />
    )
    expect(screen.queryByText('Publish Show')).not.toBeInTheDocument()
  })

  it('renders dialog title and description when open', () => {
    render(
      <PublishShowDialog show={makeShow()} open={true} onOpenChange={onOpenChange} />
    )
    expect(screen.getByText('Publish Show')).toBeInTheDocument()
    expect(screen.getByText(/Test Show/)).toBeInTheDocument()
    expect(screen.getByText(/visible to everyone/)).toBeInTheDocument()
  })

  it('shows Publish button when venue is verified', () => {
    render(
      <PublishShowDialog show={makeShow()} open={true} onOpenChange={onOpenChange} />
    )
    expect(screen.getByRole('button', { name: 'Publish' })).toBeInTheDocument()
  })

  it('shows Submit for Review button when venue is unverified', () => {
    const show = makeShow({
      venues: [
        { id: 1, slug: 'venue', name: 'New Venue', city: 'Phoenix', state: 'AZ', verified: false },
      ],
    })
    render(
      <PublishShowDialog show={show} open={true} onOpenChange={onOpenChange} />
    )
    expect(screen.getByRole('button', { name: 'Submit for Review' })).toBeInTheDocument()
  })

  it('shows unverified venue warning when venue is unverified', () => {
    const show = makeShow({
      venues: [
        { id: 1, slug: 'venue', name: 'New Venue', city: 'Phoenix', state: 'AZ', verified: false },
      ],
    })
    render(
      <PublishShowDialog show={show} open={true} onOpenChange={onOpenChange} />
    )
    expect(screen.getByText(/unverified venue/)).toBeInTheDocument()
    expect(screen.getByText(/admin review/)).toBeInTheDocument()
  })

  it('does not show unverified warning when venue is verified', () => {
    render(
      <PublishShowDialog show={makeShow()} open={true} onOpenChange={onOpenChange} />
    )
    expect(screen.queryByText(/unverified venue/)).not.toBeInTheDocument()
  })

  it('calls mutate with show ID on publish', async () => {
    const user = userEvent.setup()
    render(
      <PublishShowDialog show={makeShow({ id: 5 })} open={true} onOpenChange={onOpenChange} />
    )

    await user.click(screen.getByRole('button', { name: 'Publish' }))
    expect(mockMutate).toHaveBeenCalledWith(5, expect.any(Object))
  })

  it('calls onOpenChange(false) on cancel', async () => {
    const user = userEvent.setup()
    render(
      <PublishShowDialog show={makeShow()} open={true} onOpenChange={onOpenChange} />
    )

    await user.click(screen.getByRole('button', { name: 'Cancel' }))
    expect(onOpenChange).toHaveBeenCalledWith(false)
  })

  it('shows pending state during mutation', () => {
    mockPublishHook.mockReturnValue({
      mutate: mockMutate,
      isPending: true,
      isError: false,
      error: null,
    })
    render(
      <PublishShowDialog show={makeShow()} open={true} onOpenChange={onOpenChange} />
    )
    expect(screen.getByText('Publishing...')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Cancel' })).toBeDisabled()
  })

  it('shows error message on failure', () => {
    mockPublishHook.mockReturnValue({
      mutate: mockMutate,
      isPending: false,
      isError: true,
      error: { message: 'Publish failed' },
    })
    render(
      <PublishShowDialog show={makeShow()} open={true} onOpenChange={onOpenChange} />
    )
    expect(screen.getByText('Publish failed')).toBeInTheDocument()
  })

  it('uses artist names in description when title is empty', () => {
    render(
      <PublishShowDialog
        show={makeShow({ title: '' })}
        open={true}
        onOpenChange={onOpenChange}
      />
    )
    expect(screen.getByText(/Some Artist/)).toBeInTheDocument()
  })
})
