import { Button } from '../ui/button'
import { Badge } from '../ui/badge'
import { ThemeToggle } from './ThemeToggle'
import { useWizard } from '../../context/WizardContext'
import { useSettings } from '../../context/SettingsContext'

export function Header() {
  const { step, setStep } = useWizard()
  const { hasToken } = useSettings()

  // Discovery steps are venues, preview, select, import
  const isDiscoveryStep = ['venues', 'preview', 'select', 'import'].includes(step)

  return (
    <header className="bg-card border-b px-6 py-4">
      <div className="max-w-6xl mx-auto flex items-center justify-between">
        <div>
          <h1 className="text-xl font-semibold text-foreground">Venue Discovery</h1>
          <p className="text-sm text-muted-foreground">Psychic Homily Admin Tool</p>
        </div>
        <div className="flex items-center gap-4">
          {!hasToken && (
            <Badge variant="secondary" className="bg-amber-100 text-amber-700 hover:bg-amber-100 dark:bg-amber-900/30 dark:text-amber-400">
              Token not configured
            </Badge>
          )}
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
