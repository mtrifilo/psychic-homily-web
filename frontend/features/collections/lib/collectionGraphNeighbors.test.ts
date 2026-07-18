import { describe, it, expect } from 'vitest'
import {
  collectionGraphNeighbors,
  formatArtistNameList,
  indexCollectionNodes,
  SHOW_ARTIST_EDGE_TYPES,
  VENUE_ARTIST_EDGE_TYPES,
} from '../lib/collectionGraphNeighbors'
import type {
  CollectionGraphLink,
  CollectionGraphNode,
} from '../types'

const nodes: CollectionGraphNode[] = [
  {
    id: 1,
    entity_type: 'artist',
    name: 'Diners',
    slug: 'diners',
    upcoming_show_count: 0,
    is_isolate: false,
  },
  {
    id: 2,
    entity_type: 'artist',
    name: 'Glass Heels',
    slug: 'glass-heels',
    upcoming_show_count: 0,
    is_isolate: false,
  },
  {
    id: 10,
    entity_type: 'venue',
    name: 'Valley Bar',
    slug: 'valley-bar',
    upcoming_show_count: 0,
    is_isolate: false,
  },
  {
    id: 20,
    entity_type: 'show',
    name: 'Fri Jul 24',
    slug: 'fri-jul-24',
    upcoming_show_count: 0,
    is_isolate: false,
  },
]

const links: CollectionGraphLink[] = [
  { source_id: 1, target_id: 10, type: 'played_at', score: 1 },
  { source_id: 2, target_id: 10, type: 'played_at', score: 1 },
  { source_id: 20, target_id: 1, type: 'show_lineup', score: 1 },
  { source_id: 20, target_id: 2, type: 'show_lineup', score: 1 },
]

describe('collectionGraphNeighbors', () => {
  it('finds artists that played a venue via played_at', () => {
    const byId = indexCollectionNodes(nodes)
    const artists = collectionGraphNeighbors(
      10,
      links,
      byId,
      VENUE_ARTIST_EDGE_TYPES,
      'artist',
    )
    expect(artists.map(a => a.name)).toEqual(['Diners', 'Glass Heels'])
  })

  it('finds bill artists for a show via show_lineup', () => {
    const byId = indexCollectionNodes(nodes)
    const artists = collectionGraphNeighbors(
      20,
      links,
      byId,
      SHOW_ARTIST_EDGE_TYPES,
      'artist',
    )
    expect(artists).toHaveLength(2)
  })

  it('formats truncated name lists', () => {
    expect(
      formatArtistNameList(nodes.filter(n => n.entity_type === 'artist'), {
        max: 1,
      }),
    ).toBe('Diners +1')
  })
})
