import { describe, it, expect } from 'vitest'
import { createRef } from 'react'
import { render, screen } from '@testing-library/react'
import { Checkbox } from './checkbox'

describe('Checkbox', () => {
  it('renders without throwing', () => {
    render(<Checkbox aria-label="agree" />)
    expect(screen.getByRole('checkbox', { name: 'agree' })).toBeInTheDocument()
  })

  it('merges a custom className', () => {
    render(<Checkbox aria-label="agree" className="custom-class" />)
    expect(screen.getByRole('checkbox')).toHaveClass('custom-class')
  })

  it('forwards a ref to the underlying button element', () => {
    const ref = createRef<HTMLButtonElement>()
    render(<Checkbox aria-label="agree" ref={ref} />)
    expect(ref.current).toBeInstanceOf(HTMLButtonElement)
  })

  it('reflects the checked state via aria-checked', () => {
    render(<Checkbox aria-label="agree" defaultChecked />)
    expect(screen.getByRole('checkbox')).toBeChecked()
  })

  it('honors the disabled prop', () => {
    render(<Checkbox aria-label="agree" disabled />)
    expect(screen.getByRole('checkbox')).toBeDisabled()
  })
})
