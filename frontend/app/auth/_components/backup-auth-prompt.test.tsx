import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { renderWithProviders } from '@/test/utils'
import { BackupAuthPrompt } from './backup-auth-prompt'

const OAUTH_BACKEND = 'https://oauth-backend.test'

vi.mock('@/lib/api-base', () => ({
  OAUTH_BACKEND_URL: 'https://oauth-backend.test',
}))

const mockStartLinkMutateAsync = vi.fn()
vi.mock('@/features/auth/hooks/useAuth', () => ({
  useStartOAuthLink: () => ({ mutateAsync: mockStartLinkMutateAsync }),
}))

const mockCaptureException = vi.fn()
vi.mock('@sentry/nextjs', () => ({
  captureException: (...args: unknown[]) => mockCaptureException(...args),
}))

const PROMPT = { name: 'Add a backup sign-in method' }
const CONNECT = { name: /Connect Google account/ }

const originalLocation = window.location
const navigate = vi.fn()

function renderPrompt() {
  const onOpenChange = vi.fn()
  const user = userEvent.setup()
  renderWithProviders(<BackupAuthPrompt open onOpenChange={onOpenChange} />)
  const prompt = screen.getByRole('dialog', PROMPT)
  return { user, prompt, onOpenChange }
}

describe('BackupAuthPrompt', () => {
  beforeEach(() => {
    mockStartLinkMutateAsync.mockReset()
    mockCaptureException.mockReset()
    navigate.mockReset()
    Object.defineProperty(window, 'location', {
      value: { ...originalLocation, href: '' },
      writable: true,
      configurable: true,
    })
    Object.defineProperty(window.location, 'href', {
      set: navigate,
      configurable: true,
    })
  })

  afterEach(() => {
    Object.defineProperty(window, 'location', {
      value: originalLocation,
      writable: true,
      configurable: true,
    })
  })

  it('starts the authenticated link route with a freshly minted token', async () => {
    // Characters a token must not carry raw in a query string, so the test
    // also pins the encoding.
    mockStartLinkMutateAsync.mockResolvedValue({ token: 'tok+en/=&x' })
    const { user, prompt } = renderPrompt()

    await user.click(within(prompt).getByRole('button', CONNECT))

    expect(mockStartLinkMutateAsync).toHaveBeenCalledTimes(1)
    await waitFor(() => {
      expect(navigate).toHaveBeenCalledWith(
        `${OAUTH_BACKEND}/auth/link/google?t=${encodeURIComponent('tok+en/=&x')}`
      )
    })
    expect(navigate).toHaveBeenCalledTimes(1)
  })

  it('does not navigate until the token exists, and holds the button meanwhile', async () => {
    let resolveMint: (value: { token: string }) => void = () => {}
    mockStartLinkMutateAsync.mockReturnValue(
      new Promise(resolve => {
        resolveMint = resolve
      })
    )
    const { user, prompt } = renderPrompt()

    await user.click(within(prompt).getByRole('button', CONNECT))

    expect(navigate).not.toHaveBeenCalled()
    expect(within(prompt).getByRole('button', CONNECT)).toBeDisabled()

    resolveMint({ token: 'later-token' })

    await waitFor(() => {
      expect(navigate).toHaveBeenCalledWith(
        `${OAUTH_BACKEND}/auth/link/google?t=later-token`
      )
    })
  })

  it('stays open with the button back when the token cannot be minted', async () => {
    const failure = new Error('mint refused')
    mockStartLinkMutateAsync.mockRejectedValue(failure)
    const { user, prompt, onOpenChange } = renderPrompt()

    await user.click(within(prompt).getByRole('button', CONNECT))

    await waitFor(() => {
      expect(within(prompt).getByRole('button', CONNECT)).toBeEnabled()
    })
    expect(navigate).not.toHaveBeenCalled()
    expect(onOpenChange).not.toHaveBeenCalled()
    expect(screen.getByRole('dialog', PROMPT)).toBeInTheDocument()
    expect(mockCaptureException).toHaveBeenCalledWith(
      failure,
      expect.objectContaining({ tags: { service: 'backup-auth-prompt' } })
    )
  })

  it('closes through onOpenChange when skipped, without starting a link', async () => {
    const { user, prompt, onOpenChange } = renderPrompt()

    await user.click(
      within(prompt).getByRole('button', { name: "I'll do this later" })
    )

    expect(onOpenChange).toHaveBeenCalledWith(false)
    expect(mockStartLinkMutateAsync).not.toHaveBeenCalled()
    expect(navigate).not.toHaveBeenCalled()
  })
})
