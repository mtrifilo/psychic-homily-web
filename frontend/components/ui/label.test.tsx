import { describe, it, expect } from 'vitest'
import { render, screen } from '@testing-library/react'
import { Label } from './label'

describe('Label', () => {
  it('renders its children without throwing', () => {
    render(<Label>Email</Label>)
    expect(screen.getByText('Email')).toBeInTheDocument()
  })

  it('merges a custom className', () => {
    render(<Label className="custom-class">Email</Label>)
    expect(screen.getByText('Email')).toHaveClass('custom-class')
  })

  it('associates with a control via htmlFor', () => {
    render(<Label htmlFor="email-input">Email</Label>)
    expect(screen.getByText('Email')).toHaveAttribute('for', 'email-input')
  })
})
