import { describe, it, expect } from 'vitest'
import * as hooks from './index'

// Low-cost guard against accidental rename/delete in the barrel: assert each
// named hook re-export resolves to a callable function. Type-only re-exports
// (CreateLabelInput, UpdateLabelInput) have no runtime presence and are not
// asserted.
describe('labels hooks barrel', () => {
  it('re-exports the read hooks', () => {
    expect(typeof hooks.useLabels).toBe('function')
    expect(typeof hooks.useLabel).toBe('function')
    expect(typeof hooks.useArtistLabels).toBe('function')
    expect(typeof hooks.useLabelRoster).toBe('function')
    expect(typeof hooks.useLabelCatalog).toBe('function')
  })

  it('re-exports the label search hook', () => {
    expect(typeof hooks.useLabelSearch).toBe('function')
  })

  it('re-exports the admin mutation hooks', () => {
    expect(typeof hooks.useCreateLabel).toBe('function')
    expect(typeof hooks.useUpdateLabel).toBe('function')
    expect(typeof hooks.useDeleteLabel).toBe('function')
  })
})
