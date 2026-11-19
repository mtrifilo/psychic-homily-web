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
let mockAuthStatus = 'anonymous'
vi.mock('@/lib/context/AuthContext', () => ({
  useAuthContext: () => ({ setUser: mockSetUser, authStatus: mockAuthStatus }),
}))

interface RequestState {
  isSuccess: boolean
  isError: boolean
  isPending: boolean
  data: unknown
  error: unknown
}

const idle = (): RequestState => ({ isSuccess: false, isError: false, isPending: true, data: undefined, error: null })

let checkState: RequestState = idle()
let completeState: RequestState = { ...idle(), isPending: false }
const mockCheck = vi.fn()
const mockCheckRefetch = vi.fn()
const mockComplete = vi.fn()
// The calendar feed status State 4A reads; undefined while loading or failed.
let feedStatus: unknown = undefined
const mockFeedStatus = vi.fn()

vi.mock('@/features/auth', async () => ({
  useCheckPasswordReset: (token: string | null) => {
    mockCheck(token)
    return { ...checkState, refetch: mockCheckRefetch }
  },
  useCompletePasswordReset: () => ({ ...completeState, mutate: mockComplete }),
  useCalendarFeedStatus: () => {
    mockFeedStatus()
    return { data: feedStatus }
  },
  calendarLinkWasResetBy: (
    await vi.importActual<typeof import('@/features/auth/hooks/useCalendarFeed')>(
      '@/features/auth/hooks/useCalendarFeed'
    )
  ).calendarLinkWasResetBy,
}))

function refusal(code: string, message = 'refused') {
  return new AuthError(message, code as AuthErrorCodeType, { status: 400 })
}

function checked(email = 'owner@example.com'): RequestState {
  return { ...idle(), isPending: false, isSuccess: true, data: { success: true, email } }
}

function checkRefused(error: unknown): RequestState {
  return { ...idle(), isPending: false, isError: true, error }
}

