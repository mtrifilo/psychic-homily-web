import { describe, it, expect } from 'vitest'
import * as hooks from './index'

// Low-cost guard against accidental rename/delete in the barrel: assert each
// named hook re-export resolves to a callable function.
describe('community hooks barrel', () => {
  it('re-exports the leaderboard hook', () => {
    expect(typeof hooks.useLeaderboard).toBe('function')
  })
})
