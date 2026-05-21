import { describe, it, expect } from 'vitest'
import { createRef } from 'react'
import { render, screen } from '@testing-library/react'
import { HoverCard, HoverCardTrigger, HoverCardContent } from './hover-card'

describe('HoverCard', () => {
  it('renders the trigger without throwing', () => {
    render(
      <HoverCard>
        <HoverCardTrigger>Hover me</HoverCardTrigger>
        <HoverCardContent>Card</HoverCardContent>
      </HoverCard>
    )
    expect(screen.getByText('Hover me')).toBeInTheDocument()
  })

  it('renders portal content when open', () => {
    render(
      <HoverCard open>
        <HoverCardTrigger>Hover me</HoverCardTrigger>
        <HoverCardContent>Card content</HoverCardContent>
      </HoverCard>
    )
    expect(screen.getByText('Card content')).toBeInTheDocument()
  })

  it('merges a custom className on the content', () => {
    render(
      <HoverCard open>
        <HoverCardTrigger>Hover me</HoverCardTrigger>
        <HoverCardContent className="custom-class">Card content</HoverCardContent>
      </HoverCard>
    )
    expect(screen.getByText('Card content')).toHaveClass('custom-class')
  })

  it('forwards a ref to the content', () => {
    const ref = createRef<HTMLDivElement>()
    render(
      <HoverCard open>
        <HoverCardTrigger>Hover me</HoverCardTrigger>
        <HoverCardContent ref={ref}>Card content</HoverCardContent>
      </HoverCard>
    )
    expect(ref.current).toBeInstanceOf(HTMLDivElement)
  })
})
