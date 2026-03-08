import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { TagPill } from './TagPill'

describe('TagPill', () => {
  it('renders label text', () => {
    render(<TagPill label="post-punk" />)
    expect(screen.getByText('post-punk')).toBeInTheDocument()
  })

  it('renders as a button when no href is provided', () => {
    render(<TagPill label="shoegaze" />)
    expect(screen.getByRole('button')).toBeInTheDocument()
  })

  it('renders as a link when href is provided', () => {
    render(<TagPill label="shoegaze" href="/shows?tag=shoegaze" />)
    const link = screen.getByRole('link')
    expect(link).toBeInTheDocument()
    expect(link).toHaveAttribute('href', '/shows?tag=shoegaze')
  })

  it('displays positive vote count with + prefix', () => {
    render(<TagPill label="post-punk" voteCount={12} />)
    expect(screen.getByText('+12')).toBeInTheDocument()
  })

  it('displays zero vote count with + prefix', () => {
    render(<TagPill label="post-punk" voteCount={0} />)
    expect(screen.getByText('+0')).toBeInTheDocument()
  })

  it('displays negative vote count without + prefix', () => {
    render(<TagPill label="post-punk" voteCount={-3} />)
    expect(screen.getByText('-3')).toBeInTheDocument()
  })

  it('does not display vote count when undefined', () => {
    render(<TagPill label="post-punk" />)
    expect(screen.queryByText(/\+/)).not.toBeInTheDocument()
  })

  it('calls onClick handler when button is clicked', async () => {
    const user = userEvent.setup()
    const onClick = vi.fn()
    render(<TagPill label="shoegaze" onClick={onClick} />)

    await user.click(screen.getByRole('button'))
    expect(onClick).toHaveBeenCalledOnce()
  })

  it('applies custom className', () => {
    render(<TagPill label="jazz" className="mt-2" />)
    const button = screen.getByRole('button')
    expect(button.className).toContain('mt-2')
  })

  it('renders both label and vote count together in a link', () => {
    render(<TagPill label="noise-rock" voteCount={7} href="/tag/noise-rock" />)
    const link = screen.getByRole('link')
    expect(link).toHaveTextContent('noise-rock')
    expect(link).toHaveTextContent('+7')
  })

  it('button has type="button" attribute', () => {
    render(<TagPill label="ambient" />)
    expect(screen.getByRole('button')).toHaveAttribute('type', 'button')
  })
})
