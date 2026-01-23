import { redirect } from 'next/navigation'

/**
 * Redirect old /shows/saved URL to new /collection page
 * This maintains backwards compatibility for bookmarked URLs
 */
export default async function SavedShowsRedirect({
  searchParams,
}: {
  searchParams: Promise<{ [key: string]: string | string[] | undefined }>
}) {
  const resolvedParams = await searchParams

  // Preserve query params when redirecting (e.g., ?submitted=pending)
  const params = new URLSearchParams()
  for (const [key, value] of Object.entries(resolvedParams)) {
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
