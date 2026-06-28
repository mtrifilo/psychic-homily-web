import { describe, it, expect } from 'vitest'
import type {
  SceneListItem,
  SceneListResponse,
  SceneStats,
  ScenePulse,
  SceneDetail,
  SceneArtist,
  SceneArtistsResponse,
  GenreCount,
  SceneGenreResponse,
  SceneGraphInfo,
  SceneGraphCluster,
  SceneGraphNode,
  SceneGraphLink,
  SceneGraphResponse,
} from './types'

// PSY-690: types.ts is interface-only (no runtime exports), mirroring the
// backend scene_service.go response shapes. There's no behavior to exercise,
// so these are compile-time contract guards: each fixture must satisfy its
// interface for `tsc --noEmit` (run by `bun run typecheck`) to pass, which
// catches accidental field renames/removals. The runtime expects double as a
// readable record of the expected shape and exercise the optional/nullable
// fields (description, node city/state, link detail).

describe('scene types contract', () => {
  it('SceneListItem / SceneListResponse', () => {
    const item: SceneListItem = {
      city: 'Phoenix',
      state: 'AZ',
      slug: 'phoenix-az',
      venue_count: 12,
      upcoming_show_count: 45,
      total_show_count: 200,
    }
    const response: SceneListResponse = { scenes: [item], count: 1 }

    expect(response.count).toBe(1)
    expect(response.scenes[0].slug).toBe('phoenix-az')
  })

  it('SceneStats / ScenePulse / SceneDetail (nullable description)', () => {
    const stats: SceneStats = {
      venue_count: 12,
      artist_count: 85,
      upcoming_show_count: 45,
      festival_count: 2,
    }
    const pulse: ScenePulse = {
      shows_this_month: 30,
      shows_prev_month: 25,
      shows_trend: 5,
      new_artists_30d: 8,
      active_venues_this_month: 10,
      shows_by_month: [20, 22, 25, 28, 30, 30],
    }
    const detail: SceneDetail = {
      city: 'Phoenix',
      state: 'AZ',
      slug: 'phoenix-az',
      description: null,
      stats,
      pulse,
    }

    // description is explicitly nullable.
    expect(detail.description).toBeNull()
    const described: SceneDetail = { ...detail, description: 'A desert scene.' }
    expect(described.description).toBe('A desert scene.')
    expect(detail.pulse.shows_by_month).toHaveLength(6)
  })

  it('SceneArtist / SceneArtistsResponse', () => {
    const artist: SceneArtist = {
      id: 1,
      slug: 'gatecreeper',
      name: 'Gatecreeper',
      city: 'Phoenix',
      state: 'AZ',
      show_count: 5,
      is_active: true,
    }
    const response: SceneArtistsResponse = { artists: [artist], total: 1 }

    expect(response.total).toBe(1)
    expect(response.artists[0].show_count).toBe(5)
  })

  it('GenreCount / SceneGenreResponse', () => {
    const genre: GenreCount = { tag_id: 1, name: 'punk', slug: 'punk', count: 12 }
    const response: SceneGenreResponse = {
      genres: [genre],
      diversity_index: 0.8,
      diversity_label: 'High diversity',
    }

    expect(response.diversity_index).toBeCloseTo(0.8)
    expect(response.genres[0].slug).toBe('punk')
  })

  it('SceneGraphInfo / SceneGraphCluster', () => {
    const info: SceneGraphInfo = {
      slug: 'phoenix-az',
      city: 'Phoenix',
      state: 'AZ',
      artist_count: 12,
      edge_count: 4,
    }
    const cluster: SceneGraphCluster = {
      id: 'v_1',
      label: 'Valley Bar',
      size: 6,
      color_index: 0,
    }
    // "other" cluster uses color_index -1 per the field doc.
    const otherCluster: SceneGraphCluster = {
      id: 'other',
      label: 'Other',
      size: 3,
      color_index: -1,
    }

    expect(info.edge_count).toBe(4)
    expect(cluster.color_index).toBe(0)
    expect(otherCluster.color_index).toBe(-1)
  })

  it('SceneGraphNode (optional city/state) / SceneGraphLink (optional detail)', () => {
    const node: SceneGraphNode = {
      id: 1,
      name: 'Gatecreeper',
      slug: 'gatecreeper',
      upcoming_show_count: 0,
      cluster_id: 'v_1',
      is_isolate: false,
    }
    // city/state are optional.
    const locatedNode: SceneGraphNode = { ...node, city: 'Phoenix', state: 'AZ' }

    const link: SceneGraphLink = {
      source_id: 1,
      target_id: 2,
      type: 'shared_bills',
      score: 0.5,
      is_cross_cluster: false,
    }
    // detail is optional and carries arbitrary metadata.
    const detailedLink: SceneGraphLink = {
      ...link,
      detail: { shared_show_count: 3 },
    }

    expect(node.city).toBeUndefined()
    expect(locatedNode.state).toBe('AZ')
    expect(link.detail).toBeUndefined()
    expect(detailedLink.detail).toEqual({ shared_show_count: 3 })
  })

  it('SceneGraphResponse composes the graph payload', () => {
    const response: SceneGraphResponse = {
      scene: {
        slug: 'phoenix-az',
        city: 'Phoenix',
        state: 'AZ',
        artist_count: 2,
        edge_count: 1,
      },
      clusters: [{ id: 'v_1', label: 'Valley Bar', size: 2, color_index: 0 }],
      nodes: [
        {
          id: 1,
          name: 'Gatecreeper',
          slug: 'gatecreeper',
          upcoming_show_count: 0,
          cluster_id: 'v_1',
          is_isolate: false,
        },
        {
          id: 2,
          name: 'Sundressed',
          slug: 'sundressed',
          upcoming_show_count: 1,
          cluster_id: 'v_1',
          is_isolate: false,
        },
      ],
      links: [
        {
          source_id: 1,
          target_id: 2,
          type: 'shared_bills',
          score: 0.5,
          is_cross_cluster: false,
        },
      ],
    }

    expect(response.nodes).toHaveLength(2)
    expect(response.links).toHaveLength(1)
    expect(response.clusters[0].id).toBe('v_1')
    expect(response.scene.artist_count).toBe(2)
  })
})
