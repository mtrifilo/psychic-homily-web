import { describe, it, expect } from 'vitest'
import { createRef } from 'react'
import { render, screen } from '@testing-library/react'
import {
  Tooltip,
  TooltipTrigger,
  TooltipContent,
  TooltipProvider,
} from './tooltip'

describe('Tooltip', () => {
  it('renders the trigger without throwing', () => {
    render(
      <TooltipProvider>
        <Tooltip>
          <TooltipTrigger>Trigger</TooltipTrigger>
          <TooltipContent>Tip</TooltipContent>
        </Tooltip>
      </TooltipProvider>
    )
    expect(screen.getByText('Trigger')).toBeInTheDocument()
  })

  it('renders portal content when open', () => {
    render(
      <TooltipProvider>
        <Tooltip open>
          <TooltipTrigger>Trigger</TooltipTrigger>
          <TooltipContent>Tip content</TooltipContent>
        </Tooltip>
      </TooltipProvider>
    )
    // Radix renders a visible tooltip plus a visually-hidden a11y copy.
    expect(screen.getAllByText('Tip content').length).toBeGreaterThan(0)
  })

  it('merges a custom className on the content', () => {
    render(
      <TooltipProvider>
        <Tooltip open>
          <TooltipTrigger>Trigger</TooltipTrigger>
          <TooltipContent className="custom-class">Tip content</TooltipContent>
        </Tooltip>
      </TooltipProvider>
    )
    const matches = screen.getAllByText('Tip content')
    expect(matches.some(el => el.closest('.custom-class'))).toBe(true)
  })

  it('forwards a ref to the content', () => {
    const ref = createRef<HTMLDivElement>()
    render(
      <TooltipProvider>
        <Tooltip open>
          <TooltipTrigger>Trigger</TooltipTrigger>
          <TooltipContent ref={ref}>Tip content</TooltipContent>
        </Tooltip>
      </TooltipProvider>
    )
    expect(ref.current).toBeInstanceOf(HTMLDivElement)
  })
})
