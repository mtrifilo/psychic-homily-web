import { describe, it, expect } from 'vitest'
import { gapLineCopy } from './sceneGapLine'

describe('gapLineCopy', () => {
  it('names the count, the city and the listen-link gap', () => {
    expect(gapLineCopy(11, 'Phoenix')).toBe(
      '11 Phoenix bands have no listen link → Help finish Phoenix'
    )
  })

  it('reads as a singular sentence at one', () => {
    expect(gapLineCopy(1, 'Tucson')).toBe(
      '1 Tucson band has no listen link → Help finish Tucson'
    )
  })

  it('uses no em dashes', () => {
    expect(gapLineCopy(11, 'Phoenix')).not.toContain('—')
  })
})
