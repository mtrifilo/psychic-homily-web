import { describe, it, expect } from 'vitest'
import {
  buildExpandAnnouncement,
  buildCollapseAnnouncement,
  buildExpandErrorAnnouncement,
  buildCollapseAllAnnouncement,
  buildFilterAnnouncement,
} from './graphAnnouncements'

describe('graphAnnouncements', () => {
  it('expand: pluralizes and names the count + node', () => {
    expect(buildExpandAnnouncement('Dehd', 4)).toBe('Added 4 artists connected to Dehd.')
    expect(buildExpandAnnouncement('Dehd', 1)).toBe('Added 1 artist connected to Dehd.')
  })

  it('expand: distinct copy when the fetch revealed nothing new', () => {
    expect(buildExpandAnnouncement('Dehd', 0)).toBe('No new artists connected to Dehd.')
  })

  it('collapse: names the node', () => {
    expect(buildCollapseAnnouncement('Dehd')).toBe('Collapsed the connections under Dehd.')
  })

  it('expand error: names the node + prompts retry', () => {
    expect(buildExpandErrorAnnouncement('Dehd')).toBe("Couldn't load connections for Dehd. Try again.")
  })

  it('collapse-all: names the bulk reset', () => {
    expect(buildCollapseAllAnnouncement()).toBe('Collapsed all expansions back to the starting graph.')
  })

  it('filter: reflects shown/hidden', () => {
    expect(buildFilterAnnouncement('Shared bills', true)).toBe('Shared bills connections shown.')
    expect(buildFilterAnnouncement('Shared bills', false)).toBe('Shared bills connections hidden.')
  })
})
