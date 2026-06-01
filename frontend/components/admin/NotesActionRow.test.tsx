import { describe, it, expect, vi } from 'vitest'
import { fireEvent, render, screen } from '@testing-library/react'

import { NotesActionRow, type NotesAction } from './NotesActionRow'

// Mirrors the EntityReportCard wiring: Resolve (default/check) + Dismiss
// (outline/x), both with optional notes.
const resolveDismiss: [NotesAction, NotesAction] = [
  {
    key: 'resolve',
    restingLabel: 'Resolve',
    confirmLabel: 'Confirm Resolve',
    variant: 'default',
    icon: 'check',
    notesPlaceholder: 'Admin notes (optional) -- describe the action taken',
  },
  {
    key: 'dismiss',
    restingLabel: 'Dismiss',
    confirmLabel: 'Confirm Dismiss',
    variant: 'outline',
    icon: 'x',
    notesPlaceholder: 'Admin notes (optional) -- explain why this was dismissed',
  },
]

// Mirrors the CollectionReportCard wiring: destructive Hide (disabled +
// titled when the collection was deleted) + Dismiss.
const hideDismiss: [NotesAction, NotesAction] = [
  {
    key: 'hide',
    restingLabel: 'Hide from Public Browse',
    confirmLabel: 'Confirm Hide',
    variant: 'destructive',
    icon: 'x',
    notesPlaceholder: 'Reason for hiding from public browse (optional)',
  },
  {
    key: 'dismiss',
    restingLabel: 'Dismiss Report',
    confirmLabel: 'Confirm Dismiss',
    variant: 'outline',
    icon: 'check',
    notesPlaceholder: 'Notes for dismissal (optional)',
  },
]

function setup(
  actions: [NotesAction, NotesAction] = resolveDismiss,
  overrides?: { isActioning?: boolean }
) {
  const onConfirm = vi.fn()
  render(
    <NotesActionRow
      actions={actions}
      onConfirm={onConfirm}
      isActioning={overrides?.isActioning ?? false}
    />
  )
  return { onConfirm }
}

describe('NotesActionRow', () => {
  it('renders both resting actions with their labels', () => {
    setup()

    expect(screen.getByRole('button', { name: /resolve/i })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /^dismiss$/i })).toBeInTheDocument()
    // No textarea until an action is chosen.
    expect(screen.queryByPlaceholderText(/admin notes/i)).not.toBeInTheDocument()
  })

  it('expands an OPTIONAL-notes textarea for the chosen action with its placeholder + confirm label', () => {
    setup()

    fireEvent.click(screen.getByRole('button', { name: /resolve/i }))

    expect(
      screen.getByPlaceholderText('Admin notes (optional) -- describe the action taken')
    ).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /confirm resolve/i })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /cancel/i })).toBeInTheDocument()
  })

  it('confirms with EMPTY notes (notes are optional, not required)', () => {
    const { onConfirm } = setup()

    fireEvent.click(screen.getByRole('button', { name: /resolve/i }))
    // No text entered — confirm should still be enabled and fire.
    const confirm = screen.getByRole('button', { name: /confirm resolve/i })
    expect(confirm).not.toBeDisabled()
    fireEvent.click(confirm)

    expect(onConfirm).toHaveBeenCalledWith('resolve', '')
  })

  it('passes the chosen action key + trimmed notes to onConfirm', () => {
    const { onConfirm } = setup()

    fireEvent.click(screen.getByRole('button', { name: /^dismiss$/i }))
    fireEvent.change(
      screen.getByPlaceholderText('Admin notes (optional) -- explain why this was dismissed'),
      { target: { value: '  duplicate report  ' } }
    )
    fireEvent.click(screen.getByRole('button', { name: /confirm dismiss/i }))

    expect(onConfirm).toHaveBeenCalledWith('dismiss', 'duplicate report')
  })

  it('shows the placeholder + confirm label specific to whichever action was chosen', () => {
    setup()

    // Choosing the second action surfaces its (different) placeholder.
    fireEvent.click(screen.getByRole('button', { name: /^dismiss$/i }))
    expect(
      screen.getByPlaceholderText('Admin notes (optional) -- explain why this was dismissed')
    ).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /confirm dismiss/i })).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: /confirm resolve/i })).not.toBeInTheDocument()
  })

  it('Cancel returns to the resting state and discards notes', () => {
    const { onConfirm } = setup()

    fireEvent.click(screen.getByRole('button', { name: /resolve/i }))
    fireEvent.change(
      screen.getByPlaceholderText('Admin notes (optional) -- describe the action taken'),
      { target: { value: 'half typed' } }
    )
    fireEvent.click(screen.getByRole('button', { name: /cancel/i }))

    expect(onConfirm).not.toHaveBeenCalled()
    expect(screen.getByRole('button', { name: /resolve/i })).toBeInTheDocument()

    // Re-opening shows an empty textarea.
    fireEvent.click(screen.getByRole('button', { name: /resolve/i }))
    expect(
      screen.getByPlaceholderText('Admin notes (optional) -- describe the action taken')
    ).toHaveValue('')
  })

  it('applies the per-action variant to the confirm button (destructive Hide)', () => {
    setup(hideDismiss)

    fireEvent.click(screen.getByRole('button', { name: /hide from public browse/i }))
    const confirm = screen.getByRole('button', { name: /confirm hide/i })
    // Destructive buttons carry the bg-destructive class from the Button variant.
    expect(confirm.className).toMatch(/destructive/)
  })

  it('disables a resting action + sets its title via the disabled/title descriptor (CollectionReport deleted)', () => {
    const deletedHide: [NotesAction, NotesAction] = [
      { ...hideDismiss[0], disabled: true, title: 'Cannot hide — collection was deleted' },
      hideDismiss[1],
    ]
    setup(deletedHide)

    const hide = screen.getByRole('button', { name: /hide from public browse/i })
    expect(hide).toBeDisabled()
    expect(hide).toHaveAttribute('title', 'Cannot hide — collection was deleted')

    // Dismiss stays available so admins can clear stale reports.
    expect(screen.getByRole('button', { name: /dismiss report/i })).not.toBeDisabled()
  })

  it('disables every button while a mutation is in flight (isActioning)', () => {
    setup(resolveDismiss, { isActioning: true })

    expect(screen.getByRole('button', { name: /resolve/i })).toBeDisabled()
    expect(screen.getByRole('button', { name: /^dismiss$/i })).toBeDisabled()
  })

  it('shows a spinner on the confirm button while isActioning', () => {
    const props = {
      actions: resolveDismiss,
      onConfirm: vi.fn(),
    }
    const { rerender } = render(<NotesActionRow {...props} isActioning={false} />)

    fireEvent.click(screen.getByRole('button', { name: /resolve/i }))
    rerender(<NotesActionRow {...props} isActioning />)

    const confirm = screen.getByRole('button', { name: /confirm resolve/i })
    expect(confirm.querySelector('.animate-spin')).toBeInTheDocument()
  })
})
