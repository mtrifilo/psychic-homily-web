import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { SectionHeader } from './SectionHeader'
import { BracketLink } from './BracketLink'

describe('SectionHeader', () => {
  it('renders the title', () => {
    render(<SectionHeader title="Statistics" />)
    expect(screen.getByText('Statistics')).toBeInTheDocument()
  })

  it('renders as an h3 by default', () => {
    render(<SectionHeader title="Tags" />)
    expect(screen.getByRole('heading', { level: 3, name: 'Tags' })).toBeInTheDocument()
  })

  it('honors the `as` prop for heading level', () => {
    render(<SectionHeader title="Tags" as="h2" />)
    expect(screen.getByRole('heading', { level: 2, name: 'Tags' })).toBeInTheDocument()
  })

  it('renders an action when provided', () => {
    render(
      <SectionHeader
        title="Past shows"
        action={<BracketLink label="Toggle" onClick={vi.fn()} />}
      />
    )
    expect(screen.getByRole('button', { name: 'Toggle' })).toBeInTheDocument()
  })

  it('does not render an action slot when action is omitted', () => {
    render(<SectionHeader title="Statistics" />)
    expect(screen.queryByRole('button')).not.toBeInTheDocument()
  })

  it('action click fires parent handler (parent owns state)', async () => {
    const user = userEvent.setup()
    const onToggle = vi.fn()
    render(
      <SectionHeader
        title="Past shows"
        action={<BracketLink label="Toggle" onClick={onToggle} />}
      />
    )
    await user.click(screen.getByRole('button', { name: 'Toggle' }))
    expect(onToggle).toHaveBeenCalledOnce()
  })

  it('toggles the underline divider based on underline prop', () => {
    const { container, rerender } = render(<SectionHeader title="Tags" />)
    const wrapper = container.firstChild as HTMLElement
    expect(wrapper.className).toContain('border-b')

    rerender(<SectionHeader title="Tags" underline={false} />)
    expect(wrapper.className).not.toContain('border-b')
  })

  it('forwards custom className onto the wrapping element', () => {
    const { container } = render(<SectionHeader title="Tags" className="mt-8" />)
    const wrapper = container.firstChild as HTMLElement
    expect(wrapper.className).toContain('mt-8')
  })

  describe('variant (PSY-1062)', () => {
    it('defaults to the caps treatment', () => {
      render(<SectionHeader title="Statistics" />)
      const heading = screen.getByRole('heading', { name: 'Statistics' })
      expect(heading.className).toContain('uppercase')
      expect(heading.className).toContain('text-muted-foreground')
    })

    it('renders the title treatment without caps styling', () => {
      render(<SectionHeader title="Field notes" variant="title" size="md" />)
      const heading = screen.getByRole('heading', { name: 'Field notes' })
      expect(heading.className).not.toContain('uppercase')
      expect(heading.className).toContain('text-foreground')
      expect(heading.className).toContain('text-base')
    })

    it('renders the small title treatment at text-sm', () => {
      render(<SectionHeader title="Field notes" variant="title" />)
      const heading = screen.getByRole('heading', { name: 'Field notes' })
      expect(heading.className).toContain('text-sm')
      expect(heading.className).not.toContain('tracking-wider')
    })
  })
})
