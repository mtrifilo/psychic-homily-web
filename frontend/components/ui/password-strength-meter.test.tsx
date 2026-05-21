import { describe, it, expect } from 'vitest'
import { render, screen } from '@testing-library/react'
import {
  PasswordStrengthMeter,
  calculateStrength,
  checkRequirements,
  getStrengthInfo,
} from './password-strength-meter'

describe('calculateStrength', () => {
  it('returns 0 for an empty password', () => {
    expect(calculateStrength('')).toBe(0)
  })

  it('scores a short single-class password as weak', () => {
    // "abc": hasLower (+10) only; no length bonuses (< 12 chars).
    expect(calculateStrength('abc')).toBe(10)
  })

  it('scores a 12-char four-class password in the middle range', () => {
    // 12 chars: length>=12 (+20); all four character classes (+40);
    // entropy bonus requires uniqueChars/length > 0.5 at length>=12.
    const score = calculateStrength('Abcdef123!@#')
    expect(score).toBeGreaterThanOrEqual(50)
    expect(score).toBeLessThan(90)
  })

  it('scores a long high-variety password as strong (capped at 100)', () => {
    const score = calculateStrength('Abcdefgh123!@#$%^&*()')
    expect(score).toBeGreaterThanOrEqual(90)
    expect(score).toBeLessThanOrEqual(100)
  })
})

describe('getStrengthInfo', () => {
  it('labels low scores as Weak', () => {
    expect(getStrengthInfo(10).label).toBe('Weak')
  })

  it('labels mid scores as Good', () => {
    expect(getStrengthInfo(60).label).toBe('Good')
  })

  it('labels high scores as Excellent', () => {
    expect(getStrengthInfo(100).label).toBe('Excellent')
  })
})

describe('checkRequirements', () => {
  it('marks all requirements unmet for a weak password', () => {
    const reqs = checkRequirements('abc')
    expect(reqs.every(r => !r.met)).toBe(true)
  })

  it('marks all requirements met for a strong password', () => {
    const reqs = checkRequirements('Abcdefgh123!@#')
    expect(reqs.every(r => r.met)).toBe(true)
  })
})

describe('PasswordStrengthMeter', () => {
  it('renders nothing for an empty password', () => {
    const { container } = render(<PasswordStrengthMeter password="" />)
    expect(container).toBeEmptyDOMElement()
  })

  it('renders the strength label for a non-empty password', () => {
    render(<PasswordStrengthMeter password="abc" />)
    expect(screen.getByText('Password strength')).toBeInTheDocument()
    expect(screen.getByText('Weak')).toBeInTheDocument()
  })

  it('merges a custom className on the wrapper', () => {
    const { container } = render(
      <PasswordStrengthMeter password="abc" className="custom-class" />
    )
    expect(container.firstChild).toHaveClass('custom-class')
  })

  it('omits the requirements checklist when showRequirements is false', () => {
    render(<PasswordStrengthMeter password="abc" showRequirements={false} />)
    expect(screen.queryByText('At least 12 characters')).not.toBeInTheDocument()
  })

  it('shows the requirements checklist by default', () => {
    render(<PasswordStrengthMeter password="abc" />)
    expect(screen.getByText('At least 12 characters')).toBeInTheDocument()
  })
})
