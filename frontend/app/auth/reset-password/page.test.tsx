import { describe, it, expect, vi, beforeEach } from 'vitest'
import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { renderWithProviders } from '@/test/utils'
import { AuthError, AuthErrorCode, type AuthErrorCodeType } from '@/lib/errors'
import ResetPasswordPage from './page'

// The page an emailed reset link opens: State 3 (the form), 4A (set and
// signed in), 5 (the link no longer works), and the shipped treatments for a
// closed account and a server failure.

let mockToken: string | null = 'reset-link'
vi.mock('next/navigation', () => ({
  useRouter: () => ({ push: vi.fn(), replace: vi.fn() }),
  useSearchParams: () => new URLSearchParams(mockToken === null ? '' : `token=${mockToken}`),
  usePathname: () => '/auth/reset-password',
}))

const mockSetUser = vi.fn()
vi.mock('@/lib/context/AuthContext', () => ({
  useAuthContext: () => ({ setUser: mockSetUser }),
}))

interface MutationState {
  isSuccess: boolean
  isError: boolean
  isPending: boolean
  data: unknown
  error: unknown
}

const idle = (): MutationState => ({ isSuccess: false, isError: false, isPending: false, data: undefined, error: null })

let checkState: MutationState = idle()
let completeState: MutationState = idle()
const mockCheck = vi.fn()
const mockCheckReset = vi.fn()
const mockComplete = vi.fn()

vi.mock('@/features/auth', () => ({
  useCheckPasswordReset: () => ({ ...checkState, mutate: mockCheck, reset: mockCheckReset }),
  useCompletePasswordReset: () => ({ ...completeState, mutate: mockComplete }),
}))

function refusal(code: string, message = 'refused') {
  return new AuthError(message, code as AuthErrorCodeType, { status: 400 })
}

function checked(email = 'owner@example.com'): MutationState {
  return { ...idle(), isSuccess: true, data: { success: true, email } }
}

const NEW_PASSWORD = 'owners-new-password-1!'

async function fillAndSubmit() {
  const user = userEvent.setup()
  await user.type(screen.getByLabelText('New password'), NEW_PASSWORD)
  await user.type(screen.getByLabelText('Confirm password'), NEW_PASSWORD)
  await user.click(screen.getByRole('button', { name: 'Set new password' }))
  return user
}

function expectLinkNoLongerWorks() {
  expect(screen.getByRole('heading', { name: 'That link no longer works.' })).toBeInTheDocument()
  expect(screen.getByText('Password reset · link no longer works')).toBeInTheDocument()
  expect(
    screen.getByText(
      'Reset links work once and expire 15 minutes after they are sent. Your password has not changed.'
    )
  ).toBeInTheDocument()
  expect(screen.getByRole('link', { name: 'Request a new link' })).toHaveAttribute('href', '/auth')
  expect(screen.getByRole('link', { name: 'Sign in' })).toHaveAttribute('href', '/auth')
}

