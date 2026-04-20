import React from 'react'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { screen, fireEvent, waitFor } from '@testing-library/react'
import { renderWithProviders } from '@/test/utils'
import { buildHierarchyTree } from './TagHierarchyEditor'
import type { GenreHierarchyTag } from '../types'

// ──────────────────────────────────────────────
// Hoisted hook stubs so individual tests can drive them.
// ──────────────────────────────────────────────

const mockGenreHierarchy = vi.fn()
const mockSetTagParentMutate = vi.fn()
const mockUseSetTagParent = vi.fn(() => ({
  mutate: mockSetTagParentMutate,
  isPending: false,
}))
const mockUseSearchTags = vi.fn(() => ({
  data: { tags: [] },
  isLoading: false,
}))

vi.mock('./useAdminTags', () => ({
  useGenreHierarchy: () => mockGenreHierarchy(),
  useSetTagParent: () => mockUseSetTagParent(),
}))

vi.mock('../hooks', () => ({
  useSearchTags: () => mockUseSearchTags(),
}))

// Import after mocks so the component sees the stubs.
import { TagHierarchyEditor } from './TagHierarchyEditor'

function tag(overrides: Partial<GenreHierarchyTag>): GenreHierarchyTag {
  return {
    id: 1,
    name: 'rock',
    slug: 'rock',
    parent_id: null,
    usage_count: 0,
    is_official: false,
    ...overrides,
  }
}

// ──────────────────────────────────────────────
// Pure tree assembly
// ──────────────────────────────────────────────

describe('buildHierarchyTree', () => {
  it('returns a flat list of roots when no parent_id is set', () => {
    const tree = buildHierarchyTree([
      tag({ id: 1, name: 'rock' }),
      tag({ id: 2, name: 'post-punk' }),
    ])
    expect(tree).toHaveLength(2)
    // Alphabetical ordering.
    expect(tree[0].name).toBe('post-punk')
    expect(tree[1].name).toBe('rock')
    expect(tree[0].depth).toBe(0)
    expect(tree[0].children).toEqual([])
  })

  it('nests children under their parent with incremented depth', () => {
    const tree = buildHierarchyTree([
      tag({ id: 1, name: 'post-punk' }),
      tag({ id: 2, name: 'shoegaze', parent_id: 1 }),
      tag({ id: 3, name: 'nu-gaze', parent_id: 2 }),
    ])
    expect(tree).toHaveLength(1)
    expect(tree[0].name).toBe('post-punk')
    expect(tree[0].depth).toBe(0)
    expect(tree[0].children).toHaveLength(1)
    expect(tree[0].children[0].name).toBe('shoegaze')
    expect(tree[0].children[0].depth).toBe(1)
    expect(tree[0].children[0].children[0].name).toBe('nu-gaze')
    expect(tree[0].children[0].children[0].depth).toBe(2)
  })

  it('promotes orphans to roots when parent_id points at a missing tag', () => {
    const tree = buildHierarchyTree([
      tag({ id: 1, name: 'rock' }),
      // parent_id=99 is not in the list.
      tag({ id: 2, name: 'orphan', parent_id: 99 }),
    ])
    expect(tree).toHaveLength(2)
    expect(tree.map(t => t.name).sort()).toEqual(['orphan', 'rock'])
  })

  it('denormalizes parent_name onto child nodes for breadcrumb chips', () => {
    // PSY-486: filter view flattens children to depth 0 and strips
    // tree context, so the row needs `parent_name` available without
    // re-walking the tree.
    const tree = buildHierarchyTree([
      tag({ id: 1, name: 'post-punk' }),
      tag({ id: 2, name: 'shoegaze', parent_id: 1 }),
      tag({ id: 3, name: 'nu-gaze', parent_id: 2 }),
    ])
    const root = tree[0]
    expect(root.parent_name).toBeNull()
    const shoegaze = root.children[0]
    expect(shoegaze.parent_name).toBe('post-punk')
    expect(shoegaze.children[0].parent_name).toBe('shoegaze')
  })
})

