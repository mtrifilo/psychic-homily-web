import { createContext, useContext, useState, useCallback, type ReactNode } from 'react'
import { getSettings, saveSettings } from '../lib/api'
import type { AppSettings } from '../lib/types'

interface SettingsContextValue {
  settings: AppSettings
  updateSettings: (updates: Partial<AppSettings>) => void
  hasToken: boolean
  hasLocalToken: boolean
  hasTargetToken: boolean
  targetEnv: 'Stage' | 'Production'
}

const SettingsContext = createContext<SettingsContextValue | null>(null)

export function SettingsProvider({ children }: { children: ReactNode }) {
  const [settings, setSettings] = useState<AppSettings>(getSettings())

  const updateSettings = useCallback((updates: Partial<AppSettings>) => {
    setSettings(prev => {
      const next = { ...prev, ...updates }
      saveSettings(next)
      return next
    })
  }, [])

  const hasToken = settings.targetEnvironment === 'production'
    ? Boolean(settings.productionToken?.length)
    : Boolean(settings.stageToken?.length)

  const hasLocalToken = Boolean(settings.localToken?.length)

  const hasTargetToken = settings.targetEnvironment === 'production'
    ? Boolean(settings.productionToken?.length)
    : Boolean(settings.stageToken?.length)

  const targetEnv = settings.targetEnvironment === 'production' ? 'Production' : 'Stage'

  const value: SettingsContextValue = {
    settings,
    updateSettings,
    hasToken,
    hasLocalToken,
    hasTargetToken,
    targetEnv,
  }

  return (
    <SettingsContext.Provider value={value}>
      {children}
    </SettingsContext.Provider>
  )
}

export function useSettings() {
  const context = useContext(SettingsContext)
  if (!context) {
    throw new Error('useSettings must be used within a SettingsProvider')
  }
  return context
}
