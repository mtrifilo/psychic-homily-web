// Re-export all hooks from domain subdirectories for backward compatibility.
// Prefer importing from specific subdirectories (e.g., '@/lib/hooks/shows')
// for clarity, but imports from '@/lib/hooks' continue to work.
//
// Auth, user, and contributor profile hooks have been migrated to '@/features/auth'.

export * from './admin'
export * from './shows'
export * from './artists'
export * from './venues'
export * from './common'
