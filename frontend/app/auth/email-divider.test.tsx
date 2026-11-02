import { describe, it, expect, vi, beforeEach } from 'vitest'
import { screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { renderWithProviders } from '@/test/utils'
import { buildReauthHref } from '@/lib/auth-href'
import AuthPage from './page'

// The email group on both tabs: the divider that names it, the text link that
// sends a sign-in link under Sign in, and the password fields' placeholders.

let mockSearchParams = new URLSearchParams()
vi.mock('next/navigation', () => ({
  useRouter: () => ({ push: vi.fn(), replace: vi.fn() }),
  useSearchParams: () => mockSearchParams,
  usePathname: () => '/auth',
}))

let mockAuthStatus: 'anonymous' | 'authenticated' = 'anonymous'
vi.mock('@/lib/context/AuthContext', async () => {
  const { deriveMockAuthSignals } = await import('@/test/authFixture')
  return {
    useAuthContext: () =>
      deriveMockAuthSignals({ setUser: vi.fn(), authStatus: mockAuthStatus }),
  }
})

type SendMagicLinkOptions = { onSuccess?: (data: { success: boolean }) => void }
const mockSendMagicLink = vi.fn()
vi.mock('@/features/auth', () => ({
  useLogin: () => ({ mutate: vi.fn(), reset: vi.fn(), isPending: false, error: null }),
  useRegister: () => ({ mutate: vi.fn(), isPending: false, error: null as Error | null }),
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
vi.mock('@/app/auth/_components/passkey-signup', () => ({
  PasskeySignupButton: (): null => null,
}))
vi.mock('@/app/auth/_components/google-oauth-button', () => ({
  GoogleOAuthButton: (): null => null,
}))

const GROUP_NAME = 'Or continue with email'
const LINK_NAME = 'Email me a sign-in link instead'

function emailGroup() {
  return screen.getByRole('group', { name: GROUP_NAME })
}

function expectOneEmailGroup() {
  expect(screen.getAllByRole('group', { name: GROUP_NAME })).toHaveLength(1)
}

beforeEach(() => {
  mockSearchParams = new URLSearchParams()
  mockAuthStatus = 'anonymous'
  mockSendMagicLink.mockReset()
})

async function openCreateAccount() {
  const user = userEvent.setup()
  await user.click(screen.getByRole('tab', { name: 'Create account' }))
  return user
}

describe('the email group', () => {
  it('names the Sign in tab email fields with a sentence case legend', () => {
    renderWithProviders(<AuthPage />)

    const group = emailGroup()
    expect(group.tagName).toBe('FIELDSET')
    const legend = group.querySelector('legend')
    // The capitals are CSS; the text a screen reader reads is sentence case.
    expect(legend?.textContent).toBe(GROUP_NAME)
    expect(within(legend as HTMLElement).getByText(GROUP_NAME)).toHaveClass(
      'font-mono',
      'uppercase',
      'text-muted-foreground'
    )
    expect(within(group).getByLabelText('Email')).toBeInTheDocument()
    expect(within(group).getByLabelText('Password')).toBeInTheDocument()
    expectOneEmailGroup()
  })

  it('names the Create account tab email fields, with one divider only', async () => {
    renderWithProviders(<AuthPage />)
    await openCreateAccount()

    const group = emailGroup()
    expect(group.querySelector('legend')?.textContent).toBe(GROUP_NAME)
    expect(within(group).getByLabelText('Email')).toBeInTheDocument()
    expect(within(group).getByLabelText('Password')).toBeInTheDocument()
    expectOneEmailGroup()
    expect(screen.queryByRole('button', { name: LINK_NAME })).not.toBeInTheDocument()
  })

  it('names the email fields on the re-auth arrival and offers the text link', async () => {
    mockAuthStatus = 'authenticated'
    mockSearchParams = new URLSearchParams(buildReauthHref('/settings').split('?')[1])
    renderWithProviders(<AuthPage />)

    await screen.findByRole('heading', { level: 1, name: "Confirm it's you" })
    expect(emailGroup().querySelector('legend')?.textContent).toBe(GROUP_NAME)
    expectOneEmailGroup()
    expect(screen.getByRole('button', { name: LINK_NAME })).toBeInTheDocument()
  })
})

describe('the sign-in link under Sign in', () => {
  it('replaces the second divider and the outline button with one text link', () => {
    renderWithProviders(<AuthPage />)

    const link = screen.getByRole('button', { name: LINK_NAME })
    expect(link).toHaveAttribute('type', 'button')
    expect(link).toHaveClass('underline', 'text-sm', 'text-muted-foreground')
    expect(
      screen.queryByRole('button', { name: 'Email me a sign-in link' })
    ).not.toBeInTheDocument()
    expect(screen.queryByText(/^or$/i)).not.toBeInTheDocument()
  })

  it('sends the sign-in link for the typed email and shows the sent banner in its place', async () => {
    mockSendMagicLink.mockImplementation((_vars: { email: string }, options: SendMagicLinkOptions) => {
      options.onSuccess?.({ success: true })
    })
    const user = userEvent.setup()
    renderWithProviders(<AuthPage />)

    await user.type(screen.getByLabelText('Email'), 'user@example.com')
    await user.click(screen.getByRole('button', { name: LINK_NAME }))

    expect(mockSendMagicLink).toHaveBeenCalledTimes(1)
    expect(mockSendMagicLink.mock.calls[0][0]).toEqual({ email: 'user@example.com' })
    expect(screen.getByText(/Check your email for a sign-in link/)).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: LINK_NAME })).not.toBeInTheDocument()
  })

  it('asks for an email first and sends nothing when the field is empty', async () => {
    const user = userEvent.setup()
    renderWithProviders(<AuthPage />)

    await user.click(screen.getByRole('button', { name: LINK_NAME }))

    expect(mockSendMagicLink).not.toHaveBeenCalled()
    expect(screen.getByRole('alert')).toHaveTextContent('Please enter a valid email address first')
    expect(screen.getByRole('button', { name: LINK_NAME })).toBeInTheDocument()
  })
})

describe('placeholders', () => {
  it('keeps only the email format hint on Sign in', () => {
    renderWithProviders(<AuthPage />)

    expect(screen.getByLabelText('Email')).toHaveAttribute('placeholder', 'you@example.com')
    expect(screen.getByLabelText('Password')).not.toHaveAttribute('placeholder')
    expect(screen.queryByPlaceholderText('Enter your password')).not.toBeInTheDocument()
  })

  it('keeps only the email format hint on Create account', async () => {
    renderWithProviders(<AuthPage />)
    await openCreateAccount()

    expect(screen.getByLabelText('Email')).toHaveAttribute('placeholder', 'you@example.com')
    expect(screen.getByLabelText('Password')).not.toHaveAttribute('placeholder')
    expect(screen.queryByPlaceholderText('Create a password')).not.toBeInTheDocument()
  })
})
