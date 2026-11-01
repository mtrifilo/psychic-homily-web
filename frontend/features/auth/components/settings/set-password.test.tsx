import { describe, it, expect, vi, beforeEach } from 'vitest'
import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { renderWithProviders } from '@/test/utils'
import { AuthError, AuthErrorCode } from '@/lib/errors'
import { SetPassword, passwordlessSignInDescription } from './set-password'

const mockMutate = vi.fn()
let mockMutationState = {
  isPending: false,
  isError: false,
  error: null as unknown,
}

vi.mock('@/features/auth', () => ({
  useSetPassword: () => ({
    mutate: mockMutate,
    ...mockMutationState,
  }),
}))

const RETURN_TO = '/profile?tab=settings#password'
const STRONG = 'Correct-Horse-Battery-9'

function renderCard(props: Partial<Parameters<typeof SetPassword>[0]> = {}) {
  const user = userEvent.setup()
  const onPasswordSet = vi.fn()
  renderWithProviders(
    <SetPassword
      providers={['google']}
      passkeyCount={0}
      reauthReturnTo={RETURN_TO}
      onPasswordSet={onPasswordSet}
      {...props}
    />
  )
  return { user, onPasswordSet }
}

async function openForm(user: ReturnType<typeof userEvent.setup>) {
  await user.click(screen.getByRole('button', { name: 'Set a password' }))
}

async function fillValid(user: ReturnType<typeof userEvent.setup>) {
  await user.type(screen.getByLabelText('New password'), STRONG)
  await user.type(screen.getByLabelText('Confirm password'), STRONG)
}

describe('passwordlessSignInDescription', () => {
  it.each([
    [['google'], 0, 'You sign in with Google. This account has no password.'],
    [['google'], 2, 'You sign in with Google or a passkey. This account has no password.'],
    [[], 1, 'You sign in with a passkey.'],
    [['github', 'google'], 0, 'You sign in with Google. This account has no password.'],
  ])('providers %j with %d passkeys', (providers, passkeys, want) => {
    expect(passwordlessSignInDescription(providers, passkeys)).toBe(want)
  })

  // No approved sentence names another provider, so none is claimed.
  it.each([
    [[], 0],
    [['github'], 0],
    [['apple'], 0],
  ])('providers %j with %d passkeys has no sentence', (providers, passkeys) => {
    expect(passwordlessSignInDescription(providers, passkeys)).toBeNull()
  })
})

