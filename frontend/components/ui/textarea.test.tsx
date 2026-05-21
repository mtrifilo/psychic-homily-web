import { describe, it, expect } from 'vitest'
import { createRef } from 'react'
import { render, screen } from '@testing-library/react'
import { Textarea } from './textarea'

describe('Textarea', () => {
  it('renders without throwing', () => {
    render(<Textarea aria-label="bio" />)
    expect(screen.getByLabelText('bio')).toBeInTheDocument()
  })

  it('merges a custom className', () => {
    render(<Textarea aria-label="bio" className="custom-class" />)
    expect(screen.getByLabelText('bio')).toHaveClass('custom-class')
  })

  it('forwards a ref to the underlying textarea element', () => {
    const ref = createRef<HTMLTextAreaElement>()
    render(<Textarea aria-label="bio" ref={ref} />)
    expect(ref.current).toBeInstanceOf(HTMLTextAreaElement)
  })

  it('honors the disabled prop', () => {
    render(<Textarea aria-label="bio" disabled />)
    expect(screen.getByLabelText('bio')).toBeDisabled()
  })
})
