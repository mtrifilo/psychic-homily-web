import { describe, it, expect, vi } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/react'
import { AdminFormLayout, AdminFormRow, AdminFormField } from './AdminFormLayout'
import { Input } from '@/components/ui/input'
import { Button } from '@/components/ui/button'

function noop() {}

describe('AdminFormLayout', () => {
  it('renders the sheet variant with title, description, fields, and footer', () => {
    render(
      <AdminFormLayout
        open
        onOpenChange={noop}
        title="Add Radio Station"
        description="Create a new station."
        onSubmit={(e) => e.preventDefault()}
        footer={<Button type="submit">Create Station</Button>}
      >
        <AdminFormRow cols={2}>
          <AdminFormField label="Name *" htmlFor="name">
            <Input id="name" />
          </AdminFormField>
        </AdminFormRow>
      </AdminFormLayout>
    )
    expect(screen.getByText('Add Radio Station')).toBeInTheDocument()
    expect(screen.getByText('Create a new station.')).toBeInTheDocument()
    expect(screen.getByLabelText('Name *')).toBeInTheDocument()
    expect(
      screen.getByRole('button', { name: 'Create Station' })
    ).toBeInTheDocument()
  })

  it('renders the modal variant inside a dialog', () => {
    render(
      <AdminFormLayout
        variant="modal"
        open
        onOpenChange={noop}
        title="Confirm"
        description="Confirm this action."
        onSubmit={(e) => e.preventDefault()}
        footer={<Button type="submit">OK</Button>}
      >
        <p>body</p>
      </AdminFormLayout>
    )
    expect(screen.getByRole('dialog')).toBeInTheDocument()
    expect(screen.getByText('Confirm')).toBeInTheDocument()
  })

  it('fires onSubmit when the footer submit button is clicked', () => {
    const onSubmit = vi.fn((e: React.FormEvent) => e.preventDefault())
    render(
      <AdminFormLayout
        open
        onOpenChange={noop}
        title="T"
        description="A form."
        onSubmit={onSubmit}
        footer={<Button type="submit">Save</Button>}
      >
        <AdminFormField label="X" htmlFor="x">
          <Input id="x" />
        </AdminFormField>
      </AdminFormLayout>
    )
    fireEvent.click(screen.getByRole('button', { name: 'Save' }))
    expect(onSubmit).toHaveBeenCalledTimes(1)
  })

  it('renders nothing when closed', () => {
    render(
      <AdminFormLayout
        open={false}
        onOpenChange={noop}
        title="Hidden"
        onSubmit={noop}
        footer={null}
      >
        <p>nope</p>
      </AdminFormLayout>
    )
    expect(screen.queryByText('Hidden')).not.toBeInTheDocument()
  })
})

describe('AdminFormRow', () => {
  it('applies the responsive column class for cols={2}', () => {
    const { container } = render(
      <AdminFormRow cols={2}>
        <div>a</div>
        <div>b</div>
      </AdminFormRow>
    )
    const row = container.firstElementChild as HTMLElement
    expect(row).toHaveClass('grid', 'grid-cols-1', 'sm:grid-cols-2')
  })

  it('defaults to a single column (no sm:grid-cols-* class)', () => {
    const { container } = render(
      <AdminFormRow>
        <div>a</div>
      </AdminFormRow>
    )
    const row = container.firstElementChild as HTMLElement
    expect(row).toHaveClass('grid-cols-1')
    expect(row.className).not.toContain('sm:grid-cols-')
  })
})

describe('AdminFormField', () => {
  it('renders the label associated with the control via htmlFor', () => {
    render(
      <AdminFormField label="Email" htmlFor="email">
        <Input id="email" />
      </AdminFormField>
    )
    // getByLabelText resolves the label→control association.
    expect(screen.getByLabelText('Email')).toBeInTheDocument()
  })
})
