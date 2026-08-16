export { default as Footer } from './Footer'
// SessionProviders is deliberately not re-exported: only AuthHydrator may mount
// it, and it does so by relative import. Exporting it here would invite a caller
// to place it outside the hydration boundary.
export { Providers } from './providers'
export { AuthHydrator } from './AuthHydrator'
export { ClickReplayScript } from './ClickReplayScript'
export { ThemeProvider } from './theme-provider'
export { ModeToggle, useThemeToggle } from './mode-toggle'
export { CookieConsentBanner } from './CookieConsentBanner'
export { CookiePreferencesDialog } from './CookiePreferencesDialog'
export { PostHogProvider } from './PostHogProvider'
export { PostHogIdentify } from './PostHogIdentify'
export { Sidebar } from './Sidebar'
export { AppShell } from './AppShell'
export { TopBar } from './TopBar'
export { CommandPalette } from './CommandPalette'
