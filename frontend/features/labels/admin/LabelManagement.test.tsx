import { describe, it, expect, vi, beforeEach } from 'vitest'
import { screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { renderWithProviders } from '@/test/utils'
import type { LabelDetail, LabelListItem } from '../types'

// List + detail hooks
const mockUseLabels = vi.fn()
const mockUseLabel = vi.fn()
vi.mock('../hooks/useLabels', () => ({
  useLabels: (...args: unknown[]) => mockUseLabels(...args),
  useLabel: (...args: unknown[]) => mockUseLabel(...args),
}))

// Admin mutation hooks
const mockCreateMutate = vi.fn()
const mockUpdateMutate = vi.fn()
const mockDeleteMutate = vi.fn()
vi.mock('../hooks/useAdminLabels', () => ({
  useCreateLabel: () => ({ mutate: mockCreateMutate, isPending: false }),
  useUpdateLabel: () => ({ mutate: mockUpdateMutate, isPending: false }),
  useDeleteLabel: () => ({ mutate: mockDeleteMutate, isPending: false }),
}))

import { LabelManagement, EditLabelFormFields } from './LabelManagement'

function makeLabelDetail(overrides: Partial<LabelDetail> = {}): LabelDetail {
  return {
    id: 1,
    name: 'Sub Pop',
    slug: 'sub-pop',
    city: 'Seattle',
    state: 'WA',
    country: 'US',
    founded_year: 1986,
    status: 'active',
    description: 'Independent record label.',
    social: {
      instagram: null,
      facebook: null,
      twitter: null,
      youtube: null,
      spotify: null,
      soundcloud: null,
      bandcamp: null,
      website: 'https://subpop.com',
    },
    artist_count: 0,
    release_count: 0,
    created_at: '2024-01-01T00:00:00Z',
    updated_at: '2024-01-01T00:00:00Z',
    ...overrides,
  }
}

function makeLabel(overrides: Partial<LabelListItem> = {}): LabelListItem {
  return {
    id: 1,
    name: 'Sub Pop',
    slug: 'sub-pop',
    city: 'Seattle',
    state: 'WA',
    status: 'active',
    artist_count: 12,
    release_count: 340,
    ...overrides,
  }
}

describe('LabelManagement', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockUseLabel.mockReturnValue({ data: null, isLoading: false })
    mockUseLabels.mockReturnValue({
      data: { labels: [], count: 0 },
      isLoading: false,
      error: null,
    })
  })

  it('renders the header and the New Label button', () => {
    renderWithProviders(<LabelManagement />)
    expect(
      screen.getByRole('heading', { name: 'Labels' })
    ).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /New Label/ })).toBeInTheDocument()
  })

  it('shows the loading spinner while labels are fetching', () => {
    mockUseLabels.mockReturnValue({ data: undefined, isLoading: true, error: null })

    renderWithProviders(<LabelManagement />)
    expect(document.querySelector('.animate-spin')).toBeInTheDocument()
  })

  it('shows the query error banner when the list fails', () => {
    mockUseLabels.mockReturnValue({
      data: undefined,
      isLoading: false,
      error: new Error('Failed to load labels.'),
    })

    renderWithProviders(<LabelManagement />)
    expect(screen.getByText('Failed to load labels.')).toBeInTheDocument()
  })

  it('shows the first-run empty state when there are no labels and no filters', () => {
    renderWithProviders(<LabelManagement />)
    expect(screen.getByText('No Labels Found')).toBeInTheDocument()
    expect(
      screen.getByText('No labels yet. Create your first label to get started.')
    ).toBeInTheDocument()
  })

  it('renders a row per label with status, location, and counts', () => {
    mockUseLabels.mockReturnValue({
      data: {
        labels: [makeLabel({ artist_count: 1, release_count: 2 })],
        count: 1,
      },
      isLoading: false,
      error: null,
    })

    renderWithProviders(<LabelManagement />)
    // Scope to the label row so the "Active" status filter <option> in the
    // toolbar doesn't collide with the row's "Active" status badge. `.border`
    // is unique to the outermost row container.
    const row = screen.getByText('Sub Pop').closest('div.border') as HTMLElement
    expect(row).toBeInTheDocument()
    expect(within(row).getByText('Active')).toBeInTheDocument()
    expect(within(row).getByText('Seattle, WA')).toBeInTheDocument()
    expect(within(row).getByText('1 artist')).toBeInTheDocument()
    expect(within(row).getByText('2 releases')).toBeInTheDocument()
    expect(screen.getByText('1 label')).toBeInTheDocument()
  })

  it('filters the visible rows client-side by the debounced search input', async () => {
    const user = userEvent.setup()
    mockUseLabels.mockReturnValue({
      data: {
        labels: [
          makeLabel({ id: 1, name: 'Sub Pop', slug: 'sub-pop' }),
          makeLabel({ id: 2, name: 'Merge Records', slug: 'merge-records' }),
        ],
        count: 2,
      },
      isLoading: false,
      error: null,
    })

    renderWithProviders(<LabelManagement />)
    expect(screen.getByText('Sub Pop')).toBeInTheDocument()
    expect(screen.getByText('Merge Records')).toBeInTheDocument()

    await user.type(screen.getByPlaceholderText('Search labels...'), 'merge')

    // Search is debounced 300ms; wait for the filtered view to settle.
    await screen.findByText('1 label matching "merge"')
    expect(screen.getByText('Merge Records')).toBeInTheDocument()
    expect(screen.queryByText('Sub Pop')).not.toBeInTheDocument()
  })

  it('passes the selected status filter through to useLabels', async () => {
    const user = userEvent.setup()
    renderWithProviders(<LabelManagement />)

    const statusSelect = screen.getByRole('combobox')
    await user.selectOptions(statusSelect, 'defunct')

    expect(mockUseLabels).toHaveBeenLastCalledWith({ status: 'defunct' })
  })

  it('opens the create dialog form when New Label is clicked', async () => {
    const user = userEvent.setup()
    renderWithProviders(<LabelManagement />)

    await user.click(screen.getByRole('button', { name: /New Label/ }))

    const dialog = await screen.findByRole('dialog')
    // "Create Label" is both the dialog title and the submit button label —
    // assert the title via its heading role to disambiguate.
    expect(
      within(dialog).getByRole('heading', { name: 'Create Label' })
    ).toBeInTheDocument()
    expect(within(dialog).getByLabelText('Name *')).toBeInTheDocument()
  })

  it('blocks create submission and shows an error when name is empty', async () => {
    const user = userEvent.setup()
    renderWithProviders(<LabelManagement />)

    await user.click(screen.getByRole('button', { name: /New Label/ }))
    const dialog = await screen.findByRole('dialog')
    await user.click(within(dialog).getByRole('button', { name: 'Create Label' }))

    expect(within(dialog).getByText('Name is required')).toBeInTheDocument()
    expect(mockCreateMutate).not.toHaveBeenCalled()
  })

  it('submits the create mutation with a trimmed name', async () => {
    const user = userEvent.setup()
    renderWithProviders(<LabelManagement />)

    await user.click(screen.getByRole('button', { name: /New Label/ }))
    const dialog = await screen.findByRole('dialog')
    await user.type(within(dialog).getByLabelText('Name *'), '  Hardly Art  ')
    await user.click(within(dialog).getByRole('button', { name: 'Create Label' }))

    expect(mockCreateMutate).toHaveBeenCalledTimes(1)
    expect(mockCreateMutate.mock.calls[0][0]).toMatchObject({ name: 'Hardly Art' })
  })

  it('opens the delete confirmation and fires the delete mutation', async () => {
    const user = userEvent.setup()
    mockUseLabels.mockReturnValue({
      data: { labels: [makeLabel()], count: 1 },
      isLoading: false,
      error: null,
    })

    renderWithProviders(<LabelManagement />)

    // The trash button is the second icon-only ghost button on the row.
    const rowButtons = screen.getAllByRole('button')
    const deleteButton = rowButtons[rowButtons.length - 1]
    await user.click(deleteButton)

    const dialog = await screen.findByRole('dialog')
    expect(within(dialog).getByText(/Are you sure you want to delete/)).toBeInTheDocument()
    await user.click(within(dialog).getByRole('button', { name: 'Delete Label' }))

    expect(mockDeleteMutate).toHaveBeenCalledWith(1, expect.any(Object))
  })

  describe('EditLabelFormFields: label switch resets fields via key prop', () => {
    // Pins PSY-768: the inner form initializes local state from the label
    // prop on mount, with no useEffect and no `initialized` ratchet. Callers
    // pass `key={label.id}` so React unmounts + remounts with fresh state
    // when the label switches. The two assertions below are the load-bearing
    // pair — without both, a future maintainer could re-add a label-prop-based
    // reset and the tests would still pass.

    it('resets fields when re-rendered with a different label (via key prop)', async () => {
      const user = userEvent.setup()
      const labelA = makeLabelDetail({
        id: 1,
        name: 'Sub Pop',
        city: 'Seattle',
      })
      const labelB = makeLabelDetail({
        id: 2,
        name: 'Merge Records',
        city: 'Durham',
        state: 'NC',
      })

      const { rerender } = renderWithProviders(
        <EditLabelFormFields
          key={labelA.id}
          label={labelA}
          onSuccess={vi.fn()}
          onCancel={vi.fn()}
        />
      )

      const nameInput = screen.getByLabelText('Name *')
      expect(nameInput).toHaveValue('Sub Pop')

      await user.clear(nameInput)
      await user.type(nameInput, 'Dirty Edit')
      expect(nameInput).toHaveValue('Dirty Edit')

      rerender(
        <EditLabelFormFields
          key={labelB.id}
          label={labelB}
          onSuccess={vi.fn()}
          onCancel={vi.fn()}
        />
      )

      expect(screen.getByLabelText('Name *')).toHaveValue('Merge Records')
      expect(screen.getByLabelText('City')).toHaveValue('Durham')
      expect(screen.getByLabelText('State')).toHaveValue('NC')
    })

    it('preserves dirty edits when re-rendered with the same key', async () => {
      const user = userEvent.setup()
      const label = makeLabelDetail({ id: 1, name: 'Sub Pop' })

      const { rerender } = renderWithProviders(
        <EditLabelFormFields
          key={label.id}
          label={label}
          onSuccess={vi.fn()}
          onCancel={vi.fn()}
        />
      )

      const nameInput = screen.getByLabelText('Name *')
      await user.clear(nameInput)
      await user.type(nameInput, 'Dirty Edit')

      rerender(
        <EditLabelFormFields
          key={label.id}
          label={label}
          onSuccess={vi.fn()}
          onCancel={vi.fn()}
        />
      )

      expect(screen.getByLabelText('Name *')).toHaveValue('Dirty Edit')
    })
  })
})
