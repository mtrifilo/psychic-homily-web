'use client'

import { HomeShowListView } from './HomeShowListView'
import { useHomeShowCitySelection } from '../hooks/useHomeShowCitySelection'

/**
 * The home page's upcoming-shows list, owning its own city selection.
 *
 * The selection and the rendering live in `useHomeShowCitySelection` and
 * `HomeShowListView` (PSY-2103) so the signed-in home can render the same list
 * beneath a header that names the same city. Callers that need that pairing use
 * those two directly; everyone else uses this.
 */
export function HomeShowList() {
  const selection = useHomeShowCitySelection()
  return <HomeShowListView selection={selection} />
}
