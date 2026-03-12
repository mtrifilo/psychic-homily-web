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
 * - ownership: /collection (saved + submissions)
 * - context: artist/venue/favorite-venues embedded lists
 */
export const SHOW_LIST_FEATURE_POLICY: Record<ShowListContext, ShowListFeaturePolicy> = {
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
