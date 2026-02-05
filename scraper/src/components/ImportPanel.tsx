import { useState } from 'react'
import type { ScrapedEvent, ImportResult, AppSettings } from '../lib/types'
import { importEvents } from '../lib/api'

interface Props {
  events: ScrapedEvent[]
  settings: AppSettings
  onBack: () => void
  onStartOver: () => void
}

export function ImportPanel({ events, settings, onBack, onStartOver }: Props) {
  const [loading, setLoading] = useState(false)
  const [isDryRun, setIsDryRun] = useState(true)
  const [result, setResult] = useState<ImportResult | null>(null)
  const [error, setError] = useState<string>('')

  const handleImport = async () => {
    setLoading(true)
    setError('')
    setResult(null)

    try {
      const importResult = await importEvents(events, isDryRun)
      setResult(importResult)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Import failed')
    } finally {
      setLoading(false)
    }
  }

  const targetEnv = settings.targetEnvironment === 'production' ? 'Production' : 'Stage'

  return (
    <div className="space-y-6">
      <div>
        <h2 className="text-lg font-semibold text-gray-900">Import Events</h2>
        <p className="text-sm text-gray-500 mt-1">
          Review and import {events.length} scraped event{events.length !== 1 ? 's' : ''} to {targetEnv}
        </p>
      </div>

      {!settings.apiToken && (
        <div className="bg-amber-50 border border-amber-200 rounded-lg px-4 py-3 text-amber-800">
          <p className="font-medium">API Token Required</p>
          <p className="text-sm mt-1">
            Go to Settings to configure your API token before importing.
          </p>
        </div>
      )}

      {error && (
        <div className="bg-red-50 border border-red-200 rounded-lg px-4 py-3 text-red-700">
          {error}
        </div>
      )}

      {/* Event Summary */}
      <div className="bg-white rounded-lg border border-gray-200">
        <div className="px-4 py-3 bg-gray-50 border-b">
          <h3 className="font-medium text-gray-900">Events to Import</h3>
        </div>
        <div className="max-h-64 overflow-y-auto">
          <table className="w-full text-sm">
            <thead className="bg-gray-50 sticky top-0">
              <tr>
                <th className="px-4 py-2 text-left text-gray-600 font-medium">Date</th>
                <th className="px-4 py-2 text-left text-gray-600 font-medium">Event</th>
                <th className="px-4 py-2 text-left text-gray-600 font-medium">Venue</th>
                <th className="px-4 py-2 text-left text-gray-600 font-medium">Artists</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-gray-100">
              {events.map(event => (
                <tr key={`${event.venueSlug}-${event.id}`} className="hover:bg-gray-50">
                  <td className="px-4 py-2 text-gray-500 whitespace-nowrap">
                    {formatDate(event.date)}
                  </td>
                  <td className="px-4 py-2 text-gray-900">{event.title}</td>
                  <td className="px-4 py-2 text-gray-500">{event.venue}</td>
                  <td className="px-4 py-2 text-gray-500 truncate max-w-xs">
                    {event.artists.join(', ')}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </div>

      {/* Import Options */}
      <div className="bg-white rounded-lg border border-gray-200 p-4">
        <div className="flex items-center justify-between">
          <div>
            <h3 className="font-medium text-gray-900">Import Mode</h3>
            <p className="text-sm text-gray-500 mt-1">
              {isDryRun
                ? 'Preview what would be imported without making changes'
                : 'Import events to the database'}
            </p>
          </div>
          <label className="flex items-center gap-2 cursor-pointer">
            <input
              type="checkbox"
              checked={!isDryRun}
              onChange={(e) => setIsDryRun(!e.target.checked)}
              className="w-4 h-4 text-blue-600 rounded border-gray-300"
            />
            <span className="text-sm text-gray-700">Live Import</span>
          </label>
        </div>

        <div className="mt-4 flex items-center justify-between">
          <div>
            <span className="text-sm text-gray-600">Target: </span>
            <span className={`text-sm font-medium ${
              settings.targetEnvironment === 'production'
                ? 'text-red-600'
                : 'text-blue-600'
            }`}>
              {targetEnv}
            </span>
          </div>
          <button
            onClick={handleImport}
            disabled={loading || !settings.apiToken}
            className={`px-4 py-2 rounded-lg font-medium flex items-center gap-2 ${
              loading || !settings.apiToken
                ? 'bg-gray-200 text-gray-400 cursor-not-allowed'
                : isDryRun
                ? 'bg-blue-600 text-white hover:bg-blue-700'
                : 'bg-green-600 text-white hover:bg-green-700'
            }`}
          >
            {loading && (
              <div className="w-4 h-4 border-2 border-white border-t-transparent rounded-full animate-spin" />
            )}
            {isDryRun ? 'Preview Import' : 'Import Now'}
          </button>
        </div>
      </div>

      {/* Import Results */}
      {result && (
        <div className="bg-white rounded-lg border border-gray-200 overflow-hidden">
          <div className="px-4 py-3 bg-gray-50 border-b">
            <h3 className="font-medium text-gray-900">
              {isDryRun ? 'Preview Results' : 'Import Results'}
            </h3>
          </div>
          <div className="p-4 space-y-4">
            <div className="grid grid-cols-2 md:grid-cols-4 gap-4">
              <StatCard
                label="Total"
                value={result.total}
                color="gray"
              />
              <StatCard
                label={isDryRun ? 'Would Import' : 'Imported'}
                value={result.imported}
                color="green"
              />
              <StatCard
                label="Duplicates"
                value={result.duplicates}
                color="blue"
              />
              <StatCard
                label="Rejected"
                value={result.rejected}
                color="amber"
              />
              {result.errors > 0 && (
                <StatCard
                  label="Errors"
                  value={result.errors}
                  color="red"
                />
              )}
            </div>

            {result.messages.length > 0 && (
              <div className="mt-4">
                <h4 className="text-sm font-medium text-gray-700 mb-2">Details</h4>
                <div className="bg-gray-50 rounded-lg p-3 max-h-48 overflow-y-auto">
                  <ul className="text-xs font-mono space-y-1">
                    {result.messages.map((msg, i) => (
                      <li
                        key={i}
                        className={
                          msg.startsWith('IMPORTED') || msg.startsWith('WOULD IMPORT')
                            ? 'text-green-700'
                            : msg.startsWith('DUPLICATE')
                            ? 'text-blue-600'
                            : msg.startsWith('REJECTED')
                            ? 'text-amber-600'
                            : msg.startsWith('ERROR') || msg.startsWith('SKIP')
                            ? 'text-red-600'
                            : 'text-gray-600'
                        }
                      >
                        {msg}
                      </li>
                    ))}
                  </ul>
                </div>
              </div>
            )}
          </div>
        </div>
      )}

      <div className="flex justify-between">
        <button
          onClick={onBack}
          className="px-4 py-2 rounded-lg text-gray-600 hover:bg-gray-100"
        >
          Back
        </button>
        <button
          onClick={onStartOver}
          className="px-4 py-2 rounded-lg text-blue-600 hover:bg-blue-50"
        >
          Start Over
        </button>
      </div>
    </div>
  )
}

function StatCard({
  label,
  value,
  color,
}: {
  label: string
  value: number
  color: 'gray' | 'green' | 'blue' | 'amber' | 'red'
}) {
  const colors = {
    gray: 'bg-gray-50 text-gray-900',
    green: 'bg-green-50 text-green-700',
    blue: 'bg-blue-50 text-blue-700',
    amber: 'bg-amber-50 text-amber-700',
    red: 'bg-red-50 text-red-700',
  }

  return (
    <div className={`rounded-lg p-3 ${colors[color]}`}>
      <div className="text-2xl font-bold">{value}</div>
      <div className="text-sm">{label}</div>
    </div>
  )
}

function formatDate(dateStr: string): string {
  const date = new Date(dateStr)
  return date.toLocaleDateString('en-US', {
    month: 'short',
    day: 'numeric',
  })
}
