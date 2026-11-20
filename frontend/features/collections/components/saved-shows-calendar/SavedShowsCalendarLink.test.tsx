import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { RefObject } from 'react'
import { renderToString } from 'react-dom/server'
import { QueryClientProvider } from '@tanstack/react-query'
import userEvent from '@testing-library/user-event'
import {
  createTestQueryClient,
  renderWithProviders,
  screen,
} from '@/test/utils'
import type {
  CalendarFeedRead,
  CalendarFeedStatusResponse,
} from '@/features/shows/types'

const HOUR = 60 * 60 * 1000
const DAY = 24 * HOUR

let mockStatus: CalendarFeedStatusResponse | undefined
const statusFromMock = (enabled?: boolean) => ({
  data: enabled ? mockStatus : undefined,
})
const mockUseCalendarFeedStatus = vi.fn(statusFromMock)
vi.mock('@/features/auth/hooks/useCalendarFeed', async importOriginal => ({
  ...(await importOriginal<object>()),
  useCalendarFeedStatus: (enabled?: boolean) =>
    mockUseCalendarFeedStatus(enabled),
}))

let mockViewer: { id: string } | null = { id: '1' }
vi.mock('@/lib/context/AuthContext', () => ({
  useAuthContext: () => ({
    authStatus: mockViewer ? 'authenticated' : 'pending',
    user: mockViewer,
  }),
}))

// The panel's own suite covers what it shows; here it only has to report how
// it was opened and close itself the way Radix does.
const mockPanelProps = vi.fn<
  (props: {
    open: boolean
    align?: string
    anchorRef: RefObject<HTMLElement | null>
    triggerRef?: RefObject<HTMLElement | null>
    onLoadFailed?: () => void
  }) => void
>()
vi.mock('./LazySavedShowsCalendarPanel', () => ({
  preloadSavedShowsCalendarPanel: vi.fn(),
  LazySavedShowsCalendarPanel: (props: {
    open: boolean
    onOpenChange: (open: boolean) => void
    align?: string
    anchorRef: RefObject<HTMLElement | null>
    triggerRef?: RefObject<HTMLElement | null>
    onLoadFailed?: () => void
  }) => {
    mockPanelProps(props)
    return props.open ? (
      <div data-testid="calendar-panel">
        <button type="button" onClick={() => props.onOpenChange(false)}>
          Close panel
        </button>
      </div>
    ) : null
  },
}))

import {
  CALENDAR_LINK_LABELS,
  SavedShowsCalendarLink,
} from './SavedShowsCalendarLink'
import { reloadIntoCalendarPanel } from './calendarPanelArrival'

function read(
  client: CalendarFeedRead['client'],
  ageMs: number
): CalendarFeedRead {
  const last = new Date(Date.now() - ageMs).toISOString()
  return {
    client,
    first_read_at: new Date(Date.now() - ageMs - 30 * DAY).toISOString(),
    last_read_at: last,
  }
}

const STATUS = {
  none: { has_token: false, reads: [] },
  connected: { has_token: true, reads: [read('google', 2 * HOUR)] },
  stopped: { has_token: true, reads: [read('google', 12 * DAY)] },
} satisfies Record<string, CalendarFeedStatusResponse>

const ALL_LABELS = [
  'Add to calendar',
  'Calendar: connected',
  'Calendar: stopped',
]

function sizerLabels(container: HTMLElement): string[] {
  return Array.from(
    container.querySelectorAll('[data-calendar-link-sizer]'),
    sizer => sizer.textContent ?? ''
  )
}

beforeEach(() => {
  mockStatus = undefined
  mockViewer = { id: '1' }
  mockPanelProps.mockClear()
  mockUseCalendarFeedStatus.mockReset()
  mockUseCalendarFeedStatus.mockImplementation(statusFromMock)
})

afterEach(() => {
  window.history.replaceState(null, '', '/')
})

