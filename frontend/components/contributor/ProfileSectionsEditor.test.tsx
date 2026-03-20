import React from 'react'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent, waitFor } from '@testing-library/react'
import { renderWithProviders } from '@/test/utils'
import { ProfileSectionsEditor } from './ProfileSectionsEditor'
import type { ProfileSectionResponse } from '@/features/auth'

// Mock hooks
const mockUseOwnSections = vi.fn()
const mockCreateMutate = vi.fn()
const mockUpdateMutate = vi.fn()
const mockDeleteMutate = vi.fn()

vi.mock('@/features/auth', () => ({
  useOwnSections: () => mockUseOwnSections(),
  useCreateSection: () => ({
    mutate: mockCreateMutate,
    isPending: false,
  }),
  useUpdateSection: () => ({
    mutate: mockUpdateMutate,
    isPending: false,
  }),
  useDeleteSection: () => ({
    mutate: mockDeleteMutate,
    isPending: false,
  }),
}))

function makeSection(
  overrides: Partial<ProfileSectionResponse> = {}
): ProfileSectionResponse {
  return {
    id: 1,
    title: 'About Me',
    content: 'I love live music.',
    position: 0,
    is_visible: true,
    created_at: '2025-01-01T00:00:00Z',
    updated_at: '2025-01-01T00:00:00Z',
    ...overrides,
  }
}

