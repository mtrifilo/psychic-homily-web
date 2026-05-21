import { describe, it, expect } from 'vitest'
import { createRef } from 'react'
import { render, screen } from '@testing-library/react'
import { Input } from './input'

describe('Input', () => {
  it('renders without throwing', () => {
    render(<Input aria-label="name" />)
    expect(screen.getByLabelText('name')).toBeInTheDocument()
  })

  it('merges a custom className', () => {
    render(<Input aria-label="name" className="custom-class" />)
    expect(screen.getByLabelText('name')).toHaveClass('custom-class')
  })

  it('forwards a ref to the underlying input element', () => {
    const ref = createRef<HTMLInputElement>()
    render(<Input aria-label="name" ref={ref} />)
    expect(ref.current).toBeInstanceOf(HTMLInputElement)
  })

  it('forwards the type attribute', () => {
    render(<Input aria-label="pw" type="password" />)
    expect(screen.getByLabelText('pw')).toHaveAttribute('type', 'password')
  })

  it('honors the disabled prop', () => {
    render(<Input aria-label="name" disabled />)
    expect(screen.getByLabelText('name')).toBeDisabled()
  })
})
