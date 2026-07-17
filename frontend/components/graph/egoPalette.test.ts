import { describe, it, expect } from 'vitest'
import {
  EGO_FAMILY_CHART_INDEX,
  EGO_FAMILY_PRIORITY,
  egoFamilyByNodeId,
  egoFamilyFill,
  egoFamilyFillCSS,
  egoFamilyForEdgeType,
  egoFamilyForTypes,
  egoLegendRows,
} from './egoPalette'
import { CHART_TOKEN_COUNT, type GraphPalette } from './graphPalette'

// PSY-1453 locked design "Option B": ego neighbor fills key to the
// relationship type of the connecting edge via the shared chart tokens.
// The mapping and the multi-type tie-break are the locked contract.

const palette: GraphPalette = {
  edges: {},
  unknownEdge: '#71717a',
  chart: ['#111111', '#222222', '#333333', '#444444', '#555555', '#666666', '#777777', '#888888'],
  otherCluster: '#94A3B8',
  labelText: '#eee7d9',
  labelHalo: '#0d0805',
  primary: '#e89960',
  mutedForeground: '#9c8c7c',
}

describe('locked family → chart-token mapping', () => {
  it('pins bills → chart-1, label → chart-6, members → chart-7, radio → chart-8', () => {
    expect(EGO_FAMILY_CHART_INDEX).toEqual({ bills: 0, label: 5, members: 6, radio: 7 })
  })

  it('every family index is a real chart token', () => {
    for (const family of EGO_FAMILY_PRIORITY) {
      const index = EGO_FAMILY_CHART_INDEX[family]
      expect(index).toBeGreaterThanOrEqual(0)
      expect(index).toBeLessThan(CHART_TOKEN_COUNT)
    }
  })

  it('maps each canonical edge type to its family (side_project rides with members)', () => {
    expect(egoFamilyForEdgeType('shared_bills')).toBe('bills')
    expect(egoFamilyForEdgeType('shared_label')).toBe('label')
    expect(egoFamilyForEdgeType('member_of')).toBe('members')
    expect(egoFamilyForEdgeType('side_project')).toBe('members')
    expect(egoFamilyForEdgeType('radio_cooccurrence')).toBe('radio')
  })

  it('maps out-of-family types (similar, festival_cobill, unknown) to null', () => {
    expect(egoFamilyForEdgeType('similar')).toBeNull()
    expect(egoFamilyForEdgeType('festival_cobill')).toBeNull()
    expect(egoFamilyForEdgeType('played_at')).toBeNull()
  })
})

describe('multi-type tie-break: bills > members > label > radio (locked)', () => {
  it('pins the priority order', () => {
    expect(EGO_FAMILY_PRIORITY).toEqual(['bills', 'members', 'label', 'radio'])
  })

  it('bills beats every other family', () => {
    expect(
      egoFamilyForTypes(['radio_cooccurrence', 'shared_label', 'member_of', 'shared_bills']),
    ).toBe('bills')
  })

  it('members beats label and radio', () => {
    expect(egoFamilyForTypes(['radio_cooccurrence', 'shared_label', 'member_of'])).toBe('members')
    expect(egoFamilyForTypes(['shared_label', 'side_project'])).toBe('members')
  })

  it('label beats radio', () => {
    expect(egoFamilyForTypes(['radio_cooccurrence', 'shared_label'])).toBe('label')
  })

  it('single-type and unmapped inputs resolve directly', () => {
    expect(egoFamilyForTypes(['radio_cooccurrence'])).toBe('radio')
    expect(egoFamilyForTypes(['similar'])).toBeNull()
    expect(egoFamilyForTypes([])).toBeNull()
  })

  it('unmapped types never influence the tie-break', () => {
    expect(egoFamilyForTypes(['similar', 'radio_cooccurrence', 'festival_cobill'])).toBe('radio')
  })
})