function submitted(state: Partial<RequestState>): RequestState {
  return { ...idle(), isPending: false, ...state }
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
  expect(screen.getByText('Password reset · link no longer works')).toHaveClass(
    'font-data',
    'text-xs',
    'uppercase',
    'tracking-[0.66px]',
    'text-muted-foreground'
  )
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
    mockAuthStatus = 'anonymous'
    checkState = idle()
    completeState = submitted({})
    mockCheck.mockReset()
    mockCheckRefetch.mockReset()
    mockComplete.mockReset()
    mockSetUser.mockReset()
    mockFeedStatus.mockReset()
    feedStatus = undefined
  })

  it('checks the link on open, waits for the answer and spends nothing', () => {
    renderWithProviders(<ResetPasswordPage />)

    expect(mockCheck).toHaveBeenCalledWith('reset-link')
    expect(screen.getByRole('status', { name: 'Checking your link' })).toBeInTheDocument()
    expect(mockComplete).not.toHaveBeenCalled()
  })

  it('shows State 5 for a link with no token, and checks nothing', () => {
    mockToken = null
    renderWithProviders(<ResetPasswordPage />)

    expectLinkNoLongerWorks()
    expect(mockCheck).toHaveBeenCalledWith(null)
  })

  it('shows State 5 when the check says the link no longer works', () => {
    checkState = checkRefused(refusal(AuthErrorCode.INVALID_TOKEN))
    renderWithProviders(<ResetPasswordPage />)

    expectLinkNoLongerWorks()
    expect(screen.getByRole('heading', { name: 'That link no longer works.' })).toHaveFocus()
  })

  it('shows the form for a live link, with the address and the approved copy', () => {
    checkState = checked()
    renderWithProviders(<ResetPasswordPage />)

    expect(screen.getByRole('heading', { name: 'Reset your password' })).toBeInTheDocument()
    expect(screen.getByText((_, node) => node?.tagName === 'P' && node.textContent === 'For owner@example.com')).toBeInTheDocument()
    expect(screen.getByText('When you set it')).toHaveClass(
      'font-data',
      'text-xs',
      'uppercase',
      'tracking-[0.66px]',
      'text-muted-foreground'
    )
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
    completeState = submitted({ isSuccess: true, data: { success: true } })
    renderWithProviders(<ResetPasswordPage />)

    const heading = screen.getByRole('heading', { name: 'New password set.' })
    expect(heading).toHaveFocus()
    expect(screen.getByText('Password reset · signed in')).toHaveClass(
      'font-data',
      'text-xs',
      'uppercase',
      'tracking-[0.66px]',
      'text-muted-foreground'
    )
    expect(
      screen.getByText("You're signed in on this device. Every other device was signed out. Your passkeys still work.")
    ).toBeInTheDocument()
    expect(screen.getByRole('link', { name: 'Continue' })).toHaveAttribute('href', '/')
    expect(screen.getByRole('link', { name: 'Review signed-in devices' })).toHaveAttribute(
      'href',
      '/profile?tab=settings#sessions'
    )
    expect(screen.queryByText(/calendar/i)).not.toBeInTheDocument()
  })

  describe('State 4A and the calendar link', () => {
    const SIGNED_IN_LINE =
      "You're signed in on this device. Every other device was signed out. Your passkeys still work."
    const RESET_LINE = `${SIGNED_IN_LINE} Your calendar link was reset too; add it again in Settings.`
    const resetBy = (reason: string) => ({
      has_token: false,
      link_reset_at: '2026-11-10T12:00:00Z',
      link_reset_reason: reason,
    })

    function renderDone() {
      checkState = checked()
      completeState = submitted({ isSuccess: true, data: { success: true } })
      renderWithProviders(<ResetPasswordPage />)
      return screen.getByRole('heading', { name: 'New password set.' })
    }

    function signedInParagraph() {
      return screen.getByText((_, node) =>
        node?.tagName === 'P' && (node.textContent ?? '').startsWith("You're signed in")
      )
    }

    it('appends the reset sentence when the reset left its marker, linking to the calendar card', () => {
      feedStatus = resetBy('password_reset')
      renderDone()

      expect(signedInParagraph().textContent).toBe(RESET_LINE)
      const settings = screen.getByRole('link', { name: 'Settings' })
      expect(settings).toHaveAttribute('href', '/profile?tab=settings#calendar')
      expect(settings).toHaveClass('text-link', 'underline')
    })

    it('reads the status only once the reset has succeeded', () => {
      checkState = checked()
      renderWithProviders(<ResetPasswordPage />)

      expect(screen.getByRole('heading', { name: 'Reset your password' })).toBeInTheDocument()
      expect(mockFeedStatus).not.toHaveBeenCalled()

      renderDone()
      expect(mockFeedStatus).toHaveBeenCalled()
    })

    it('leaves the line as shipped while the status is unknown', () => {
      feedStatus = undefined
      renderDone()

      expect(signedInParagraph().textContent).toBe(SIGNED_IN_LINE)
      expect(screen.queryByRole('link', { name: 'Settings' })).not.toBeInTheDocument()
    })

    it('leaves the line as shipped when no link was reset', () => {
      feedStatus = { has_token: false }
      renderDone()

      expect(signedInParagraph().textContent).toBe(SIGNED_IN_LINE)
    })

    it('ignores a marker another action left', () => {
      feedStatus = resetBy('sign_out_everywhere')
      renderDone()

      expect(signedInParagraph().textContent).toBe(SIGNED_IN_LINE)
    })

    it('leaves the "When you set it" box silent about the calendar', () => {
      feedStatus = resetBy('password_reset')
      checkState = checked()
      renderWithProviders(<ResetPasswordPage />)

      expect(
        screen.getByText('Every other device signed in to this account is signed out. Your passkeys keep working.')
      ).toBeInTheDocument()
      expect(screen.queryByText(/calendar/i)).not.toBeInTheDocument()
    })
  })

  it('shows the validator refusal above the fields and keeps the form', () => {
    checkState = checked()
    completeState = submitted({
      isError: true,
      error: refusal(AuthErrorCode.VALIDATION_FAILED, 'This password has appeared in a data breach. Please choose a different password.'),
    })
    renderWithProviders(<ResetPasswordPage />)

    expect(screen.getByRole('alert')).toHaveTextContent(
      'This password has appeared in a data breach. Please choose a different password.'
    )
    expect(screen.getByLabelText('New password')).toBeInTheDocument()
  })

  it('shows State 5 when the set finds the link already used', () => {
    checkState = checked()
    completeState = submitted({ isError: true, error: refusal(AuthErrorCode.INVALID_TOKEN) })
    renderWithProviders(<ResetPasswordPage />)

    expectLinkNoLongerWorks()
  })

  it('keeps the form on a server failure of the set, so the same link is retried', () => {
    checkState = checked()
    completeState = submitted({
      isError: true,
      error: Object.assign(new Error('Service temporarily unavailable'), { status: 503 }),
    })
    renderWithProviders(<ResetPasswordPage />)

    expect(screen.getByRole('alert')).toHaveTextContent('Service temporarily unavailable')
    expect(screen.getByRole('button', { name: 'Set new password' })).toBeInTheDocument()
  })

  it('offers a retry of the same link when the check fails on our end', async () => {
    checkState = checkRefused(Object.assign(new Error('boom'), { status: 503 }))
    renderWithProviders(<ResetPasswordPage />)

    expect(screen.getByText('Something went wrong')).toBeInTheDocument()
    await userEvent.setup().click(screen.getByRole('button', { name: 'Try Again' }))
    expect(mockCheckRefetch).toHaveBeenCalled()
  })

  // A signed-in reader is sent on by /auth, so a new link comes from Settings.
  it('sends a signed-in reader to Settings for a new link', () => {
    mockAuthStatus = 'authenticated'
    checkState = checkRefused(refusal(AuthErrorCode.INVALID_TOKEN))
    renderWithProviders(<ResetPasswordPage />)

    expect(screen.getByRole('link', { name: 'Request a new link' })).toHaveAttribute(
      'href',
      '/profile?tab=settings#password'
    )
  })

  // A throttled check did not judge the link, so it is not called spent.
  it('offers a retry, not State 5, when the check is throttled', async () => {
    checkState = checkRefused(Object.assign(new Error('Too Many Requests'), { status: 429 }))
    renderWithProviders(<ResetPasswordPage />)

    expect(screen.queryByRole('heading', { name: 'That link no longer works.' })).not.toBeInTheDocument()
    expect(screen.getByText(/Your link still works/)).toBeInTheDocument()
    await userEvent.setup().click(screen.getByRole('button', { name: 'Try Again' }))
    expect(mockCheckRefetch).toHaveBeenCalled()
  })

  // One notice replacing another (a retried check that finds the account
  // closed) moves focus to the new heading, not to the page.
  it('moves focus when the retry card gives way to the closed-account card', async () => {
    checkState = checkRefused(Object.assign(new Error('boom'), { status: 503 }))
    const { rerender } = renderWithProviders(<ResetPasswordPage />)
    // The reader presses Try Again, which takes focus and then leaves with the card.
    await userEvent.setup().click(screen.getByRole('button', { name: 'Try Again' }))
    expect(screen.getByRole('button', { name: 'Try Again' })).toHaveFocus()

    checkState = checkRefused(refusal(AuthErrorCode.ACCOUNT_INACTIVE))
    rerender(<ResetPasswordPage />)

    expect(screen.getByRole('heading', { name: "Can't sign you in" })).toHaveFocus()
  })

  it('shows the closed-account card for an account that may not sign in', () => {
    checkState = checkRefused(refusal(AuthErrorCode.ACCOUNT_INACTIVE))
    renderWithProviders(<ResetPasswordPage />)

    expect(screen.getByRole('heading', { name: "Can't sign you in" })).toHaveFocus()
    expect(screen.queryByLabelText('New password')).not.toBeInTheDocument()
  })
})
