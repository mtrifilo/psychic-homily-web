export type ShowListContext = 'discovery' | 'ownership' | 'context'

export interface ShowListFeaturePolicy {
  showDetailsLink: boolean
  showSaveButton: boolean
  showExpandMusic: boolean
  showAdminActions: boolean
  showOwnerActions: boolean
  useCompactLayout: boolean
}

/**
 * Feature policy for show-list contexts:
 * - discovery: home + /shows
 * - ownership: /library saved shows + /contribute/submissions owner console
 * - context: compact embedded rows (the venues directory's `VenueCard`)
 *
 * The artist and venue pages' show tables do not read this policy: their
 * upcoming rows carry Save through the tables' own `saveColumn` prop, and their
 * past rows carry none.
 */
export const SHOW_LIST_FEATURE_POLICY: Record<
  ShowListContext,
  ShowListFeaturePolicy
> = {
  discovery: {
    showDetailsLink: true,
    showSaveButton: true,
    showExpandMusic: true,
    showAdminActions: true,
    showOwnerActions: true,
    useCompactLayout: false,
  },
  ownership: {
    showDetailsLink: true,
    showSaveButton: true,
    showExpandMusic: false,
    showAdminActions: true,
    showOwnerActions: true,
    useCompactLayout: false,
  },
  context: {
    showDetailsLink: true,
    showSaveButton: false,
    showExpandMusic: false,
    showAdminActions: false,
    showOwnerActions: false,
    useCompactLayout: true,
  },
}
