'use client'

import { Suspense, useEffect, useState } from 'react'
import { useRouter, useSearchParams } from 'next/navigation'
import Link from 'next/link'
import { useForm } from '@tanstack/react-form'
import { z } from 'zod'
import { AlertCircle, Loader2, Mail, CheckCircle2, RefreshCw, ArrowLeft } from 'lucide-react'
import { useRequestAccountRecovery, useConfirmAccountRecovery } from '@/features/auth'
import { useAuthContext } from '@/lib/context/AuthContext'
import { getUniqueErrors } from '@/lib/utils/formErrors'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Alert, AlertDescription } from '@/components/ui/alert'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'

// Validation schema
const emailSchema = z.object({
  email: z.string().email('Please enter a valid email address'),
})

type EmailFormData = z.infer<typeof emailSchema>

// Step 1: Email entry form
function EmailForm({ onSubmit, isLoading }: {
  onSubmit: (email: string) => void
  isLoading: boolean
}) {
  const form = useForm({
    defaultValues: {
      email: '',
    } as EmailFormData,
    onSubmit: async ({ value }) => {
      onSubmit(value.email)
    },
    validators: {
      onSubmit: emailSchema,
    },
  })

  return (
    <form
      onSubmit={e => {
        e.preventDefault()
        e.stopPropagation()
        form.handleSubmit()
      }}
      className="space-y-4"
    >
      <form.Field name="email">
        {field => (
          <div className="space-y-2">
            <Label htmlFor={field.name}>Email</Label>
            <div className="relative">
              <Mail className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
              <Input
                id={field.name}
                name={field.name}
                type="email"
                placeholder="you@example.com"
                value={field.state.value}
                onBlur={field.handleBlur}
                onChange={e => field.handleChange(e.target.value)}
                className="pl-10"
                aria-invalid={field.state.meta.errors.length > 0}
                autoFocus
              />
            </div>
            {field.state.meta.errors.length > 0 && (
              <p role="alert" className="text-sm text-destructive">
                {getUniqueErrors(field.state.meta.errors)}
              </p>
            )}
          </div>
        )}
      </form.Field>

      <form.Subscribe selector={state => [state.canSubmit, state.isSubmitting]}>
        {([canSubmit, isSubmitting]) => (
          <Button
            type="submit"
            className="w-full"
            disabled={!canSubmit || isSubmitting || isLoading}
          >
            {isSubmitting || isLoading ? (
              <>
                <Loader2 className="h-4 w-4 animate-spin" />
                Sending...
              </>
            ) : (
              'Send Recovery Email'
            )}
          </Button>
        )}
      </form.Subscribe>
    </form>
  )
}

// Recovery email sent confirmation. PSY-774: shown for every well-formed
// email submission — the response is generic, so the UI is too. Recovery
// detail (days remaining, eligibility) moves behind token confirmation.
function RecoveryEmailSent({ email, onBack }: {
  email: string
  onBack: () => void
}) {
  return (
    <div className="space-y-4">
      <div className="flex items-center gap-2 rounded-md bg-emerald-500/10 p-3 text-sm text-emerald-600 dark:text-emerald-400">
        <CheckCircle2 className="h-4 w-4 shrink-0" />
        <span>If an account exists for <strong>{email}</strong> and is eligible for recovery, a recovery email has been sent.</span>
      </div>

      <p className="text-sm text-muted-foreground">
        Check your inbox for a recovery link. The link expires in 1 hour.
      </p>

      <Button
        type="button"
        variant="outline"
        className="w-full"
        onClick={onBack}
      >
        <ArrowLeft className="h-4 w-4" />
        Try a different email
      </Button>
    </div>
  )
}

// Token confirmation component (when user clicks the link from email)
function TokenConfirmation({ token }: { token: string }) {
  const router = useRouter()
  const confirmMutation = useConfirmAccountRecovery()
  const { setUser } = useAuthContext()
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    confirmMutation.mutate(token, {
      onSuccess: data => {
        if (data.user) {
          setUser({
            id: data.user.id,
            email: data.user.email,
            first_name: data.user.first_name,
            last_name: data.user.last_name,
            email_verified: false,
          })
        }
        router.push('/')
      },
      onError: err => {
        setError(err.message)
      },
    })
  }, [token]) // eslint-disable-line react-hooks/exhaustive-deps

  if (error) {
    return (
      <div className="space-y-4">
        <Alert variant="destructive">
          <AlertCircle className="h-4 w-4" />
          <AlertDescription>{error}</AlertDescription>
        </Alert>

        <p className="text-sm text-muted-foreground">
          The recovery link may have expired or already been used.
        </p>

        <Link href="/auth/recover">
          <Button variant="outline" className="w-full">
            <ArrowLeft className="h-4 w-4" />
            Request a new recovery link
          </Button>
        </Link>
      </div>
    )
  }

  return (
    <div className="flex flex-col items-center gap-4 py-8">
      <Loader2 className="h-8 w-8 animate-spin text-primary" />
      <p className="text-sm text-muted-foreground">Recovering your account...</p>
    </div>
  )
}

