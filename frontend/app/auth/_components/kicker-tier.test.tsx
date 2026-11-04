import { describe, it, expect } from 'vitest'
import { render, screen } from '@testing-library/react'
import { StepHeading } from './step-card'
import { SignupIntentPanel } from './signup-intent-panel'

// The auth kickers sit in the data-face kicker tier, a different typographic
// tier from field labels and body text. The capitals are CSS; the text stays
// sentence case.
const KICKER_TIER = [
  'font-data',
  'text-xs',
  'uppercase',
  'tracking-[0.66px]',
  'text-muted-foreground',
]

describe('auth kickers', () => {
  it('sets the step heading eyebrow in the kicker tier', () => {
    render(<StepHeading eyebrow="Step 2 of 3 · Verify" title="Check your inbox." />)

    expect(screen.getByText('Step 2 of 3 · Verify')).toHaveClass(...KICKER_TIER)
  })

  it('sets the sign-up intent kicker in the kicker tier', () => {
    render(<SignupIntentPanel />)

    expect(screen.getByText('Psychic Homily account')).toHaveClass(...KICKER_TIER)
  })
})
