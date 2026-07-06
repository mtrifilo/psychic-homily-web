import { describe, it, expect } from 'vitest'
import {
  buildExpandAnnouncement,
  buildCollapseAnnouncement,
  buildExpandErrorAnnouncement,
  buildCollapseAllAnnouncement,
  buildFilterAnnouncement,
  buildDepthAnnouncement,
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

  it('depth: pluralizes the added count at depth 2, and speaks the collapse at depth 1 (PSY-1303)', () => {
    expect(buildDepthAnnouncement(2, 5)).toBe('Showing 2 hops — added 5 artists from the top connections.')
    expect(buildDepthAnnouncement(2, 1)).toBe('Showing 2 hops — added 1 artist from the top connections.')
    expect(buildDepthAnnouncement(2, 0)).toBe('Showing 2 hops — no new artists to add.')
    expect(buildDepthAnnouncement(1, 0)).toBe('Back to 1 hop — collapsed the second-hop connections.')
  })
})