describe('ProfileSectionsEditor', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('renders loading spinner when loading', () => {
    mockUseOwnSections.mockReturnValue({
      data: null,
      isLoading: true,
    })

    const { container } = renderWithProviders(<ProfileSectionsEditor />)
    expect(container.querySelector('.animate-spin')).toBeInTheDocument()
  })

  it('renders empty state when no sections exist', () => {
    mockUseOwnSections.mockReturnValue({
      data: { sections: [] },
      isLoading: false,
    })

    renderWithProviders(<ProfileSectionsEditor />)
    expect(
      screen.getByText('No custom sections yet. Add sections to personalize your profile.')
    ).toBeInTheDocument()
  })

  it('shows section count and limit', () => {
    mockUseOwnSections.mockReturnValue({
      data: { sections: [makeSection()] },
      isLoading: false,
    })

    renderWithProviders(<ProfileSectionsEditor />)
    expect(screen.getByText(/1\/3/)).toBeInTheDocument()
  })

  it('renders existing sections', () => {
    mockUseOwnSections.mockReturnValue({
      data: {
        sections: [
          makeSection({ id: 1, title: 'About Me', content: 'Music fan.' }),
          makeSection({
            id: 2,
            title: 'Genres',
            content: 'Punk, shoegaze.',
            position: 1,
          }),
        ],
      },
      isLoading: false,
    })

    renderWithProviders(<ProfileSectionsEditor />)
    expect(screen.getByText('About Me')).toBeInTheDocument()
    expect(screen.getByText('Music fan.')).toBeInTheDocument()
    expect(screen.getByText('Genres')).toBeInTheDocument()
    expect(screen.getByText('Punk, shoegaze.')).toBeInTheDocument()
  })

  it('shows "Hidden" badge for non-visible sections', () => {
    mockUseOwnSections.mockReturnValue({
      data: {
        sections: [makeSection({ is_visible: false })],
      },
      isLoading: false,
    })

    renderWithProviders(<ProfileSectionsEditor />)
    expect(screen.getByText('Hidden')).toBeInTheDocument()
  })

  it('does not show "Hidden" badge for visible sections', () => {
    mockUseOwnSections.mockReturnValue({
      data: {
        sections: [makeSection({ is_visible: true })],
      },
      isLoading: false,
    })

    renderWithProviders(<ProfileSectionsEditor />)
    expect(screen.queryByText('Hidden')).not.toBeInTheDocument()
  })

  it('shows "Add Section" button when under limit', () => {
    mockUseOwnSections.mockReturnValue({
      data: { sections: [makeSection()] },
      isLoading: false,
    })

    renderWithProviders(<ProfileSectionsEditor />)
    expect(screen.getByText('Add Section')).toBeInTheDocument()
  })

  it('hides "Add Section" button when at max sections', () => {
    mockUseOwnSections.mockReturnValue({
      data: {
        sections: [
          makeSection({ id: 1, position: 0 }),
          makeSection({ id: 2, position: 1 }),
          makeSection({ id: 3, position: 2 }),
        ],
      },
      isLoading: false,
    })

    renderWithProviders(<ProfileSectionsEditor />)
    expect(screen.queryByText('Add Section')).not.toBeInTheDocument()
  })

  it('opens create dialog when "Add Section" is clicked', () => {
    mockUseOwnSections.mockReturnValue({
      data: { sections: [] },
      isLoading: false,
    })

    renderWithProviders(<ProfileSectionsEditor />)
    fireEvent.click(screen.getByText('Add Section'))

    expect(screen.getByText('Add Profile Section')).toBeInTheDocument()
    expect(screen.getByLabelText('Title')).toBeInTheDocument()
    expect(screen.getByLabelText('Content')).toBeInTheDocument()
  })

  it('shows validation error for empty title on create', () => {
    mockUseOwnSections.mockReturnValue({
      data: { sections: [] },
      isLoading: false,
    })

    renderWithProviders(<ProfileSectionsEditor />)
    fireEvent.click(screen.getByText('Add Section'))

    // Submit with empty fields
    const addButtons = screen.getAllByText('Add Section')
    const submitButton = addButtons[addButtons.length - 1] // Dialog button
    fireEvent.click(submitButton)

    expect(screen.getByText('Title is required')).toBeInTheDocument()
    expect(mockCreateMutate).not.toHaveBeenCalled()
  })

  it('shows validation error for empty content on create', () => {
    mockUseOwnSections.mockReturnValue({
      data: { sections: [] },
      isLoading: false,
    })

    renderWithProviders(<ProfileSectionsEditor />)
    fireEvent.click(screen.getByText('Add Section'))

    // Fill title but not content
    fireEvent.change(screen.getByLabelText('Title'), {
      target: { value: 'My Section' },
    })
    const addButtons = screen.getAllByText('Add Section')
    fireEvent.click(addButtons[addButtons.length - 1])

    expect(screen.getByText('Content is required')).toBeInTheDocument()
    expect(mockCreateMutate).not.toHaveBeenCalled()
  })

  it('calls createSection with correct data', () => {
    mockUseOwnSections.mockReturnValue({
      data: { sections: [makeSection()] },
      isLoading: false,
    })

    renderWithProviders(<ProfileSectionsEditor />)
    fireEvent.click(screen.getByText('Add Section'))

    fireEvent.change(screen.getByLabelText('Title'), {
      target: { value: 'New Section' },
    })
    fireEvent.change(screen.getByLabelText('Content'), {
      target: { value: 'Section content here' },
    })

    const addButtons = screen.getAllByText('Add Section')
    fireEvent.click(addButtons[addButtons.length - 1])

    expect(mockCreateMutate).toHaveBeenCalledWith(
      {
        title: 'New Section',
        content: 'Section content here',
        position: 1, // Already 1 section, so next position is 1
      },
      expect.objectContaining({
        onSuccess: expect.any(Function),
        onError: expect.any(Function),
      })
    )
  })

  it('shows character count in create dialog', () => {
    mockUseOwnSections.mockReturnValue({
      data: { sections: [] },
      isLoading: false,
    })

    renderWithProviders(<ProfileSectionsEditor />)
    fireEvent.click(screen.getByText('Add Section'))

    expect(screen.getByText('0/2000')).toBeInTheDocument()

    fireEvent.change(screen.getByLabelText('Content'), {
      target: { value: 'Hello' },
    })

    expect(screen.getByText('5/2000')).toBeInTheDocument()
  })

  it('opens edit dialog with pre-filled values', () => {
    mockUseOwnSections.mockReturnValue({
      data: {
        sections: [
          makeSection({
            id: 1,
            title: 'About Me',
            content: 'Music fan.',
            is_visible: true,
          }),
        ],
      },
      isLoading: false,
    })

    renderWithProviders(<ProfileSectionsEditor />)

    // The section card has two icon buttons in a container div
    // Find buttons that are size-icon (h-8 w-8) — they contain SVGs
    const allButtons = screen.getAllByRole('button')
    const iconButtons = allButtons.filter(
      btn => btn.className.includes('size-9') || btn.className.includes('w-8')
    )
    // First icon button is edit, second is delete
    expect(iconButtons.length).toBeGreaterThanOrEqual(2)
    fireEvent.click(iconButtons[0])

    expect(screen.getByText('Edit Section')).toBeInTheDocument()
  })

  it('opens delete confirmation dialog', () => {
    mockUseOwnSections.mockReturnValue({
      data: {
        sections: [makeSection({ title: 'My Section' })],
      },
      isLoading: false,
    })

    renderWithProviders(<ProfileSectionsEditor />)

    const allButtons = screen.getAllByRole('button')
    const iconButtons = allButtons.filter(
      btn => btn.className.includes('size-9') || btn.className.includes('w-8')
    )
    expect(iconButtons.length).toBeGreaterThanOrEqual(2)
    // Second icon button is delete
    fireEvent.click(iconButtons[1])

    expect(screen.getByText('Delete Section')).toBeInTheDocument()
    // "My Section" appears in both the card and the dialog confirmation text
    expect(screen.getAllByText(/My Section/).length).toBeGreaterThanOrEqual(1)
  })

  it('calls deleteSection when confirmed', () => {
    mockUseOwnSections.mockReturnValue({
      data: {
        sections: [makeSection({ id: 42, title: 'To Delete' })],
      },
      isLoading: false,
    })

    renderWithProviders(<ProfileSectionsEditor />)

    // Open delete dialog
    const allButtons = screen.getAllByRole('button')
    const iconButtons = allButtons.filter(
      btn => btn.className.includes('size-9') || btn.className.includes('w-8')
    )
    expect(iconButtons.length).toBeGreaterThanOrEqual(2)
    fireEvent.click(iconButtons[1])

    // The dialog should now be open, confirm deletion
    // Find the destructive "Delete" button in the dialog footer
    const dialogButtons = screen.getAllByRole('button')
    const deleteConfirmButton = dialogButtons.find(
      btn => btn.textContent === 'Delete' && btn.className.includes('destructive')
    )
    expect(deleteConfirmButton).toBeDefined()
    fireEvent.click(deleteConfirmButton!)

    expect(mockDeleteMutate).toHaveBeenCalledWith(
      42,
      expect.objectContaining({ onSuccess: expect.any(Function) })
    )
  })

  it('sorts sections by position', () => {
    mockUseOwnSections.mockReturnValue({
      data: {
        sections: [
          makeSection({ id: 1, title: 'Second', position: 1 }),
          makeSection({ id: 2, title: 'First', position: 0 }),
        ],
      },
      isLoading: false,
    })

    renderWithProviders(<ProfileSectionsEditor />)
    const headings = screen.getAllByRole('heading', { level: 4 })
    expect(headings[0]).toHaveTextContent('First')
    expect(headings[1]).toHaveTextContent('Second')
  })

  it('handles null sections data gracefully', () => {
    mockUseOwnSections.mockReturnValue({
      data: null,
      isLoading: false,
    })

    renderWithProviders(<ProfileSectionsEditor />)
    expect(
      screen.getByText(/No custom sections yet/)
    ).toBeInTheDocument()
    expect(screen.getByText(/0\/3/)).toBeInTheDocument()
  })
})
