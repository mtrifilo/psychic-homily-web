import { describe, it, expect, vi, beforeEach } from 'vitest'
import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { renderWithProviders } from '@/test/utils'
import { ChangePassword } from './change-password'

// --- Mocks ---

const mockMutate = vi.fn()
let mockMutationState = {
  isPending: false,
  isError: false,
  error: null as Error | null,
}

vi.mock('@/lib/hooks/useAuth', () => ({
  useChangePassword: () => ({
    mutate: mockMutate,
    ...mockMutationState,
  }),
}))

// --- Helpers ---

function renderForm() {
  const user = userEvent.setup()
  renderWithProviders(<ChangePassword />)
  return { user }
}

// --- Tests ---

describe('ChangePassword', () => {
  beforeEach(() => {
    mockMutate.mockReset()
    mockMutationState = { isPending: false, isError: false, error: null }
  })

  it('renders all 3 fields and submit button without errors initially', () => {
    renderForm()

    expect(screen.getByLabelText('Current Password')).toBeInTheDocument()
    expect(screen.getByLabelText('New Password')).toBeInTheDocument()
    expect(screen.getByLabelText('Confirm New Password')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Change Password' })).toBeEnabled()
    expect(screen.queryAllByRole('alert')).toHaveLength(0)
  })

  it('shows currentPassword error on submit with empty fields', async () => {
    const { user } = renderForm()

    await user.click(screen.getByRole('button', { name: 'Change Password' }))

    await waitFor(() => {
      expect(screen.getByText('Current password is required')).toBeInTheDocument()
    })
  })

  it('shows newPassword error for password shorter than 12 chars', async () => {
    const { user } = renderForm()

    await user.type(screen.getByLabelText('Current Password'), 'oldpassword123')
    await user.type(screen.getByLabelText('New Password'), 'short')
    await user.type(screen.getByLabelText('Confirm New Password'), 'short')
    await user.click(screen.getByRole('button', { name: 'Change Password' }))

    await waitFor(() => {
      expect(screen.getByText(/Password must be at least 12 characters/)).toBeInTheDocument()
    })
  })

  it('shows confirmPassword error (role="alert") when confirm is empty on submit', async () => {
    const { user } = renderForm()

    // Submit empty form — all 3 field errors should appear
    await user.click(screen.getByRole('button', { name: 'Change Password' }))

    await waitFor(() => {
      expect(screen.getByText('Please confirm your new password')).toBeInTheDocument()
    })
    // The error should be in an element with role="alert"
    const alerts = screen.getAllByRole('alert')
    const confirmAlert = alerts.find(el => el.textContent?.includes('Please confirm your new password'))
    expect(confirmAlert).toBeDefined()
  })

  it('shows "Passwords do not match" Zod error when passwords differ', async () => {
    const { user } = renderForm()

    await user.type(screen.getByLabelText('Current Password'), 'oldpassword123')
    await user.type(screen.getByLabelText('New Password'), 'newSecurePassword123!')
    await user.type(screen.getByLabelText('Confirm New Password'), 'differentPassword456!')
    await user.click(screen.getByRole('button', { name: 'Change Password' }))

    await waitFor(() => {
      expect(screen.getByText('Passwords do not match')).toBeInTheDocument()
    })
  })

  it('shows "New password must be different from current password" error', async () => {
    const { user } = renderForm()
    const samePassword = 'samePassword123!'

    await user.type(screen.getByLabelText('Current Password'), samePassword)
    await user.type(screen.getByLabelText('New Password'), samePassword)
    await user.type(screen.getByLabelText('Confirm New Password'), samePassword)
    await user.click(screen.getByRole('button', { name: 'Change Password' }))

    await waitFor(() => {
      expect(screen.getByText('New password must be different from current password')).toBeInTheDocument()
    })
  })

  it('shows PasswordStrengthMeter for new password field', async () => {
    const { user } = renderForm()

    await user.type(screen.getByLabelText('New Password'), 'abc')

    // The strength meter renders its section heading when password is non-empty
    await waitFor(() => {
      expect(screen.getByText('Password strength')).toBeInTheDocument()
    })
  })

  it('shows password match indicator when confirm field has value and no errors', async () => {
    const { user } = renderForm()

    await user.type(screen.getByLabelText('Current Password'), 'oldpassword123')
    await user.type(screen.getByLabelText('New Password'), 'newSecurePassword123!')
    await user.type(screen.getByLabelText('Confirm New Password'), 'newSecurePassword123!')

    await waitFor(() => {
      expect(screen.getByText('Passwords match')).toBeInTheDocument()
    })
  })

  it('does not show duplicate error messages', async () => {
    const { user } = renderForm()

    await user.click(screen.getByRole('button', { name: 'Change Password' }))

    await waitFor(() => {
      expect(screen.getAllByRole('alert').length).toBeGreaterThanOrEqual(1)
    })

    // Each alert text should appear exactly once
    const alerts = screen.getAllByRole('alert')
    const texts = alerts.map(el => el.textContent)
    const unique = [...new Set(texts)]
    expect(texts).toEqual(unique)
  })

  it('calls mutation with correct payload on valid submit', async () => {
    const { user } = renderForm()

    await user.type(screen.getByLabelText('Current Password'), 'oldpassword123')
    await user.type(screen.getByLabelText('New Password'), 'newSecurePassword123!')
    await user.type(screen.getByLabelText('Confirm New Password'), 'newSecurePassword123!')
    await user.click(screen.getByRole('button', { name: 'Change Password' }))

    await waitFor(() => {
      expect(mockMutate).toHaveBeenCalledWith(
        { current_password: 'oldpassword123', new_password: 'newSecurePassword123!' },
        expect.any(Object),
      )
    })
  })

  it('shows success message after successful mutation', async () => {
    mockMutate.mockImplementation((_data: unknown, opts: { onSuccess: (d: { message: string }) => void }) => {
      opts.onSuccess({ message: 'Password changed successfully' })
    })
    const { user } = renderForm()

    await user.type(screen.getByLabelText('Current Password'), 'oldpassword123')
    await user.type(screen.getByLabelText('New Password'), 'newSecurePassword123!')
    await user.type(screen.getByLabelText('Confirm New Password'), 'newSecurePassword123!')
    await user.click(screen.getByRole('button', { name: 'Change Password' }))

    await waitFor(() => {
      expect(screen.getByText('Password changed successfully')).toBeInTheDocument()
    })
  })

  it('shows mutation error message on failure', () => {
    mockMutationState = {
      isPending: false,
      isError: true,
      error: new Error('Current password is incorrect'),
    }
    renderForm()

    expect(screen.getByText('Current password is incorrect')).toBeInTheDocument()
  })

  it('toggles show/hide for current password field', async () => {
    const { user } = renderForm()

    const input = screen.getByLabelText('Current Password')
    expect(input).toHaveAttribute('type', 'password')

    const toggleButtons = screen.getAllByRole('button', { name: 'Show password' })
    await user.click(toggleButtons[0])

    expect(input).toHaveAttribute('type', 'text')
    expect(screen.getAllByRole('button', { name: 'Hide password' }).length).toBeGreaterThanOrEqual(1)
  })

  it('toggles show/hide for new password field', async () => {
    const { user } = renderForm()

    const input = screen.getByLabelText('New Password')
    expect(input).toHaveAttribute('type', 'password')

    // Second toggle button is for new password
    const toggleButtons = screen.getAllByRole('button', { name: 'Show password' })
    await user.click(toggleButtons[1])

    expect(input).toHaveAttribute('type', 'text')
  })

  it('toggles show/hide for confirm password field', async () => {
    const { user } = renderForm()

    const input = screen.getByLabelText('Confirm New Password')
    expect(input).toHaveAttribute('type', 'password')

    // Third toggle button is for confirm password
    const toggleButtons = screen.getAllByRole('button', { name: 'Show password' })
    await user.click(toggleButtons[2])

    expect(input).toHaveAttribute('type', 'text')
  })
})
