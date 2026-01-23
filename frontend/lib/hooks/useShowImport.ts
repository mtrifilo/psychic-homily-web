'use client'

import { useMutation, useQueryClient } from '@tanstack/react-query'
import { apiRequest, API_ENDPOINTS } from '../api'
import { createInvalidateQueries } from '../queryClient'
import type { ShowResponse } from '../types/show'

/**
 * Types for show import functionality
 */

export interface ExportShowData {
  title: string
  event_date: string
  city?: string
  state?: string
  price?: number
  age_requirement?: string
  status: string
}

export interface VenueMatchResult {
  name: string
  city: string
  state: string
  existing_id?: number
  will_create: boolean
}

export interface ArtistMatchResult {
  name: string
  position: number
  set_type: string
  existing_id?: number
  will_create: boolean
}

export interface ImportPreviewResponse {
  show: ExportShowData
  venues: VenueMatchResult[]
  artists: ArtistMatchResult[]
  warnings: string[]
  can_import: boolean
}

interface ImportPreviewRequest {
  content: string // base64-encoded markdown content
}

interface ImportConfirmRequest {
  content: string // base64-encoded markdown content
}

/**
 * Hook for previewing a show import (admin only)
 */
export function useShowImportPreview() {
  return useMutation({
    mutationFn: async (content: string) => {
      // Convert content to base64
      const base64Content = btoa(content)
      const body: ImportPreviewRequest = { content: base64Content }
      return apiRequest<ImportPreviewResponse>(
        API_ENDPOINTS.ADMIN.SHOWS.IMPORT_PREVIEW,
        {
          method: 'POST',
          body: JSON.stringify(body),
        }
      )
    },
  })
}

/**
 * Hook for confirming a show import (admin only)
 */
export function useShowImportConfirm() {
  const queryClient = useQueryClient()
  const invalidateQueries = createInvalidateQueries(queryClient)

  return useMutation({
    mutationFn: async (content: string) => {
      // Convert content to base64
      const base64Content = btoa(content)
      const body: ImportConfirmRequest = { content: base64Content }
      return apiRequest<ShowResponse>(
        API_ENDPOINTS.ADMIN.SHOWS.IMPORT_CONFIRM,
        {
          method: 'POST',
          body: JSON.stringify(body),
        }
      )
    },
    onSuccess: () => {
      // Invalidate shows list since a new show was created
      invalidateQueries.shows()
    },
  })
}
