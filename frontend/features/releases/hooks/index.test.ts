import { describe, it, expect } from 'vitest'
import * as hooks from './index'

// Low-cost guard against accidental rename/delete in the barrel: assert each
// named hook re-export resolves to a callable function. Type-only re-exports
// (CreateReleaseArtistInput, CreateReleaseLinkInput, CreateReleaseInput,
// UpdateReleaseInput) have no runtime presence and are not asserted.
describe('releases hooks barrel', () => {
  it('re-exports the read hooks', () => {
    expect(typeof hooks.useReleases).toBe('function')
    expect(typeof hooks.useRelease).toBe('function')
    expect(typeof hooks.useArtistReleases).toBe('function')
    expect(typeof hooks.useSavedReleases).toBe('function')
    expect(typeof hooks.useReleaseSaveCount).toBe('function')
    expect(typeof hooks.useReleaseSaveCountBatch).toBe('function')
    expect(typeof hooks.useReleaseSaveToggle).toBe('function')
  })

  it('re-exports the admin mutation hooks', () => {
    expect(typeof hooks.useCreateRelease).toBe('function')
    expect(typeof hooks.useUpdateRelease).toBe('function')
    expect(typeof hooks.useDeleteRelease).toBe('function')
    expect(typeof hooks.useAddReleaseLink).toBe('function')
    expect(typeof hooks.useRemoveReleaseLink).toBe('function')
  })
})
