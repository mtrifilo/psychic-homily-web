import { describe, it, expect } from 'vitest'
import { isStationVisibleOnIndex } from './types'
import type { RadioNetworkInfo } from './types'

// PSY-673: filter used by /radio (RadioHub) to hide non-flagship siblings of
// any network. Sub-streams remain reachable by direct URL and via the
// flagship's tab bar (PSY-674); the index just stops listing them as
// standalone broadcasters.
describe('isStationVisibleOnIndex', () => {
  it('shows network-less stations (KEXP, NTS today)', () => {
    expect(isStationVisibleOnIndex({ network: null })).toBe(true)
  })

  it('shows the flagship of a network (WFMU 91.1)', () => {
    const network: RadioNetworkInfo = {
      slug: 'wfmu',
      name: 'WFMU',
      is_flagship: true,
    }
    expect(isStationVisibleOnIndex({ network })).toBe(true)
  })

  it('hides non-flagship siblings (Drummer / Rock\'n\'Soul / Sheena\'s)', () => {
    const network: RadioNetworkInfo = {
      slug: 'wfmu',
      name: 'WFMU',
      is_flagship: false,
    }
    expect(isStationVisibleOnIndex({ network })).toBe(false)
  })

  // Guards against a regression where the function returns truthy for any
  // non-null network instead of specifically checking is_flagship.
  it('does not depend on network slug or name, only is_flagship', () => {
    const flagshipOfDifferentNetwork: RadioNetworkInfo = {
      slug: 'somafm',
      name: 'SomaFM',
      is_flagship: true,
    }
    expect(isStationVisibleOnIndex({ network: flagshipOfDifferentNetwork })).toBe(true)

    const siblingOfDifferentNetwork: RadioNetworkInfo = {
      slug: 'somafm',
      name: 'SomaFM',
      is_flagship: false,
    }
    expect(isStationVisibleOnIndex({ network: siblingOfDifferentNetwork })).toBe(false)
  })
})
