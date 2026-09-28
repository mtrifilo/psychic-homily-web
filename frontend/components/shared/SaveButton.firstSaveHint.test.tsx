import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient } from '@tanstack/react-query'
import { createWrapperWithClient } from '@/test/utils'
import { queryKeys } from '@/lib/queryClient'
import { SaveButton } from './SaveButton'

// The one-time first-save hint, end to end through SaveButton: the real hint
// module and a real query client holding the viewer's profile, with only the
// save itself and the network stubbed.

const saved = new Set<number>()
const mockToggle = vi.fn()

vi.mock('@/features/shows', () => ({
  useSaveShowToggle: (showId: number, isSaved: boolean) => ({
    isLoading: false,
    toggle: () => mockToggle(showId, isSaved),
    error: null,
  }),
  useShowSaveCount: (showId: number) => ({
    data: { show_id: showId, save_count: 0, is_saved: saved.has(showId) },
  }),
}))

vi.mock('next/navigation', () => ({
  useRouter: () => ({ push: vi.fn() }),
  usePathname: () => '/shows/1',
}))

let authStatus: 'authenticated' | 'anonymous' = 'authenticated'
vi.mock('@/lib/context/AuthContext', () => ({
  useAuthContext: () => ({
    authStatus,
    isAuthenticated: authStatus === 'authenticated',
    user: authStatus === 'authenticated' ? { id: '7', email: 'a@b.c' } : null,
    isLoading: false,
    logout: vi.fn(),
  }),
}))

const apiRequest = vi.fn()
vi.mock('@/lib/api', async importOriginal => ({
  ...(await importOriginal<object>()),
  apiRequest: (...args: unknown[]) => apiRequest(...args),
}))

/** Server state the stubbed endpoints answer from. */
let savedTotal = 0
let storedDismissal: string | null = null

function createClient(dismissedAt: string | null): QueryClient {
  const client = new QueryClient({
    defaultOptions: {
      queries: { retry: false },
      mutations: { retry: false },
    },
  })
  client.setQueryData(queryKeys.auth.profile, {
    success: true,
    user: {
      id: '7',
      preferences: { first_save_hint_dismissed_at: dismissedAt },
    },
  })
  return client
}

function readFlag(client: QueryClient): unknown {
  const cached = client.getQueryData(queryKeys.auth.profile) as {
    user: { preferences: { first_save_hint_dismissed_at: unknown } }
  }
  return cached.user.preferences.first_save_hint_dismissed_at
}

function putCount(): number {
  return apiRequest.mock.calls.filter(
    ([endpoint, options]) =>
      String(endpoint).endsWith('/auth/preferences/first-save-hint') &&
      (options as { method?: string } | undefined)?.method === 'PUT'
  ).length
}

function countReads(): number {
  return apiRequest.mock.calls.filter(([endpoint]) =>
    String(endpoint).includes('/saved-shows?')
  ).length
}

function renderSave(client: QueryClient, showId = 1, hintAlign?: 'start' | 'end') {
  return render(
    <SaveButton showId={showId} variant="bracket" hintAlign={hintAlign} />,
    { wrapper: createWrapperWithClient(client) }
  )
}

async function clickSave(user: ReturnType<typeof userEvent.setup>, showId = 1) {
  const buttons = screen.getAllByRole('button', { name: /^Save show/ })
  await user.click(buttons[0])
  expect(mockToggle).toHaveBeenLastCalledWith(showId, false)
}

beforeEach(() => {
  vi.clearAllMocks()
  saved.clear()
  savedTotal = 0
  storedDismissal = null
  authStatus = 'authenticated'
  mockToggle.mockImplementation(async (showId: number, isSaved: boolean) => {
    if (isSaved) {
      saved.delete(showId)
      savedTotal -= 1
    } else {
      saved.add(showId)
      savedTotal += 1
    }
  })
  apiRequest.mockImplementation(
    async (endpoint: string, options: { method: string }) => {
      if (String(endpoint).includes('/saved-shows?')) {
        return { shows: [], total: savedTotal, limit: 1, offset: 0 }
      }
      if (
        String(endpoint).endsWith('/auth/preferences/first-save-hint') &&
        options.method === 'PUT'
      ) {
        storedDismissal ??= '2026-09-01T12:00:00Z'
        return { success: true, first_save_hint_dismissed_at: storedDismissal }
      }
      throw new Error(`unexpected request ${options.method} ${endpoint}`)
    }
  )
})

