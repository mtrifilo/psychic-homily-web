import { Button } from '../ui/button'
import { ThemeToggle } from './ThemeToggle'
import { useWizard } from '../../context/WizardContext'
import { useSettings } from '../../context/SettingsContext'

export function Header() {
  const { step, setStep } = useWizard()
  const { settings, updateSettings } = useSettings()

  const hasStageToken = Boolean(settings.stageToken?.length)
  const hasProductionToken = Boolean(settings.productionToken?.length)
  const isStage = settings.targetEnvironment === 'stage'

  // Discovery steps are venues, preview, select, import
  const isDiscoveryStep = ['venues', 'preview', 'import'].includes(step)

  return (
    <header className="bg-card border-b px-6 py-4">
      <div className="max-w-6xl mx-auto flex items-center justify-between">
        <div>
          <h1 className="text-xl font-semibold text-foreground">Venue Discovery</h1>
          <p className="text-sm text-muted-foreground">Psychic Homily Admin Tool</p>
        </div>
        <div className="flex items-center gap-4">
          <div className="flex items-center rounded-md border overflow-hidden text-sm">
            <button
              className={`px-3 py-1.5 font-medium transition-colors ${
                isStage
                  ? 'bg-primary text-primary-foreground'
                  : 'bg-muted text-muted-foreground hover:text-foreground'
              } ${!hasStageToken ? 'opacity-50 cursor-not-allowed' : 'cursor-pointer'}`}
              onClick={() => updateSettings({ targetEnvironment: 'stage' })}
              disabled={!hasStageToken}
              title={hasStageToken ? 'Switch to Stage' : 'Stage token not configured'}
            >
              Stage
            </button>
            <button
              className={`px-3 py-1.5 font-medium transition-colors ${
                !isStage
                  ? 'bg-primary text-primary-foreground'
                  : 'bg-muted text-muted-foreground hover:text-foreground'
              } ${!hasProductionToken ? 'opacity-50 cursor-not-allowed' : 'cursor-pointer'}`}
              onClick={() => updateSettings({ targetEnvironment: 'production' })}
              disabled={!hasProductionToken}
              title={hasProductionToken ? 'Switch to Production' : 'Production token not configured'}
            >
              Prod
            </button>
          </div>
          <nav className="flex items-center gap-1">
            <Button
              variant={isDiscoveryStep ? 'secondary' : 'ghost'}
              size="sm"
              onClick={() => setStep('venues')}
            >
              Discovery
            </Button>
            <Button
              variant={step === 'data-export' ? 'secondary' : 'ghost'}
              size="sm"
              onClick={() => setStep('data-export')}
            >
              Data Export
            </Button>
            <Button
              variant={step === 'settings' ? 'secondary' : 'ghost'}
              size="sm"
              onClick={() => setStep('settings')}
            >
              Settings
            </Button>
          </nav>
          <ThemeToggle />
        </div>
      </div>
    </header>
  )
}
