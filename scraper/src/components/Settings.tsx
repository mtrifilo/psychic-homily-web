import { useState } from 'react'
import type { AppSettings } from '../lib/types'
import { saveSettings } from '../lib/api'

interface Props {
  settings: AppSettings
  onSave: (settings: AppSettings) => void
  onBack: () => void
}

export function Settings({ settings, onSave, onBack }: Props) {
  const [localSettings, setLocalSettings] = useState<AppSettings>(settings)
  const [saved, setSaved] = useState(false)

  const handleSave = () => {
    saveSettings(localSettings)
    onSave(localSettings)
    setSaved(true)
    setTimeout(() => setSaved(false), 2000)
  }

  const update = (key: keyof AppSettings, value: string) => {
    setLocalSettings(prev => ({ ...prev, [key]: value }))
    setSaved(false)
  }

  return (
    <div className="space-y-6 max-w-2xl">
      <div className="flex items-center justify-between">
        <div>
          <h2 className="text-lg font-semibold text-gray-900">Settings</h2>
          <p className="text-sm text-gray-500 mt-1">
            Configure your API token and target environment
          </p>
        </div>
        <button
          onClick={onBack}
          className="text-sm text-gray-600 hover:text-gray-900"
        >
          Back to Scraper
        </button>
      </div>

      <div className="bg-white rounded-lg border border-gray-200 divide-y">
        {/* API Token */}
        <div className="p-4">
          <label className="block">
            <span className="text-sm font-medium text-gray-700">API Token</span>
            <p className="text-xs text-gray-500 mt-1">
              Generate a token from Admin Settings in the web app
            </p>
            <input
              type="password"
              value={localSettings.apiToken}
              onChange={(e) => update('apiToken', e.target.value)}
              placeholder="phk_..."
              className="mt-2 w-full px-3 py-2 border border-gray-300 rounded-lg text-sm font-mono focus:ring-2 focus:ring-blue-500 focus:border-blue-500"
            />
          </label>
        </div>

        {/* Target Environment */}
        <div className="p-4">
          <span className="text-sm font-medium text-gray-700">Target Environment</span>
          <p className="text-xs text-gray-500 mt-1">
            Where to import scraped events
          </p>
          <div className="mt-3 space-y-2">
            <label className="flex items-center gap-3 cursor-pointer">
              <input
                type="radio"
                name="environment"
                checked={localSettings.targetEnvironment === 'stage'}
                onChange={() => update('targetEnvironment', 'stage')}
                className="w-4 h-4 text-blue-600"
              />
              <div>
                <span className="text-sm text-gray-900">Stage</span>
                <p className="text-xs text-gray-500">Test imports in staging environment</p>
              </div>
            </label>
            <label className="flex items-center gap-3 cursor-pointer">
              <input
                type="radio"
                name="environment"
                checked={localSettings.targetEnvironment === 'production'}
                onChange={() => update('targetEnvironment', 'production')}
                className="w-4 h-4 text-blue-600"
              />
              <div>
                <span className="text-sm text-gray-900">Production</span>
                <p className="text-xs text-red-500">Import directly to live site</p>
              </div>
            </label>
          </div>
        </div>

        {/* API URLs */}
        <div className="p-4">
          <span className="text-sm font-medium text-gray-700">API URLs</span>
          <p className="text-xs text-gray-500 mt-1">
            Backend API endpoints
          </p>
          <div className="mt-3 space-y-3">
            <label className="block">
              <span className="text-xs text-gray-600">Stage URL</span>
              <input
                type="url"
                value={localSettings.stageUrl}
                onChange={(e) => update('stageUrl', e.target.value)}
                className="mt-1 w-full px-3 py-2 border border-gray-300 rounded-lg text-sm focus:ring-2 focus:ring-blue-500 focus:border-blue-500"
              />
            </label>
            <label className="block">
              <span className="text-xs text-gray-600">Production URL</span>
              <input
                type="url"
                value={localSettings.productionUrl}
                onChange={(e) => update('productionUrl', e.target.value)}
                className="mt-1 w-full px-3 py-2 border border-gray-300 rounded-lg text-sm focus:ring-2 focus:ring-blue-500 focus:border-blue-500"
              />
            </label>
          </div>
        </div>
      </div>

      <div className="flex items-center gap-4">
        <button
          onClick={handleSave}
          className="px-4 py-2 bg-blue-600 text-white rounded-lg font-medium hover:bg-blue-700"
        >
          Save Settings
        </button>
        {saved && (
          <span className="text-sm text-green-600">Settings saved!</span>
        )}
      </div>

      {/* Help */}
      <div className="bg-blue-50 rounded-lg p-4 mt-8">
        <h3 className="text-sm font-medium text-blue-900">Getting an API Token</h3>
        <ol className="mt-2 text-sm text-blue-800 space-y-1 list-decimal list-inside">
          <li>Go to your Profile page on the web app</li>
          <li>Scroll to the "API Tokens" section (admin only)</li>
          <li>Click "Create Token"</li>
          <li>Copy the token and paste it here</li>
        </ol>
      </div>
    </div>
  )
}
