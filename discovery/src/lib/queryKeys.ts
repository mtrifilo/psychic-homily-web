// Query key factory for consistent cache management
export const queryKeys = {
  // Preview queries
  preview: {
    all: ['preview'] as const,
    venue: (venueSlug: string) => ['preview', 'venue', venueSlug] as const,
    batch: (venueSlugs: string[]) => ['preview', 'batch', ...venueSlugs] as const,
  },

  // Export queries
  export: {
    all: ['export'] as const,
    shows: (params?: Record<string, unknown>) =>
      params ? (['export', 'shows', params] as const) : (['export', 'shows'] as const),
    artists: (params?: Record<string, unknown>) =>
      params ? (['export', 'artists', params] as const) : (['export', 'artists'] as const),
    venues: (params?: Record<string, unknown>) =>
      params ? (['export', 'venues', params] as const) : (['export', 'venues'] as const),
  },

  // Settings queries
  settings: {
    all: ['settings'] as const,
    current: () => ['settings', 'current'] as const,
  },
} as const