describe('Reset password page', () => {
  beforeEach(() => {
    mockToken = 'reset-link'
    checkState = idle()
    completeState = idle()
    mockCheck.mockReset()
    mockCheckReset.mockReset()
    mockComplete.mockReset()
    mockSetUser.mockReset()
  })

  it('checks the link once on open and spends nothing', () => {
    const { rerender } = renderWithProviders(<ResetPasswordPage />)
    rerender(<ResetPasswordPage />)

    expect(mockCheck).toHaveBeenCalledTimes(1)
    expect(mockCheck).toHaveBeenCalledWith('reset-link')
    expect(mockComplete).not.toHaveBeenCalled()
  })

  it('shows State 5 for a link with no token, and checks nothing', () => {
    mockToken = null
    renderWithProviders(<ResetPasswordPage />)

    expectLinkNoLongerWorks()
    expect(mockCheck).not.toHaveBeenCalled()
  })

  it('shows State 5 when the check says the link no longer works', () => {
    checkState = { ...idle(), isError: true, error: refusal(AuthErrorCode.INVALID_TOKEN) }
    renderWithProviders(<ResetPasswordPage />)

    expectLinkNoLongerWorks()
  })

  it('shows the form for a live link, with the address and the approved copy', () => {
    checkState = checked()
    renderWithProviders(<ResetPasswordPage />)

    expect(screen.getByRole('heading', { name: 'Reset your password' })).toBeInTheDocument()
    expect(screen.getByText((_, node) => node?.tagName === 'P' && node.textContent === 'For owner@example.com')).toBeInTheDocument()
    expect(screen.getByText('When you set it')).toBeInTheDocument()
    expect(
      screen.getByText('Every other device signed in to this account is signed out. Your passkeys keep working.')
    ).toBeInTheDocument()
    expect(screen.getByRole('link', { name: 'Cancel and go back to sign in' })).toHaveAttribute('href', '/auth')
    expect(screen.queryByLabelText(/current password/i)).not.toBeInTheDocument()
  })

  it('sets the password with the link and the typed password', async () => {
    checkState = checked()
    renderWithProviders(<ResetPasswordPage />)

    await fillAndSubmit()

    await waitFor(() => expect(mockComplete).toHaveBeenCalledTimes(1))
    expect(mockComplete.mock.calls[0][0]).toEqual({ token: 'reset-link', newPassword: NEW_PASSWORD })
    const user = { id: '9', email: 'owner@example.com', email_verified: true }
    mockComplete.mock.calls[0][1].onSuccess({ success: true, user })
    expect(mockSetUser).toHaveBeenCalledWith(user)
  })

  it('refuses mismatched passwords before sending anything', async () => {
    checkState = checked()
    renderWithProviders(<ResetPasswordPage />)
    const user = userEvent.setup()

    await user.type(screen.getByLabelText('New password'), NEW_PASSWORD)
    await user.type(screen.getByLabelText('Confirm password'), 'something-else-entirely-1!')
    await user.click(screen.getByRole('button', { name: 'Set new password' }))

    expect(mockComplete).not.toHaveBeenCalled()
  })

  it('shows State 4A once the password is set', () => {
    checkState = checked()
    completeState = { ...idle(), isSuccess: true, data: { success: true } }
    renderWithProviders(<ResetPasswordPage />)

    const heading = screen.getByRole('heading', { name: 'New password set.' })
    expect(heading).toHaveFocus()
    expect(screen.getByText('Password reset · signed in')).toBeInTheDocument()
    expect(
      screen.getByText("You're signed in on this device. Every other device was signed out. Your passkeys still work.")
    ).toBeInTheDocument()
    expect(screen.getByRole('link', { name: 'Continue' })).toHaveAttribute('href', '/')
    expect(screen.getByRole('link', { name: 'Review signed-in devices' })).toHaveAttribute(
      'href',
      '/profile?tab=settings#sessions'
    )
  })

  it('shows the validator refusal above the fields and keeps the form', () => {
    checkState = checked()
    completeState = {
      ...idle(),
      isError: true,
      error: refusal(AuthErrorCode.VALIDATION_FAILED, 'This password has appeared in a data breach. Please choose a different password.'),
    }
    renderWithProviders(<ResetPasswordPage />)

    expect(screen.getByRole('alert')).toHaveTextContent(
      'This password has appeared in a data breach. Please choose a different password.'
    )
    expect(screen.getByLabelText('New password')).toBeInTheDocument()
  })

  it('shows State 5 when the set finds the link already used', () => {
    checkState = checked()
    completeState = { ...idle(), isError: true, error: refusal(AuthErrorCode.INVALID_TOKEN) }
    renderWithProviders(<ResetPasswordPage />)

    expectLinkNoLongerWorks()
  })

  it('keeps the form on a server failure of the set, so the same link is retried', () => {
    checkState = checked()
    completeState = {
      ...idle(),
      isError: true,
      error: Object.assign(new Error('Service temporarily unavailable'), { status: 503 }),
    }
    renderWithProviders(<ResetPasswordPage />)

    expect(screen.getByRole('alert')).toHaveTextContent('Service temporarily unavailable')
    expect(screen.getByRole('button', { name: 'Set new password' })).toBeInTheDocument()
  })

  it('offers a retry of the same link when the check fails on our end', async () => {
    checkState = { ...idle(), isError: true, error: Object.assign(new Error('boom'), { status: 503 }) }
    renderWithProviders(<ResetPasswordPage />)

    expect(screen.getByText('Something went wrong')).toBeInTheDocument()
    await userEvent.setup().click(screen.getByRole('button', { name: 'Try Again' }))
    expect(mockCheckReset).toHaveBeenCalled()
  })

  it('shows the closed-account card for an account that may not sign in', () => {
    checkState = { ...idle(), isError: true, error: refusal(AuthErrorCode.ACCOUNT_INACTIVE) }
    renderWithProviders(<ResetPasswordPage />)

    expect(screen.getByText("Can't sign you in")).toBeInTheDocument()
    expect(screen.queryByLabelText('New password')).not.toBeInTheDocument()
  })
})
