'use client'

import { FeaturedAdmin } from '@/features/admin/components/FeaturedAdmin'

/**
 * /admin/featured — admin curation surface for the /explore landing's
 * editorial slots. Sits under the admin layout, which delegates auth +
 * IsAdmin gating to `<AdminGuard>` so this page only renders for
 * authenticated admins.
 *
 * Implementation lives at frontend/features/admin/components/FeaturedAdmin.
 */
export default function AdminFeaturedPage() {
  return <FeaturedAdmin />
}
