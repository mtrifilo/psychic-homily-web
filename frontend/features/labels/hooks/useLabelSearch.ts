'use client'

import { createSearchHook } from '@/lib/hooks/factories'
import { labelEndpoints, labelQueryKeys } from '@/features/labels/api'
import type { LabelsListResponse } from '../types'

/**
 * Hook for searching labels with debounced input.
 * Used for autocomplete in the labels browse page.
 */
export const useLabelSearch = createSearchHook<LabelsListResponse>(
  labelEndpoints.SEARCH,
  labelQueryKeys.search,
)
