// Re-export all hooks from domain subdirectories for backward compatibility.
// Prefer importing from specific subdirectories for clarity, but imports
// from '@/lib/hooks' continue to work.
//
// Migrated to feature modules:
// - Auth, user, contributor profile hooks → '@/features/auth'
// - Show hooks → '@/features/shows'
// - Artist hooks → '@/features/artists'
// - Venue hooks → '@/features/venues'

export * from './admin'
export * from './common'
