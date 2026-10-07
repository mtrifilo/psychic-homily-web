'use client'

import { useEffect } from 'react'
import { armAtlasMapReadyFallbacks } from '@/lib/atlasMapReady'

/**
 * Arms the Atlas-ready signal's cap and first-input release for as long as the
 * Atlas page is mounted (lib/atlasMapReady.ts). Renders nothing.
 */
export function AtlasMapReadyFallbacks() {
  useEffect(() => armAtlasMapReadyFallbacks(), [])
  return null
}
