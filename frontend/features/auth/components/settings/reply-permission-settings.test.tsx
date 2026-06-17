import { describe, it, expect, vi, beforeEach } from 'vitest'
import { screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { renderWithProviders } from '@/test/utils'
import { ReplyPermissionSettings } from './reply-permission-settings'

// --- Mocks ---

let mockProfileData: {
  user?: {
    preferences?: {
      default_reply_permission?: 'anyone' | 'followers' | 'author_only'
    }
  }
} = {}

const mockMutate = vi.fn()
let mockMutationState = {
  isPending: false,
  isError: false,
  error: null as Error | null,
}

vi.mock('@/features/auth', () => ({
  useProfile: () => ({ data: mockProfileData }),
}))

vi.mock('@/features/comments', async () => {
  const actual = await vi.importActual<typeof import('@/features/comments')>(
    '@/features/comments'
  )
  return {
    // Re-export the real ReplyPermissionSelect component so we exercise the
    // actual <select> DOM and the user-visible labels.
    ReplyPermissionSelect: actual.ReplyPermissionSelect,
    useSetDefaultReplyPermission: () => ({
      mutate: mockMutate,
      ...mockMutationState,
    }),
  }
})

// --- Tests ---

describe('ReplyPermissionSettings', () => {
  beforeEach(() => {
    mockProfileData = {}
    mockMutate.mockReset()
    mockMutationState = { isPending: false, isError: false, error: null }
  })

  it('renders card title and helper copy', () => {
    renderWithProviders(<ReplyPermissionSettings />)

    expect(screen.getByText('Default reply permission')).toBeInTheDocument()
    expect(
      screen.getByText(
        /Applied to new comments you create. You can still change this per-comment./
      )
    ).toBeInTheDocument()
  })

  it('renders the labeled select control with the "Who can reply" label', () => {
    renderWithProviders(<ReplyPermissionSettings />)

    expect(screen.getByLabelText('Who can reply')).toBeInTheDocument()
    expect(screen.getByTestId('reply-permission-select')).toBeInTheDocument()
  })

  it('defaults to "anyone" when no preference is set on the user profile', () => {
    renderWithProviders(<ReplyPermissionSettings />)

    const select = screen.getByTestId(
      'reply-permission-select'
    ) as HTMLSelectElement
    expect(select.value).toBe('anyone')
  })

  it('reflects the saved preference when the profile carries default_reply_permission', () => {
    mockProfileData = {
      user: { preferences: { default_reply_permission: 'followers' } },
    }
    renderWithProviders(<ReplyPermissionSettings />)

    const select = screen.getByTestId(
      'reply-permission-select'
    ) as HTMLSelectElement
    expect(select.value).toBe('followers')
  })

  it('reflects "author_only" (Replies disabled) when saved', () => {
    mockProfileData = {
      user: { preferences: { default_reply_permission: 'author_only' } },
    }
    renderWithProviders(<ReplyPermissionSettings />)

    const select = screen.getByTestId(
      'reply-permission-select'
    ) as HTMLSelectElement
    expect(select.value).toBe('author_only')
  })

  it('calls the mutation with the new value when the user selects "Followers only"', async () => {
    mockProfileData = {
      user: { preferences: { default_reply_permission: 'anyone' } },
    }
    const user = userEvent.setup()
    renderWithProviders(<ReplyPermissionSettings />)

    const select = screen.getByTestId('reply-permission-select')
    await user.selectOptions(select, 'followers')

    expect(mockMutate).toHaveBeenCalledTimes(1)
    expect(mockMutate).toHaveBeenCalledWith('followers')
  })

  it('calls the mutation with "author_only" when the user selects "Replies disabled"', async () => {
    mockProfileData = {
      user: { preferences: { default_reply_permission: 'anyone' } },
    }
    const user = userEvent.setup()
    renderWithProviders(<ReplyPermissionSettings />)

    const select = screen.getByTestId('reply-permission-select')
    await user.selectOptions(select, 'author_only')

    expect(mockMutate).toHaveBeenCalledWith('author_only')
  })

  it('does NOT call the mutation when the selected value matches the current value', async () => {
    mockProfileData = {
      user: { preferences: { default_reply_permission: 'followers' } },
    }
    const user = userEvent.setup()
    renderWithProviders(<ReplyPermissionSettings />)

    // Re-select the already-active option — should be a no-op.
    const select = screen.getByTestId('reply-permission-select')
    await user.selectOptions(select, 'followers')

    expect(mockMutate).not.toHaveBeenCalled()
  })

  it('disables the select while the mutation is pending', () => {
    mockMutationState = { isPending: true, isError: false, error: null }
    renderWithProviders(<ReplyPermissionSettings />)

    const select = screen.getByTestId(
      'reply-permission-select'
    ) as HTMLSelectElement
    expect(select).toBeDisabled()
  })

  it('renders an inline error message when the mutation fails', () => {
    mockMutationState = {
      isPending: false,
      isError: true,
      error: new Error('Server error'),
    }
    renderWithProviders(<ReplyPermissionSettings />)

    expect(
      screen.getByText('Failed to update setting. Please try again.')
    ).toBeInTheDocument()
  })

  it('renders all three user-facing reply-permission options in the dropdown', () => {
    renderWithProviders(<ReplyPermissionSettings />)

    // The labels are sourced from REPLY_PERMISSION_LABELS in the real module.
    expect(screen.getByRole('option', { name: 'Everyone' })).toBeInTheDocument()
    expect(
      screen.getByRole('option', { name: 'Followers only' })
    ).toBeInTheDocument()
    expect(
      screen.getByRole('option', { name: 'Replies disabled' })
    ).toBeInTheDocument()
  })

  it('persists across reload — re-rendering with the new server state reflects the new value', () => {
    mockProfileData = {
      user: { preferences: { default_reply_permission: 'anyone' } },
    }
    const { unmount } = renderWithProviders(<ReplyPermissionSettings />)
    expect(
      (screen.getByTestId('reply-permission-select') as HTMLSelectElement).value
    ).toBe('anyone')
    unmount()

    // Simulate the profile query coming back with the saved followers value.
    mockProfileData = {
      user: { preferences: { default_reply_permission: 'followers' } },
    }
    renderWithProviders(<ReplyPermissionSettings />)
    expect(
      (screen.getByTestId('reply-permission-select') as HTMLSelectElement).value
    ).toBe('followers')
  })
})
