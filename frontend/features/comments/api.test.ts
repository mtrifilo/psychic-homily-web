import { describe, it, expect } from 'vitest'
import { API_BASE_URL } from '@/lib/api-base'
import {
  commentEndpoints,
  commentPreferencesEndpoints,
  fieldNoteEndpoints,
  commentQueryKeys,
  fieldNoteQueryKeys,
} from './api'

describe('commentEndpoints', () => {
  it('builds polymorphic entity comment endpoints from type + id', () => {
    expect(commentEndpoints.LIST('artist', 42)).toBe(
      `${API_BASE_URL}/entities/artist/42/comments`
    )
    expect(commentEndpoints.CREATE('venue', 7)).toBe(
      `${API_BASE_URL}/entities/venue/7/comments`
    )
  })

  it('builds comment-scoped endpoints from a comment id', () => {
    expect(commentEndpoints.REPLY(5)).toBe(
      `${API_BASE_URL}/comments/5/replies`
    )
    expect(commentEndpoints.UPDATE(5)).toBe(`${API_BASE_URL}/comments/5`)
    expect(commentEndpoints.DELETE(5)).toBe(`${API_BASE_URL}/comments/5`)
    expect(commentEndpoints.VOTE(5)).toBe(`${API_BASE_URL}/comments/5/vote`)
    expect(commentEndpoints.THREAD(5)).toBe(`${API_BASE_URL}/comments/5/thread`)
    expect(commentEndpoints.REPLY_PERMISSION(5)).toBe(
      `${API_BASE_URL}/comments/5/reply-permission`
    )
  })
})

describe('commentPreferencesEndpoints', () => {
  it('exposes the default-reply-permission preference endpoint', () => {
    expect(commentPreferencesEndpoints.DEFAULT_REPLY_PERMISSION).toBe(
      `${API_BASE_URL}/auth/preferences/default-reply-permission`
    )
  })
})

describe('fieldNoteEndpoints', () => {
  it('builds show-scoped field-note endpoints from a show id', () => {
    expect(fieldNoteEndpoints.LIST(11)).toBe(
      `${API_BASE_URL}/shows/11/field-notes`
    )
    expect(fieldNoteEndpoints.CREATE(11)).toBe(
      `${API_BASE_URL}/shows/11/field-notes`
    )
  })

  // PSY-1590. The rollup hangs off a VENUE even though the rows it returns are
  // show-scoped, so the id in this path is a venue id and reads as a show id at
  // a glance — worth pinning.
  it('builds the venue rollup endpoint from a venue id', () => {
    expect(fieldNoteEndpoints.LIST_FOR_VENUE(7)).toBe(
      `${API_BASE_URL}/venues/7/field-notes`
    )
  })
})

describe('commentQueryKeys', () => {
  it('uses a stable root key for cache invalidation', () => {
    expect(commentQueryKeys.all).toEqual(['comments'])
  })

  it('scopes the entity key by both the type and the numeric id', () => {
    expect(commentQueryKeys.entity('artist', 42)).toEqual([
      'comments',
      'artist',
      42,
    ])
  })

  it('namespaces the thread key under the comment id', () => {
    expect(commentQueryKeys.thread(5)).toEqual(['comments', 'thread', 5])
  })
})

describe('fieldNoteQueryKeys', () => {
  it('uses a stable root key for cache invalidation', () => {
    expect(fieldNoteQueryKeys.all).toEqual(['field-notes'])
  })

  it('scopes the show key by the numeric show id', () => {
    expect(fieldNoteQueryKeys.show(11)).toEqual(['field-notes', 11])
  })

  // PSY-1590/PSY-1698: the venue key carries the LIMIT as well as the id, so a
  // surface asking for a page of notes cannot be answered by the Atlas
  // teaser's one-note entry (or vice versa) depending on which landed first.
  it('scopes the venue key by both the venue id and the page size', () => {
    expect(fieldNoteQueryKeys.venue(7, 1)).toEqual([
      'field-notes',
      'venue',
      7,
      1,
    ])
    expect(fieldNoteQueryKeys.venue(7, 25)).not.toEqual(
      fieldNoteQueryKeys.venue(7, 1)
    )
  })

  // The venue key must not collide with a show key that happens to share the
  // number — different questions, different entities.
  it('does not collide with the show key for the same numeric id', () => {
    expect(fieldNoteQueryKeys.venue(11, 1)).not.toEqual(
      fieldNoteQueryKeys.show(11)
    )
  })
})
