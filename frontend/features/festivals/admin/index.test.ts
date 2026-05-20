import { describe, it, expect } from 'vitest'
import * as admin from './index'

// admin/index.ts is the barrel for the admin festival management surface.
describe('festivals admin barrel', () => {
  it('re-exports the FestivalManagement component', () => {
    expect(typeof admin.FestivalManagement).toBe('function')
  })
})
