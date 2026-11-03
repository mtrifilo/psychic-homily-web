import { describe, it, expect } from 'vitest'
import { createElement } from 'react'
import { tabularNumsIfDigitsOnly } from './tabularNums'

describe('tabularNumsIfDigitsOnly', () => {
  it('returns the class for digits-only strings and non-negative integers', () => {
    expect(tabularNumsIfDigitsOnly('2024')).toBe('tabular-nums')
    expect(tabularNumsIfDigitsOnly('0')).toBe('tabular-nums')
    expect(tabularNumsIfDigitsOnly(1204)).toBe('tabular-nums')
  })

  it('returns undefined for content holding punctuation, signs, spaces or prose', () => {
    for (const content of ['1,204', '91.1 MHz', 'Tue, Nov 3', '7:00 PM', '84.2%', '12 shows', '', ' 12']) {
      expect(tabularNumsIfDigitsOnly(content)).toBeUndefined()
    }
    expect(tabularNumsIfDigitsOnly(-3)).toBeUndefined()
    expect(tabularNumsIfDigitsOnly(1.5)).toBeUndefined()
  })

  it('returns undefined for nodes whose text it cannot see', () => {
    expect(tabularNumsIfDigitsOnly(createElement('span', null, '12'))).toBeUndefined()
    expect(tabularNumsIfDigitsOnly(null)).toBeUndefined()
    expect(tabularNumsIfDigitsOnly(undefined)).toBeUndefined()
  })
})
