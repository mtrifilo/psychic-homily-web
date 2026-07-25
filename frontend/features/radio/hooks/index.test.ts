import { describe, it, expect } from 'vitest'
import * as hooks from './index'

// Low-cost guard against accidental rename/delete in the barrel: assert each
// named hook re-export resolves to a callable function.
describe('radio hooks barrel', () => {
  it('re-exports the radio hooks', () => {
    expect(typeof hooks.useRadioStations).toBe('function')
    expect(typeof hooks.useRadioStation).toBe('function')
    expect(typeof hooks.useRadioShows).toBe('function')
    expect(typeof hooks.useRadioShow).toBe('function')
    expect(typeof hooks.useRadioEpisodes).toBe('function')
    expect(typeof hooks.useRadioEpisode).toBe('function')
    expect(typeof hooks.useRadioTopArtists).toBe('function')
    expect(typeof hooks.useRadioTopLabels).toBe('function')
    expect(typeof hooks.useArtistRadioPlays).toBe('function')
    expect(typeof hooks.useReleaseRadioPlays).toBe('function')
    expect(typeof hooks.useNewReleaseRadar).toBe('function')
    expect(typeof hooks.useRadioStats).toBe('function')
  })
})
