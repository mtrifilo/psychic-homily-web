import { describe, it, expect, vi } from 'vitest'
import { createRef } from 'react'
import { render, screen, fireEvent } from '@testing-library/react'
import { DateInput } from './date-input'

describe('DateInput', () => {
  it('renders a native date input', () => {
    render(<DateInput aria-label="release date" />)
    const input = screen.getByLabelText('release date')
    expect(input).toBeInTheDocument()
    expect(input).toHaveAttribute('type', 'date')
  })

  it('merges a custom className alongside the DS token classes', () => {
    render(<DateInput aria-label="release date" className="custom-class" />)
    const input = screen.getByLabelText('release date')
    expect(input).toHaveClass('custom-class')
    // DS state-treatment token still present (mirrors Input).
    expect(input).toHaveClass('border-input')
  })

  it('forwards a ref to the underlying input element', () => {
    const ref = createRef<HTMLInputElement>()
    render(<DateInput aria-label="release date" ref={ref} />)
    expect(ref.current).toBeInstanceOf(HTMLInputElement)
  })

  it('round-trips a controlled value', () => {
    render(<DateInput aria-label="release date" value="2026-06-01" readOnly />)
    expect(screen.getByLabelText('release date')).toHaveValue('2026-06-01')
  })

  it('fires onChange with the edited value', () => {
    const onChange = vi.fn()
    render(<DateInput aria-label="release date" onChange={onChange} />)
    fireEvent.change(screen.getByLabelText('release date'), {
      target: { value: '2026-07-04' },
    })
    expect(onChange).toHaveBeenCalledTimes(1)
    expect(onChange.mock.calls[0][0].target.value).toBe('2026-07-04')
  })

  it('honors the disabled prop', () => {
    render(<DateInput aria-label="release date" disabled />)
    expect(screen.getByLabelText('release date')).toBeDisabled()
  })

  it('reflects aria-invalid for the destructive state', () => {
    render(<DateInput aria-label="release date" aria-invalid />)
    expect(screen.getByLabelText('release date')).toHaveAttribute(
      'aria-invalid',
      'true'
    )
  })
})
