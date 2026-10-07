import { expect } from 'vitest'
import { COARSE_POINTER_HIT_AREA_CLASS } from '@/components/shared/touchTarget'

/**
 * Asserts that the element's only coarse-pointer classes are the shared hit
 * area, so a phone gets the 24px target and nothing else that could resize or
 * restyle the control.
 */
export function expectOnlyCoarseHitArea(element: Element) {
  const coarseOnly = [...element.classList].filter(token =>
    token.startsWith('pointer-coarse:')
  )
  expect(coarseOnly.sort()).toEqual(
    COARSE_POINTER_HIT_AREA_CLASS.split(' ').sort()
  )
}
