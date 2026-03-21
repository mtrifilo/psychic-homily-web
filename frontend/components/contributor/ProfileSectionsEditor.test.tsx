import { describe, it, expect, vi, beforeEach } from 'vitest'
import { screen } from '@testing-library/react'
import { renderWithProviders } from '@/test/utils'

const mockSections = [
  { id: 1, title: 'About Me', content: 'Hello world', position: 0, is_visible: true, created_at: '', updated_at: '' },
  { id: 2, title: 'Favorites', content: 'Some favorites', position: 1, is_visible: false, created_at: '', updated_at: '' },
]

vi.mock('@/features/auth', () => ({
  useOwnSections: () => ({
    data: { sections: mockSections },
    isLoading: false,
  }),
  useCreateSection: () => ({
    mutate: vi.fn(),
    isPending: false,
  }),
  useUpdateSection: () => ({
    mutate: vi.fn(),
    isPending: false,
  }),
  useDeleteSection: () => ({
    mutate: vi.fn(),
    isPending: false,
  }),
}))

import { ProfileSectionsEditor } from './ProfileSectionsEditor'

describe('ProfileSectionsEditor aria-labels', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('renders edit buttons with aria-label="Edit section"', () => {
    renderWithProviders(<ProfileSectionsEditor />)
    const editButtons = screen.getAllByRole('button', { name: 'Edit section' })
    expect(editButtons).toHaveLength(mockSections.length)
  })

  it('renders delete buttons with aria-label="Delete section"', () => {
    renderWithProviders(<ProfileSectionsEditor />)
    const deleteButtons = screen.getAllByRole('button', { name: 'Delete section' })
    expect(deleteButtons).toHaveLength(mockSections.length)
  })

  it('each section has both an edit and delete button with aria-labels', () => {
    renderWithProviders(<ProfileSectionsEditor />)

    // One edit and one delete per section
    const editButtons = screen.getAllByRole('button', { name: 'Edit section' })
    const deleteButtons = screen.getAllByRole('button', { name: 'Delete section' })

    expect(editButtons).toHaveLength(2)
    expect(deleteButtons).toHaveLength(2)

    // Verify each has the correct aria-label attribute
    editButtons.forEach(btn => {
      expect(btn).toHaveAttribute('aria-label', 'Edit section')
    })
    deleteButtons.forEach(btn => {
      expect(btn).toHaveAttribute('aria-label', 'Delete section')
    })
  })
})
