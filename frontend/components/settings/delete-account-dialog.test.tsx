import { describe, it, expect, vi, beforeEach } from 'vitest'
import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { renderWithProviders } from '@/test/utils'
import { DeleteAccountDialog } from './delete-account-dialog'

// --- Mocks ---

const mockRouterPush = vi.fn()
vi.mock('next/navigation', () => ({
  useRouter: () => ({ push: mockRouterPush }),
}))

vi.mock('@sentry/nextjs', () => ({
  captureException: vi.fn(),
}))

const mockRefetch = vi.fn()
let mockDeletionSummaryState = {
  data: null as {
    shows_count: number
    saved_shows_count: number
    passkeys_count: number
    has_password: boolean
  } | null,
  isLoading: false,
  isError: false,
  refetch: mockRefetch,
}

const mockDeleteMutateAsync = vi.fn()
let mockDeleteMutationState = {
  isPending: false,
  isError: false,
  error: null as Error | null,
  reset: vi.fn(),
}

vi.mock('@/lib/hooks/useAuth', () => ({
  useDeletionSummary: () => ({
    ...mockDeletionSummaryState,
  }),
  useDeleteAccount: () => ({
    mutateAsync: mockDeleteMutateAsync,
    ...mockDeleteMutationState,
  }),
}))

// --- Helpers ---

function renderDialog(open = true) {
  const onOpenChange = vi.fn()
  const user = userEvent.setup()
  renderWithProviders(
    <DeleteAccountDialog open={open} onOpenChange={onOpenChange} />
  )
  return { user, onOpenChange }
}

// --- Tests ---

