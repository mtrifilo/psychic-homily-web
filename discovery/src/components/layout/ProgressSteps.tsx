import { Button } from '../ui/button'
import { useWizard, type WizardStep } from '../../context/WizardContext'
import { cn } from '../../lib/utils'
import { Check } from 'lucide-react'

interface StepConfig {
  id: WizardStep
  label: string
  canNavigate: (state: {
    selectedVenues: number
    previewedVenues: number
    scrapedEvents: number
  }) => boolean
  isComplete: (state: {
    selectedVenues: number
    previewedVenues: number
    scrapedEvents: number
    currentStep: WizardStep
  }) => boolean
}

const STEPS: StepConfig[] = [
  {
    id: 'venues',
    label: '1. Venues',
    canNavigate: () => true,
    isComplete: ({ selectedVenues, currentStep }) =>
      selectedVenues > 0 && currentStep !== 'venues',
  },
  {
    id: 'preview',
    label: '2. Preview & Select',
    canNavigate: ({ selectedVenues }) => selectedVenues > 0,
    isComplete: ({ scrapedEvents, currentStep }) =>
      scrapedEvents > 0 && currentStep !== 'preview',
  },
  {
    id: 'import',
    label: '3. Import',
    canNavigate: ({ scrapedEvents }) => scrapedEvents > 0,
    isComplete: () => false,
  },
]

export function ProgressSteps() {
  const { step, setStep, selectedVenues, previewEvents, scrapedEvents } = useWizard()

  // Don't show for settings or data-export
  if (step === 'settings' || step === 'data-export') {
    return null
  }

  const state = {
    selectedVenues: selectedVenues.length,
    previewedVenues: Object.keys(previewEvents).length,
    scrapedEvents: scrapedEvents.length,
    currentStep: step,
  }

  return (
    <div className="bg-card border-b px-6 py-3">
      <div className="max-w-6xl mx-auto">
        <div className="flex items-center gap-2 text-sm">
          {STEPS.map((stepConfig, index) => (
            <div key={stepConfig.id} className="flex items-center">
              {index > 0 && <span className="text-muted-foreground mx-2">/</span>}
              <StepIndicator
                label={stepConfig.label}
                active={step === stepConfig.id}
                complete={stepConfig.isComplete(state)}
                disabled={!stepConfig.canNavigate(state)}
                onClick={() => {
                  if (stepConfig.canNavigate(state)) {
                    setStep(stepConfig.id)
                  }
                }}
              />
            </div>
          ))}
        </div>
      </div>
    </div>
  )
}

function StepIndicator({
  label,
  active,
  complete,
  disabled,
  onClick,
}: {
  label: string
  active: boolean
  complete: boolean
  disabled: boolean
  onClick: () => void
}) {
  return (
    <button
      onClick={onClick}
      disabled={disabled}
      className={cn(
        'px-2 py-1 rounded transition-colors inline-flex items-center gap-1',
        active && 'text-primary font-medium bg-primary/10',
        complete && !active && 'text-green-600 hover:bg-green-50 dark:text-green-400 dark:hover:bg-green-950/30',
        disabled && 'text-muted-foreground/50 cursor-not-allowed',
        !active && !complete && !disabled && 'text-muted-foreground hover:text-foreground hover:bg-muted'
      )}
    >
      {label}
      {complete && <Check className="h-3 w-3" />}
    </button>
  )
}