// ──────────────────────────────────────────────
// Component rendering
// ──────────────────────────────────────────────

describe('TagHierarchyEditor', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockUseSetTagParent.mockReturnValue({
      mutate: mockSetTagParentMutate,
      isPending: false,
    })
    mockUseSearchTags.mockReturnValue({ data: { tags: [] }, isLoading: false })
  })

  it('renders a tree of genre tags with indentation per depth', () => {
    mockGenreHierarchy.mockReturnValue({
      data: {
        tags: [
          tag({ id: 1, name: 'post-punk' }),
          tag({ id: 2, name: 'shoegaze', parent_id: 1 }),
          tag({ id: 3, name: 'rock' }),
        ],
      },
      isLoading: false,
      error: null,
    })

    renderWithProviders(<TagHierarchyEditor />)

    // The container description mentions "post-punk" in an example, so scope
    // the assertions to the tree itself via data-testid.
    const tree = screen.getByTestId('hierarchy-tree')
    const rows = screen.getAllByTestId('hierarchy-row')
    expect(rows).toHaveLength(3)

    const rowById = (id: number) =>
      rows.find(r => r.getAttribute('data-tag-id') === String(id))!

    expect(rowById(1)).toHaveTextContent('post-punk')
    expect(rowById(2)).toHaveTextContent('shoegaze')
    expect(rowById(3)).toHaveTextContent('rock')
    // Child has indentation; root does not.
    expect(rowById(2)).toHaveStyle({ paddingLeft: '28px' })
    expect(rowById(1)).toHaveStyle({ paddingLeft: '8px' })
    expect(tree).toBeInTheDocument()
  })

  it('opens the parent picker when the edit icon is clicked', async () => {
    mockGenreHierarchy.mockReturnValue({
      data: {
        tags: [
          tag({ id: 1, name: 'post-punk' }),
          tag({ id: 2, name: 'shoegaze', parent_id: 1 }),
        ],
      },
      isLoading: false,
      error: null,
    })

    renderWithProviders(<TagHierarchyEditor />)

    const editButtons = screen.getAllByRole('button', {
      name: /edit parent of/i,
    })
    fireEvent.click(editButtons[0])

    await waitFor(() => {
      expect(
        screen.getByText(/Set parent for/i)
      ).toBeInTheDocument()
    })
  })

  it('fires the set-parent mutation when a candidate is selected and confirmed', async () => {
    mockGenreHierarchy.mockReturnValue({
      data: {
        tags: [
          tag({ id: 1, name: 'post-punk' }),
          tag({ id: 2, name: 'shoegaze' }),
          tag({ id: 3, name: 'indie' }),
        ],
      },
      isLoading: false,
      error: null,
    })

    // Search returns 'post-punk' as a candidate.
    mockUseSearchTags.mockReturnValue({
      data: {
        tags: [
          {
            id: 1,
            name: 'post-punk',
            slug: 'post-punk',
            category: 'genre',
            is_official: false,
            usage_count: 5,
            created_at: '2025-01-01T00:00:00Z',
          },
        ],
      },
      isLoading: false,
    })

    renderWithProviders(<TagHierarchyEditor />)

    // Open the picker for 'shoegaze'.
    const editButton = screen.getByRole('button', {
      name: /edit parent of shoegaze/i,
    })
    fireEvent.click(editButton)

    // Type enough to flip debounced query on.
    const searchBox = await screen.findByPlaceholderText(/Search genre tags/i)
    fireEvent.change(searchBox, { target: { value: 'post' } })

    // Candidates list has an aria-label; scope findByText to it so we don't
    // match the "post-punk" that appears in the tree under the dialog.
    const candidateList = await screen.findByRole('list', {
      name: /candidate parent tags/i,
    })
    const candidate = await screen.findByRole('button', {
      name: /post-punk/i,
    })
    expect(candidateList).toContainElement(candidate)
    fireEvent.click(candidate)

    const saveBtn = screen.getByRole('button', { name: /^Save$/i })
    fireEvent.click(saveBtn)

    await waitFor(() => {
      expect(mockSetTagParentMutate).toHaveBeenCalledWith(
        { tagId: 2, parentId: 1 },
        expect.any(Object)
      )
    })
  })

  it('surfaces a backend error message (e.g. cycle detection) in the dialog', async () => {
    // Two unrelated genre tags, so the pre-filter doesn't hide either
    // candidate. The error comes from the mutation itself (simulating
    // backend cycle detection even though this particular setup wouldn't
    // actually create a cycle — the point is that the UI surfaces whatever
    // error the backend returns).
    mockGenreHierarchy.mockReturnValue({
      data: {
        tags: [tag({ id: 1, name: 'alpha' }), tag({ id: 2, name: 'beta' })],
      },
      isLoading: false,
      error: null,
    })

    mockUseSearchTags.mockReturnValue({
      data: {
        tags: [
          {
            id: 2,
            name: 'beta',
            slug: 'beta',
            category: 'genre',
            is_official: false,
            usage_count: 1,
            created_at: '2025-01-01T00:00:00Z',
          },
        ],
      },
      isLoading: false,
    })

    // Simulate the backend returning a cycle-detection error.
    mockSetTagParentMutate.mockImplementation((_vars, opts) => {
      opts?.onError?.(
        new Error("Cannot set parent: 'alpha' is an ancestor of 'beta'")
      )
    })

    renderWithProviders(<TagHierarchyEditor />)

    const editButton = screen.getByRole('button', {
      name: /edit parent of alpha/i,
    })
    fireEvent.click(editButton)

    const searchBox = await screen.findByPlaceholderText(/Search genre tags/i)
    fireEvent.change(searchBox, { target: { value: 'beta' } })

    const candidate = await screen.findByRole('button', {
      name: /beta/i,
    })
    fireEvent.click(candidate)

    const saveBtn = screen.getByRole('button', { name: /^Save$/i })
    fireEvent.click(saveBtn)

    await waitFor(() => {
      expect(screen.getByRole('alert')).toHaveTextContent(
        /is an ancestor of/i
      )
    })
  })

  it('clears the parent when the Clear parent checkbox is used', async () => {
    mockGenreHierarchy.mockReturnValue({
      data: {
        tags: [
          tag({ id: 1, name: 'post-punk' }),
          tag({ id: 2, name: 'shoegaze', parent_id: 1 }),
        ],
      },
      isLoading: false,
      error: null,
    })

    renderWithProviders(<TagHierarchyEditor />)

    const editButton = screen.getByRole('button', {
      name: /edit parent of shoegaze/i,
    })
    fireEvent.click(editButton)

    const clearCheckbox = await screen.findByLabelText(
      /Clear parent \(make this tag a root\)/i
    )
    fireEvent.click(clearCheckbox)

    const saveBtn = screen.getByRole('button', { name: /^Save$/i })
    fireEvent.click(saveBtn)

    await waitFor(() => {
      expect(mockSetTagParentMutate).toHaveBeenCalledWith(
        { tagId: 2, parentId: null },
        expect.any(Object)
      )
    })
  })

  it('shows an empty state when there are no genre tags', () => {
    mockGenreHierarchy.mockReturnValue({
      data: { tags: [] },
      isLoading: false,
      error: null,
    })

    renderWithProviders(<TagHierarchyEditor />)
    expect(screen.getByText(/No Genre Tags/i)).toBeInTheDocument()
  })

  it('surfaces a load error', () => {
    mockGenreHierarchy.mockReturnValue({
      data: undefined,
      isLoading: false,
      error: new Error('boom'),
    })

    renderWithProviders(<TagHierarchyEditor />)
    expect(screen.getByRole('alert')).toHaveTextContent(/boom/)
  })

  // PSY-486: surface "N children" badge so admins can see hierarchy shape
  // at a glance without expanding every parent.
  it('renders a children count badge on rows that have descendants', () => {
    mockGenreHierarchy.mockReturnValue({
      data: {
        tags: [
          tag({ id: 1, name: 'post-punk' }),
          tag({ id: 2, name: 'shoegaze', parent_id: 1 }),
          tag({ id: 3, name: 'no-wave', parent_id: 1 }),
          tag({ id: 4, name: 'rock' }),
        ],
      },
      isLoading: false,
      error: null,
    })

    renderWithProviders(<TagHierarchyEditor />)

    const rows = screen.getAllByTestId('hierarchy-row')
    const rowById = (id: number) =>
      rows.find(r => r.getAttribute('data-tag-id') === String(id))!

    // post-punk has 2 children → badge present, plural label
    const postPunk = rowById(1)
    const postPunkBadge = postPunk.querySelector(
      '[data-testid="child-count-badge"]'
    )
    expect(postPunkBadge).not.toBeNull()
    expect(postPunkBadge).toHaveTextContent('2 children')

    // rock and the leaf children get no badge
    const rock = rowById(4)
    expect(
      rock.querySelector('[data-testid="child-count-badge"]')
    ).toBeNull()
    const shoegaze = rowById(2)
    expect(
      shoegaze.querySelector('[data-testid="child-count-badge"]')
    ).toBeNull()
  })

  it('uses singular "1 child" when a parent has exactly one descendant', () => {
    mockGenreHierarchy.mockReturnValue({
      data: {
        tags: [
          tag({ id: 1, name: 'post-punk' }),
          tag({ id: 2, name: 'shoegaze', parent_id: 1 }),
        ],
      },
      isLoading: false,
      error: null,
    })

    renderWithProviders(<TagHierarchyEditor />)

    const rows = screen.getAllByTestId('hierarchy-row')
    const postPunkBadge = rows
      .find(r => r.getAttribute('data-tag-id') === '1')!
      .querySelector('[data-testid="child-count-badge"]')
    expect(postPunkBadge).toHaveTextContent('1 child')
  })

  // PSY-486: filter view flattens to depth 0 — show `parent ›` chip so the
  // admin can still see the tree position of each match.
  it('renders parent breadcrumb chip and child count on filtered matches', () => {
    mockGenreHierarchy.mockReturnValue({
      data: {
        tags: [
          tag({ id: 1, name: 'post-punk' }),
          tag({ id: 2, name: 'shoegaze', parent_id: 1 }),
          tag({ id: 3, name: 'nu-gaze', parent_id: 2 }),
        ],
      },
      isLoading: false,
      error: null,
    })

    renderWithProviders(<TagHierarchyEditor />)

    // Type a search that only matches the middle child.
    const filterInput = screen.getByPlaceholderText(/Filter genre tags/i)
    fireEvent.change(filterInput, { target: { value: 'shoe' } })

    const rows = screen.getAllByTestId('hierarchy-row')
    expect(rows).toHaveLength(1)
    const shoegaze = rows[0]

    // Parent breadcrumb chip is now visible beside the name even though
    // the row is rendered at depth 0.
    const breadcrumb = shoegaze.querySelector(
      '[data-testid="parent-breadcrumb"]'
    )
    expect(breadcrumb).not.toBeNull()
    expect(breadcrumb).toHaveTextContent('post-punk')

    // Children count badge survives the flattening — shoegaze owns nu-gaze.
    const badge = shoegaze.querySelector(
      '[data-testid="child-count-badge"]'
    )
    expect(badge).not.toBeNull()
    expect(badge).toHaveTextContent('1 child')
  })

  it('does not render parent breadcrumb chip when not filtering', () => {
    mockGenreHierarchy.mockReturnValue({
      data: {
        tags: [
          tag({ id: 1, name: 'post-punk' }),
          tag({ id: 2, name: 'shoegaze', parent_id: 1 }),
        ],
      },
      isLoading: false,
      error: null,
    })

    renderWithProviders(<TagHierarchyEditor />)

    // Tree view: parent context comes from indentation, so the chip is hidden.
    expect(
      screen.queryByTestId('parent-breadcrumb')
    ).not.toBeInTheDocument()
  })
})
