export function getSavedReleasePageBounds(
  requestedPage: number,
  total: number,
  pageSize: number
) {
  const page =
    Number.isFinite(requestedPage) && requestedPage > 0 ? requestedPage : 1
  const totalPages = Math.max(1, Math.ceil(total / pageSize))

  return {
    page,
    totalPages,
    targetPage: Math.min(page, totalPages),
  }
}
