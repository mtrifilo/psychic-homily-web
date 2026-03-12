import { describe, it, expect } from 'vitest'
import { SHOW_LIST_FEATURE_POLICY } from './showListFeaturePolicy'

describe('SHOW_LIST_FEATURE_POLICY', () => {
  it('defines three contexts: discovery, ownership, context', () => {
    expect(Object.keys(SHOW_LIST_FEATURE_POLICY).sort()).toEqual([
      'context',
      'discovery',
      'ownership',
    ])
  })

  describe('discovery context', () => {
    const policy = SHOW_LIST_FEATURE_POLICY.discovery

    it('enables details link', () => {
      expect(policy.showDetailsLink).toBe(true)
    })

    it('enables save button', () => {
      expect(policy.showSaveButton).toBe(true)
    })

    it('enables expand music', () => {
      expect(policy.showExpandMusic).toBe(true)
    })

    it('enables admin actions', () => {
      expect(policy.showAdminActions).toBe(true)
    })

    it('enables owner actions', () => {
      expect(policy.showOwnerActions).toBe(true)
    })

    it('does not use compact layout', () => {
      expect(policy.useCompactLayout).toBe(false)
    })
  })

  describe('ownership context', () => {
    const policy = SHOW_LIST_FEATURE_POLICY.ownership

    it('enables details link', () => {
      expect(policy.showDetailsLink).toBe(true)
    })

    it('enables save button', () => {
      expect(policy.showSaveButton).toBe(true)
    })

    it('disables expand music', () => {
      expect(policy.showExpandMusic).toBe(false)
    })

    it('enables admin actions', () => {
      expect(policy.showAdminActions).toBe(true)
    })

    it('enables owner actions', () => {
      expect(policy.showOwnerActions).toBe(true)
    })

    it('does not use compact layout', () => {
      expect(policy.useCompactLayout).toBe(false)
    })
  })

  describe('context context', () => {
    const policy = SHOW_LIST_FEATURE_POLICY.context

    it('enables details link', () => {
      expect(policy.showDetailsLink).toBe(true)
    })

    it('disables save button', () => {
      expect(policy.showSaveButton).toBe(false)
    })

    it('disables expand music', () => {
      expect(policy.showExpandMusic).toBe(false)
    })

    it('disables admin actions', () => {
      expect(policy.showAdminActions).toBe(false)
    })

    it('disables owner actions', () => {
      expect(policy.showOwnerActions).toBe(false)
    })

    it('uses compact layout', () => {
      expect(policy.useCompactLayout).toBe(true)
    })
  })
})