describe('DeleteAccountDialog', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockDeletionSummaryState = {
      data: {
        shows_count: 5,
        saved_shows_count: 12,
        passkeys_count: 2,
        has_password: true,
      },
      isLoading: false,
      isError: false,
      refetch: mockRefetch,
    }
    mockDeleteMutationState = {
      isPending: false,
      isError: false,
      error: null,
      reset: vi.fn(),
    }
    mockDeleteMutateAsync.mockReset()
    mockRouterPush.mockReset()
  })

  it('does not render when open is false', () => {
    renderDialog(false)
    expect(screen.queryByText('Delete Account')).not.toBeInTheDocument()
  })

  it('renders warning step with title and description when open', () => {
    renderDialog()

    expect(screen.getByText('Delete Account')).toBeInTheDocument()
    expect(
      screen.getByText(
        'This action will schedule your account for permanent deletion.'
      )
    ).toBeInTheDocument()
  })

  it('shows data summary counts on warning step', () => {
    renderDialog()

    expect(screen.getByText('5')).toBeInTheDocument()
    expect(screen.getByText(/shows you submitted/)).toBeInTheDocument()
    expect(screen.getByText('12')).toBeInTheDocument()
    expect(screen.getByText(/saved shows/)).toBeInTheDocument()
    expect(screen.getByText('2')).toBeInTheDocument()
    expect(screen.getByText(/passkeys/)).toBeInTheDocument()
  })

  it('shows 30-day grace period notice', () => {
    renderDialog()

    expect(screen.getByText('30-Day Grace Period')).toBeInTheDocument()
    expect(
      screen.getByText(/Your account will be deactivated immediately/)
    ).toBeInTheDocument()
  })

  it('shows loading state when deletion summary is loading', () => {
    mockDeletionSummaryState = {
      ...mockDeletionSummaryState,
      data: null,
      isLoading: true,
    }
    renderDialog()

    // Continue button should be disabled while loading
    const continueButton = screen.getByRole('button', { name: /Continue/ })
    expect(continueButton).toBeDisabled()
  })

  it('shows error state when deletion summary fails', () => {
    mockDeletionSummaryState = {
      ...mockDeletionSummaryState,
      data: null,
      isLoading: false,
      isError: true,
    }
    renderDialog()

    expect(
      screen.getByText('Failed to load account data. Please try again.')
    ).toBeInTheDocument()

    const continueButton = screen.getByRole('button', { name: /Continue/ })
    expect(continueButton).toBeDisabled()
  })

  it('navigates to confirm step when Continue is clicked', async () => {
    const { user } = renderDialog()

    await user.click(screen.getByRole('button', { name: /Continue/ }))

    expect(screen.getByText('Confirm Account Deletion')).toBeInTheDocument()
  })

  it('shows password field on confirm step when user has password', async () => {
    const { user } = renderDialog()

    await user.click(screen.getByRole('button', { name: /Continue/ }))

    expect(screen.getByLabelText('Password')).toBeInTheDocument()
    expect(
      screen.getByText('Enter your password to confirm deletion.')
    ).toBeInTheDocument()
  })

  it('shows OAuth notice on confirm step when user has no password', async () => {
    mockDeletionSummaryState = {
      ...mockDeletionSummaryState,
      data: {
        shows_count: 0,
        saved_shows_count: 0,
        passkeys_count: 0,
        has_password: false,
      },
    }
    const { user } = renderDialog()

    await user.click(screen.getByRole('button', { name: /Continue/ }))

    expect(
      screen.getByText(/OAuth accounts require email confirmation/)
    ).toBeInTheDocument()
  })

  it('shows reason textarea on confirm step', async () => {
    const { user } = renderDialog()

    await user.click(screen.getByRole('button', { name: /Continue/ }))

    expect(
      screen.getByLabelText('Why are you leaving? (optional)')
    ).toBeInTheDocument()
  })

  it('shows confirmation checkbox on confirm step', async () => {
    const { user } = renderDialog()

    await user.click(screen.getByRole('button', { name: /Continue/ }))

    expect(
      screen.getByText(
        /I understand that my account will be deactivated/
      )
    ).toBeInTheDocument()
  })

  it('disables Delete button until checkbox is checked and password entered', async () => {
    const { user } = renderDialog()

    await user.click(screen.getByRole('button', { name: /Continue/ }))

    const deleteButton = screen.getByRole('button', {
      name: 'Delete My Account',
    })
    expect(deleteButton).toBeDisabled()

    // Enter password
    await user.type(screen.getByLabelText('Password'), 'mypassword123')
    expect(deleteButton).toBeDisabled()

    // Check confirmation checkbox
    await user.click(screen.getByRole('checkbox'))
    expect(deleteButton).toBeEnabled()
  })

  it('navigates back to warning step when Back is clicked', async () => {
    const { user } = renderDialog()

    // Go to confirm step
    await user.click(screen.getByRole('button', { name: /Continue/ }))
    expect(screen.getByText('Confirm Account Deletion')).toBeInTheDocument()

    // Go back
    await user.click(screen.getByRole('button', { name: /Back/ }))
    expect(screen.getByText('Delete Account')).toBeInTheDocument()
  })

  it('toggles password visibility on confirm step', async () => {
    const { user } = renderDialog()

    await user.click(screen.getByRole('button', { name: /Continue/ }))

    const passwordInput = screen.getByLabelText('Password')
    expect(passwordInput).toHaveAttribute('type', 'password')

    await user.click(
      screen.getByRole('button', { name: 'Show password' })
    )
    expect(passwordInput).toHaveAttribute('type', 'text')

    await user.click(
      screen.getByRole('button', { name: 'Hide password' })
    )
    expect(passwordInput).toHaveAttribute('type', 'password')
  })

  it('calls deleteAccount mutation on submit', async () => {
    mockDeleteMutateAsync.mockResolvedValue({
      success: true,
      deletion_date: '2026-04-06T00:00:00Z',
    })
    const { user } = renderDialog()

    // Navigate to confirm step
    await user.click(screen.getByRole('button', { name: /Continue/ }))

    // Fill in required fields
    await user.type(screen.getByLabelText('Password'), 'mypassword123')
    await user.click(screen.getByRole('checkbox'))

    // Submit
    await user.click(
      screen.getByRole('button', { name: 'Delete My Account' })
    )

    expect(mockDeleteMutateAsync).toHaveBeenCalledWith({
      password: 'mypassword123',
      reason: undefined,
    })
  })

  it('shows success step after successful deletion', async () => {
    mockDeleteMutateAsync.mockResolvedValue({
      success: true,
      deletion_date: '2026-04-06T00:00:00Z',
    })
    const { user } = renderDialog()

    // Navigate to confirm step
    await user.click(screen.getByRole('button', { name: /Continue/ }))

    // Fill in required fields
    await user.type(screen.getByLabelText('Password'), 'mypassword123')
    await user.click(screen.getByRole('checkbox'))

    // Submit
    await user.click(
      screen.getByRole('button', { name: 'Delete My Account' })
    )

    await waitFor(() => {
      expect(
        screen.getByText('Account Scheduled for Deletion')
      ).toBeInTheDocument()
    })

    // Date displayed depends on local timezone; just check the year is present
    expect(screen.getByText(/2026/)).toBeInTheDocument()
    expect(screen.getByText('Redirecting to home page...')).toBeInTheDocument()
    expect(
      screen.getByRole('button', { name: 'Go to Home' })
    ).toBeInTheDocument()
  })

  it('shows mutation error on confirm step', async () => {
    mockDeleteMutationState = {
      ...mockDeleteMutationState,
      isError: true,
      error: new Error('Incorrect password'),
    }
    const { user } = renderDialog()

    await user.click(screen.getByRole('button', { name: /Continue/ }))

    expect(screen.getByText('Incorrect password')).toBeInTheDocument()
  })

  it('calls onOpenChange when Cancel is clicked on warning step', async () => {
    const { user, onOpenChange } = renderDialog()

    await user.click(screen.getByRole('button', { name: 'Cancel' }))

    expect(onOpenChange).toHaveBeenCalledWith(false)
  })
})
