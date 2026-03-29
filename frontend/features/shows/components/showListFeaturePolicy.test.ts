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

  it('matches the expected policy snapshot', () => {
    expect(SHOW_LIST_FEATURE_POLICY).toMatchInlineSnapshot(`
      {
        "context": {
          "showAdminActions": false,
          "showDetailsLink": true,
          "showExpandMusic": false,
          "showOwnerActions": false,
          "showSaveButton": false,
          "useCompactLayout": true,
        },
        "discovery": {
          "showAdminActions": true,
          "showDetailsLink": true,
          "showExpandMusic": true,
          "showOwnerActions": true,
          "showSaveButton": true,
          "useCompactLayout": false,
        },
        "ownership": {
          "showAdminActions": true,
          "showDetailsLink": true,
          "showExpandMusic": false,
          "showOwnerActions": true,
          "showSaveButton": true,
          "useCompactLayout": false,
        },
      }
    `)
  })

  it('every policy has the same set of feature flags', () => {
    const expectedKeys = [
      'showDetailsLink',
      'showSaveButton',
      'showExpandMusic',
      'showAdminActions',
      'showOwnerActions',
      'useCompactLayout',
    ]
    for (const context of Object.keys(SHOW_LIST_FEATURE_POLICY)) {
      expect(Object.keys(SHOW_LIST_FEATURE_POLICY[context as keyof typeof SHOW_LIST_FEATURE_POLICY]).sort())
        .toEqual([...expectedKeys].sort())
    }
  })
})