describe('SavedShowsCalendarLink', () => {
  it('carries the spec labels verbatim', () => {
    expect(Object.values(CALENDAR_LINK_LABELS)).toEqual(ALL_LABELS)
  })

  it.each([
    ['none', 'Add to calendar'],
    ['connected', 'Calendar: connected'],
    ['stopped', 'Calendar: stopped'],
  ] as const)('labels the %s state "[%s]"', (state, label) => {
    mockStatus = STATUS[state]
    renderWithProviders(<SavedShowsCalendarLink placement="section-header" />)

    const link = screen.getByRole('button', { name: label })
    expect(link).toHaveTextContent(`[${label}]`)
    expect(link).toHaveAttribute('aria-haspopup', 'dialog')
    expect(link).toHaveAttribute('aria-expanded', 'false')
  })

  it('reads stopped when a security action reset a link an app was checking', () => {
    mockStatus = {
      has_token: false,
      reads: [],
      link_reset_at: new Date().toISOString(),
      link_reset_reason: 'sign_out_everywhere',
    }
    renderWithProviders(<SavedShowsCalendarLink placement="module-footer" />)

    expect(
      screen.getByRole('button', { name: 'Calendar: stopped' })
    ).toBeInTheDocument()
  })

  it('reads the status only for a signed-in viewer', () => {
    mockViewer = null
    renderWithProviders(<SavedShowsCalendarLink placement="module-footer" />)

    expect(mockUseCalendarFeedStatus).toHaveBeenCalledWith(false)
    expect(
      screen.getByRole('button', { name: 'Add to calendar' })
    ).toBeInTheDocument()
  })

  it.each([
    ['section-header', 'none', 'accent'],
    ['section-header', 'connected', 'muted'],
    ['section-header', 'stopped', 'accent'],
    ['module-footer', 'none', 'muted'],
    ['module-footer', 'connected', 'muted'],
    ['module-footer', 'stopped', 'accent'],
  ] as const)('colours the %s link in the %s state %s, as drawn', (placement, state, tone) => {
    mockStatus = STATUS[state]
    renderWithProviders(<SavedShowsCalendarLink placement={placement} />)

    const link = screen.getByRole('button')
    if (tone === 'accent') {
      expect(link).toHaveClass('text-link')
      expect(link).not.toHaveClass('text-muted-foreground')
    } else {
      expect(link).toHaveClass('text-muted-foreground')
      expect(link).not.toHaveClass('text-link')
    }
  })

  // jsdom has no layout, so the reserved width is pinned by its mechanism:
  // every label sits invisibly in the slot's one grid cell in every state, so
  // the slot is always as wide as the longest and its neighbours never move.
  it.each(['section-header', 'module-footer'] as const)(
    'reserves the width of every label in the %s slot, whatever the state',
    placement => {
      for (const state of ['none', 'connected', 'stopped'] as const) {
        mockStatus = STATUS[state]
        const { container, unmount } = renderWithProviders(
          <SavedShowsCalendarLink placement={placement} />
        )
        const slot = screen.getByTestId('saved-shows-calendar-link-slot')
        expect(slot).toHaveClass('grid')
        expect(sizerLabels(container)).toEqual(ALL_LABELS.map(l => `[${l}]`))
        for (const sizer of container.querySelectorAll(
          '[data-calendar-link-sizer]'
        )) {
          expect(sizer).toHaveClass('invisible', 'col-start-1', 'row-start-1')
          expect(sizer).toHaveAttribute('aria-hidden', 'true')
        }
        expect(screen.getByRole('button')).toHaveClass(
          'col-start-1',
          'row-start-1'
        )
        unmount()
      }
    }
  )

  it('sits on its own line at the start below sm in a header, at the end from sm up', () => {
    renderWithProviders(<SavedShowsCalendarLink placement="section-header" />)

    expect(screen.getByTestId('saved-shows-calendar-link-slot')).toHaveClass(
      'w-full',
      'sm:w-auto'
    )
    expect(screen.getByRole('button')).toHaveClass(
      'justify-self-start',
      'sm:justify-self-end'
    )
  })

  it('sits at the end of a footer row', () => {
    renderWithProviders(<SavedShowsCalendarLink placement="module-footer" />)

    expect(screen.getByRole('button')).toHaveClass('justify-self-end')
  })

  // The server has no status, so the link must be in its HTML with the
  // no-calendar label and the full reserved slot: nothing appears or changes
  // width at hydration. The real status hook runs here, against an empty cache.
  it('is in server HTML with the neutral label and every sizer', async () => {
    const actual = await vi.importActual<
      typeof import('@/features/auth/hooks/useCalendarFeed')
    >('@/features/auth/hooks/useCalendarFeed')
    mockUseCalendarFeedStatus.mockImplementation(
      enabled => actual.useCalendarFeedStatus(enabled) as never
    )

    for (const placement of ['section-header', 'module-footer'] as const) {
      const html = renderToString(
        <QueryClientProvider client={createTestQueryClient()}>
          <SavedShowsCalendarLink placement={placement} />
        </QueryClientProvider>
      )
      const host = document.createElement('div')
      host.innerHTML = html
      const button = host.querySelector('button')
      expect(button?.getAttribute('aria-label')).toBe('Add to calendar')
      expect(button?.textContent).toBe('[Add to calendar]')
      expect(button?.getAttribute('aria-expanded')).toBe('false')
      expect(sizerLabels(host)).toEqual(ALL_LABELS.map(l => `[${l}]`))
      // No panel in server HTML: it mounts on the first open.
      expect(html).not.toContain('calendar-panel')
    }
    expect(mockPanelProps).not.toHaveBeenCalled()
  })

  it('opens the panel in place, end-aligned, and closes it again', async () => {
    const user = userEvent.setup()
    renderWithProviders(<SavedShowsCalendarLink placement="module-footer" />)
    const link = screen.getByRole('button', { name: 'Add to calendar' })
    expect(mockPanelProps).not.toHaveBeenCalled()

    await user.click(link)

    expect(screen.getByTestId('calendar-panel')).toBeInTheDocument()
    expect(link).toHaveAttribute('aria-expanded', 'true')
    expect(mockPanelProps).toHaveBeenLastCalledWith(
      expect.objectContaining({ open: true, align: 'end' })
    )
    expect(window.location.pathname).toBe('/')

    await user.click(screen.getByRole('button', { name: 'Close panel' }))
    expect(screen.queryByTestId('calendar-panel')).toBeNull()
    expect(link).toHaveAttribute('aria-expanded', 'false')
    // Stays mounted, closed, so it can animate out and return focus.
    expect(mockPanelProps).toHaveBeenLastCalledWith(
      expect.objectContaining({ open: false })
    )
  })

  it('toggles from the keyboard', async () => {
    const user = userEvent.setup()
    renderWithProviders(<SavedShowsCalendarLink placement="section-header" />)
    screen.getByRole('button', { name: 'Add to calendar' }).focus()

    await user.keyboard('{Enter}')
    expect(screen.getByTestId('calendar-panel')).toBeInTheDocument()

    screen.getByRole('button', { name: 'Add to calendar' }).focus()
    await user.keyboard('{Enter}')
    expect(screen.queryByTestId('calendar-panel')).toBeNull()
  })

  // The panel's own suite proves a press inside the trigger does not dismiss
  // the popover; here the link has to name its slot as that trigger, so its
  // own click is what closes it.
  it('anchors the panel to its slot and names the link as its trigger, so a press on it toggles', async () => {
    const user = userEvent.setup()
    renderWithProviders(<SavedShowsCalendarLink placement="section-header" />)
    const link = screen.getByRole('button', { name: 'Add to calendar' })

    await user.click(link)
    const { triggerRef, anchorRef } = mockPanelProps.mock.lastCall![0]
    expect(anchorRef.current).toBe(
      screen.getByTestId('saved-shows-calendar-link-slot')
    )
    // The link alone: a press on the slot's empty reserved width is outside.
    expect(triggerRef?.current).toBe(link)

    await user.click(link)
    expect(screen.queryByTestId('calendar-panel')).toBeNull()
  })

  // The copies that reserve the width are the link's own label markup and
  // type, so a change to how a bracket link lays out its label moves both.
  it.each(['none', 'connected', 'stopped'] as const)(
    'measures the %s label with the markup the link renders',
    state => {
      mockStatus = STATUS[state]
      const { container } = renderWithProviders(
        <SavedShowsCalendarLink placement="module-footer" />
      )
      const link = screen.getByRole('button')
      const sizer = Array.from(
        container.querySelectorAll<HTMLElement>('[data-calendar-link-sizer]')
      ).find(candidate => candidate.textContent === link.textContent)

      expect(sizer?.innerHTML).toBe(link.innerHTML)
      for (const shared of [
        'inline-flex',
        'items-baseline',
        'whitespace-nowrap',
        'font-data',
        'text-xs',
      ]) {
        expect(sizer).toHaveClass(shared)
        expect(link).toHaveClass(shared)
      }
    }
  )

  it('closes the panel when the viewer changes', async () => {
    const user = userEvent.setup()
    const { rerender } = renderWithProviders(
      <SavedShowsCalendarLink placement="module-footer" />
    )
    await user.click(screen.getByRole('button', { name: 'Add to calendar' }))
    expect(screen.getByTestId('calendar-panel')).toBeInTheDocument()

    mockViewer = { id: '2' }
    rerender(<SavedShowsCalendarLink placement="module-footer" />)

    expect(screen.queryByTestId('calendar-panel')).toBeNull()
  })

  describe('the old #calendar-feed fragment', () => {
    it('opens the panel on arrival, and closing drops the fragment so it opens once', async () => {
      const user = userEvent.setup()
      window.history.replaceState(null, '', '/library?x=1#calendar-feed')

      const { unmount } = renderWithProviders(
        <SavedShowsCalendarLink placement="section-header" opensFromHash />
      )

      expect(screen.getByTestId('calendar-panel')).toBeInTheDocument()
      expect(
        screen.getByRole('button', { name: 'Add to calendar' })
      ).toHaveAttribute('aria-expanded', 'true')

      await user.click(screen.getByRole('button', { name: 'Close panel' }))
      expect(screen.queryByTestId('calendar-panel')).toBeNull()
      expect(window.location.hash).toBe('')
      expect(window.location.pathname + window.location.search).toBe(
        '/library?x=1'
      )

      // A later mount (the header re-rendering, a tab switch back) does not
      // open it again.
      unmount()
      renderWithProviders(
        <SavedShowsCalendarLink placement="section-header" opensFromHash />
      )
      expect(screen.queryByTestId('calendar-panel')).toBeNull()
    })

    // A failed load of the panel's code falls back to a fresh Library load at
    // the fragment, except for an open that came from the fragment, which may
    // itself be that fresh load.
    it('falls back to a fresh load only for an open the person clicked', async () => {
      const user = userEvent.setup()
      window.history.replaceState(null, '', '/library#calendar-feed')
      renderWithProviders(
        <SavedShowsCalendarLink placement="section-header" opensFromHash />
      )
      expect(mockPanelProps.mock.lastCall![0].onLoadFailed).toBeUndefined()

      await user.click(screen.getByRole('button', { name: 'Close panel' }))
      await user.click(screen.getByRole('button', { name: 'Add to calendar' }))

      expect(mockPanelProps.mock.lastCall![0]).toMatchObject({
        open: true,
        onLoadFailed: reloadIntoCalendarPanel,
      })
    })

    it('closes a panel it opened when the viewer changes', () => {
      window.history.replaceState(null, '', '/library#calendar-feed')
      const { rerender } = renderWithProviders(
        <SavedShowsCalendarLink placement="section-header" opensFromHash />
      )
      expect(screen.getByTestId('calendar-panel')).toBeInTheDocument()

      mockViewer = { id: '2' }
      rerender(
        <SavedShowsCalendarLink placement="section-header" opensFromHash />
      )

      expect(screen.queryByTestId('calendar-panel')).toBeNull()
    })

    it('is ignored by a link that does not open from it', () => {
      window.history.replaceState(null, '', '/#calendar-feed')
      renderWithProviders(<SavedShowsCalendarLink placement="module-footer" />)

      expect(screen.queryByTestId('calendar-panel')).toBeNull()
    })

    it('leaves any other fragment alone', () => {
      window.history.replaceState(null, '', '/library#elsewhere')
      renderWithProviders(
        <SavedShowsCalendarLink placement="section-header" opensFromHash />
      )

      expect(screen.queryByTestId('calendar-panel')).toBeNull()
      expect(window.location.hash).toBe('#elsewhere')
    })
  })
})
