import { describe, it, expect, vi, beforeEach } from 'vitest'
import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { renderWithProviders } from '@/test/utils'
import { EMAIL_SEND_FAILED_MESSAGE } from '@/features/auth/components/verification-resend'
import AuthPage from './page'

// The Sign in tab's two emailed links: "Forgot password?" asks for the
// password reset link and swaps in its sent state; "Email me a sign-in link
// instead" still sends the sign-in link.

vi.mock('next/navigation', () => ({
  useRouter: () => ({ push: vi.fn(), replace: vi.fn() }),
  useSearchParams: () => new URLSearchParams(),
  usePathname: () => '/auth',
}))

vi.mock('@/lib/context/AuthContext', async () => {
  const { deriveMockAuthSignals } = await import('@/test/authFixture')
  return {
    useAuthContext: () =>
      deriveMockAuthSignals({ setUser: vi.fn(), authStatus: 'anonymous' }),
  }
})

type MutateOptions = { onSuccess?: (data: unknown) => void; onError?: (error: unknown) => void }

const mockRequestReset = vi.fn<(email: string, options?: MutateOptions) => void>()
const mockRequestResetAsync = vi.fn<(email: string) => Promise<unknown>>()
const mockSendMagicLink = vi.fn()

vi.mock('@/features/auth', () => ({
  useLogin: () => ({ mutate: vi.fn(), reset: vi.fn(), isPending: false, error: null }),
  useRegister: () => ({ mutate: vi.fn(), isPending: false, error: null as Error | null }),
  useRequestPasswordReset: () => ({
    mutate: mockRequestReset,
    mutateAsync: mockRequestResetAsync,
    isPending: false,
  }),
  useSendMagicLink: () => ({ mutate: mockSendMagicLink, isPending: false }),
  useSendVerificationEmail: () => ({
    mutate: vi.fn(),
    isPending: false,
    isSuccess: false,
    isError: false,
    error: null as Error | null,
  }),
}))

vi.mock('@/app/auth/_components/passkey-login', () => ({
  PasskeyLoginButton: (): null => null,
}))

vi.mock('@/app/auth/_components/google-oauth-button', () => ({
  GoogleOAuthButton: (): null => null,
}))

async function askForReset(email: string) {
  const user = userEvent.setup()
  renderWithProviders(<AuthPage />)
  if (email) await user.type(screen.getByLabelText('Email'), email)
  await user.click(screen.getByRole('button', { name: 'Forgot password?' }))
  return user
}

describe('Forgot password? on the Sign in tab', () => {
  beforeEach(() => {
    mockRequestReset.mockReset()
    mockRequestResetAsync.mockReset()
    mockSendMagicLink.mockReset()
  })

  it('asks for the reset link for the typed address, not a sign-in link', async () => {
    await askForReset('owner@example.com')

    expect(mockRequestReset).toHaveBeenCalledTimes(1)
    expect(mockRequestReset.mock.calls[0][0]).toBe('owner@example.com')
    expect(mockSendMagicLink).not.toHaveBeenCalled()
  })

  it('asks for a valid address first and sends nothing', async () => {
    await askForReset('not-an-address')

    expect(screen.getByRole('alert')).toHaveTextContent('Please enter a valid email address first')
    expect(mockRequestReset).not.toHaveBeenCalled()
  })

  it('shows the sent state in place, with the approved copy and the address', async () => {
    mockRequestReset.mockImplementation((_email, options) => options?.onSuccess?.({ success: true }))
    await askForReset('owner@example.com')

    const heading = await screen.findByRole('heading', { name: 'Check your inbox.' })
    expect(heading).toHaveFocus()
    expect(screen.getByText('One email sent · password reset')).toHaveClass(
      'font-data',
      'text-xs',
      'uppercase',
      'tracking-[0.66px]',
      'text-muted-foreground'
    )
    expect(
      screen.getByText((_, node) =>
        node?.tagName === 'P' &&
        node.textContent ===
          'If an account uses owner@example.com, we sent it a link to choose a new password. It expires in 15 minutes.'
      )
    ).toBeInTheDocument()
    expect(screen.getByText('Nothing changes until you open it')).toHaveClass(
      'font-data',
      'text-xs',
      'uppercase',
      'tracking-[0.66px]',
      'text-muted-foreground'
    )
    expect(
      screen.getByText(
        'Your current password, your signed-in devices and your passkeys stay as they are. The link lets you choose a new password, and setting it signs out every other device.'
      )
    ).toBeInTheDocument()
    expect(screen.queryByLabelText('Password')).not.toBeInTheDocument()
  })

  it('resends to the same address from the sent state', async () => {
    mockRequestReset.mockImplementation((_email, options) => options?.onSuccess?.({ success: true }))
    mockRequestResetAsync.mockResolvedValue({ success: true })
    const user = await askForReset('owner@example.com')

    await user.click(await screen.findByRole('button', { name: 'Resend email' }))

    expect(mockRequestResetAsync).toHaveBeenCalledWith('owner@example.com')
    await waitFor(() => {
      expect(screen.getByRole('status')).toHaveTextContent('Sent again. Give it a minute to arrive.')
    })
    expect(screen.getByText(/^Resend available in \d+s$/)).toHaveClass(
      'font-data',
      'text-xs',
      'uppercase',
      'tracking-[0.66px]',
      'text-link'
    )
    expect(screen.getByRole('button', { name: 'Resend email' })).toBeDisabled()
  })

  it('goes back to the Sign in tab, focused, from the sent state', async () => {
    mockRequestReset.mockImplementation((_email, options) => options?.onSuccess?.({ success: true }))
    const user = await askForReset('owner@example.com')

    await user.click(await screen.findByRole('button', { name: 'Back to sign in' }))

    expect(screen.getByLabelText('Password')).toBeInTheDocument()
    await waitFor(() => {
      expect(screen.getByRole('tab', { name: 'Sign in' })).toHaveFocus()
    })
  })

  it('says a failed request failed, and stays on the form', async () => {
    mockRequestReset.mockImplementation((_email, options) =>
      options?.onError?.(Object.assign(new Error('Service unavailable'), { status: 503 }))
    )
    await askForReset('owner@example.com')

    expect(screen.getByRole('alert')).toHaveTextContent(EMAIL_SEND_FAILED_MESSAGE)
    expect(screen.queryByRole('heading', { name: 'Check your inbox.' })).not.toBeInTheDocument()
  })

  it('shows the server\'s words for an address it refused as malformed', async () => {
    mockRequestReset.mockImplementation((_email, options) =>
      options?.onError?.(Object.assign(new Error('Email must be a valid email address'), { status: 400 }))
    )
    await askForReset('owner@example.com')

    expect(screen.getByRole('alert')).toHaveTextContent('Email must be a valid email address')
  })

  it('leaves "Email me a sign-in link instead" sending the sign-in link', async () => {
    const user = userEvent.setup()
    renderWithProviders(<AuthPage />)
    await user.type(screen.getByLabelText('Email'), 'owner@example.com')

    await user.click(screen.getByRole('button', { name: 'Email me a sign-in link instead' }))

    expect(mockSendMagicLink).toHaveBeenCalledTimes(1)
    expect(mockSendMagicLink.mock.calls[0][0]).toEqual({ email: 'owner@example.com' })
    expect(mockRequestReset).not.toHaveBeenCalled()
  })
})
