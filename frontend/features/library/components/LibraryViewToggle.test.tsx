import { describe, it, expect, vi } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/react'
import { LibraryViewToggle } from './LibraryViewToggle'

describe('LibraryViewToggle', () => {
  it('marks table active by default selection', () => {
    render(<LibraryViewToggle view="table" onViewChange={vi.fn()} />)
    expect(screen.getByRole('radio', { name: 'Table view' })).toHaveAttribute(
      'aria-pressed',
      'true'
    )
    expect(screen.getByRole('radio', { name: 'Wall view' })).not.toHaveAttribute(
      'aria-pressed'
    )
  })

  it('calls onViewChange when wall is clicked', () => {
    const onViewChange = vi.fn()
    render(<LibraryViewToggle view="table" onViewChange={onViewChange} />)
    fireEvent.click(screen.getByRole('radio', { name: 'Wall view' }))
    expect(onViewChange).toHaveBeenCalledWith('wall')
  })

  it('marks wall active when selected', () => {
    render(<LibraryViewToggle view="wall" onViewChange={vi.fn()} />)
    expect(screen.getByRole('radio', { name: 'Wall view' })).toHaveAttribute(
      'aria-pressed',
      'true'
    )
  })
})
