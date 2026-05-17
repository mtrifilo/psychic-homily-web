import { describe, it, expect } from 'vitest'
import { isStationVisibleOnIndex, getStationDetailUrl } from './types'
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

// PSY-674: URL builder used by NetworkTabBar + cards to construct canonical
// /radio detail URLs. Sub-streams live under /radio/{network}/{local-slug};
// flagship + network-less stations stay at /radio/{slug}.
describe('getStationDetailUrl', () => {
  it('network-less stations use /radio/{slug}', () => {
    expect(getStationDetailUrl('kexp', null)).toBe('/radio/kexp')
    expect(getStationDetailUrl('nts-radio', null)).toBe('/radio/nts-radio')
  })

  it('flagship stations stay at /radio/{slug} (no network segment)', () => {
    const network = { slug: 'wfmu', is_flagship: true }
    expect(getStationDetailUrl('wfmu', network)).toBe('/radio/wfmu')
  })

  it('sub-streams use /radio/{network}/channel/{local-slug} with prefix stripped', () => {
    const network = { slug: 'wfmu', is_flagship: false }
    expect(getStationDetailUrl('wfmu-drummer', network)).toBe('/radio/wfmu/channel/drummer')
    expect(getStationDetailUrl('wfmu-rocknsoulradio', network)).toBe('/radio/wfmu/channel/rocknsoulradio')
    expect(getStationDetailUrl('wfmu-sheena', network)).toBe('/radio/wfmu/channel/sheena')
  })

  // Guards against a future station whose slug doesn't follow the
  // network-prefix convention. Better to ship an honest /radio/wfmu/channel/foo
  // URL than to silently produce /radio/wfmu/channel/ (empty local-slug).
  it('falls back to full slug when prefix is absent', () => {
    const network = { slug: 'wfmu', is_flagship: false }
    expect(getStationDetailUrl('legacy-show', network)).toBe('/radio/wfmu/channel/legacy-show')
  })
})
