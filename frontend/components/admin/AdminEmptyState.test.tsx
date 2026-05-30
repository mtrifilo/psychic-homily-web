import { describe, it, expect } from 'vitest'
import { render, screen } from '@testing-library/react'
import { Inbox } from 'lucide-react'

import { AdminEmptyState } from './AdminEmptyState'

describe('AdminEmptyState', () => {
  it('renders the title as a heading and the message', () => {
    render(
      <AdminEmptyState
        icon={Inbox}
        title="No pending items"
        message="You're all caught up."
      />
    )

    expect(
      screen.getByRole('heading', { name: 'No pending items' })
    ).toBeInTheDocument()
    expect(screen.getByText("You're all caught up.")).toBeInTheDocument()
  })

  it('renders the supplied icon as an svg', () => {
    const { container } = render(
      <AdminEmptyState icon={Inbox} title="Empty" message="Nothing here." />
    )

    expect(container.querySelector('svg')).toBeInTheDocument()
  })

  it('renders an action when provided', () => {
    render(
      <AdminEmptyState
        icon={Inbox}
        title="Empty"
        message="Nothing here."
        action={<button>Add one</button>}
      />
    )

    expect(screen.getByRole('button', { name: 'Add one' })).toBeInTheDocument()
  })

  it('omits the action wrapper when no action is provided', () => {
    render(<AdminEmptyState icon={Inbox} title="Empty" message="Nothing here." />)

    expect(screen.queryByRole('button')).not.toBeInTheDocument()
  })

  it('applies the canonical card + square-icon-chip shape', () => {
    render(
      <AdminEmptyState
        icon={Inbox}
        title="Empty"
        message="Nothing here."
        testId="empty"
      />
    )

    const root = screen.getByTestId('empty')
    expect(root).toHaveClass('rounded-lg', 'border', 'bg-card', 'text-center')
    // Icon chip is a square (rounded-md), never rounded-full (banned).
    const chip = root.querySelector('.rounded-md')
    expect(chip).toBeInTheDocument()
    expect(root.querySelector('.rounded-full')).not.toBeInTheDocument()
  })

  it('merges className with the defaults rather than replacing them', () => {
    render(
      <AdminEmptyState
        icon={Inbox}
        title="Empty"
        message="Nothing here."
        testId="empty"
        className="my-8"
      />
    )

    const root = screen.getByTestId('empty')
    expect(root).toHaveClass('my-8', 'rounded-lg', 'bg-card')
  })

  it('forwards testId as data-testid', () => {
    render(
      <AdminEmptyState
        icon={Inbox}
        title="Empty"
        message="Nothing here."
        testId="moderation-empty"
      />
    )

    expect(screen.getByTestId('moderation-empty')).toBeInTheDocument()
  })
})
