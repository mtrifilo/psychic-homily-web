import { describe, it, expect } from 'vitest'
import { render } from '@testing-library/react'
import { Skeleton } from './skeleton'

describe('Skeleton', () => {
  it('renders without throwing and applies the pulse animation class', () => {
    const { container } = render(<Skeleton />)
    expect(container.firstChild).toHaveClass('animate-pulse')
  })

  it('merges a custom className', () => {
    const { container } = render(<Skeleton className="h-4 w-10" />)
    expect(container.firstChild).toHaveClass('h-4')
    expect(container.firstChild).toHaveClass('w-10')
  })

  it('forwards arbitrary props to the underlying div', () => {
    const { container } = render(<Skeleton data-testid="sk" />)
    expect(container.firstChild).toHaveAttribute('data-testid', 'sk')
  })
})
