import { describe, it, expect, vi } from 'vitest'
import { render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import {
  DayGroupedShowListHeader,
  DayGroupedShowRow,
} from './DayGroupedShowRow'
import { SHOW_LIST_FEATURE_POLICY } from './showListFeaturePolicy'
import type { ShowResponse } from '../types'

vi.mock('@/lib/context/AuthContext', () => ({
  useAuthContext: () => ({
    user: null,
    isAuthenticated: false,
    isLoading: false,
    logout: vi.fn(),
  }),
}))

// The edit form and the delete dialog are heavy trees that this file's subject
// does not include: what it pins is which CONTROLS the row offers, and the
// controls' own behaviour is tested where those components live.
vi.mock('./ShowForm', () => ({ ShowForm: () => <div data-testid="show-form" /> }))
// Rendered whenever it is MOUNTED, so a test can tell "mounted and closed"
// from "not mounted at all".
vi.mock('./DeleteShowDialog', () => ({
  DeleteShowDialog: ({ open }: { open: boolean }) => (
    <div data-testid={open ? 'delete-dialog' : 'delete-dialog-closed'} />
  ),
}))
// The PANEL is stubbed and the predicates are kept real: what decides whether
// the expand control appears is `showHasArtistMusic`, and stubbing that would
// test the stub. The panel itself mounts `MusicEmbed`, which needs a query
// client this file has no reason to provide.
vi.mock('./ShowArtistMusic', async importOriginal => {
  const actual = await importOriginal<typeof import('./ShowArtistMusic')>()
  return {
    ...actual,
    ShowArtistMusicPanel: () => <div data-testid="artist-music-panel" />,
  }
})

vi.mock('@/components/shared/SaveButton', () => ({
  SaveButton: () => (
    <button type="button" aria-label="Save show">
      save
    </button>
  ),
}))

function makeShow(overrides: Partial<ShowResponse> = {}): ShowResponse {
  return {
    id: 1,
    slug: 'desert-doom',
    title: 'Desert Doom at Valley Bar',
    event_date: '2026-09-12T02:00:00Z',
    status: 'approved',
    city: 'Phoenix',
    state: 'AZ',
    price: 15,
    age_requirement: '21+',
    venues: [
      { id: 7, name: 'Valley Bar', slug: 'valley-bar', timezone: 'America/Phoenix' },
    ] as never,
    artists: [
      {
        id: 1,
        name: 'Sunn Amps',
        slug: 'sunn-amps',
        is_headliner: true,
        socials: { bandcamp: 'https://sunnamps.bandcamp.com' },
      },
      { id: 2, name: 'Low Ceiling', slug: 'low-ceiling', is_headliner: false },
    ] as never,
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z',
    is_sold_out: false,
    is_cancelled: false,
    ...overrides,
  } as ShowResponse
}

function renderRow(overrides: Partial<ShowResponse> = {}, props = {}) {
  return render(
    <DayGroupedShowRow
      show={makeShow(overrides)}
      density="comfortable"
      isAdmin={false}
      index={0}
      showCity={false}
      actionsFootprint="viewer"
      {...props}
    />
  )
}

describe('DayGroupedShowRow', () => {
  // The E2E save and list-action specs address a specific seeded show with
  // `getByRole('article', { name })`, so the accessible name is a contract and
  // not decoration.
  it('exposes the show title as the row s accessible name', () => {
    renderRow()

    expect(
      screen.getByRole('article', { name: 'Desert Doom at Valley Bar' })
    ).toBeInTheDocument()
  })

  it('leads the bill with the headliner, linked to the show', () => {
    renderRow()

    const row = screen.getByRole('article')
    expect(
      within(row).getByRole('link', { name: 'Sunn Amps' })
    ).toHaveAttribute('href', '/shows/desert-doom')
  })

  it('carries the support billing beside the headliner', () => {
    renderRow()

    expect(screen.getByTestId('row-support')).toHaveTextContent('w/ Low Ceiling')
  })

  it('links the venue', () => {
    renderRow()

    expect(
      screen.getByRole('link', { name: 'Valley Bar' })
    ).toHaveAttribute('href', '/venues/valley-bar')
  })

  // A single-metro list would repeat one city on every row: the column's whole
  // width spent on no information. Asserted on the CITY TEXT rather than on the
  // separator, so changing the separator cannot turn this green while the city
  // renders on every row.
  it('omits the city until the list spans more than one', () => {
    const { unmount } = renderRow()
    expect(screen.queryByText(/Phoenix/)).toBeNull()
    unmount()

    renderRow({}, { showCity: true })
    expect(screen.getByText(/Phoenix, AZ/)).toBeInTheDocument()
  })

  // Two metros can share a name across state lines, so the city alone names
  // nothing on a list that spans them.
  it('names the state alongside the city', () => {
    renderRow({ city: 'Kansas City', state: 'KS' }, { showCity: true })

    expect(screen.getByText(/Kansas City, KS/)).toBeInTheDocument()
  })

  // The venue column truncates, which hides the tail with no other way to read
  // it.
  it('titles the venue cell with its full text', () => {
    renderRow({}, { showCity: true })

    expect(
      screen.getByTitle('Valley Bar, Phoenix, AZ')
    ).toBeInTheDocument()
  })

  // The site's whole shape is Shows to Artists to Releases. Every billed act
  // that has a page is reachable from the row.
  it('links every billed act to its artist page', () => {
    renderRow()

    expect(screen.getByRole('link', { name: 'Low Ceiling' })).toHaveAttribute(
      'href',
      '/artists/low-ceiling'
    )
  })

  it('renders an act with no page as plain text', () => {
    renderRow({
      artists: [
        { id: 1, name: 'Sunn Amps', slug: 'sunn-amps', is_headliner: true },
        { id: 2, name: 'No Page', is_headliner: false },
      ] as never,
    })

    expect(screen.queryByRole('link', { name: 'No Page' })).toBeNull()
    expect(screen.getByTestId('row-support')).toHaveTextContent('No Page')
  })

  it('renders the price and the age requirement', () => {
    renderRow()

    expect(screen.getByText('$15')).toBeInTheDocument()
    expect(screen.getByText('21+')).toBeInTheDocument()
  })

  // A blank cell reads as "nobody filled this in"; the dash reads as "there is
  // no value", which is what the row knows.
  // The dash is `aria-hidden`: a column that looks continuous to a sighted
  // reader and is simply absent to a screen reader, rather than an unlabelled
  // "en dash" announced between the bill and the price.
  it('holds the price column open with a dash hidden from assistive tech', () => {
    renderRow({ price: null, door_price: null } as never)

    const dash = screen.getByText('–')
    expect(dash).toHaveAttribute('aria-hidden', 'true')
  })

  it('carries the sold-out badge', () => {
    renderRow({ is_sold_out: true })

    expect(screen.getByText('SOLD OUT')).toBeInTheDocument()
  })

  // PRIMARY on this surface, not the card's destructive red: among muted mono
  // columns a red block reads as an error state rather than as a status.
  it('paints the cancelled badge in primary', () => {
    renderRow({ is_cancelled: true })

    const badge = screen.getByText('CANCELLED')
    expect(badge.className).toContain('bg-primary')
    expect(badge.className).not.toContain('bg-destructive')
  })

  it('carries the save and outbound controls', () => {
    renderRow()

    const row = screen.getByRole('article')
    expect(
      within(row).getByRole('button', { name: 'Save show' })
    ).toBeInTheDocument()
    expect(
      within(row).getByRole('link', { name: 'View show details' })
    ).toHaveAttribute('href', '/shows/desert-doom')
  })

  // The frame drew the ACTIONS column simplified; it was never a decision to
  // remove capability. This row offers what `ShowCard` offers the same viewer.
  describe('the action set', () => {
    it('offers the expand-music control when the bill has music', () => {
      renderRow()

      expect(
        screen.getByRole('button', { name: 'Discover artist music' })
      ).toBeInTheDocument()
    })

    it('offers no expand control when no act has music', () => {
      renderRow({
        artists: [
          { id: 1, name: 'Sunn Amps', slug: 'sunn-amps', is_headliner: true },
        ] as never,
      })

      expect(
        screen.queryByRole('button', { name: 'Discover artist music' })
      ).toBeNull()
    })

    // Players open on one click, never behind a facade (locked decision).
    it('opens the players in place', async () => {
      const user = userEvent.setup()
      renderRow()

      await user.click(
        screen.getByRole('button', { name: 'Discover artist music' })
      )

      expect(
        screen.getByRole('button', { name: 'Hide artist music' })
      ).toBeInTheDocument()
      expect(screen.getByTestId('artist-music-panel')).toBeInTheDocument()
    })

    // Export is deliberately NOT asserted here: `ExportShowButton` renders
    // nothing outside development and carries no aria-label, so any assertion
    // on it would pass against a stub and prove nothing.
    it('hides the admin controls from an ordinary viewer', () => {
      renderRow()

      expect(screen.queryByRole('button', { name: 'Edit show' })).toBeNull()
      expect(screen.queryByRole('button', { name: 'Delete show' })).toBeNull()
    })

    it('offers the admin controls to an admin', () => {
      renderRow({}, { isAdmin: true })

      expect(
        screen.getByRole('button', { name: 'Edit show' })
      ).toBeInTheDocument()
      expect(
        screen.getByRole('button', { name: 'Delete show' })
      ).toBeInTheDocument()
    })

    it('opens the edit form in place for an admin', async () => {
      const user = userEvent.setup()
      renderRow({}, { isAdmin: true })

      await user.click(screen.getByRole('button', { name: 'Edit show' }))

      expect(screen.getByTestId('show-form')).toBeInTheDocument()
    })

    // The owner of a submission can delete it without being an admin.
    it('offers delete to the show s submitter', () => {
      renderRow({ submitted_by: 42 } as never, { userId: '42' })

      expect(
        screen.getByRole('button', { name: 'Delete show' })
      ).toBeInTheDocument()
      expect(screen.queryByRole('button', { name: 'Edit show' })).toBeNull()
    })

    it('offers delete to nobody else', () => {
      renderRow({ submitted_by: 42 } as never, { userId: '7' })

      expect(screen.queryByRole('button', { name: 'Delete show' })).toBeNull()
    })

    // `AuthContext` records that the declared `id: string` is narrower than
    // what arrives at runtime. Comparing a number against a string with `===`
    // would take the submitter's own delete control away silently.
    it('recognises the submitter when the viewer id is a number', () => {
      renderRow({ submitted_by: 42 } as never, { userId: 42 as never })

      expect(
        screen.getByRole('button', { name: 'Delete show' })
      ).toBeInTheDocument()
    })

    it('opens the delete dialog from the row', async () => {
      const user = userEvent.setup()
      renderRow({}, { isAdmin: true })

      expect(screen.queryByTestId('delete-dialog')).toBeNull()
      await user.click(screen.getByRole('button', { name: 'Delete show' }))

      expect(screen.getByTestId('delete-dialog')).toBeInTheDocument()
    })

    // `DeleteShowDialog` calls `useShowDelete` at mount, so mounting it for a
    // reader who can never open it is 50 mutations a page for nothing.
    it('mounts no delete dialog at all for a viewer who cannot delete', () => {
      renderRow()

      expect(screen.queryByTestId('delete-dialog')).toBeNull()
    })

    // The expanded density auto-opens the music, and the toggle outranks it.
    it('opens the players already at the expanded density', () => {
      renderRow({}, { density: 'expanded' })

      expect(screen.getByTestId('artist-music-panel')).toBeInTheDocument()
      expect(
        screen.getByRole('button', { name: 'Hide artist music' })
      ).toBeInTheDocument()
    })

    // The control is in server HTML before hydration wires it, so the replay
    // root is what stops that first click being swallowed.
    it('marks the expand control for click replay', () => {
      renderRow()

      expect(
        screen.getByRole('button', { name: 'Discover artist music' })
      ).toHaveAttribute('data-replay-on-hydrate')
    })
  })

  // The row carries no date of its own: the day heading above it states the
  // day, and a date column would print the same value on every row of a group.
  it('prints no date', () => {
    renderRow()

    expect(screen.queryByText(/SEP 11|SEP 12/)).toBeNull()
  })

  describe('the alternating fill', () => {
    it('fills even rows and leaves odd ones bare', () => {
      const { container, unmount } = renderRow({}, { index: 0 })
      expect(container.querySelector('.bg-muted\\/20')).not.toBeNull()
      unmount()

      const second = renderRow({}, { index: 1 })
      expect(second.container.querySelector('.bg-muted\\/20')).toBeNull()
    })
  })

  describe('density', () => {
    // The frame collapses support and age first.
    it('drops support and age in compact', () => {
      renderRow({}, { density: 'compact' })

      expect(screen.queryByTestId('row-support')).toBeNull()
      expect(screen.queryByText('21+')).toBeNull()
    })

    it('keeps support and age in comfortable', () => {
      renderRow({}, { density: 'comfortable' })

      expect(screen.getByTestId('row-support')).toHaveTextContent(
        'w/ Low Ceiling'
      )
      expect(screen.getByText('21+')).toBeInTheDocument()
    })

    // Expanded gives support its own line under the headliner.
    it('moves support onto its own line in expanded', () => {
      renderRow({}, { density: 'expanded' })

      const bill = screen.getByTestId('row-support')
      const headliner = screen.getByRole('link', { name: 'Sunn Amps' })
      expect(bill.parentElement).not.toBe(headliner.parentElement)
    })

    it('keeps every column in all three densities', () => {
      for (const density of ['compact', 'comfortable', 'expanded'] as const) {
        const { unmount } = renderRow({}, { density })
        const row = screen.getByRole('article')
        expect(
          within(row).getByRole('link', { name: 'Sunn Amps' })
        ).toBeInTheDocument()
        expect(
          within(row).getByRole('link', { name: 'Valley Bar' })
        ).toBeInTheDocument()
        expect(within(row).getByText('$15')).toBeInTheDocument()
        unmount()
      }
    })
  })
})

// jsdom has no layout, so what this pins is the CLASS that fixes the width:
// one `lg:w-[...]` utility per footprint, the same on the header and on every
// row whatever controls the row itself carries. A width derived from a row's
// own controls fails the matrix below. Whether the controls FIT the width is
// a layout fact no test here can see.
describe('the actions column width', () => {
  function widthClasses(element: HTMLElement): string[] {
    return [...element.classList].filter(c => /(^|:)(min-|max-)?w-/.test(c))
  }

  const noMusic = {
    artists: [
      { id: 3, name: 'Quiet Act', slug: 'quiet-act', is_headliner: true },
    ] as never,
  }

  // Each viewer with the rows a list can hand them. Every case pairs a row
  // with the expand control and one without it; the owner case also pairs an
  // owned row (delete control) with a row someone else submitted.
  const viewers = [
    {
      viewer: 'an anonymous reader',
      footprint: 'viewer',
      rowProps: {},
      rows: [{}, noMusic],
    },
    {
      viewer: 'a signed-in reader who submitted no row',
      footprint: 'viewer',
      rowProps: { userId: '42' },
      rows: [{ submitted_by: 7 }, { ...noMusic, submitted_by: 7 }],
    },
    {
      viewer: 'a signed-in reader who submitted a row',
      footprint: 'owner',
      rowProps: { userId: '42' },
      rows: [
        { submitted_by: 42 },
        { ...noMusic, submitted_by: 42 },
        { submitted_by: 7 },
        { ...noMusic, submitted_by: 7 },
      ],
    },
    {
      viewer: 'an admin',
      footprint: 'admin',
      rowProps: { isAdmin: true, userId: '1' },
      rows: [{}, noMusic],
    },
  ] as const

  it.each(viewers)(
    'is one fixed width on the header and every row for $viewer',
    ({ footprint, rowProps, rows }) => {
      render(
        <DayGroupedShowListHeader
          density="comfortable"
          actionsFootprint={footprint}
        />
      )
      const header = widthClasses(
        screen.getByTestId('show-list-header-actions')
      )
      expect(header).toHaveLength(1)
      expect(header[0]).toMatch(/^lg:w-\[\d+px\]$/)

      for (const [i, overrides] of rows.entries()) {
        renderRow(
          { ...overrides, id: i + 1, title: `Row ${i + 1}` },
          { ...rowProps, actionsFootprint: footprint }
        )
      }

      const articles = screen.getAllByRole('article')
      expect(articles).toHaveLength(rows.length)
      // Both variants are really on the page, so the comparison below covers
      // a row with the expand control and a row without it.
      const expandable = articles.filter(article =>
        within(article).queryByRole('button', {
          name: 'Discover artist music',
        })
      )
      expect(expandable).toHaveLength(rows.length / 2)

      for (const article of articles) {
        expect(
          widthClasses(within(article).getByTestId('row-actions'))
        ).toEqual(header)
      }
    }
  )

  // `ACTIONS_WIDTH` is sized for exactly these controls. A flag added to or
  // changed in the discovery policy fails here, so the widths get revisited.
  it('is sized for the discovery policy as it stands', () => {
    expect(SHOW_LIST_FEATURE_POLICY.discovery).toEqual({
      showDetailsLink: true,
      showSaveButton: true,
      showExpandMusic: true,
      showAdminActions: true,
      showOwnerActions: true,
      useCompactLayout: false,
    })
  })

  // When the fixed columns overflow a narrow row, the age cell is the one that
  // gives; the header's must give the same way or its later columns drift.
  it('lets the header age cell shrink exactly as the row age cell does', () => {
    render(
      <DayGroupedShowListHeader density="comfortable" actionsFootprint="admin" />
    )
    renderRow({}, { actionsFootprint: 'admin', isAdmin: true })

    const shrinkClasses = (element: HTMLElement) =>
      [...element.classList].filter(c => /^(shrink(-0)?|truncate)$/.test(c))
    expect(shrinkClasses(screen.getByText('Age'))).toEqual(
      shrinkClasses(screen.getByText('21+'))
    )
  })

  it('gives each footprint its own width, wider as it holds more controls', () => {
    const pixels = (['viewer', 'owner', 'admin'] as const).map(footprint => {
      const { unmount } = render(
        <DayGroupedShowListHeader
          density="comfortable"
          actionsFootprint={footprint}
        />
      )
      const [width] = widthClasses(
        screen.getByTestId('show-list-header-actions')
      )
      unmount()
      return Number(width.match(/\d+/)?.[0])
    })

    expect(pixels[0]).toBeLessThan(pixels[1])
    expect(pixels[1]).toBeLessThan(pixels[2])
  })
})

describe('DayGroupedShowListHeader', () => {
  it('labels every column the row renders', () => {
    render(
      <DayGroupedShowListHeader
        density="comfortable"
        actionsFootprint="viewer"
      />
    )

    for (const label of ['Time', 'Bill', 'Venue', 'Price', 'Age']) {
      expect(screen.getByText(label)).toBeInTheDocument()
    }
  })

  // Compact renders no age, and a label over fifty empty cells is a column that
  // is not there.
  it('drops the Age label in compact, where the column is empty', () => {
    render(
      <DayGroupedShowListHeader density="compact" actionsFootprint="viewer" />
    )

    expect(screen.queryByText('Age')).toBeNull()
    expect(screen.getByText('Time')).toBeInTheDocument()
  })

  // These rows are articles, not a table: with no header-to-cell association
  // the labels would arrive as five orphan words before the list.
  it('is hidden from assistive tech', () => {
    render(
      <DayGroupedShowListHeader
        density="comfortable"
        actionsFootprint="viewer"
      />
    )

    expect(screen.getByTestId('show-list-header')).toHaveAttribute(
      'aria-hidden',
      'true'
    )
  })
})
