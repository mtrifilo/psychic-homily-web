import { useState } from 'react'
import { Button } from './ui/button'
import { Input } from './ui/input'
import { Label } from './ui/label'
import { Card, CardContent, CardHeader, CardTitle } from './ui/card'
import { Alert, AlertDescription, AlertTitle } from './ui/alert'
import { Badge } from './ui/badge'
import { TokenInput } from './settings/TokenInput'
import { EnvironmentSelector } from './settings/EnvironmentSelector'
import { Check, Info } from 'lucide-react'
import { saveSettings } from '../lib/api'
import type { AppSettings } from '../lib/types'

interface Props {
  settings: AppSettings
  onSave: (settings: Partial<AppSettings>) => void
}

export function Settings({ settings, onSave }: Props) {
  const [localSettings, setLocalSettings] = useState<AppSettings>(settings)
  const [saved, setSaved] = useState(false)

  const handleSave = () => {
    saveSettings(localSettings)
    onSave(localSettings)
    setSaved(true)
    setTimeout(() => setSaved(false), 2000)
  }

  const update = <K extends keyof AppSettings>(key: K, value: AppSettings[K]) => {
    setLocalSettings(prev => ({ ...prev, [key]: value }))
    setSaved(false)
  }

  const hasStageToken = Boolean(localSettings.stageToken?.length)
  const hasProductionToken = Boolean(localSettings.productionToken?.length)
  const hasLocalToken = Boolean(localSettings.localToken?.length)

  return (
    <div className="space-y-6 max-w-2xl">
      <div>
        <h2 className="text-lg font-semibold text-foreground">Settings</h2>
        <p className="text-sm text-muted-foreground mt-1">
          Configure your API tokens and target environment
        </p>
      </div>

      <Card>
        <CardHeader>
          <CardTitle className="text-base">Target Environment</CardTitle>
        </CardHeader>
        <CardContent>
          <EnvironmentSelector
            value={localSettings.targetEnvironment}
            onChange={(value) => update('targetEnvironment', value)}
            hasStageToken={hasStageToken}
            hasProductionToken={hasProductionToken}
          />
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle className="text-base">API Tokens</CardTitle>
          <p className="text-xs text-muted-foreground">
            Each environment requires its own token. Generate tokens from the Profile
            page in each environment.
          </p>
        </CardHeader>
        <CardContent className="space-y-4">
          <TokenInput
            id="stage-token"
            label="Stage Token"
            description={`From: ${localSettings.stageUrl}`}
            value={localSettings.stageToken}
            onChange={(value) => update('stageToken', value)}
          />

          <TokenInput
            id="production-token"
            label="Production Token"
            description={`From: ${localSettings.productionUrl}`}
            value={localSettings.productionToken}
            onChange={(value) => update('productionToken', value)}
          />

          <TokenInput
            id="local-token"
            label="Local Token (for Data Export)"
            description="From: http://localhost:8080 (your local backend)"
            value={localSettings.localToken}
            onChange={(value) => update('localToken', value)}
          />
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle className="text-base">API URLs</CardTitle>
          <p className="text-xs text-muted-foreground">
            Backend API endpoints (usually don't need to change)
          </p>
        </CardHeader>
        <CardContent className="space-y-4">
          <div className="space-y-2">
            <Label htmlFor="stage-url" className="text-xs text-muted-foreground">
              Stage URL
            </Label>
            <Input
              id="stage-url"
              type="url"
              value={localSettings.stageUrl}
              onChange={(e) => update('stageUrl', e.target.value)}
            />
          </div>
          <div className="space-y-2">
            <Label htmlFor="production-url" className="text-xs text-muted-foreground">
              Production URL
            </Label>
            <Input
              id="production-url"
              type="url"
              value={localSettings.productionUrl}
              onChange={(e) => update('productionUrl', e.target.value)}
            />
          </div>
        </CardContent>
      </Card>

      <div className="flex items-center gap-4">
        <Button onClick={handleSave}>Save Settings</Button>
        {saved && (
          <Badge variant="secondary" className="bg-green-100 text-green-700 dark:bg-green-950/50 dark:text-green-400">
            <Check className="h-3 w-3 mr-1" />
            Settings saved!
          </Badge>
        )}
      </div>

      {/* Help */}
      <Card className="bg-primary/5 border-primary/20">
        <CardContent className="pt-6">
          <div className="flex gap-3">
            <Info className="h-5 w-5 text-primary shrink-0 mt-0.5" />
            <div>
              <h3 className="text-sm font-medium text-foreground">
                Getting API Tokens
              </h3>
              <ol className="mt-2 text-sm text-muted-foreground space-y-1 list-decimal list-inside">
                <li>Go to your Profile page on the target environment</li>
                <li>Scroll to the "API Tokens" section (admin only)</li>
                <li>Click "Create Token"</li>
                <li>Copy the token and paste it in the appropriate field above</li>
              </ol>
              <p className="mt-3 text-sm text-muted-foreground">
                <strong>Important:</strong> Tokens are environment-specific. A token
                from Stage won't work on Production and vice versa.
              </p>
            </div>
          </div>
        </CardContent>
      </Card>
    </div>
  )
}
