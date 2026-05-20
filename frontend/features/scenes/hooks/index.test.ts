import { describe, it, expect } from 'vitest'
import * as hooks from './index'

// Low-cost guard against accidental rename/delete in the barrel: assert each
// named hook re-export resolves to a callable function.
describe('scenes hooks barrel', () => {
  it('re-exports the scenes hooks', () => {
    expect(typeof hooks.useScenes).toBe('function')
    expect(typeof hooks.useSceneDetail).toBe('function')
    expect(typeof hooks.useSceneArtists).toBe('function')
    expect(typeof hooks.useSceneGenres).toBe('function')
    expect(typeof hooks.useSceneGraph).toBe('function')
  })
})
