import { describe, it, expect } from 'vitest'
import * as hooks from './index'

// Low-cost guard against accidental rename/delete in the barrel: assert each
// named hook re-export resolves to a callable function. Type-only re-exports
// (EntityAttribution) has no runtime presence and is not asserted.
describe('contributions hooks barrel', () => {
  it('re-exports the contribution hooks', () => {
    expect(typeof hooks.useSuggestEdit).toBe('function')
    expect(typeof hooks.useShowEdit).toBe('function')
    expect(typeof hooks.useEntityAttribution).toBe('function')
    expect(typeof hooks.useReportEntity).toBe('function')
    expect(typeof hooks.useContributeOpportunities).toBe('function')
    expect(typeof hooks.useContributeCategory).toBe('function')
    expect(typeof hooks.useEntitySaveSuccessBanner).toBe('function')
    expect(typeof hooks.useMyPendingEdits).toBe('function')
    expect(typeof hooks.useCancelPendingEdit).toBe('function')
  })
})