describe('SetPassword', () => {
  beforeEach(() => {
    mockMutate.mockReset()
    mockMutationState = { isPending: false, isError: false, error: null }
  })

  it('shows the Password card with its description, line and button', () => {
    renderCard({ passkeyCount: 1 })

    expect(screen.getByText('Password')).toBeInTheDocument()
    expect(
      screen.getByText('You sign in with Google or a passkey. This account has no password.')
    ).toBeInTheDocument()
    expect(
      screen.getByText(
        'Add one to also sign in with your email and password. Your other devices stay signed in.'
      )
    ).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Set a password' })).toBeEnabled()
    expect(screen.queryByLabelText('New password')).toBeNull()
  })

  it('omits the description for an account no sentence describes', () => {
    renderCard({ providers: ['github'], passkeyCount: 0 })

    expect(screen.queryByText(/You sign in with/)).toBeNull()
    expect(screen.getByRole('button', { name: 'Set a password' })).toBeInTheDocument()
  })

  it('opens the form with no current-password field and the stale-proof line', async () => {
    const { user } = renderCard()
    await openForm(user)

    expect(screen.getByText('Set a password')).toBeInTheDocument()
    expect(
      screen.getByText(
        'Adds email and password as a way to sign in. Your other devices stay signed in.'
      )
    ).toBeInTheDocument()
    expect(screen.getByLabelText('New password')).toBeInTheDocument()
    expect(screen.getByLabelText('Confirm password')).toBeInTheDocument()
    expect(screen.queryByLabelText('Current password')).toBeNull()
    expect(screen.getByRole('button', { name: 'Set password' })).toBeEnabled()
    expect(screen.getByRole('button', { name: 'Cancel' })).toBeInTheDocument()
    expect(
      screen.getByText("If you signed in a while ago, we'll ask you to sign in again first.")
    ).toBeInTheDocument()

    await user.type(screen.getByLabelText('New password'), 'abc')
    expect(screen.getByText('Password strength')).toBeInTheDocument()
  })

  it('cancel closes the form', async () => {
    const { user } = renderCard()
    await openForm(user)
    await user.click(screen.getByRole('button', { name: 'Cancel' }))

    expect(screen.queryByLabelText('New password')).toBeNull()
    expect(screen.getByRole('button', { name: 'Set a password' })).toBeInTheDocument()
  })

  it('shows field errors and disables submit while they stand', async () => {
    const { user } = renderCard()
    await openForm(user)
    await user.type(screen.getByLabelText('New password'), 'short')
    await user.type(screen.getByLabelText('Confirm password'), 'shorter')

    await waitFor(() => {
      expect(screen.getByText('Password must be at least 12 characters')).toBeInTheDocument()
      expect(screen.getByText('Passwords do not match')).toBeInTheDocument()
    })
    expect(screen.getByRole('button', { name: 'Set password' })).toBeDisabled()
    expect(mockMutate).not.toHaveBeenCalled()
  })

  it('submits the new password and reports the confirmation on success', async () => {
    mockMutate.mockImplementation((_password: string, options?: { onSuccess?: () => void }) => {
      options?.onSuccess?.()
    })
    const { user, onPasswordSet } = renderCard()
    await openForm(user)
    await fillValid(user)
    expect(screen.getByText('Passwords match')).toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: 'Set password' }))

    await waitFor(() => expect(mockMutate).toHaveBeenCalledTimes(1))
    expect(mockMutate.mock.calls[0][0]).toBe(STRONG)
    expect(onPasswordSet).toHaveBeenCalledWith(
      'Password set. You can now also sign in with your email and password.'
    )
  })

  it('a stale sign-in shows the re-auth alert linking back to this card', async () => {
    mockMutationState = {
      isPending: false,
      isError: true,
      error: new AuthError('refused', AuthErrorCode.REAUTH_REQUIRED, { status: 403 }),
    }
    const { user } = renderCard()
    await openForm(user)

    const alert = screen.getByRole('alert')
    expect(alert).toHaveTextContent('For security, sign in again before making this change.')
    const link = screen.getByRole('link', { name: 'Sign in again' })
    expect(link).toHaveAttribute(
      'href',
      `/auth?returnTo=${encodeURIComponent(RETURN_TO)}&reason=REAUTH_REQUIRED`
    )
  })

  it('a password already set shows the reload line, not the response text', async () => {
    mockMutationState = {
      isPending: false,
      isError: true,
      error: new AuthError('server words', AuthErrorCode.PASSWORD_ALREADY_SET, { status: 400 }),
    }
    const { user } = renderCard()
    await openForm(user)

    expect(screen.getByRole('alert')).toHaveTextContent(
      'This account already has a password. Reload the page to change it.'
    )
    expect(screen.queryByRole('link', { name: 'Sign in again' })).toBeNull()
  })

  it('a validator refusal shows the server message', async () => {
    mockMutationState = {
      isPending: false,
      isError: true,
      error: new AuthError(
        'This password has been exposed in a data breach and should not be used',
        AuthErrorCode.VALIDATION_FAILED,
        { status: 400 }
      ),
    }
    const { user } = renderCard()
    await openForm(user)

    expect(screen.getByRole('alert')).toHaveTextContent(
      'This password has been exposed in a data breach and should not be used'
    )
  })

  it('a throttled set shows the shared throttle line', async () => {
    const throttled = Object.assign(new Error('Too Many Requests'), { status: 429, retryAfter: 60 })
    mockMutationState = { isPending: false, isError: true, error: throttled }
    const { user } = renderCard()
    await openForm(user)

    expect(screen.getByRole('alert')).toHaveTextContent('Too many password attempts. Try again in 60s.')
  })
})
