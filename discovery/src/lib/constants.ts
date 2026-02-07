// Environment configuration
export const ENVIRONMENTS = {
  stage: {
    label: 'Stage',
    description: 'Test imports in staging environment',
    isProduction: false,
  },
  production: {
    label: 'Production',
    description: 'Import directly to live site',
    isProduction: true,
  },
} as const

// Status colors
export const STATUS_COLORS = {
  approved: {
    bg: 'bg-green-100',
    text: 'text-green-700',
    variant: 'default' as const,
  },
  pending: {
    bg: 'bg-amber-100',
    text: 'text-amber-700',
    variant: 'secondary' as const,
  },
  rejected: {
    bg: 'bg-red-100',
    text: 'text-red-700',
    variant: 'destructive' as const,
  },
  default: {
    bg: 'bg-gray-100',
    text: 'text-gray-600',
    variant: 'outline' as const,
  },
} as const

// Import result message prefixes
export const MESSAGE_PREFIXES = {
  imported: ['IMPORTED', 'WOULD IMPORT'],
  duplicate: ['DUPLICATE'],
  error: ['ERROR', 'SKIP'],
} as const

// Token validation
export const TOKEN_PREFIX = 'phk_'
export const MIN_TOKEN_LENGTH = 20

// Pagination defaults
export const DEFAULT_PAGE_SIZE = 100

// Local API URL
export const LOCAL_API_URL = 'http://localhost:8080'