describe('egoFamilyFill / egoFamilyFillCSS', () => {
  it('resolves each family to its chart token color', () => {
    expect(egoFamilyFill(palette, 'bills')).toBe('#111111')
    expect(egoFamilyFill(palette, 'label')).toBe('#666666')
    expect(egoFamilyFill(palette, 'members')).toBe('#777777')
    expect(egoFamilyFill(palette, 'radio')).toBe('#888888')
  })

  it('falls back to the neutral other-cluster grey for null', () => {
    expect(egoFamilyFill(palette, null)).toBe('#94A3B8')
    expect(egoFamilyFillCSS(null)).toBe('#94A3B8')
  })

  it('emits theme-reactive var() expressions for DOM swatches', () => {
    expect(egoFamilyFillCSS('bills')).toBe('var(--chart-1, #e89960)')
    expect(egoFamilyFillCSS('radio')).toBe('var(--chart-8, #6db3a6)')
  })
})

describe('egoFamilyByNodeId — connecting-edge assignment', () => {
  const CENTER = 1

  it('colors 1-hop neighbors by their center edge (no hop map = bill graph)', () => {
    const out = egoFamilyByNodeId(
      [
        { sourceId: CENTER, targetId: 2, type: 'shared_bills' },
        { sourceId: 3, targetId: CENTER, type: 'radio_cooccurrence' },
      ],
      CENTER,
    )
    expect(out.get(2)).toBe('bills')
    expect(out.get(3)).toBe('radio')
  })

  it('same-hop cross-connections color neither endpoint', () => {
    const out = egoFamilyByNodeId(
      [
        { sourceId: CENTER, targetId: 2, type: 'radio_cooccurrence' },
        { sourceId: CENTER, targetId: 3, type: 'radio_cooccurrence' },
        // Cross edge between two hop-1 nodes: 2 stays radio, never bills.
        { sourceId: 2, targetId: 3, type: 'shared_bills' },
      ],
      CENTER,
    )
    expect(out.get(2)).toBe('radio')
    expect(out.get(3)).toBe('radio')
  })

  it('tie-breaks a neighbor connected by multiple center-edge types', () => {
    const out = egoFamilyByNodeId(
      [
        { sourceId: CENTER, targetId: 2, type: 'radio_cooccurrence' },
        { sourceId: CENTER, targetId: 2, type: 'member_of' },
      ],
      CENTER,
    )
    expect(out.get(2)).toBe('members')
  })

  it('colors hop-2 nodes by their edge to the hop-1 hub (expanded ego)', () => {
    const hopByNodeId = new Map<number, number>([
      [CENTER, 0],
      [2, 1],
      [4, 2],
    ])
    const out = egoFamilyByNodeId(
      [
        { sourceId: CENTER, targetId: 2, type: 'shared_bills' },
        { sourceId: 2, targetId: 4, type: 'shared_label' },
      ],
      CENTER,
      hopByNodeId,
    )
    expect(out.get(2)).toBe('bills')
    expect(out.get(4)).toBe('label')
  })

  it('a neighbor whose only connecting type is unmapped resolves to null', () => {
    const out = egoFamilyByNodeId(
      [{ sourceId: CENTER, targetId: 2, type: 'similar' }],
      CENTER,
    )
    expect(out.get(2)).toBeNull()
  })
})

describe('egoLegendRows', () => {
  it('renders one row per family present, in mocked display order', () => {
    const rows = egoLegendRows(['radio', 'bills', 'members'])
    expect(rows.map(r => r.key)).toEqual(['bills', 'members', 'radio'])
    expect(rows.map(r => r.label)).toEqual(['bills', 'members', 'radio'])
  })

  it('dedupes repeated families and appends a single neutral other row', () => {
    const rows = egoLegendRows(['bills', 'bills', null, null])
    expect(rows.map(r => r.key)).toEqual(['bills', 'other'])
    expect(rows[1].swatchCSS).toBe('#94A3B8')
  })

  it('returns no rows for an empty graph', () => {
    expect(egoLegendRows([])).toEqual([])
  })
})
