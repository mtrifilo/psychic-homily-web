import { describe, it, expect, vi, beforeEach } from 'vitest'
import { screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { renderWithProviders } from '@/test/utils'

const mockSections = [
  { id: 1, title: 'About Me', content: 'Hello world', position: 0, is_visible: true, created_at: '', updated_at: '' },
  { id: 2, title: 'Favorites', content: 'Some favorites', position: 1, is_visible: false, created_at: '', updated_at: '' },
]

// Mock mutate functions are reset per-test below.
const mockCreateMutate = vi.fn()
const mockUpdateMutate = vi.fn()
const mockDeleteMutate = vi.fn()

type SectionsHookValue = {
  data: { sections: typeof mockSections } | { sections: [] } | undefined
  isLoading: boolean
}

const mockUseOwnSections = vi.fn<() => SectionsHookValue>(() => ({
  data: { sections: mockSections },
  isLoading: false,
}))

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

import { ProfileSectionsEditor } from './ProfileSectionsEditor'

describe('ProfileSectionsEditor', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockUseOwnSections.mockReturnValue({
      data: { sections: mockSections },
      isLoading: false,
    })
  })

  describe('rendering', () => {
    it('shows a loading spinner while sections are loading', () => {
      mockUseOwnSections.mockReturnValue({
        data: undefined,
        isLoading: true,
      })
      const { container } = renderWithProviders(<ProfileSectionsEditor />)
      // Loader2 has animate-spin class
      expect(container.querySelector('.animate-spin')).toBeInTheDocument()
      // Header text only renders after load
      expect(screen.queryByText('Custom Sections')).not.toBeInTheDocument()
    })

    it('renders header with section count out of MAX_SECTIONS (3)', () => {
      renderWithProviders(<ProfileSectionsEditor />)
      expect(screen.getByText(/2\/3/)).toBeInTheDocument()
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

    it('shows "Hidden" badge for sections that are not visible', () => {
      renderWithProviders(<ProfileSectionsEditor />)
      // Only the second section (Favorites) has is_visible: false
      expect(screen.getByText('Hidden')).toBeInTheDocument()
    })

    it('does not show "Hidden" badge for visible sections', () => {
      mockUseOwnSections.mockReturnValue({
        data: {
          sections: [
            { id: 1, title: 'A', content: 'a', position: 0, is_visible: true, created_at: '', updated_at: '' },
          ],
        },
        isLoading: false,
      })
      renderWithProviders(<ProfileSectionsEditor />)
      expect(screen.queryByText('Hidden')).not.toBeInTheDocument()
    })

    it('renders empty-state card when there are no sections', () => {
      mockUseOwnSections.mockReturnValue({
        data: { sections: [] },
        isLoading: false,
      })
      renderWithProviders(<ProfileSectionsEditor />)
      expect(
        screen.getByText(/No custom sections yet/i)
      ).toBeInTheDocument()
    })

    it('hides the "Add Section" button when at MAX_SECTIONS (3) sections', () => {
      mockUseOwnSections.mockReturnValue({
        data: {
          sections: [
            { id: 1, title: 'A', content: 'a', position: 0, is_visible: true, created_at: '', updated_at: '' },
            { id: 2, title: 'B', content: 'b', position: 1, is_visible: true, created_at: '', updated_at: '' },
            { id: 3, title: 'C', content: 'c', position: 2, is_visible: true, created_at: '', updated_at: '' },
          ],
        },
        isLoading: false,
      })
      renderWithProviders(<ProfileSectionsEditor />)
      expect(
        screen.queryByRole('button', { name: /Add Section/i })
      ).not.toBeInTheDocument()
    })

    it('shows the "Add Section" button when below MAX_SECTIONS', () => {
      renderWithProviders(<ProfileSectionsEditor />)
      expect(
        screen.getByRole('button', { name: /Add Section/i })
      ).toBeInTheDocument()
    })

    it('sorts sections by position', () => {
      mockUseOwnSections.mockReturnValue({
        data: {
          sections: [
            { id: 1, title: 'Third', content: 'c', position: 2, is_visible: true, created_at: '', updated_at: '' },
            { id: 2, title: 'First', content: 'a', position: 0, is_visible: true, created_at: '', updated_at: '' },
            { id: 3, title: 'Second', content: 'b', position: 1, is_visible: true, created_at: '', updated_at: '' },
          ],
        },
        isLoading: false,
      })
      renderWithProviders(<ProfileSectionsEditor />)
      const titles = screen.getAllByRole('heading', { level: 4 })
      expect(titles[0]).toHaveTextContent('First')
      expect(titles[1]).toHaveTextContent('Second')
      expect(titles[2]).toHaveTextContent('Third')
    })
  })

  describe('create flow', () => {
    it('opens the create dialog when the "Add Section" button is clicked', async () => {
      const user = userEvent.setup()
      renderWithProviders(<ProfileSectionsEditor />)

      await user.click(screen.getByRole('button', { name: /Add Section/i }))

      // Dialog title appears (rendered via Radix Dialog)
      expect(screen.getByRole('dialog')).toBeInTheDocument()
      expect(screen.getByText('Add Profile Section')).toBeInTheDocument()
    })

    it('shows a "Title is required" error when title is empty', async () => {
      const user = userEvent.setup()
      renderWithProviders(<ProfileSectionsEditor />)

      await user.click(screen.getByRole('button', { name: /Add Section/i }))

      const dialog = screen.getByRole('dialog')
      const submitButton = within(dialog).getByRole('button', { name: /Add Section/i })
      await user.click(submitButton)

      expect(screen.getByText('Title is required')).toBeInTheDocument()
      expect(mockCreateMutate).not.toHaveBeenCalled()
    })

    it('shows a "Content is required" error when content is empty but title is set', async () => {
      const user = userEvent.setup()
      renderWithProviders(<ProfileSectionsEditor />)

      await user.click(screen.getByRole('button', { name: /Add Section/i }))

      const titleInput = screen.getByLabelText('Title')
      await user.type(titleInput, 'My Title')

      const dialog = screen.getByRole('dialog')
      const submitButton = within(dialog).getByRole('button', { name: /Add Section/i })
      await user.click(submitButton)

      expect(screen.getByText('Content is required')).toBeInTheDocument()
      expect(mockCreateMutate).not.toHaveBeenCalled()
    })

    it('calls createSection.mutate with trimmed title/content and next position', async () => {
      const user = userEvent.setup()
      renderWithProviders(<ProfileSectionsEditor />)

      await user.click(screen.getByRole('button', { name: /Add Section/i }))

      await user.type(screen.getByLabelText('Title'), '  My Title  ')
      await user.type(screen.getByLabelText('Content'), '  Some body  ')

      const dialog = screen.getByRole('dialog')
      const submitButton = within(dialog).getByRole('button', { name: /Add Section/i })
      await user.click(submitButton)

      expect(mockCreateMutate).toHaveBeenCalledTimes(1)
      const [payload] = mockCreateMutate.mock.calls[0]
      expect(payload).toEqual({
        title: 'My Title',
        content: 'Some body',
        position: mockSections.length,
      })
    })

    it('clears state and closes the dialog on create success', async () => {
      // Simulate success callback firing
      mockCreateMutate.mockImplementation(
        (_input: unknown, opts: { onSuccess?: () => void }) => {
          opts.onSuccess?.()
        }
      )

      const user = userEvent.setup()
      renderWithProviders(<ProfileSectionsEditor />)

      await user.click(screen.getByRole('button', { name: /Add Section/i }))

      await user.type(screen.getByLabelText('Title'), 'New Section')
      await user.type(screen.getByLabelText('Content'), 'New content')

      const dialog = screen.getByRole('dialog')
      const submitButton = within(dialog).getByRole('button', { name: /Add Section/i })
      await user.click(submitButton)

      // Dialog should close after success
      expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
    })

    it('surfaces backend error message on create failure', async () => {
      mockCreateMutate.mockImplementation(
        (_input: unknown, opts: { onError?: (err: Error) => void }) => {
          opts.onError?.(new Error('Backend rejected the section'))
        }
      )

      const user = userEvent.setup()
      renderWithProviders(<ProfileSectionsEditor />)

      await user.click(screen.getByRole('button', { name: /Add Section/i }))

      await user.type(screen.getByLabelText('Title'), 'New Section')
      await user.type(screen.getByLabelText('Content'), 'New content')

      const dialog = screen.getByRole('dialog')
      const submitButton = within(dialog).getByRole('button', { name: /Add Section/i })
      await user.click(submitButton)

      expect(
        screen.getByText('Backend rejected the section')
      ).toBeInTheDocument()
    })

    it('cancel button closes the create dialog without calling mutate', async () => {
      const user = userEvent.setup()
      renderWithProviders(<ProfileSectionsEditor />)

      await user.click(screen.getByRole('button', { name: /Add Section/i }))

      const dialog = screen.getByRole('dialog')
      const cancelButton = within(dialog).getByRole('button', { name: /Cancel/i })
      await user.click(cancelButton)

      expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
      expect(mockCreateMutate).not.toHaveBeenCalled()
    })

    it('displays the live character count for the content textarea', async () => {
      const user = userEvent.setup()
      renderWithProviders(<ProfileSectionsEditor />)

      await user.click(screen.getByRole('button', { name: /Add Section/i }))

      expect(screen.getByText('0/2000')).toBeInTheDocument()
      await user.type(screen.getByLabelText('Content'), 'hello')
      expect(screen.getByText('5/2000')).toBeInTheDocument()
    })
  })

  describe('edit flow', () => {
    it('opens the edit dialog seeded with the section data', async () => {
      const user = userEvent.setup()
      renderWithProviders(<ProfileSectionsEditor />)

      const editButtons = screen.getAllByRole('button', { name: 'Edit section' })
      await user.click(editButtons[0])

      expect(screen.getByRole('dialog')).toBeInTheDocument()
      expect(screen.getByText('Edit Section')).toBeInTheDocument()
      // Inputs pre-filled with first section's data
      expect(screen.getByLabelText('Title')).toHaveValue('About Me')
      expect(screen.getByLabelText('Content')).toHaveValue('Hello world')
    })

    it('toggles visibility via the "Visible on profile" switch', async () => {
      const user = userEvent.setup()
      renderWithProviders(<ProfileSectionsEditor />)

      // Open the edit dialog for the FIRST section (is_visible: true)
      const editButtons = screen.getAllByRole('button', { name: 'Edit section' })
      await user.click(editButtons[0])

      const visibilitySwitch = screen.getByLabelText('Visible on profile')
      expect(visibilitySwitch).toHaveAttribute('aria-checked', 'true')

      await user.click(visibilitySwitch)
      expect(visibilitySwitch).toHaveAttribute('aria-checked', 'false')
    })

    it('passes is_visible:false to updateSection when toggled off and saved', async () => {
      const user = userEvent.setup()
      renderWithProviders(<ProfileSectionsEditor />)

      const editButtons = screen.getAllByRole('button', { name: 'Edit section' })
      await user.click(editButtons[0])

      // Toggle visibility off
      const visibilitySwitch = screen.getByLabelText('Visible on profile')
      await user.click(visibilitySwitch)

      // Save
      const dialog = screen.getByRole('dialog')
      await user.click(within(dialog).getByRole('button', { name: /Save Changes/i }))

      expect(mockUpdateMutate).toHaveBeenCalledTimes(1)
      const [payload] = mockUpdateMutate.mock.calls[0]
      expect(payload).toMatchObject({
        sectionId: mockSections[0].id,
        data: expect.objectContaining({
          is_visible: false,
        }),
      })
    })

    it('shows validation error when edit title is blanked out', async () => {
      const user = userEvent.setup()
      renderWithProviders(<ProfileSectionsEditor />)

      const editButtons = screen.getAllByRole('button', { name: 'Edit section' })
      await user.click(editButtons[0])

      const titleInput = screen.getByLabelText('Title')
      await user.clear(titleInput)

      const dialog = screen.getByRole('dialog')
      await user.click(within(dialog).getByRole('button', { name: /Save Changes/i }))

      expect(screen.getByText('Title is required')).toBeInTheDocument()
      expect(mockUpdateMutate).not.toHaveBeenCalled()
    })

    it('closes the edit dialog without saving on Cancel', async () => {
      const user = userEvent.setup()
      renderWithProviders(<ProfileSectionsEditor />)

      const editButtons = screen.getAllByRole('button', { name: 'Edit section' })
      await user.click(editButtons[0])

      const dialog = screen.getByRole('dialog')
      const cancelButton = within(dialog).getByRole('button', { name: /Cancel/i })
      await user.click(cancelButton)

      expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
      expect(mockUpdateMutate).not.toHaveBeenCalled()
    })

    it('surfaces backend error message on update failure', async () => {
      mockUpdateMutate.mockImplementation(
        (_input: unknown, opts: { onError?: (err: Error) => void }) => {
          opts.onError?.(new Error('Update failed: server error'))
        }
      )

      const user = userEvent.setup()
      renderWithProviders(<ProfileSectionsEditor />)

      const editButtons = screen.getAllByRole('button', { name: 'Edit section' })
      await user.click(editButtons[0])

      const dialog = screen.getByRole('dialog')
      await user.click(within(dialog).getByRole('button', { name: /Save Changes/i }))

      expect(
        screen.getByText('Update failed: server error')
      ).toBeInTheDocument()
    })
  })

  describe('delete flow', () => {
    it('opens a delete-confirmation dialog naming the section', async () => {
      const user = userEvent.setup()
      renderWithProviders(<ProfileSectionsEditor />)

      const deleteButtons = screen.getAllByRole('button', { name: 'Delete section' })
      await user.click(deleteButtons[0])

      expect(screen.getByRole('dialog')).toBeInTheDocument()
      expect(screen.getByText('Delete Section')).toBeInTheDocument()
      // Confirmation copy mentions the section title (using a regex to handle smart quotes)
      const description = screen.getByText(/Are you sure you want to delete/i)
      expect(description.textContent).toContain('About Me')
    })

    it('calls deleteSection.mutate with the section id on Delete confirm', async () => {
      const user = userEvent.setup()
      renderWithProviders(<ProfileSectionsEditor />)

      const deleteButtons = screen.getAllByRole('button', { name: 'Delete section' })
      await user.click(deleteButtons[0])

      const dialog = screen.getByRole('dialog')
      const confirmButton = within(dialog).getByRole('button', { name: /^Delete$/ })
      await user.click(confirmButton)

      expect(mockDeleteMutate).toHaveBeenCalledTimes(1)
      const [id] = mockDeleteMutate.mock.calls[0]
      expect(id).toBe(mockSections[0].id)
    })

    it('closes the delete dialog on Cancel without calling mutate', async () => {
      const user = userEvent.setup()
      renderWithProviders(<ProfileSectionsEditor />)

      const deleteButtons = screen.getAllByRole('button', { name: 'Delete section' })
      await user.click(deleteButtons[0])

      const dialog = screen.getByRole('dialog')
      const cancelButton = within(dialog).getByRole('button', { name: /Cancel/i })
      await user.click(cancelButton)

      expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
      expect(mockDeleteMutate).not.toHaveBeenCalled()
    })
  })
})
