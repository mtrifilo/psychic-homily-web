// Re-export all hooks from domain subdirectories for backward compatibility.
// Prefer importing from specific subdirectories (e.g., '@/lib/hooks/shows')
// for clarity, but imports from '@/lib/hooks' continue to work.

export * from './admin'
export * from './shows'
export * from './venues'
export * from './auth'
export * from './user'
export * from './common'
