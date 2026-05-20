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
})