describe('SaveButton first-save hint', () => {
  it('shows the hint once, on the save that makes the first saved show', async () => {
    const user = userEvent.setup()
    renderSave(createClient(null))

    await clickSave(user)

    const hint = await screen.findByRole('status')
    expect(hint).toHaveTextContent(
      'Saved. Find it on your home page and in Library.'
    )
    expect(within(hint).getByRole('link', { name: 'home page' })).toHaveAttribute(
      'href',
      '/'
    )
    expect(within(hint).getByRole('link', { name: 'Library' })).toHaveAttribute(
      'href',
      '/library'
    )
    expect(within(hint).getByRole('button', { name: 'Dismiss' })).toBeVisible()
  })

  it('dismissing stamps the account, and later saves never show it again', async () => {
    const user = userEvent.setup()
    const client = createClient(null)
    const first = renderSave(client, 1)

    await clickSave(user, 1)
    await user.click(await screen.findByRole('button', { name: 'Dismiss' }))

    expect(screen.queryByRole('status')).not.toBeInTheDocument()
    await waitFor(() => expect(putCount()).toBe(1))
    expect(readFlag(client)).toEqual(expect.any(String))
    expect(apiRequest).toHaveBeenCalledWith(
      expect.stringMatching(/\/auth\/preferences\/first-save-hint$/),
      { method: 'PUT' }
    )

    // Back to zero, then a new first save: the account flag holds, so no
    // hint and no count request.
    const readsBefore = countReads()
    first.unmount()
    saved.clear()
    savedTotal = 0
    renderSave(client, 2)
    await clickSave(user, 2)
    await waitFor(() => expect(saved.has(2)).toBe(true))
    expect(screen.queryByRole('status')).not.toBeInTheDocument()
    expect(countReads()).toBe(readsBefore)
  })

  it('does not show when the account has already dismissed it', async () => {
    const user = userEvent.setup()
    renderSave(createClient('2026-08-01T00:00:00Z'))

    await clickSave(user)

    await waitFor(() => expect(saved.has(1)).toBe(true))
    expect(screen.queryByRole('status')).not.toBeInTheDocument()
    expect(countReads()).toBe(0)
  })

  it('does not show on a second save', async () => {
    const user = userEvent.setup()
    savedTotal = 1
    renderSave(createClient(null))

    await clickSave(user)

    await waitFor(() => expect(countReads()).toBe(1))
    expect(screen.queryByRole('status')).not.toBeInTheDocument()
  })

  it('never shows for an anonymous viewer', async () => {
    const user = userEvent.setup()
    authStatus = 'anonymous'
    renderSave(createClient(null))

    await user.click(screen.getByRole('button', { name: 'Sign in to save' }))

    expect(mockToggle).not.toHaveBeenCalled()
    expect(countReads()).toBe(0)
    expect(screen.queryByRole('status')).not.toBeInTheDocument()
  })

  it.each([
    ['Escape', async (user: ReturnType<typeof userEvent.setup>) => {
      await user.keyboard('{Escape}')
    }],
    ['the home page link', async (user: ReturnType<typeof userEvent.setup>) => {
      await user.click(screen.getByRole('link', { name: 'home page' }))
    }],
    ['the Library link', async (user: ReturnType<typeof userEvent.setup>) => {
      await user.click(screen.getByRole('link', { name: 'Library' }))
    }],
  ])('closing with %s also stamps the account', async (_label, close) => {
    const user = userEvent.setup()
    const client = createClient(null)
    renderSave(client)

    await clickSave(user)
    await screen.findByRole('status')
    await close(user)

    expect(screen.queryByRole('status')).not.toBeInTheDocument()
    await waitFor(() => expect(putCount()).toBe(1))
    expect(readFlag(client)).toEqual(expect.any(String))
  })

  // An Escape a Radix layer above the hint already consumed closed that
  // layer; it is not the viewer dismissing the hint.
  it('ignores an Escape another layer already handled', async () => {
    const user = userEvent.setup()
    const client = createClient(null)
    renderSave(client)

    await clickSave(user)
    await screen.findByRole('status')
    const consumed = new KeyboardEvent('keydown', {
      key: 'Escape',
      bubbles: true,
      cancelable: true,
    })
    consumed.preventDefault()
    document.dispatchEvent(consumed)

    expect(screen.getByRole('status')).toBeInTheDocument()
    expect(readFlag(client)).toBeNull()
  })

  // Unsaving closes it without stamping: the hint's "Saved." is no longer
  // true, but the viewer never dismissed it.
  it('closes when the show is unsaved, without stamping the account', async () => {
    const user = userEvent.setup()
    const client = createClient(null)
    renderSave(client)

    await clickSave(user)
    await screen.findByRole('status')
    await user.click(screen.getByRole('button', { name: /Saved, remove|Remove from saved shows/ }))

    await waitFor(() =>
      expect(screen.queryByRole('status')).not.toBeInTheDocument()
    )
    expect(readFlag(client)).toBeNull()
  })

  // The hint is an extra: a failed count read shows nothing, and in
  // particular not the save-failure message.
  it('shows nothing when the count read fails', async () => {
    const user = userEvent.setup()
    apiRequest.mockRejectedValue(new Error('network'))
    renderSave(createClient(null))

    await clickSave(user)

    await waitFor(() => expect(countReads()).toBe(1))
    expect(screen.queryByRole('status')).not.toBeInTheDocument()
    expect(screen.queryByText(/Failed to/)).not.toBeInTheDocument()
  })

  // No layout shift: the hint overlays what follows rather than taking space,
  // anchored inside the control's own positioned wrapper.
  it.each([
    ['start', 'left-0'],
    ['end', 'right-0'],
  ] as const)('overlays from the control, aligned %s', async (align, edge) => {
    const user = userEvent.setup()
    renderSave(createClient(null), 1, align)

    await clickSave(user)

    const hint = await screen.findByRole('status')
    expect(hint).toHaveClass('absolute', 'top-full', edge)
    expect(hint.parentElement).toHaveClass('relative')
  })
})
