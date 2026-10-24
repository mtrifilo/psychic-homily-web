import { describe, expect, it } from 'vitest'
import { unpadNumericId } from './unpadNumericId'

describe('unpadNumericId', () => {
  it('drops the leading zeros of an all-digit id', () => {
    expect(unpadNumericId('0042')).toBe('42')
    expect(unpadNumericId('42')).toBe('42')
  })

  it('keeps one digit of an all-zero id', () => {
    expect(unpadNumericId('0')).toBe('0')
    expect(unpadNumericId('000')).toBe('0')
  })

  it('passes anything that is not all digits through unchanged', () => {
    expect(unpadNumericId('0042abc')).toBe('0042abc')
    expect(unpadNumericId('the-rebel-lounge')).toBe('the-rebel-lounge')
    expect(unpadNumericId('')).toBe('')
  })
})
