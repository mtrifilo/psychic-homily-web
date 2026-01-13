import { redirect } from 'next/navigation'

/**
 * Redirect old /shows/saved URL to new /collection page
 * This maintains backwards compatibility for bookmarked URLs
 */
export default function SavedShowsRedirect({
  searchParams,
}: {
  searchParams: { [key: string]: string | string[] | undefined }
}) {
  // Preserve query params when redirecting (e.g., ?submitted=pending)
  const params = new URLSearchParams()
  for (const [key, value] of Object.entries(searchParams)) {
    if (typeof value === 'string') {
      params.set(key, value)
    } else if (Array.isArray(value)) {
      value.forEach(v => params.append(key, v))
    }
  }

  const queryString = params.toString()
  const destination = queryString ? `/collection?${queryString}` : '/collection'

  redirect(destination)
}
