import { describe, it, expect, vi, beforeEach } from 'vitest'
import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { renderWithProviders } from '@/test/utils'
import { ChangePassword } from './change-password'

// Settings States 6 and 6S: the Change password card's Reset by email, which
// sends the reset link to the account's own address with no recent sign-in.

const mockSendOwn = vi.fn<() => Promise<unknown>>()
const mockChangePassword = vi.fn()

vi.mock('@/features/auth', () => ({
  useChangePassword: () => ({ mutate: mockChangePassword, isPending: false, isError: false, error: null }),
  useSendOwnPasswordReset: () => ({ mutateAsync: mockSendOwn, isPending: false }),
}))

const EMAIL = 'owner@example.com'
const SENT = `Reset link sent to ${EMAIL}. It expires in 15 minutes. Nothing changes until you open it.`

function renderCard(resetEmail: string | null = EMAIL) {
  const user = userEvent.setup()
  renderWithProviders(
    <ChangePassword resetEmail={resetEmail} signInAgainHref="/auth?returnTo=%2Fprofile" />
  )
  return { user }
}

describe('Reset by email', () => {
  beforeEach(() => {
    mockSendOwn.mockReset()
    mockChangePassword.mockReset()
  })

  // The card's own form still changes the password inside the wrapper, and
  // Reset by email sends nothing on its behalf.
  it('leaves Update password working inside the wrapper', async () => {
    mockChangePassword.mockImplementation((_data: unknown, opts: { onSuccess: (d: { message: string }) => void }) => {
      opts.onSuccess({ message: 'Password changed successfully' })
    })
    const { user } = renderCard()

    await user.type(screen.getByLabelText('Current password'), 'oldpassword123')
    await user.type(screen.getByLabelText('New password'), 'newSecurePassword123!')
    await user.type(screen.getByLabelText('Confirm new password'), 'newSecurePassword123!')
    await user.click(screen.getByRole('button', { name: 'Update password' }))

    await waitFor(() => {
      expect(screen.getByText('Password changed successfully')).toBeInTheDocument()
    })
    expect(mockChangePassword.mock.calls[0][0]).toEqual({
      current_password: 'oldpassword123',
      new_password: 'newSecurePassword123!',
    })
    expect(mockSendOwn).not.toHaveBeenCalled()
    expect(screen.queryByText(SENT)).not.toBeInTheDocument()
  })

  it('sits beside Update password with the approved line naming the address', () => {
    renderCard()

    expect(screen.getByRole('button', { name: 'Update password' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Reset by email' })).toBeEnabled()
    expect(
      screen.getByText((_, node) =>
        node?.tagName === 'P' &&
        node.textContent ===
          `Don't know the current password? Reset by email sends a link to ${EMAIL}. Setting a new password with it signs out every other device.`
      )
    ).toBeInTheDocument()
    expect(screen.queryByText(SENT)).not.toBeInTheDocument()
  })

  it('is not offered when the profile names no address', () => {
    renderCard(null)

    expect(screen.queryByRole('button', { name: 'Reset by email' })).not.toBeInTheDocument()
  })

  it('sends without touching the change form, then confirms in the approved words and waits', async () => {
    mockSendOwn.mockResolvedValue({ success: true })
    const { user } = renderCard()

    await user.click(screen.getByRole('button', { name: 'Reset by email' }))

    expect(mockSendOwn).toHaveBeenCalledTimes(1)
    await waitFor(() => {
      expect(screen.getByRole('status')).toHaveTextContent(SENT)
    })
    // Once above the fields and once in the status region.
    expect(screen.getAllByText(SENT)).toHaveLength(2)
    expect(screen.getByRole('button', { name: 'Reset by email' })).toBeDisabled()
    expect(screen.queryByText('Current password is required')).not.toBeInTheDocument()
  })

  it('confirms nothing when the send fails', async () => {
    mockSendOwn.mockRejectedValue(Object.assign(new Error('Service unavailable'), { status: 503 }))
    const { user } = renderCard()

    await user.click(screen.getByRole('button', { name: 'Reset by email' }))

    await waitFor(() => {
      expect(screen.getByRole('alert')).toHaveTextContent(
        'We could not send that email just now. Please try again in a moment.'
      )
    })
    expect(screen.queryByText(SENT)).not.toBeInTheDocument()
  })

  it('treats a throttle as a wait, not a failure', async () => {
    mockSendOwn.mockRejectedValue(Object.assign(new Error('Too many requests'), { status: 429, retryAfter: 42 }))
    const { user } = renderCard()

    await user.click(screen.getByRole('button', { name: 'Reset by email' }))

    expect(await screen.findByText('Resend available in 42s')).toBeInTheDocument()
    expect(screen.queryByRole('alert')).not.toBeInTheDocument()
    expect(screen.queryByText(SENT)).not.toBeInTheDocument()
  })

  it('asks a reader whose session died to sign in again', async () => {
    mockSendOwn.mockRejectedValue(Object.assign(new Error('Unauthorized'), { status: 401 }))
    const { user } = renderCard()

    await user.click(screen.getByRole('button', { name: 'Reset by email' }))

    const alert = await screen.findByRole('alert')
    expect(alert).toHaveTextContent('Your session has expired. Sign in again to send the email.')
    expect(screen.getByRole('link', { name: 'Sign in again' })).toHaveAttribute('href', '/auth?returnTo=%2Fprofile')
  })
})
