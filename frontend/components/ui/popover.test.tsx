import { describe, it, expect } from 'vitest'
import { createRef } from 'react'
import { render, screen } from '@testing-library/react'
import { Popover, PopoverTrigger, PopoverContent } from './popover'

describe('Popover', () => {
  it('renders the trigger without throwing', () => {
    render(
      <Popover>
        <PopoverTrigger>Open</PopoverTrigger>
        <PopoverContent>Panel</PopoverContent>
      </Popover>
    )
    expect(screen.getByRole('button', { name: 'Open' })).toBeInTheDocument()
  })

  it('renders portal content when open', () => {
    render(
      <Popover open>
        <PopoverTrigger>Open</PopoverTrigger>
        <PopoverContent>Panel content</PopoverContent>
      </Popover>
    )
    expect(screen.getByText('Panel content')).toBeInTheDocument()
  })

  it('merges a custom className on the content', () => {
    render(
      <Popover open>
        <PopoverTrigger>Open</PopoverTrigger>
        <PopoverContent className="custom-class">Panel content</PopoverContent>
      </Popover>
    )
    expect(screen.getByText('Panel content')).toHaveClass('custom-class')
  })

  it('forwards a ref to the content', () => {
    const ref = createRef<HTMLDivElement>()
    render(
      <Popover open>
        <PopoverTrigger>Open</PopoverTrigger>
        <PopoverContent ref={ref}>Panel content</PopoverContent>
      </Popover>
    )
    expect(ref.current).toBeInstanceOf(HTMLDivElement)
  })
})