function RecoverAccountPageContent() {
  const router = useRouter()
  const searchParams = useSearchParams()
  const { isAuthenticated, isLoading } = useAuthContext()
  const requestRecoveryMutation = useRequestAccountRecovery()

  const [step, setStep] = useState<'email' | 'sent'>('email')
  const [email, setEmail] = useState('')
  const [error, setError] = useState<string | null>(null)

  // Check for token in URL (magic link callback)
  const token = searchParams.get('token')

  // Redirect if already authenticated
  useEffect(() => {
    if (isAuthenticated && !isLoading && !token) {
      router.push('/')
    }
  }, [isAuthenticated, isLoading, router, token])

  // Handle email submission. PSY-774: the response is enumeration-safe, so
  // we always show the same "sent" confirmation on a successful API call —
  // the backend logs per-state detail; the UI never branches on it.
  const handleEmailSubmit = async (submittedEmail: string) => {
    setError(null)
    setEmail(submittedEmail)

    requestRecoveryMutation.mutate({ email: submittedEmail }, {
      onSuccess: data => {
        if (!data.success && data.error_code) {
          // Only pre-lookup errors (validation, email-service-config) set
          // an error code post-PSY-774; surface them verbatim.
          setError(data.message)
          return
        }
        setStep('sent')
      },
      onError: err => {
        setError(err.message)
      },
    })
  }

  const handleBack = () => {
    setStep('email')
    setEmail('')
    setError(null)
  }

  // Show loading state while checking auth
  if (isLoading) {
    return (
      <div className="flex min-h-[calc(100vh-64px)] items-center justify-center">
        <Loader2 className="h-8 w-8 animate-spin text-muted-foreground" />
      </div>
    )
  }

  // Don't render if authenticated (will redirect)
  if (isAuthenticated && !token) {
    return null
  }

  return (
    <div className="flex min-h-[calc(100vh-64px)] items-center justify-center px-4 py-12">
      <div className="w-full max-w-md">
        {/* Header */}
        <div className="mb-8 text-center">
          <div className="mx-auto mb-4 flex h-12 w-12 items-center justify-center rounded-full bg-primary/10">
            <RefreshCw className="h-6 w-6 text-primary" />
          </div>
          <h1 className="text-2xl font-bold tracking-tight">
            Recover Your Account
          </h1>
          <p className="mt-2 text-sm text-muted-foreground">
            {token
              ? 'Completing account recovery...'
              : 'Restore access to your deleted account'}
          </p>
        </div>

        <Card className="border-border/50 bg-card/50 backdrop-blur-sm">
          <CardHeader className="pb-4">
            <CardTitle className="text-lg">
              {token
                ? 'Account Recovery'
                : step === 'email'
                  ? 'Enter your email'
                  : 'Check your email'}
            </CardTitle>
            {!token && (
              <CardDescription>
                {step === 'email'
                  ? 'Enter the email address of the account you want to recover'
                  : 'We sent you a recovery link if your account is eligible'}
              </CardDescription>
            )}
          </CardHeader>

          <CardContent>
            {/* Show error if any */}
            {error && (
              <Alert variant="destructive" className="mb-4">
                <AlertCircle className="h-4 w-4" />
                <AlertDescription>{error}</AlertDescription>
              </Alert>
            )}

            {/* Token confirmation (from magic link) */}
            {token ? (
              <TokenConfirmation token={token} />
            ) : (
              <>
                {/* Step 1: Email form */}
                {step === 'email' && (
                  <EmailForm
                    onSubmit={handleEmailSubmit}
                    isLoading={requestRecoveryMutation.isPending}
                  />
                )}

                {/* Step 2: Recovery email sent */}
                {step === 'sent' && (
                  <RecoveryEmailSent
                    email={email}
                    onBack={handleBack}
                  />
                )}
              </>
            )}
          </CardContent>
        </Card>

        {/* Footer */}
        <p className="mt-6 text-center text-sm text-muted-foreground">
          <Link href="/auth" className="underline underline-offset-4 hover:text-primary">
            Back to sign in
          </Link>
        </p>
      </div>
    </div>
  )
}

export default function RecoverAccountPage() {
  return (
    <Suspense fallback={
      <div className="flex min-h-[calc(100vh-64px)] items-center justify-center">
        <Loader2 className="h-8 w-8 animate-spin text-muted-foreground" />
      </div>
    }>
      <RecoverAccountPageContent />
    </Suspense>
  )
}
