'use client'

/**
 * /settings: the settings hub, one page of sections reached by fragment
 * (`/settings#account`). The anchors are declared in features/settings/sections.
 */

import { Loader2 } from 'lucide-react'
import { useAuthRouteGuard } from '@/lib/hooks/common/useAuthRouteGuard'
import { SettingsHub } from '@/features/settings/components/SettingsHub'

export default function SettingsPage() {
  const gate = useAuthRouteGuard('redirect')

  // Only 'loading' and 'ready' reach here: 'redirect' mode throws.
  if (gate !== 'ready') {
    return (
      <div className="flex min-h-[60vh] items-center justify-center">
        <Loader2 className="h-8 w-8 animate-spin text-muted-foreground" />
      </div>
    )
  }

  return <SettingsHub />
}
