import { describe, it, expect, vi, beforeEach } from 'vitest'
import { act, render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient } from '@tanstack/react-query'
import { createWrapperWithClient } from '@/test/utils'
import { queryKeys } from '@/lib/queryClient'
import { Popover, PopoverAnchor, PopoverContent } from '@/components/ui/popover'
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

// The hint element whether or not it is visible yet: it stays hidden until
// floating-ui places it, and a role query skips hidden elements, so an
// absence check by role would pass while an unplaced hint is mounted.
function hintElement(): HTMLElement | null {
  return screen.queryByTestId('first-save-hint')
}

function countReads(): number {
  return apiRequest.mock.calls.filter(([endpoint]) =>
    String(endpoint).includes('/saved-shows?')
  ).length
}

function renderSave(
  client: QueryClient,
  showId = 1,
  hintAlign?: 'start' | 'end',
  variant: 'bracket' | 'ghost' = 'bracket'
) {
  return render(
    <SaveButton showId={showId} variant={variant} hintAlign={hintAlign} />,
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

    expect(hintElement()).not.toBeInTheDocument()
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
    expect(hintElement()).not.toBeInTheDocument()
    expect(countReads()).toBe(readsBefore)
  })

  it('does not show when the account has already dismissed it', async () => {
    const user = userEvent.setup()
    renderSave(createClient('2026-08-01T00:00:00Z'))

    await clickSave(user)

    await waitFor(() => expect(saved.has(1)).toBe(true))
    expect(hintElement()).not.toBeInTheDocument()
    expect(countReads()).toBe(0)
  })

  it('does not show on a second save', async () => {
    const user = userEvent.setup()
    savedTotal = 1
    renderSave(createClient(null))

    await clickSave(user)

    await waitFor(() => expect(countReads()).toBe(1))
    expect(hintElement()).not.toBeInTheDocument()
  })

  it('never shows for an anonymous viewer', async () => {
    const user = userEvent.setup()
    authStatus = 'anonymous'
    renderSave(createClient(null))

    await user.click(screen.getByRole('button', { name: 'Sign in to save' }))

    expect(mockToggle).not.toHaveBeenCalled()
    expect(countReads()).toBe(0)
    expect(hintElement()).not.toBeInTheDocument()
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

    expect(hintElement()).not.toBeInTheDocument()
    await waitFor(() => expect(putCount()).toBe(1))
    expect(readFlag(client)).toEqual(expect.any(String))
  })

  // A real Radix popover open beside the hint (the show page's Add to
  // collection) owns its Escape: it closes, and the hint neither dismisses
  // nor stamps the account. Both orders of registering the two document
  // keydown listeners are covered: the popover's first (it opens while the
  // count read is in flight) fails if the hint stops ignoring
  // defaultPrevented events; the hint's first (the popover opens over it)
  // fails if the hint's listener moves to the capture phase.
  it.each(['while the read is in flight', 'over the open hint'] as const)(
    'leaves Escape to a popover opened %s',
    async when => {
      const user = userEvent.setup()
      const client = createClient(null)
      let resolveRead!: () => void
      if (when === 'while the read is in flight') {
        apiRequest.mockImplementationOnce(
          () =>
            new Promise(resolve => {
              resolveRead = () =>
                resolve({ shows: [], total: savedTotal, limit: 1, offset: 0 })
            })
        )
      }
      const ui = (withPopover: boolean) => (
        <>
          <SaveButton showId={1} variant="bracket" />
          {withPopover ? (
            <Popover defaultOpen>
              <PopoverAnchor>
                <span>anchor</span>
              </PopoverAnchor>
              <PopoverContent>collection picker</PopoverContent>
            </Popover>
          ) : null}
        </>
      )
      const { rerender } = render(ui(false), {
        wrapper: createWrapperWithClient(client),
      })

      await clickSave(user)
      if (when === 'while the read is in flight') {
        await waitFor(() => expect(countReads()).toBe(1))
        rerender(ui(true))
        await screen.findByText('collection picker')
        resolveRead()
        await screen.findByRole('status')
      } else {
        await screen.findByRole('status')
        rerender(ui(true))
        await screen.findByText('collection picker')
      }

      await user.keyboard('{Escape}')

      await waitFor(() =>
        expect(screen.queryByText('collection picker')).not.toBeInTheDocument()
      )
      expect(screen.getByRole('status')).toBeInTheDocument()
      expect(readFlag(client)).toBeNull()
    }
  )

  // Keyboard users reach the hint as the next tab stop after Save, before any
  // control that follows Save on the page, and closing it from inside puts
  // focus back on Save instead of <body>.
  it.each(['bracket', 'ghost'] as const)(
    'is the next tab stop after the %s control, and returns focus to Save',
    async variant => {
      const user = userEvent.setup()
      render(
        <>
          <SaveButton showId={1} variant={variant} />
          <button type="button">next control</button>
        </>,
        { wrapper: createWrapperWithClient(createClient(null)) }
      )

      await clickSave(user)
      await screen.findByRole('status')
      const save = screen.getByRole('button', {
        name: /Remove from saved shows/,
      })
      save.focus()

      await user.tab()
      expect(screen.getByRole('link', { name: 'home page' })).toHaveFocus()
      await user.tab()
      await user.tab()
      expect(screen.getByRole('button', { name: 'Dismiss' })).toHaveFocus()

      await user.keyboard('{Enter}')
      expect(hintElement()).not.toBeInTheDocument()
      expect(save).toHaveFocus()
    }
  )

  // Focus returns only when it was inside the hint: an Escape pressed while
  // the viewer is typing elsewhere closes the hint and leaves them there.
  it('leaves focus where it was when closed from outside the hint', async () => {
    const user = userEvent.setup()
    render(
      <>
        <SaveButton showId={1} variant="bracket" />
        <input aria-label="search" />
      </>,
      { wrapper: createWrapperWithClient(createClient(null)) }
    )

    await clickSave(user)
    await screen.findByRole('status')
    const input = screen.getByRole('textbox', { name: 'search' })
    input.focus()
    await user.keyboard('{Escape}')

    expect(hintElement()).not.toBeInTheDocument()
    expect(input).toHaveFocus()
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
      expect(hintElement()).not.toBeInTheDocument()
    )
    expect(readFlag(client)).toBeNull()
  })

  // Unsave, then re-save the same show after a save elsewhere made it a
  // second save: the hint must not come back without being re-checked.
  it('does not reopen when the same show is unsaved and saved again', async () => {
    const user = userEvent.setup()
    renderSave(createClient(null))

    await clickSave(user)
    await screen.findByRole('status')
    savedTotal += 1
    await user.click(
      screen.getByRole('button', { name: /Remove from saved shows/ })
    )
    await waitFor(() =>
      expect(hintElement()).not.toBeInTheDocument()
    )
    await clickSave(user)

    await waitFor(() => expect(saved.has(1)).toBe(true))
    expect(hintElement()).not.toBeInTheDocument()
  })

  // Home can render two Save controls for one show. An unsave through the
  // other one closes this control's hint too, so a later re-save here (now a
  // second save) does not bring it back.
  it('closes when another control for the same show unsaves it', async () => {
    const user = userEvent.setup()
    const client = createClient(null)
    const ui = () => (
      <>
        <SaveButton showId={1} variant="bracket" />
        <SaveButton showId={1} variant="ghost" />
      </>
    )
    const { rerender } = render(ui(), {
      wrapper: createWrapperWithClient(client),
    })

    await user.click(screen.getAllByRole('button', { name: /^Save show/ })[0])
    await screen.findByRole('status')
    savedTotal += 1
    rerender(ui())
    const removes = screen.getAllByRole('button', {
      name: /Remove from saved shows/,
    })
    await user.click(removes[1])
    await waitFor(() => expect(saved.has(1)).toBe(false))
    rerender(ui())
    expect(hintElement()).not.toBeInTheDocument()

    await user.click(screen.getAllByRole('button', { name: /^Save show/ })[0])
    await waitFor(() => expect(saved.has(1)).toBe(true))
    rerender(ui())
    expect(hintElement()).not.toBeInTheDocument()
  })

  // A count read that resolves after the viewer already unsaved answered a
  // question that no longer stands: it neither opens the hint nor settles the
  // question, so the re-save (a genuine zero-to-one save) asks for itself.
  it('ignores a count read that resolves after a later click', async () => {
    const user = userEvent.setup()
    let resolveRead!: () => void
    apiRequest.mockImplementationOnce(
      () =>
        new Promise(resolve => {
          resolveRead = () =>
            resolve({ shows: [], total: 1, limit: 1, offset: 0 })
        })
    )
    const client = createClient(null)
    const ui = () => <SaveButton showId={1} variant="bracket" />
    const { rerender } = render(ui(), {
      wrapper: createWrapperWithClient(client),
    })

    await clickSave(user)
    await waitFor(() => expect(countReads()).toBe(1))
    // The stubbed save does not re-render; this stands in for the cache
    // update that does in the app.
    rerender(ui())
    await user.click(
      screen.getByRole('button', { name: /Remove from saved shows/ })
    )
    await waitFor(() => expect(saved.has(1)).toBe(false))
    await act(async () => {
      resolveRead()
    })
    rerender(ui())
    expect(hintElement()).not.toBeInTheDocument()

    await clickSave(user)
    await waitFor(() => expect(countReads()).toBe(2))
    await screen.findByRole('status')
  })

  // Once the question is answered in a session, later saves do not re-ask:
  // the count read happens at most once per viewer per session.
  it('reads the count at most once per session', async () => {
    const user = userEvent.setup()
    const client = createClient(null)
    savedTotal = 3
    const first = renderSave(client, 1)
    await clickSave(user, 1)
    await waitFor(() => expect(countReads()).toBe(1))
    first.unmount()

    renderSave(client, 2)
    await clickSave(user, 2)
    await waitFor(() => expect(saved.has(2)).toBe(true))
    expect(countReads()).toBe(1)
  })

  // A save that fails is not a first save: the count is never read and the
  // hint never opens.
  it('does not ask for the count when the save fails', async () => {
    const user = userEvent.setup()
    mockToggle.mockRejectedValueOnce(new Error('500'))
    renderSave(createClient(null))

    await clickSave(user)

    await waitFor(() => expect(mockToggle).toHaveBeenCalledTimes(1))
    expect(countReads()).toBe(0)
    expect(hintElement()).not.toBeInTheDocument()
  })

  // The hint is an extra: a failed count read shows nothing, and in
  // particular not the save-failure message.
  it('shows nothing when the count read fails', async () => {
    const user = userEvent.setup()
    apiRequest.mockRejectedValue(new Error('network'))
    renderSave(createClient(null))

    await clickSave(user)

    await waitFor(() => expect(countReads()).toBe(1))
    expect(hintElement()).not.toBeInTheDocument()
    expect(screen.queryByText(/Failed to/)).not.toBeInTheDocument()
  })

  // No layout shift and no clipping: the hint sits right after the control
  // in the DOM (reading and tab order) but is fixed-positioned, so it takes
  // no space in the row and an overflow-hidden or scrolling ancestor cannot
  // cut it off; it lines up with the requested edge of the control.
  it.each([
    ['bracket', 'start'],
    ['bracket', 'end'],
    ['ghost', 'end'],
  ] as const)(
    'overlays in place after the %s control, aligned %s',
    async (variant, align) => {
      const user = userEvent.setup()
      renderSave(createClient(null), 1, align, variant)

      await clickSave(user)

      const hint = await screen.findByRole('status')
      const save = screen.getByRole('button', {
        name: /Remove from saved shows/,
      })
      // Renders inside the control's wrapper, after the control.
      expect(save.parentElement).toContainElement(hint)
      expect(save.compareDocumentPosition(hint)).toBe(
        Node.DOCUMENT_POSITION_FOLLOWING
      )
      expect(hint.style.position).toBe('fixed')
      // The placement floating-ui actually used keeps the requested edge.
      expect(hint.getAttribute('data-placement')).toMatch(
        new RegExp(`-${align}$`)
      )
    }
  )
})
