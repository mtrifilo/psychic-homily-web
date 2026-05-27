import { describe, it, expect, vi, beforeEach } from 'vitest'
import { screen, within, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { renderWithProviders } from '@/test/utils'
import type { ReleaseListItem, ReleaseDetail } from '../types'

// Data hooks: useReleases drives the admin list; useRelease backs the edit form.
const mockUseReleases = vi.fn()
const mockUseRelease = vi.fn()
vi.mock('../hooks/useReleases', () => ({
  useReleases: (opts: unknown) => mockUseReleases(opts),
  useRelease: (opts: unknown) => mockUseRelease(opts),
}))

// Artist search inside the create-form ArtistPicker.
vi.mock('@/features/artists', () => ({
  useArtistSearch: () => ({ data: { artists: [] as unknown[] }, isLoading: false }),
}))

// Admin mutations. Capture the create/update/delete spies so dialog flows can
// assert wiring without a live backend.
const mockCreate = vi.fn()
const mockUpdate = vi.fn()
const mockDelete = vi.fn()
const mockAddLink = vi.fn()
const mockRemoveLink = vi.fn()
vi.mock('../hooks/useAdminReleases', () => ({
  useCreateRelease: () => ({ mutate: mockCreate, isPending: false }),
  useUpdateRelease: () => ({ mutate: mockUpdate, isPending: false }),
  useDeleteRelease: () => ({ mutate: mockDelete, isPending: false }),
  useAddReleaseLink: () => ({ mutate: mockAddLink, isPending: false }),
  useRemoveReleaseLink: () => ({ mutate: mockRemoveLink, isPending: false }),
}))

import { ReleaseManagement, EditReleaseFormFields } from './ReleaseManagement'

function makeListItem(
  overrides: Partial<ReleaseListItem> = {}
): ReleaseListItem {
  return {
    id: 1,
    title: 'In Rainbows',
    slug: 'in-rainbows',
    release_type: 'lp',
    release_year: 2007,
    cover_art_url: null,
    artist_count: 1,
    artists: [{ id: 1, name: 'Radiohead', slug: 'radiohead' }],
    label_name: null,
    label_slug: null,
    ...overrides,
  }
}

function makeDetail(overrides: Partial<ReleaseDetail> = {}): ReleaseDetail {
  return {
    id: 1,
    title: 'In Rainbows',
    slug: 'in-rainbows',
    release_type: 'lp',
    release_year: 2007,
    release_date: null,
    cover_art_url: null,
    description: null,
    artists: [{ id: 1, slug: 'radiohead', name: 'Radiohead', role: 'main' }],
    labels: [],
    external_links: [],
    created_at: '2024-01-01T00:00:00Z',
    updated_at: '2024-01-01T00:00:00Z',
    ...overrides,
  }
}

describe('ReleaseManagement', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockUseReleases.mockReturnValue({
      data: { releases: [], total: 0, limit: 50, offset: 0 },
      isLoading: false,
      error: null,
    })
    mockUseRelease.mockReturnValue({ data: undefined, isLoading: false })
  })

  it('renders the management header and New Release action', () => {
    renderWithProviders(<ReleaseManagement />)
    expect(
      screen.getByRole('heading', { level: 2, name: 'Releases' })
    ).toBeInTheDocument()
    expect(
      screen.getByRole('button', { name: /New Release/ })
    ).toBeInTheDocument()
  })

  it('shows a spinner while releases load', () => {
    mockUseReleases.mockReturnValue({
      data: undefined,
      isLoading: true,
      error: null,
    })
    const { container } = renderWithProviders(<ReleaseManagement />)
    expect(container.querySelector('.animate-spin')).toBeInTheDocument()
  })

  it('shows an error banner when the query fails', () => {
    mockUseReleases.mockReturnValue({
      data: undefined,
      isLoading: false,
      error: new Error('Failed to load releases'),
    })
    renderWithProviders(<ReleaseManagement />)
    expect(screen.getByText('Failed to load releases')).toBeInTheDocument()
  })

  it('renders the empty state when there are no releases', () => {
    renderWithProviders(<ReleaseManagement />)
    expect(screen.getByText('No Releases Found')).toBeInTheDocument()
  })

  it('lists releases with their type label and year', () => {
    mockUseReleases.mockReturnValue({
      data: {
        releases: [
          makeListItem({ id: 1, title: 'In Rainbows', release_type: 'lp' }),
          makeListItem({
            id: 2,
            title: 'Spirit of Eden',
            release_type: 'ep',
            release_year: 1988,
          }),
        ],
        total: 2,
        limit: 50,
        offset: 0,
      },
      isLoading: false,
      error: null,
    })
    renderWithProviders(<ReleaseManagement />)
    expect(screen.getByText('In Rainbows')).toBeInTheDocument()
    expect(screen.getByText('Spirit of Eden')).toBeInTheDocument()
    expect(screen.getByText('2 releases')).toBeInTheDocument()
  })

  it('filters the list client-side by the debounced search input', async () => {
    const user = userEvent.setup()
    mockUseReleases.mockReturnValue({
      data: {
        releases: [
          makeListItem({ id: 1, title: 'In Rainbows' }),
          makeListItem({ id: 2, title: 'Kid A', slug: 'kid-a' }),
        ],
        total: 2,
        limit: 50,
        offset: 0,
      },
      isLoading: false,
      error: null,
    })
    renderWithProviders(<ReleaseManagement />)

    await user.type(screen.getByPlaceholderText('Search releases...'), 'kid')

    // Filtering is gated behind a 300ms debounce; poll until the non-matching
    // row drops out (real timers — waitFor's default 1s window covers it).
    await waitFor(() =>
      expect(screen.queryByText('In Rainbows')).not.toBeInTheDocument()
    )
    expect(screen.getByText('Kid A')).toBeInTheDocument()
  })

  it('passes the type filter through to useReleases', async () => {
    const user = userEvent.setup()
    renderWithProviders(<ReleaseManagement />)

    const typeSelect = screen.getByRole('combobox')
    await user.selectOptions(typeSelect, 'ep')

    expect(mockUseReleases).toHaveBeenLastCalledWith(
      expect.objectContaining({ releaseType: 'ep' })
    )
  })

  it('opens the create dialog from the New Release button', async () => {
    const user = userEvent.setup()
    renderWithProviders(<ReleaseManagement />)

    await user.click(screen.getByRole('button', { name: /New Release/ }))

    const dialog = await screen.findByRole('dialog')
    // "Create Release" appears twice in the dialog (title + submit button);
    // assert the title specifically.
    expect(
      within(dialog).getByRole('heading', { name: 'Create Release' })
    ).toBeInTheDocument()
    expect(within(dialog).getByLabelText(/Title/)).toBeInTheDocument()
  })

  it('validates that a title is required before creating', async () => {
    const user = userEvent.setup()
    renderWithProviders(<ReleaseManagement />)

    await user.click(screen.getByRole('button', { name: /New Release/ }))
    const dialog = await screen.findByRole('dialog')
    await user.click(
      within(dialog).getByRole('button', { name: 'Create Release' })
    )

    expect(within(dialog).getByText('Title is required')).toBeInTheDocument()
    expect(mockCreate).not.toHaveBeenCalled()
  })

  it('submits a valid create payload through the mutation', async () => {
    const user = userEvent.setup()
    renderWithProviders(<ReleaseManagement />)

    await user.click(screen.getByRole('button', { name: /New Release/ }))
    const dialog = await screen.findByRole('dialog')
    await user.type(within(dialog).getByLabelText(/Title/), 'New Album')
    await user.click(
      within(dialog).getByRole('button', { name: 'Create Release' })
    )

    expect(mockCreate).toHaveBeenCalledTimes(1)
    expect(mockCreate.mock.calls[0][0]).toMatchObject({ title: 'New Album' })
  })

  it('opens the edit dialog and loads the selected release', async () => {
    const user = userEvent.setup()
    mockUseReleases.mockReturnValue({
      data: {
        releases: [makeListItem({ id: 7, title: 'Editable' })],
        total: 1,
        limit: 50,
        offset: 0,
      },
      isLoading: false,
      error: null,
    })
    mockUseRelease.mockReturnValue({
      data: makeDetail({ id: 7, title: 'Editable' }),
      isLoading: false,
    })
    renderWithProviders(<ReleaseManagement />)

    // Row actions are icon-only buttons; identify edit by its lucide glyph.
    const editButtons = screen.getAllByRole('button')
    const pencil = editButtons.find((b) =>
      b.querySelector('.lucide-pencil')
    )
    expect(pencil).toBeDefined()
    await user.click(pencil!)

    const dialog = await screen.findByRole('dialog')
    expect(within(dialog).getByText('Edit Release')).toBeInTheDocument()
    expect(within(dialog).getByDisplayValue('Editable')).toBeInTheDocument()
  })

  it('opens the delete confirmation with the release title', async () => {
    const user = userEvent.setup()
    mockUseReleases.mockReturnValue({
      data: {
        releases: [makeListItem({ id: 9, title: 'Doomed Release' })],
        total: 1,
        limit: 50,
        offset: 0,
      },
      isLoading: false,
      error: null,
    })
    renderWithProviders(<ReleaseManagement />)

    const trashButton = screen
      .getAllByRole('button')
      .find((b) => b.querySelector('.lucide-trash2'))
    expect(trashButton).toBeDefined()
    await user.click(trashButton!)

    const dialog = await screen.findByRole('dialog')
    // "Delete Release" is both the dialog title and the confirm button.
    expect(
      within(dialog).getByRole('heading', { name: 'Delete Release' })
    ).toBeInTheDocument()
    expect(within(dialog).getByText(/Doomed Release/)).toBeInTheDocument()
  })

  it('calls the delete mutation when confirmed', async () => {
    const user = userEvent.setup()
    mockUseReleases.mockReturnValue({
      data: {
        releases: [makeListItem({ id: 9, title: 'Doomed Release' })],
        total: 1,
        limit: 50,
        offset: 0,
      },
      isLoading: false,
      error: null,
    })
    renderWithProviders(<ReleaseManagement />)

    const trashButton = screen
      .getAllByRole('button')
      .find((b) => b.querySelector('.lucide-trash2'))
    await user.click(trashButton!)

    const dialog = await screen.findByRole('dialog')
    await user.click(
      within(dialog).getByRole('button', { name: 'Delete Release' })
    )

    expect(mockDelete).toHaveBeenCalledWith(9, expect.anything())
  })

  describe('EditReleaseFormFields: release switch resets fields via key prop', () => {
    // Pins PSY-768: the inner form initializes local state from the release
    // prop on mount, with no useEffect and no `initialized` ratchet. Callers
    // pass `key={release.id}` so React unmounts + remounts with fresh state
    // when the release switches. The two assertions below are the
    // load-bearing pair — without both, a future maintainer could re-add a
    // release-prop-based reset and the tests would still pass.

    it('resets fields when re-rendered with a different release (via key prop)', async () => {
      const user = userEvent.setup()
      const releaseA = makeDetail({
        id: 1,
        title: 'Release A',
        release_year: 2020,
        description: 'A',
      })
      const releaseB = makeDetail({
        id: 2,
        title: 'Release B',
        release_year: 2024,
        description: 'B',
      })

      const { rerender } = renderWithProviders(
        <EditReleaseFormFields
          key={releaseA.id}
          release={releaseA}
          onSuccess={vi.fn()}
          onCancel={vi.fn()}
        />
      )

      const titleInput = screen.getByLabelText(/Title \*/i)
      expect(titleInput).toHaveValue('Release A')

      await user.clear(titleInput)
      await user.type(titleInput, 'Dirty Edit')
      expect(titleInput).toHaveValue('Dirty Edit')

      rerender(
        <EditReleaseFormFields
          key={releaseB.id}
          release={releaseB}
          onSuccess={vi.fn()}
          onCancel={vi.fn()}
        />
      )

      // Re-query after rerender — the key change unmounts the previous input.
      expect(screen.getByLabelText(/Title \*/i)).toHaveValue('Release B')
      expect(screen.getByLabelText(/Year/i)).toHaveValue(2024)
      expect(screen.getByLabelText(/Description/i)).toHaveValue('B')
    })

    it('preserves dirty edits when re-rendered with the same key', async () => {
      const user = userEvent.setup()
      const release = makeDetail({ id: 1, title: 'Release A' })

      const { rerender } = renderWithProviders(
        <EditReleaseFormFields
          key={release.id}
          release={release}
          onSuccess={vi.fn()}
          onCancel={vi.fn()}
        />
      )

      const titleInput = screen.getByLabelText(/Title \*/i)
      await user.clear(titleInput)
      await user.type(titleInput, 'Dirty Edit')

      rerender(
        <EditReleaseFormFields
          key={release.id}
          release={release}
          onSuccess={vi.fn()}
          onCancel={vi.fn()}
        />
      )

      expect(screen.getByLabelText(/Title \*/i)).toHaveValue('Dirty Edit')
    })
  })
})
